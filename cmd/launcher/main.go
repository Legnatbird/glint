// Command launcher is the application launcher.
//
// It keeps one hidden topmost window alive, listens for a global hotkey, and
// answers every keystroke from an in-memory index built by internal/sources.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"applauncher/internal/core"
	"applauncher/internal/sources"
	"applauncher/internal/ui"
	"applauncher/internal/win"
)

func main() {
	// A console-subsystem build (plain `go build ./cmd/launcher`) would keep a
	// black window alive for the whole life of the launcher. Detach before
	// anything else; -dump needs the console and is exempt.
	if !wantsDump(os.Args[1:]) {
		win.FreeConsole()
	}

	dump := flag.Bool("dump", false, "print the discovered applications and exit")
	demo := flag.String("demo", "", "open the launcher with this query and exit after -seconds")
	seconds := flag.Int("seconds", 0, "with -demo: how long to stay on screen")
	openSettings := flag.Bool("settings", false, "open the settings window at startup")
	flag.Parse()

	// The message loop and every COM call must live on this thread.
	runtime.LockOSThread()
	release := win.CoInitialize()
	defer release()

	cfg, err := core.LoadConfig(configPath())
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
	}

	if *dump {
		win.AttachParentConsole()
		runDump(cfg)
		return
	}

	// A second launch must not create a second launcher: it wakes the first one.
	if !win.AcquireSingleInstance("app-launcher") {
		win.PostToRunningInstance(win.MsgToggle)
		return
	}

	run(cfg, *demo, *seconds, *openSettings)
}

func run(cfg core.Config, demo string, seconds int, startWithSettings bool) {
	dpi := int32(win.SystemDPI())

	shell, err := win.NewWindow(win.WindowConfig{
		Title:      "Launcher",
		Width:      700 * dpi / 96,
		Height:     500 * dpi / 96,
		Background: ui.ColorBackground,
		Layered:    cfg.Backdrop == core.BackdropAcrylic,
	})
	if err != nil {
		fatal(err)
	}
	defer shell.Destroy()

	if err := applyBackdrop(shell, cfg.Backdrop, dpi); err != nil {
		debugf("backdrop: %v", err)
	}

	history, err := core.LoadHistory(historyPath(), cfg.History.MaxEntries)
	if err != nil {
		fmt.Fprintln(os.Stderr, "history:", err)
	}

	var index atomic.Pointer[core.Index]
	scanner := &rebuilder{index: &index, cfg: &cfg}
	scanner.loadCache()
	scanner.rebuild()

	launchFn := func(entry core.Entry, mode ui.Mode) error {
		if err := launch(entry, mode); err != nil {
			return err
		}
		if cfg.History.Enabled {
			history.Touch(entry.Key, time.Now())
			if err := history.Save(historyPath()); err != nil {
				fmt.Fprintln(os.Stderr, "history:", err)
			}
		}
		return nil
	}

	app := ui.New(shell, cfg, index.Load, history, launchFn)
	defer app.Close()
	app.Attach()
	app.SetSize(shell.ClientSize())
	app.SetRebuildHandler(func() { scanner.rebuild() })
	if exe, err := os.Executable(); err == nil {
		app.SetSelfPath(exe)
	}

	hotkey := &hotkeyBinding{shell: shell}
	if err := hotkey.apply(cfg.Hotkey); err != nil {
		debugf("hotkey error: %v", err)
	} else {
		debugf("hotkey registered: %s", cfg.Hotkey)
	}

	debugf("autostart=%v", cfg.Autostart)
	if err := win.SetAutostart(cfg.Autostart); err != nil {
		debugf("autostart error: %v", err)
	}

	// The settings window is created on first use. A launcher that is never
	// configured should not pay for a window, its fonts and its handles.
	var settings *win.SettingsWindow
	showSettings := func() {
		if settings == nil {
			w, err := win.NewSettingsWindow("app-launcher settings", 600*dpi/96, 640*dpi/96)
			if err != nil {
				debugf("settings window failed: %v", err)
				return
			}
			settings = w
			// The form owns every edit; each hook applies one change to the
			// running process, so nothing here waits for a restart.
			ui.NewSettings(settings, &cfg, win.NewFonts(uint32(dpi)), ui.SettingsHooks{
				Save:       func() error { return cfg.Save(configPath()) },
				Hotkey:     hotkey.apply,
				HotkeyLock: hotkey.lock,
				Backdrop:   func(mode string) error { return applyBackdrop(shell, mode, dpi) },
				Autostart:  win.SetAutostart,
				MaxResults: app.SetMaxResults,
				Rescan:     func() { scanner.rebuild() },
				ClearLog: func() error {
					history.Clear()
					return history.Save(historyPath())
				},
				Entries: history.Len,
			})
			debugf("settings window ready hwnd=%d", settings.HWND())
		}
		if !settings.Visible() {
			settings.Show()
		}
	}
	app.SetSettingsHandler(showSettings)

	shell.OnHotkey(func() {
		// While the settings window is open it owns the screen.
		if settings != nil && settings.Visible() {
			return
		}
		app.Toggle()
	})
	shell.OnClose(shell.PostQuit)

	tray, err := win.NewTray(shell.HWND(), "app-launcher")
	if err != nil {
		debugf("tray error: %v", err)
	} else {
		debugf("tray added")
		defer tray.Remove()
		shell.OnTray(func(msg uint32) {
			switch msg {
			case win.WmLButtonUp:
				app.Toggle()
			case win.WmRButtonUp:
				showTrayMenu(tray, app, &cfg, scanner, history, shell, showSettings)
			}
		})
	}

	if startWithSettings {
		showSettings()
	}

	if demo != "" {
		app.RunAfter(10, 300*time.Millisecond, func() { app.ShowQuery(demo) })
	}
	if seconds > 0 {
		app.RunAfter(11, time.Duration(seconds)*time.Second, shell.PostQuit)
	}

	if err := shell.Run(); err != nil {
		fatal(err)
	}
}

