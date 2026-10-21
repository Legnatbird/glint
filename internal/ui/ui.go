package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"glint/internal/core"
	"glint/internal/win"
)

// settingsKey marks the synthetic row that opens the settings window. It never
// enters the index: the launcher injects it while the query asks for it.
const settingsKey = "app:settings"

// settingsAliases are the words that surface that row.
var settingsAliases = []string{"settings", "config", "ajustes", "preferences"}

// settingsMinQuery is the shortest query that surfaces the row.
const settingsMinQuery = 3

// Mode is what Enter does to the selected row.
type Mode int

const (
	ModeLaunch Mode = iota // plain launch
	ModeAdmin              // Shift+Enter
	ModeReveal             // Ctrl+Enter
)

const (
	timerCaret = 1
	caretBlink = 530 * time.Millisecond
)

// App is the launcher state machine: query, results, selection and the keys
// that move between them. It owns no Win32 handle except through the Window.
type App struct {
	window  *win.Window
	fonts   *win.Fonts
	icons   *iconCache
	config  core.Config
	index   func() *core.Index
	history *core.History
	launch  func(core.Entry, Mode) error

	width, height int32
	dpi           int32

	field    textField
	results  []core.Result
	selected int
	visible  bool
	caretOn  bool
	status   string
	// statusFailed decides whether the footer paints the status as an error or
	// as progress.
	statusFailed bool
	// hover is the result under the pointer, -1 when there is none. It is
	// feedback, not selection: the keyboard selection never moves because the
	// pointer passed over a row.
	hover     int
	rebuild   func()
	open      func() // opens the settings window
	indexSeen *core.Index
	selfPath  string // the launcher executable, icon source for the settings row
	timers    map[uintptr]func()
}

// New builds the app around an existing window. index is a getter so the
// background scanner can swap the snapshot without coordinating with the UI.
func New(w *win.Window, cfg core.Config, index func() *core.Index, history *core.History, launch func(core.Entry, Mode) error) *App {
	dpi := int32(w.DPI())
	return &App{
		window:  w,
		fonts:   win.NewFonts(uint32(dpi)),
		icons:   newIconCache(128),
		config:  cfg,
		index:   index,
		history: history,
		launch:  launch,
		dpi:     dpi,
		hover:   -1,
	}
}

// Close releases the GDI and icon resources owned by the app.
func (a *App) Close() {
	a.fonts.Release()
	a.icons.close()
}

// Attach wires the app to the window's message callbacks.
func (a *App) Attach() {
	a.window.OnPaint(a.paint)
	a.window.OnChar(a.onChar)
	a.window.OnKey(a.onKey)
	a.window.OnMouseMove(a.mouseMove)
	a.window.OnMouseLeave(a.mouseLeave)
	a.window.OnClick(a.mouseClick)
	a.window.OnWheel(a.mouseWheel)
	a.window.OnDeactivate(func() {
		if a.visible {
			a.Hide()
		}
	})
	a.window.OnTimer(a.onTimer)
}

// SetSize informs the app of the client area, so the layout can be computed.
func (a *App) SetSize(width, height int32) {
	a.width, a.height = width, height
}

// SetMaxResults changes how many rows the list shows. It is the live half of the
// settings window: the next reflow uses the new limit immediately.
func (a *App) SetMaxResults(n int) {
	if n <= 0 || n == a.config.MaxResults {
		return
	}
	a.config.MaxResults = n
	a.refresh()
	a.window.Invalidate()
}

// Visible reports whether the launcher is on screen.
func (a *App) Visible() bool { return a.visible }

// Toggle shows or hides the launcher, the way the global hotkey behaves.
func (a *App) Toggle() {
	if a.visible {
		a.Hide()
		return
	}
	a.Show()
}

// Show clears the query and presents the launcher.
func debugf(format string, args ...any) {
	if os.Getenv("LAUNCHER_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "ui: "+format+"\n", args...)
	}
}

// Show clears the query and presents the launcher.
func (a *App) Show() {
	debugf("show")
	a.visible = true
	a.field.clear()
	a.selected = 0
	a.status = ""
	// A highlight from the previous appearance would point at whatever is under
	// the pointer now, which the next pointer move re-establishes anyway.
	a.hover = -1
	a.refresh()

	a.caretOn = true
	a.window.Show()
	a.window.SetTimer(timerCaret, caretBlink)
	a.window.Invalidate()
}

// ShowQuery presents the launcher with a pre-filled query. Used by -demo.
func (a *App) ShowQuery(q string) {
	a.Show()
	a.field.setText(q)
	a.refresh()
	a.window.Invalidate()
}

// Hide dismisses the launcher without touching the index.
func (a *App) Hide() {
	debugf("hide")
	a.visible = false
	a.window.KillTimer(timerCaret)
	a.window.Hide()
}

