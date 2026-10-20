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

	run(cfg, *demo, *seconds)
}

func run(cfg core.Config, demo string, seconds int) {
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

	if cfg.Backdrop == core.BackdropAcrylic {
		if err := win.RoundCorners(shell.HWND(), 10*dpi/96); err != nil {
			debugf("rounded corners: %v", err)
		}
		// AABBGGRR: dark tint at 91% alpha. The tint is what controls how much
		// of the blurred desktop reaches the eye; the paint stays opaque.
		if err := win.EnableAcrylic(shell.HWND(), 0xE8161212, 255); err != nil {
			debugf("acrylic: %v", err)
		} else {
			debugf("acrylic enabled")
		}
	}

	history, err := core.LoadHistory(historyPath(), cfg.History.MaxEntries)
	if err != nil {
		fmt.Fprintln(os.Stderr, "history:", err)
	}

	var index atomic.Pointer[core.Index]
	scanner := &rebuilder{index: &index, cfg: cfg}
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

	shell.OnHotkey(app.Toggle)
	shell.OnClose(shell.PostQuit)

	if hk, err := win.ParseHotkey(cfg.Hotkey); err != nil {
		fmt.Fprintln(os.Stderr, "hotkey:", err)
	} else if err := shell.RegisterHotkey(hk); err != nil {
		debugf("hotkey error: %v", err)
	} else {
		debugf("hotkey registered: %s", cfg.Hotkey)
	}

	debugf("autostart=%v", cfg.Autostart)
	if err := win.SetAutostart(cfg.Autostart); err != nil {
		debugf("autostart error: %v", err)
	}

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
				showTrayMenu(tray, app, &cfg, scanner, history, shell)
			}
		})
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
func showTrayMenu(tray *win.Tray, app *ui.App, cfg *core.Config, scanner *rebuilder, history *core.History, shell *win.Window) {
	switch tray.Popup([]win.MenuItem{
		{ID: 1, Label: "Show launcher"},
		{ID: 2, Label: "Rebuild index"},
		{ID: 3, Label: "Start with Windows", Checked: win.AutostartEnabled()},
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
	cfg   core.Config

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

	go func() {
		defer func() {
			r.mu.Lock()
			r.running = false
			r.mu.Unlock()
		}()

		runtime.LockOSThread()
		release := win.CoInitialize()
		defer release()

		rep := sources.Scan(sources.Options{
			CustomFolders:   r.cfg.CustomFolders,
			ExcludeFolders:  r.cfg.ExcludeFolders,
			PathScope:       r.cfg.PathScope,
			IncludeAppPaths: r.cfg.IncludeAppPaths,
			ManualApps:      r.cfg.ManualApps,
		})
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
			// Packaged apps are activated, not executed: there is no process to
			// elevate. Its execution alias could be run with "runas", but the
			// alias name is not derivable from the AppUserModelID and the stub
			// files in WindowsApps are zero bytes, so it cannot be discovered
			// reliably yet.
			return fmt.Errorf("%s is a packaged app and cannot be elevated from here", entry.Name)
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
