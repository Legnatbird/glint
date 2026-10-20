package ui

import (
	"path/filepath"
	"strconv"
	"strings"

	"applauncher/internal/core"
	"applauncher/internal/win"
)

// Settings is the form that lives inside the settings window.
//
// There are no child controls: rows are painted and hit-tested directly. That
// keeps one painting model in the program, costs no extra window or GDI handles
// per control, and lets the window stay the only surface that ever draws.
//
// Every edit is written to config.json and pushed to the running launcher at
// once, through the hooks. Nothing here waits for a restart, and nothing here
// assumes the edit succeeded: a hook that fails puts the previous value back and
// reports the error in the footer.
type Settings struct {
	window *win.SettingsWindow
	cfg    *core.Config
	fonts  *win.Fonts
	hooks  SettingsHooks
	dpi    int32

	// sel indexes rows; headers are skipped by navigation but still occupy an
	// index, which keeps hit-testing and keyboard selection on the same value.
	sel     int
	scroll  int32
	capture bool
	armed   int // row armed for a destructive action, -1 when none

	status string
	failed bool
}

// SettingsHooks connects the form to the rest of the program. Each hook is
// optional; a nil hook means the corresponding edit is only saved.
type SettingsHooks struct {
	Save       func() error            // persist config.json
	Hotkey     func(text string) error // install a new global hotkey
	HotkeyLock func(capturing bool)    // release/reclaim the hotkey while capturing

	Autostart  func(enabled bool) error
	MaxResults func(n int)
	Rescan     func() // rebuild the index after a source change
	ClearLog   func() error
	Entries    func() int
}

// Row kinds. The kind decides how a row reacts to Enter, arrows and clicks.
type rowKind int

const (
	rowHeader rowKind = iota // section label, never focusable
	rowHotkey                // Enter starts capturing a combination
	rowNumber                // left/right step the value
	rowChoice                // left/right cycle an enumeration
	rowToggle                // Enter flips it
	rowAction                // Enter runs it
	rowEntry                 // one removable list entry
	rowDanger                // Enter arms, then clears
)

type row struct {
	kind  rowKind
	label string
	value string
	y     int32 // relative to the top of the content band, before scrolling
	h     int32
	step  func(delta int)
	act   func()
}

// Layout, in points at 96 dpi; everything is scaled by the window DPI.
const (
	setPad        = 26
	setTitleY     = 16
	setContentTop = 62
	setRowH       = 30
	setHeaderH    = 38
	setValueX     = 190
	setFooterH    = 32
	setGap        = 8
)

// NewSettings wires a form to a window. The window is not shown here: the
// caller decides when the form becomes visible. A nil window is allowed, which
// is what the tests use to drive the form without a desktop.
func NewSettings(window *win.SettingsWindow, cfg *core.Config, fonts *win.Fonts, hooks SettingsHooks) *Settings {
	s := &Settings{
		window: window,
		cfg:    cfg,
		fonts:  fonts,
		hooks:  hooks,
		dpi:    96,
		armed:  -1,
	}
	if window == nil {
		return s
	}

	s.dpi = int32(window.DPI())
	window.OnPaint(s.Paint)
	window.OnKey(s.HandleKey)
	window.OnClick(s.HandleClick)
	window.OnWheel(s.HandleWheel)
	window.OnBlur(func() {
		if s.capture {
			s.capture = false
			if s.hooks.HotkeyLock != nil {
				s.hooks.HotkeyLock(false)
			}
			s.setStatus("Hotkey unchanged", false)
		}
		s.armed = -1
	})
	return s
}

// clientSize falls back to a fixed size so layout math never divides by zero and
// the form can run without a window.
func (s *Settings) clientSize() (int32, int32) {
	if s.window == nil {
		return 800, 600
	}
	return s.window.ClientSize()
}

func (s *Settings) scale(v int32) int32 { return v * s.dpi / 96 }