// rowAt returns the index of the result under client y, or -1 when the pointer
// is outside the list.
func (a *App) rowAt(y int32) int {
	l := a.computeLayout()
	if y < l.listTop {
		return -1
	}
	row := int((y - l.listTop) / l.rowH)
	if row < 0 || row >= int(l.rows) {
		return -1
	}
	idx := row + a.listOffset(l)
	if idx < 0 || idx >= len(a.results) {
		return -1
	}
	return idx
}

// listOffset is the first visible result. Painting and hit-testing both call it,
// so the row the pointer lights up is the row the pointer is actually over.
func (a *App) listOffset(l layout) int {
	if a.selected >= int(l.rows) {
		return a.selected - int(l.rows) + 1
	}
	return 0
}

func (a *App) mouseMove(x, y int32) {
	row := a.rowAt(y)
	if row == a.hover {
		return
	}
	a.hover = row
	a.window.Invalidate()
}

func (a *App) mouseLeave() {
	if a.hover == -1 {
		return
	}
	a.hover = -1
	a.window.Invalidate()
}

// mouseClick launches the row under the pointer. Hovering never moves the
// keyboard selection, but a click does: it names the row the user picked, which
// is what has to stay selected if the launch fails.
func (a *App) mouseClick(x, y int32) {
	row := a.rowAt(y)
	if row < 0 {
		return
	}
	a.selected = row
	a.activate(win.KeyEvent{})
}

// mouseWheel walks the selection the way the arrow keys do. The list never holds
// more rows than fit (max results is eight by default), so scrolling the view
// would have nothing to scroll.
func (a *App) mouseWheel(delta int32) {
	n := len(a.results)
	if n == 0 || delta == 0 {
		return
	}
	// Positive delta is a wheel turn away from the user, which walks backwards
	// through the list the way the up arrow does.
	step := 3
	if delta > 0 {
		step = -3
	}
	a.selected = ((a.selected+step)%n + n) % n
	a.window.Invalidate()
}

// Reload recomputes the list from the current index.
//
// It is the message a finished scan sends. Without it the window keeps the list
// it built when it was first painted: a cold start would show "Indexing…" until
// the next keystroke, and a rebuild started from the tray would leave its status
// on the footer for good.
func (a *App) Reload() {
	a.refresh()
	if a.visible {
		a.window.Invalidate()
	}
}

// refresh rebuilds the result list for the current query.
func (a *App) refresh() {
	idx := a.index()
	if idx == nil {
		a.results = nil
		return
	}
	// A finished scan brings icons that did not exist when the window first
	// painted (packaged apps have no icon until the AppsFolder is read), so the
	// cache is dropped when the snapshot changes.
	if idx != a.indexSeen {
		a.indexSeen = idx
		a.icons.reset()
	}

	query := a.field.String()
	results := idx.Search(core.Query{
		Text:    query,
		Limit:   a.config.MaxResults,
		History: a.history,
	})

	// The settings window is reachable by typing its name, the way every other
	// row is reachable: one list, one keyboard model, no hidden menu.
	if a.open != nil && wantsSettings(query) {
		row := core.Result{Entry: core.Entry{
			Name:     "Settings",
			Key:      settingsKey,
			IconPath: a.selfPath,
		}, Score: 1}
		results = append([]core.Result{row}, results...)
		if len(results) > a.config.MaxResults {
			results = results[:a.config.MaxResults]
		}
	}
	a.results = results

	if a.selected >= len(a.results) {
		a.selected = len(a.results) - 1
	}
	if a.selected < 0 {
		a.selected = 0
	}
}

// wantsSettings reports whether the query is asking for the settings window. A
// prefix match keeps it predictable: "sett" and "con" work, "xyz" does not.
func wantsSettings(query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	// Shorter queries would shadow every application starting with the same
	// letters, which is exactly what a launcher must not do.
	if len(query) < settingsMinQuery {
		return false
	}
	for _, alias := range settingsAliases {
		if strings.HasPrefix(alias, query) {
			return true
		}
	}
	return false
}

// resetCaret shows the caret again and restarts its blink, so it is never
// invisible at the moment the user starts typing.
func (a *App) resetCaret() {
	a.caretOn = true
	if a.visible {
		a.window.SetTimer(timerCaret, caretBlink)
	}
}

func (a *App) onChar(r rune) {
	if !a.visible {
		return
	}
	switch {
	case r == 8: // backspace
		if a.field.backspace() {
			a.afterEdit()
		}
		return
	case r < 32 || r == 127:
		return
	}

	a.field.insertRune(r)
	a.afterEdit()
}

// afterEdit is the shared tail of every change to the query: the list has to be
// recomputed, the selection goes back to the top result, and the caret is shown
// again so it is never invisible right after typing.
func (a *App) afterEdit() {
	a.selected = 0
	a.status = ""
	a.refresh()
	a.resetCaret()
	a.window.Invalidate()
}

