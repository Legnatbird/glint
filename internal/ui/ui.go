package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"applauncher/internal/core"
	"applauncher/internal/win"
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

	query    []rune
	results  []core.Result
	selected int
	visible  bool
	caretOn  bool
	status   string
	rebuild  func()
	open     func() // opens the settings window
	selfPath string // the launcher executable, icon source for the settings row
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

	query := string(a.query)
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
// SetRebuildHandler installs the callback behind F5.
func (a *App) SetRebuildHandler(fn func()) { a.rebuild = fn }

// SetSettingsHandler installs what the synthetic Settings row opens.
func (a *App) SetSettingsHandler(fn func()) { a.open = fn }

// SetSelfPath tells the app where its own executable lives. The settings row
// borrows the icon embedded in it, so the launcher never carries a second copy
// of its own mark in memory.
func (a *App) SetSelfPath(path string) { a.selfPath = path }

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