// build lays the form out. Row positions are relative to the content band, so
// the same numbers work for painting, hit-testing and scrolling.
func (s *Settings) build() ([]row, int32) {
	rows := make([]row, 0, 24)
	var y int32

	header := func(text string) {
		if len(rows) > 0 {
			y += s.scale(setGap)
		}
		rows = append(rows, row{kind: rowHeader, label: text, y: y, h: s.scale(setHeaderH)})
		y += s.scale(setHeaderH)
	}
	add := func(kind rowKind, label, value string, step func(int), act func()) {
		rows = append(rows, row{kind: kind, label: label, value: value, y: y, h: s.scale(setRowH), step: step, act: act})
		y += s.scale(setRowH)
	}

	header("General")
	hotkey := s.cfg.Hotkey
	if s.capture {
		hotkey = "press a combination…"
	}
	add(rowHotkey, "Hotkey", hotkey, nil, s.beginCapture)
	add(rowNumber, "Max results", strconv.Itoa(s.cfg.MaxResults), s.stepMaxResults, nil)

	add(rowToggle, "Start with Windows", onOff(s.cfg.Autostart), nil, s.toggleAutostart)

	header("Sources")
	add(rowChoice, "PATH scope", s.cfg.PathScope, s.stepPathScope, nil)
	add(rowAction, "Add folder to scan…", "", nil, s.addCustomFolder)
	for _, folder := range s.cfg.CustomFolders {
		add(rowEntry, baseName(folder), folder, nil, s.dropCustomFolder(folder))
	}
	add(rowAction, "Add folder to skip…", "", nil, s.addExcludedFolder)
	for _, folder := range s.cfg.ExcludeFolders {
		add(rowEntry, baseName(folder), folder, nil, s.dropExcludedFolder(folder))
	}

	header("Manual apps")
	add(rowAction, "Add application…", "", nil, s.addManualApp)
	for i, app := range s.cfg.ManualApps {
		add(rowEntry, app.Name, app.Target, nil, s.dropManualApp(i))
	}
	if len(s.cfg.ManualApps) == 0 {
		add(rowHeader, "Nothing added by hand yet", "", nil, nil)
	}

	header("History")
	entries := 0
	if s.hooks.Entries != nil {
		entries = s.hooks.Entries()
	}
	add(rowDanger, "Clear history", plural(entries, "entry", "entries"), nil, s.clearHistory)

	return rows, y
}

// Paint draws the whole form.
func (s *Settings) Paint(c *win.Canvas) {
	width, height := s.clientSize()
	if width <= 0 || height <= 0 {
		return
	}
	c.Fill(win.Rect{Left: 0, Top: 0, Right: width, Bottom: height}, ColorBackground)

	pad := s.scale(setPad)
	c.UseFont(s.fonts.Query)
	c.Text("Settings", pad, s.scale(setTitleY), colorText)

	rows, contentH := s.build()
	s.clampSelection(rows, contentH)

	top := s.scale(setContentTop) - s.scroll
	footerY := height - s.scale(setFooterH)

	restore := c.Clip(win.Rect{Left: 0, Top: s.scale(setContentTop), Right: width, Bottom: footerY})
	for i, r := range rows {
		y := top + r.y
		if y+r.h < 0 || y > footerY {
			continue
		}
		if r.kind == rowHeader {
			c.UseFont(s.fonts.Footer)
			c.TextIn(strings.ToUpper(r.label), win.Rect{
				Left:   pad,
				Top:    y + (r.h-c.LineHeight())/2,
				Right:  width - pad,
				Bottom: y + r.h,
			}, colorDim)
			continue
		}

		if i == s.sel {
			c.Fill(win.Rect{Left: 0, Top: y, Right: width, Bottom: y + r.h}, colorSelection)
			c.Fill(win.Rect{Left: 0, Top: y, Right: s.scale(3), Bottom: y + r.h}, colorAccent)
		}

		// Text is centred on the font's own line height rather than on a
		// hand-tuned offset: the offset used to be 7px, which was already low
		// for a 16px font and is clearly low for an 18px one.
		c.UseFont(s.fonts.Row)
		textY := y + (r.h-c.LineHeight())/2
		valueX := pad + s.scale(setValueX)
		c.TextIn(r.label, win.Rect{
			Left:   pad,
			Top:    textY,
			Right:  valueX - s.scale(8),
			Bottom: y + r.h,
		}, colorText)

		if r.value != "" {
			color := colorText
			switch r.kind {
			case rowEntry:
				color = colorDim
			case rowHotkey:
				if s.capture {
					color = colorAccent
				}
			}
			if i == s.sel && s.armed == i {
				color = colorError
			}
			// A manual app's target is usually longer than the window; ending it
			// in an ellipsis is the difference between "shortened" and "broken".
			c.TextIn(r.value, win.Rect{
				Left:   valueX,
				Top:    textY,
				Right:  width - pad,
				Bottom: y + r.h,
			}, color)
		}
	}
	restore()

	s.paintScrollbar(c, width, height, contentH, footerY)
	s.paintFooter(c, width, footerY, rows)
}