// showTrayMenu runs the context menu. It is called from the window thread, so
// everything it touches is already on the right thread.
func showTrayMenu(tray *win.Tray, app *ui.App, cfg *core.Config, scanner *rebuilder, history *core.History, shell *win.Window, showSettings func()) {
	switch tray.Popup([]win.MenuItem{
		{ID: 1, Label: "Show launcher"},
		{ID: 2, Label: "Rebuild index"},
		{ID: 3, Label: "Start with Windows", Checked: win.AutostartEnabled()},
		{ID: 5, Label: "Settings…"},
		{Separator: true},
		{ID: 4, Label: "Quit"},
	}) {
	case 1:
		app.Show()

	case 2:
		app.SetStatus("Rebuilding index…")
		scanner.rebuild()

	case 3:
		cfg.Autostart = !win.AutostartEnabled()
		if err := win.SetAutostart(cfg.Autostart); err != nil {
			fmt.Fprintln(os.Stderr, "autostart:", err)
			return
		}
		if err := cfg.Save(configPath()); err != nil {
			fmt.Fprintln(os.Stderr, "config:", err)
		}

	case 5:
		showSettings()

	case 4:
		if err := history.Save(historyPath()); err != nil {
			fmt.Fprintln(os.Stderr, "history:", err)
		}
		tray.Remove()
		shell.PostQuit()
	}
}

// rebuilder owns the index snapshot and guarantees one scan at a time.
type rebuilder struct {
	index *atomic.Pointer[core.Index]
	// cfg is shared with the settings form: an edit plus a rebuild is all it
	// takes to pick up a new folder, scope or manual app.
	cfg *core.Config

	mu      sync.Mutex
	running bool
}

// loadCache serves the cached snapshot immediately so the first hotkey never
// waits for a scan.
func (r *rebuilder) loadCache() {
	cache, err := core.LoadCache(cachePath())
	if err == nil && cache.Valid() {
		r.index.Store(core.NewIndex(cache.Entries))
	}
}

