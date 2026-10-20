package ui

import (
	"fmt"
	"os"
	"time"

	"applauncher/internal/core"
	"applauncher/internal/win"
)

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

	query    []rune
	results  []core.Result
	selected int
	visible  bool
	caretOn  bool
	status   string
	rebuild  func()
	timers   map[uintptr]func()
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
	a.query = a.query[:0]
	a.selected = 0
	a.status = ""
	a.refresh()

	a.caretOn = true
	a.window.Show()
	a.window.SetTimer(timerCaret, caretBlink)
	a.window.Invalidate()
}

// ShowQuery presents the launcher with a pre-filled query. Used by -demo.
func (a *App) ShowQuery(q string) {
	a.Show()
	a.query = []rune(q)
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

func (a *App) refresh() {
	idx := a.index()
	if idx == nil {
		a.results = nil
		return
	}
	a.results = idx.Search(core.Query{
		Text:    string(a.query),
		Limit:   a.config.MaxResults,
		History: a.history,
	})
	if a.selected >= len(a.results) {
		a.selected = len(a.results) - 1
	}
	if a.selected < 0 {
		a.selected = 0
	}
}

func (a *App) onChar(r rune) {
	if !a.visible {
		return
	}
	switch {
	case r == 8: // backspace
		if len(a.query) > 0 {
			a.query = a.query[:len(a.query)-1]
			a.selected = 0
			a.status = ""
			a.refresh()
			a.window.Invalidate()
		}
		return
	case r < 32 || r == 127:
		return
	}

	a.query = append(a.query, r)
	a.selected = 0
	a.status = ""
	a.refresh()
	a.window.Invalidate()
}

func (a *App) onKey(k win.KeyEvent) bool {
	if !a.visible {
		return false
	}

	switch k.VK {
	case win.KeyEscape:
		if len(a.query) > 0 {
			a.query = a.query[:0]
			a.selected = 0
			a.refresh()
			a.window.Invalidate()
			return true
		}
		a.Hide()
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
			a.status = "Rebuilding index…"
			a.window.Invalidate()
		}
		return true
	}
	return false
}

// SetRebuildHandler installs the callback behind F5 and the tray menu.
func (a *App) SetRebuildHandler(fn func()) { a.rebuild = fn }

// SetStatus replaces the footer hint until the next query change.
func (a *App) SetStatus(s string) {
	a.status = s
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
	if a.launch != nil {
		if err := a.launch(entry, mode); err != nil {
			// Keep the window open so the failure is visible.
			a.status = err.Error()
			a.window.Invalidate()
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