func (s *Settings) paintScrollbar(c *win.Canvas, width, height, contentH, footerY int32) {
	visible := footerY - s.scale(setContentTop)
	if contentH <= visible || contentH <= 0 {
		return
	}
	track := visible - s.scale(8)
	thumb := track * visible / contentH
	if thumb < s.scale(24) {
		thumb = s.scale(24)
	}
	maxScroll := contentH - visible
	if maxScroll < 1 {
		maxScroll = 1
	}
	pos := s.scale(setContentTop) + (track-thumb)*s.scroll/maxScroll
	x := width - s.scale(6)
	c.Fill(win.Rect{Left: x, Top: pos, Right: x + s.scale(3), Bottom: pos + thumb}, colorBorder)
}

func (s *Settings) paintFooter(c *win.Canvas, width, footerY int32, rows []row) {
	pad := s.scale(setPad)
	c.Fill(win.Rect{Left: pad, Top: footerY, Right: width - pad, Bottom: footerY + s.scale(1)}, colorSeparator)

	c.UseFont(s.fonts.Footer)
	text, color := s.hint(rows), colorDim
	if s.status != "" {
		text = s.status
		if s.failed {
			color = colorError
		} else {
			color = colorAccent
		}
	}
	c.Text(text, pad, footerY+s.scale(10), color)
}

// hint is the keyboard legend for the current selection.
func (s *Settings) hint(rows []row) string {
	if s.capture {
		return "Press the new combination · Esc to cancel"
	}
	if s.armed >= 0 && s.armed < len(rows) {
		return "Press Enter again to remove · Esc to cancel"
	}
	if s.sel >= 0 && s.sel < len(rows) {
		switch rows[s.sel].kind {
		case rowEntry:
			return "Enter removes this entry · ↑↓ move"
		case rowNumber, rowChoice:
			return "←→ change · ↑↓ move · Esc close"
		case rowHotkey:
			return "Enter records a new combination · Esc close"
		}
	}
	return "↑↓ move · Enter activate · Esc close"
}

func (s *Settings) setStatus(text string, failed bool) {
	s.status, s.failed = text, failed
}

func (s *Settings) setError(text string) { s.setStatus(text, true) }

// commit persists the config, reporting a failure in the footer instead of
// letting the user believe the edit stuck.
func (s *Settings) commit() bool {
	if s.hooks.Save == nil {
		return true
	}
	if err := s.hooks.Save(); err != nil {
		s.setError("Could not save config: " + err.Error())
		return false
	}
	return true
}