// rebuild scans every source off the UI thread and swaps the result in.
func (r *rebuilder) rebuild() {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	r.mu.Unlock()

	// Snapshot the options on the caller's thread. rebuild always runs from the
	// message thread, and the settings form edits the same config there: reading
	// it from the scan goroutine would race with those edits.
	opts := snapshotOptions(r.cfg)

	go func() {
		defer func() {
			r.mu.Lock()
			r.running = false
			r.mu.Unlock()
		}()

		runtime.LockOSThread()
		release := win.CoInitialize()
		defer release()

		rep := sources.Scan(opts)
		if len(rep.Entries) == 0 {
			return
		}

		r.index.Store(core.NewIndex(rep.Entries))

		cache := core.Cache{Built: time.Now().UTC(), Entries: rep.Entries}
		if err := cache.Save(cachePath()); err != nil {
			fmt.Fprintln(os.Stderr, "cache:", err)
		}
	}()
}

// snapshotOptions copies the scan settings out of the live config, including
// the slices, so the goroutine owns everything it reads.
func snapshotOptions(cfg *core.Config) sources.Options {
	return sources.Options{
		CustomFolders:   append([]string(nil), cfg.CustomFolders...),
		ExcludeFolders:  append([]string(nil), cfg.ExcludeFolders...),
		PathScope:       cfg.PathScope,
		IncludeAppPaths: cfg.IncludeAppPaths,
		ManualApps:      append([]core.ManualApp(nil), cfg.ManualApps...),
	}
}

func launch(entry core.Entry, mode ui.Mode) error {
	switch mode {
	case ui.ModeReveal:
		path := entry.Resolved
		if path == "" {
			path = entry.Target
		}
		if path == "" {
			return fmt.Errorf("no file to reveal for %s", entry.Name)
		}
		return win.RevealInExplorer(path)

	case ui.ModeAdmin:
		if entry.Kind() == core.KindUWP {
			// A packaged app is activated, not executed, so the AUMID cannot be
			// elevated. Its execution alias is an ordinary command, and the
			// shell will run that one with the "runas" verb.
			alias := win.ExecutionAlias(entry.AUMID)
			if alias == "" {
				return fmt.Errorf("%s declares no execution alias, so it cannot run elevated", entry.Name)
			}
			return win.ShellExecute(alias, "", "", true)
		}
		return win.ShellExecute(entry.Target, entry.Args, entry.WorkDir, true)

	default:
		if entry.Kind() == core.KindUWP {
			return win.ActivateApp(entry.AUMID)
		}
		return win.ShellExecute(entry.Target, entry.Args, entry.WorkDir, false)
	}
}

func runDump(cfg core.Config) {
	rep := sources.Scan(sources.Options{
		CustomFolders:   cfg.CustomFolders,
		ExcludeFolders:  cfg.ExcludeFolders,
		PathScope:       cfg.PathScope,
		IncludeAppPaths: cfg.IncludeAppPaths,
		ManualApps:      cfg.ManualApps,
	})

	fmt.Printf("scanned %d entries in %v\n", len(rep.Entries), rep.Duration.Round(time.Millisecond))
	fmt.Printf("per source: start menu=%d store apps=%d app paths=%d path(%s)=%d custom=%d\n",
		rep.Counts["start menu"], rep.Counts["store apps"], rep.Counts["app paths"],
		cfg.PathScope, rep.Counts["path"], rep.Counts["custom folders"])
	for _, w := range rep.Warnings {
		fmt.Println("warning:", w)
	}

	idx := core.NewIndex(rep.Entries)
	fmt.Printf("index after dedup: %d\n\n", idx.Len())

	for _, e := range idx.Entries() {
		target := e.Resolved
		if target == "" {
			target = e.Target
		}
		if target == "" {
			target = "(store app) " + e.AUMID
		}
		fmt.Printf("  %-38.38s %s\n", e.Name, target)
	}

	fmt.Println("\nsample searches:")
	for _, q := range []string{"code", "chr", "term", "edge", "conf"} {
		fmt.Printf("  %-6s", q)
		for i, r := range idx.Search(core.Query{Text: q, Limit: 3}) {
			if i > 0 {
				fmt.Print(" | ")
			}
			fmt.Printf("%s %.2f", r.Entry.Name, r.Score)
		}
		fmt.Println()
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "launcher:", err)
	os.Exit(1)
}

