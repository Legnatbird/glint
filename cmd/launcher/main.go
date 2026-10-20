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
	"sync/atomic"
	"time"

	"applauncher/internal/core"
	"applauncher/internal/sources"
	"applauncher/internal/ui"
	"applauncher/internal/win"
)

func main() {
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
		Width:      660 * dpi / 96,
		Height:     430 * dpi / 96,
		Background: ui.ColorBackground,
	})
	if err != nil {
		fatal(err)
	}
	defer shell.Destroy()

	var index atomic.Pointer[core.Index]
	loadIndex(&index, cfg)

	app := ui.New(shell, cfg, index.Load, nil, launch)
	app.Attach()
	app.SetSize(shell.ClientSize())

	shell.OnHotkey(app.Toggle)
	shell.OnClose(shell.PostQuit)

	if hk, err := win.ParseHotkey(cfg.Hotkey); err != nil {
		fmt.Fprintln(os.Stderr, "hotkey:", err)
	} else if err := shell.RegisterHotkey(hk); err != nil {
		fmt.Fprintln(os.Stderr, "hotkey:", err)
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

// loadIndex serves the cached snapshot immediately, then rescans in the
// background and swaps the result in. The first hotkey never waits for a scan.
func loadIndex(index *atomic.Pointer[core.Index], cfg core.Config) {
	if cache, err := core.LoadCache(cachePath()); err == nil && cache.Valid() {
		index.Store(core.NewIndex(cache.Entries))
	}

	go func() {
		runtime.LockOSThread()
		release := win.CoInitialize()
		defer release()

		rep := sources.Scan(sources.Options{
			CustomFolders:   cfg.CustomFolders,
			ExcludeFolders:  cfg.ExcludeFolders,
			PathScope:       cfg.PathScope,
			IncludeAppPaths: cfg.IncludeAppPaths,
		})
		if len(rep.Entries) == 0 {
			return
		}

		index.Store(core.NewIndex(rep.Entries))

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
			return fmt.Errorf("%s cannot run elevated", entry.Name)
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