// HandleKey processes one key press. Every key is consumed: the settings window
// has no default keyboard behaviour worth keeping.
func (s *Settings) HandleKey(k win.KeyEvent) bool {
	if s.capture {
		s.captureKey(k)
		s.invalidate()
		return true
	}

	rows, contentH := s.build()
	s.clampSelection(rows, contentH)

	// A confirmation survives only the key that confirms it: anything else
	// disarms the pending destructive action.
	if k.VK != win.KeyReturn && k.VK != win.KeySpace && k.VK != win.KeyDelete {
		s.armed = -1
	}

	switch k.VK {
	case win.KeyUp:
		s.move(rows, -1)
	case win.KeyDown, win.KeyTab:
		s.move(rows, 1)
	case win.KeyPrior:
		s.move(rows, -s.pageRows())
	case win.KeyNext:
		s.move(rows, s.pageRows())
	case win.KeyHome:
		s.move(rows, -len(rows))
	case win.KeyEnd:
		s.move(rows, len(rows))
	case win.KeyLeft:
		s.step(rows, -1)
	case win.KeyRight:
		s.step(rows, 1)
	case win.KeyReturn, win.KeySpace:
		s.activate(rows)
	case win.KeyDelete:
		if s.sel >= 0 && s.sel < len(rows) {
			if kind := rows[s.sel].kind; kind == rowEntry || kind == rowDanger {
				// Delete goes through the same confirmation as Enter: a single
				// keystroke never deletes a folder or the history.
				s.activate(rows)
			}
		}
	case win.KeyEscape:
		s.hide()
	case 0x73: // F4
		if k.Alt {
			s.hide()
		}
	}
	s.invalidate()
	return true
}

// HandleClick selects the row under the cursor and runs its action. Removable
// rows only get selected: deleting on a stray click is not worth the risk.
func (s *Settings) HandleClick(x, y int32) {
	rows, _ := s.build()
	local := y - (s.scale(setContentTop) - s.scroll)
	s.armed = -1

	for i, r := range rows {
		if r.kind == rowHeader || local < r.y || local >= r.y+r.h {
			continue
		}
		s.sel = i
		switch r.kind {
		case rowNumber, rowChoice:
			if x > s.scale(setPad+setValueX) {
				s.step(rows, 1)
			} else {
				s.step(rows, -1)
			}
		case rowEntry, rowDanger:
			// selected only
		default:
			s.activate(rows)
		}
		break
	}
	s.invalidate()
}

// HandleWheel scrolls the content band.
func (s *Settings) HandleWheel(delta int32) {
	_, contentH := s.build()
	_, height := s.clientSize()
	visible := height - s.scale(setFooterH) - s.scale(setContentTop)

	step := s.scale(setRowH) * 2
	s.scroll -= delta / 120 * step
	s.clampScroll(contentH, visible)
	s.invalidate()
}

func (s *Settings) invalidate() {
	if s.window != nil {
		s.window.Invalidate()
	}
}

func (s *Settings) hide() {
	if s.window != nil {
		s.window.Hide()
	}
}

// owner is the window a modal shell dialog is parented to.
func (s *Settings) owner() uintptr {
	if s.window == nil {
		return 0
	}
	return s.window.HWND()
}

func (s *Settings) pageRows() int {
	_, height := s.clientSize()
	visible := height - s.scale(setFooterH) - s.scale(setContentTop)
	if rowH := s.scale(setRowH); rowH > 0 {
		if n := int(visible/rowH) - 1; n > 0 {
			return n
		}
	}
	return 1
}

func (s *Settings) clampScroll(contentH, visible int32) {
	if contentH <= visible || s.scroll <= 0 {
		s.scroll = 0
		return
	}
	if max := contentH - visible; s.scroll > max {
		s.scroll = max
	}
}

// clampSelection keeps sel on a focusable row, and scrolls it into view.
func (s *Settings) clampSelection(rows []row, contentH int32) {
	if s.sel < 0 || s.sel >= len(rows) || rows[s.sel].kind == rowHeader {
		s.sel = -1
		for i, r := range rows {
			if r.kind != rowHeader {
				s.sel = i
				break
			}
		}
	}

	_, height := s.clientSize()
	visible := height - s.scale(setFooterH) - s.scale(setContentTop)
	s.clampScroll(contentH, visible)

	if s.sel >= 0 && s.sel < len(rows) {
		r := rows[s.sel]
		if r.y < s.scroll {
			s.scroll = r.y
		} else if bottom := r.y + r.h; bottom > s.scroll+visible {
			s.scroll = bottom - visible
		}
		s.clampScroll(contentH, visible)
	}
}