// adjust repaints after the caret or the selection moved. The results do not
// change, so nothing is recomputed.
func (a *App) adjust() {
	a.resetCaret()
	a.window.Invalidate()
}

// singleLine turns a pasted block into one line: a query field holds no
// newlines, and silently keeping half a path would be worse than a space.
func singleLine(r rune) rune {
	switch r {
	case '\r', '\n', '\t':
		return ' '
	}
	return r
}

func (a *App) onKey(k win.KeyEvent) bool {
	if !a.visible {
		return false
	}

	// Select all first: it is the gesture the user reaches for before replacing
	// or deleting a query, and it needs no clipboard.
	if k.Ctrl && k.VK == 'A' {
		a.field.selectAll()
		a.window.Invalidate()
		return true
	}

	switch k.VK {
	case win.KeyEscape:
		if !a.field.Empty() {
			a.field.clear()
			a.selected = 0
			a.refresh()
			a.resetCaret()
			a.window.Invalidate()
			return true
		}
		a.Hide()
		return true

	case win.KeyLeft:
		if k.Ctrl {
			a.field.wordLeft(k.Shift)
		} else {
			a.field.left(k.Shift)
		}
		a.adjust() // the results do not change with the caret
		return true

	case win.KeyRight:
		if k.Ctrl {
			a.field.wordRight(k.Shift)
		} else {
			a.field.right(k.Shift)
		}
		a.adjust()
		return true

	case win.KeyHome:
		a.field.home(k.Shift)
		a.adjust()
		return true

	case win.KeyEnd:
		a.field.end(k.Shift)
		a.adjust()
		return true

	case win.KeyDelete:
		if k.Ctrl {
			if a.field.deleteWordForward() {
				a.afterEdit()
			}
			return true
		}
		if a.field.deleteForward() {
			a.afterEdit()
		}
		return true

	case win.KeyUp:
		if len(a.results) > 0 {
			a.selected = (a.selected - 1 + len(a.results)) % len(a.results)
			a.window.Invalidate()
		}
		return true

	case win.KeyDown:
		if len(a.results) > 0 {
			a.selected = (a.selected + 1) % len(a.results)
			a.window.Invalidate()
		}
		return true

	case win.KeyReturn:
		a.activate(k)
		return true

	case win.KeyF5:
		if a.rebuild != nil {
			a.rebuild()
			a.SetStatus("Rebuilding index…")
			a.window.Invalidate()
		}
		return true
	}
	return false
}

// SetRebuildHandler installs the callback behind F5 and the tray menu.
// SetRebuildHandler installs the callback behind F5.
func (a *App) SetRebuildHandler(fn func()) { a.rebuild = fn }

// SetSettingsHandler installs what the synthetic Settings row opens.
func (a *App) SetSettingsHandler(fn func()) { a.open = fn }

// SetSelfPath tells the app where its own executable lives. The settings row
// borrows the icon embedded in it, so the launcher never carries a second copy
// of its own mark in memory.
func (a *App) SetSelfPath(path string) { a.selfPath = path }

// SetStatus puts a neutral notice in the footer until the next query change:
// progress ("Rebuilding index…") is not a failure and should not be painted like
// one.
func (a *App) SetStatus(s string) {
	a.status, a.statusFailed = s, false
	if a.visible {
		a.window.Invalidate()
	}
}

// SetError reports a failure in the footer and keeps the window open, so the
// message is not missed.
func (a *App) SetError(s string) {
	a.status, a.statusFailed = s, true
	if a.visible {
		a.window.Invalidate()
	}
}

func (a *App) activate(k win.KeyEvent) {
	if len(a.results) == 0 {
		return
	}

	mode := ModeLaunch
	switch {
	case k.Shift:
		mode = ModeAdmin
	case k.Ctrl:
		mode = ModeReveal
	}

	entry := a.results[a.selected].Entry
	if entry.Key == settingsKey {
		a.Hide()
		if a.open != nil {
			a.open()
		}
		return
	}

	if a.launch != nil {
		if err := a.launch(entry, mode); err != nil {
			// Keep the window open so the failure is visible.
			a.SetError(err.Error())
			return
		}
	}
	a.Hide()
}

func (a *App) onTimer(id uintptr) {
	if fn, ok := a.timers[id]; ok {
		delete(a.timers, id)
		fn()
		return
	}
	if id != timerCaret || !a.visible {
		return
	}
	a.caretOn = !a.caretOn
	a.window.Invalidate()
}

// RunAfter schedules fn on the message thread after d. Used for the -demo mode
// and for the startup toasts of later phases.
func (a *App) RunAfter(id uintptr, d time.Duration, fn func()) {
	if a.timers == nil {
		a.timers = make(map[uintptr]func())
	}
	a.timers[id] = fn
	a.window.SetTimer(id, d)
}