// wantsDump reports whether -dump is present, before flag.Parse runs.
func wantsDump(args []string) bool {
	for _, a := range args {
		switch a {
		case "-dump", "--dump", "-dump=true", "--dump=true":
			return true
		}
	}
	return false
}

// applyBackdrop switches the launcher window between the two backdrops without
// recreating it. Acrylic needs WS_EX_LAYERED and the undocumented accent policy;
// solid disables both and leaves the painted content on its own.
func applyBackdrop(w *win.Window, mode string, dpi int32) error {
	hwnd := w.HWND()
	if mode == core.BackdropAcrylic {
		if err := win.SetLayered(hwnd, true); err != nil {
			return err
		}
		// Shape first: the region only clips what we paint, while the accent
		// covers the whole window rectangle. Applying the accent to an already
		// shaped window keeps the tint from spilling into the corners.
		if err := shapeWindow(hwnd, dpi); err != nil {
			return err
		}
		// AABBGGRR: dark tint at 91% alpha. The tint is what controls how much
		// of the blurred desktop reaches the eye. Note that GDI paint on this
		// layered window lands at roughly half opacity, which is why the accent
		// shows through the panel at all.
		if err := win.EnableAcrylic(hwnd, 0xE8161212, 255); err != nil {
			return err
		}
		debugf("backdrop acrylic")
		w.Invalidate()
		return nil
	}

	win.DisableAcrylic(hwnd)
	if err := win.SetLayered(hwnd, false); err != nil {
		return err
	}
	debugf("backdrop solid")
	if err := shapeWindow(hwnd, dpi); err != nil {
		return err
	}
	w.Invalidate()
	return nil
}

// shapeWindow gives the launcher rounded corners.
//
// DWM is asked first: its corners are antialiased and need no region. Windows 10
// has no such attribute, so the window is clipped to a region and the arc is a
// staircase of whole pixels.
func shapeWindow(hwnd uintptr, dpi int32) error {
	if err := win.RoundCornersDWM(hwnd); err == nil {
		win.ClearRegion(hwnd)
		debugf("corners: dwm")
		return nil
	}
	return win.RoundCorners(hwnd, ui.CornerRadius*dpi/96)
}

// hotkeyBinding owns the single global hotkey id and remembers whether it is
// registered, so the settings window can release it while recording a
// combination and put it back afterwards.
type hotkeyBinding struct {
	shell      *win.Window
	text       string // last combination that registered successfully
	registered bool
}

// apply installs text as the global hotkey, replacing whatever was registered.
func (h *hotkeyBinding) apply(text string) error {
	h.release()

	hk, err := win.ParseHotkey(text)
	if err != nil {
		return err
	}
	if err := h.shell.RegisterHotkey(hk); err != nil {
		return err
	}
	h.registered = true
	h.text = text
	return nil
}

// release frees the combination without touching the config.
func (h *hotkeyBinding) release() {
	if h.registered {
		_ = h.shell.UnregisterHotkey()
		h.registered = false
	}
}

// lock is the settings hook: capturing releases the hotkey so the keystroke
// reaches the window, and leaving capture registers it again. text keeps the
// last combination that worked, so a rejected edit restores the previous one.
func (h *hotkeyBinding) lock(capturing bool) {
	if capturing {
		h.release()
		return
	}
	if h.registered {
		return
	}
	if err := h.apply(h.text); err != nil {
		debugf("hotkey restore: %v", err)
	}
}