// move walks to the next focusable row without wrapping: a form that jumps from
// the last row back to the first loses the user's place.
func (s *Settings) move(rows []row, delta int) {
	if delta == 0 {
		return
	}
	dir := 1
	if delta < 0 {
		dir = -1
	}
	left := delta
	if left < 0 {
		left = -left
	}
	for i := s.sel + dir; i >= 0 && i < len(rows) && left > 0; i += dir {
		if rows[i].kind == rowHeader {
			continue
		}
		s.sel = i
		left--
	}
}

func (s *Settings) step(rows []row, delta int) {
	if s.sel < 0 || s.sel >= len(rows) {
		return
	}
	if fn := rows[s.sel].step; fn != nil {
		fn(delta)
	}
}

func (s *Settings) activate(rows []row) {
	if s.sel < 0 || s.sel >= len(rows) {
		return
	}
	r := rows[s.sel]

	// Destructive rows take two presses: the first one arms, the second one
	// runs. Anything else would let a stray Enter delete a folder.
	if r.kind == rowEntry || r.kind == rowDanger {
		if s.armed != s.sel {
			s.armed = s.sel
			s.setStatus("Press Enter again to confirm", false)
			return
		}
		s.armed = -1
	}

	if r.kind == rowNumber || r.kind == rowChoice {
		if r.step != nil {
			r.step(1)
		}
		return
	}
	if r.act != nil {
		r.act()
	}
}

// beginCapture releases the global hotkey and waits for a combination. Without
// the release, RegisterHotKey would swallow the very keystroke the user is
// trying to record.
func (s *Settings) beginCapture() {
	if s.hooks.Hotkey == nil {
		return
	}
	s.capture = true
	s.armed = -1
	s.setStatus("Press the new combination · Esc to cancel", false)
	if s.hooks.HotkeyLock != nil {
		s.hooks.HotkeyLock(true)
	}
}

func (s *Settings) captureKey(k win.KeyEvent) {
	if k.VK == win.KeyEscape && !k.Ctrl && !k.Shift && !k.Alt {
		s.capture = false
		if s.hooks.HotkeyLock != nil {
			s.hooks.HotkeyLock(false)
		}
		s.setStatus("Hotkey unchanged", false)
		return
	}

	hk, ok := win.HotkeyFor(k.Ctrl, k.Alt, k.Shift, k.VK)
	if !ok {
		if !win.IsModifierKey(k.VK) {
			s.setStatus("Use at least one modifier: ctrl, alt or shift", true)
		}
		return
	}

	previous := s.cfg.Hotkey
	s.cfg.Hotkey = hk.Text
	s.capture = false

	if err := s.hooks.Hotkey(hk.Text); err != nil {
		s.cfg.Hotkey = previous
		if s.hooks.HotkeyLock != nil {
			s.hooks.HotkeyLock(false) // restore the previous combination
		}
		s.setError("Cannot use " + hk.Text + ": " + err.Error())
		return
	}
	if s.commit() {
		s.setStatus("Hotkey set to "+hk.Text, false)
	}
}

func (s *Settings) stepMaxResults(delta int) {
	n := s.cfg.MaxResults + delta
	if n < 1 {
		n = 1
	}
	if n > 20 {
		n = 20
	}
	if n == s.cfg.MaxResults {
		return
	}
	s.cfg.MaxResults = n
	if s.hooks.MaxResults != nil {
		s.hooks.MaxResults(n)
	}
	s.commit()
}

func (s *Settings) stepPathScope(delta int) {
	scopes := []string{core.PathOff, core.PathGUI, core.PathAll}
	current := 0
	for i, scope := range scopes {
		if scope == s.cfg.PathScope {
			current = i
		}
	}
	next := (current + delta + len(scopes)) % len(scopes)
	if scopes[next] == s.cfg.PathScope {
		return
	}
	s.cfg.PathScope = scopes[next]
	s.commit()
	s.rescan()
}

func (s *Settings) toggleAutostart() {
	enabled := !s.cfg.Autostart
	if s.hooks.Autostart != nil {
		if err := s.hooks.Autostart(enabled); err != nil {
			s.setError("Cannot change autostart: " + err.Error())
			return
		}
	}
	s.cfg.Autostart = enabled
	s.commit()
}

func (s *Settings) addCustomFolder() {
	path, err := win.PickFolder(s.owner(), "Add a folder to scan for applications")
	if err != nil {
		s.setError("Folder picker: " + err.Error())
		return
	}
	if path == "" {
		return // cancelled
	}
	s.cfg.CustomFolders = append(s.cfg.CustomFolders, path)
	s.cfg.Normalize()
	if s.commit() {
		s.setStatus("Added "+path, false)
	}
	s.rescan()
}

func (s *Settings) addExcludedFolder() {
	path, err := win.PickFolder(s.owner(), "Add a folder to skip")
	if err != nil {
		s.setError("Folder picker: " + err.Error())
		return
	}
	if path == "" {
		return
	}
	s.cfg.ExcludeFolders = append(s.cfg.ExcludeFolders, path)
	s.cfg.Normalize()
	if s.commit() {
		s.setStatus("Skipping "+path, false)
	}
	s.rescan()
}

func (s *Settings) dropCustomFolder(folder string) func() {
	return func() {
		s.cfg.CustomFolders = without(s.cfg.CustomFolders, folder)
		s.commit()
		s.setStatus("Removed "+folder, false)
		s.rescan()
	}
}

func (s *Settings) dropExcludedFolder(folder string) func() {
	return func() {
		s.cfg.ExcludeFolders = without(s.cfg.ExcludeFolders, folder)
		s.commit()
		s.setStatus("No longer skipping "+folder, false)
		s.rescan()
	}
}

func (s *Settings) addManualApp() {
	path, err := win.PickFile(s.owner(), "Add an application")
	if err != nil {
		s.setError("File picker: " + err.Error())
		return
	}
	if path == "" {
		return
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	s.cfg.ManualApps = append(s.cfg.ManualApps, core.ManualApp{Name: name, Target: path})
	if s.commit() {
		s.setStatus("Added "+name, false)
	}
	s.rescan()
}

func (s *Settings) dropManualApp(index int) func() {
	return func() {
		if index < 0 || index >= len(s.cfg.ManualApps) {
			return
		}
		name := s.cfg.ManualApps[index].Name
		kept := make([]core.ManualApp, 0, len(s.cfg.ManualApps)-1)
		kept = append(kept, s.cfg.ManualApps[:index]...)
		kept = append(kept, s.cfg.ManualApps[index+1:]...)
		s.cfg.ManualApps = kept
		s.commit()
		s.setStatus("Removed "+name, false)
		s.rescan()
	}
}

func (s *Settings) clearHistory() {
	if s.hooks.ClearLog == nil {
		return
	}
	if err := s.hooks.ClearLog(); err != nil {
		s.setError("Cannot clear history: " + err.Error())
		return
	}
	s.setStatus("History cleared", false)
}

func (s *Settings) rescan() {
	if s.hooks.Rescan != nil {
		s.hooks.Rescan()
	}
}

// without returns paths minus the one that matches case-insensitively. The
// result is a new slice: the old array may belong to a scan still in flight.
func without(paths []string, drop string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if !strings.EqualFold(p, drop) {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func baseName(path string) string {
	if base := filepath.Base(path); base != "" && base != "." && base != string(filepath.Separator) {
		return base
	}
	return path
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}
