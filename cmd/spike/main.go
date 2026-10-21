// Command spike measures the baseline cost of the launcher shell: one hidden
// topmost window, one global hotkey, one idle message loop and nothing else.
//
// Phase P0 of docs/plan.md uses it to prove (or refute) the memory and latency
// budget before any feature work starts. It prints a JSON report and exits.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"glint/internal/win"
)

type report struct {
	Boot             float64  `json:"boot_ms"`
	Hotkey           string   `json:"hotkey"`
	HotkeyOK         bool     `json:"hotkey_ok"`
	ColdShowMS       float64  `json:"cold_show_ms"`
	WarmShowMS       float64  `json:"warm_show_ms"`
	ColdFirstPaintMS float64  `json:"cold_first_paint_ms"`
	WarmFirstPaintMS float64  `json:"warm_first_paint_ms"`
	IdleWorkingSetMB float64  `json:"idle_working_set_mb"`
	IdlePrivateMB    float64  `json:"idle_private_mb"`
	FinalWorkingSet  float64  `json:"final_working_set_mb"`
	PeakWorkingSetMB float64  `json:"peak_working_set_mb"`
	IdleCPUPercent   float64  `json:"idle_cpu_percent"`
	GDIObjects       uint32   `json:"gdi_objects"`
	USERObjects      uint32   `json:"user_objects"`
	KernelHandles    uint32   `json:"kernel_handles"`
	Goroutines       int      `json:"goroutines"`
	DPI              string   `json:"dpi_awareness"`
	UptimeSeconds    float64  `json:"uptime_s"`
	Errors           []string `json:"errors,omitempty"`
}

func main() {
	seconds := flag.Int("seconds", 6, "how long to stay alive before the report")
	hotkeyText := flag.String("hotkey", "ctrl+alt+space", "global hotkey to register")
	flag.Parse()

	rep := report{Hotkey: *hotkeyText}
	fail := func(err error) {
		if err != nil {
			rep.Errors = append(rep.Errors, err.Error())
		}
	}
	emit := func() {
		out, _ := json.MarshalIndent(rep, "", "  ")
		fmt.Println(string(out))
	}

	rep.DPI = win.EnableDPIAwareness()

	bootStart := time.Now()
	shell, err := win.NewWindow(win.WindowConfig{
		Title:      "glint spike",
		Width:      640,
		Height:     420,
		Background: 0x1B1818,
	})
	if err != nil {
		fail(err)
		emit()
		os.Exit(1)
	}
	defer shell.Destroy()

	hk, err := win.ParseHotkey(*hotkeyText)
	if err != nil {
		fail(err)
	} else if err := shell.RegisterHotkey(hk); err != nil {
		fail(err)
	} else {
		rep.HotkeyOK = true
		defer shell.UnregisterHotkey()
	}
	rep.Boot = millis(time.Since(bootStart))
	rep.Goroutines = runtime.NumGoroutine()

	// Idle baseline: nothing has been shown yet.
	if mem, err := win.CurrentMemory(); err == nil {
		rep.IdleWorkingSetMB = megabytes(mem.WorkingSet)
		rep.IdlePrivateMB = megabytes(mem.Private)
	} else {
		fail(err)
	}

	cpuStart, err := win.CPUTime()
	fail(err)
	started := time.Now()

	var coldShow, warmShow, coldPaint, warmPaint time.Duration
	shell.OnTimer(func(id uintptr) {
		switch id {
		case 1:
			coldShow = shell.Show()
		case 2:
			shell.Hide()
			if d, ok := shell.PaintLatency(); ok {
				coldPaint = d
			}
			shell.ResetPaint()
		case 3:
			warmShow = shell.Show()
		case 4:
			shell.Hide()
			if d, ok := shell.PaintLatency(); ok {
				warmPaint = d
			}
			shell.ResetPaint()
		case 5:
			shell.PostQuit()
		}
	})
	shell.SetTimer(1, 300*time.Millisecond)
	shell.SetTimer(2, 700*time.Millisecond)
	shell.SetTimer(3, 1200*time.Millisecond)
	shell.SetTimer(4, 1600*time.Millisecond)
	shell.SetTimer(5, time.Duration(*seconds)*time.Second)

	if err := shell.Run(); err != nil {
		fail(err)
	}

	elapsed := time.Since(started)
	rep.UptimeSeconds = elapsed.Seconds()
	rep.ColdShowMS = millis(coldShow)
	rep.WarmShowMS = millis(warmShow)
	rep.ColdFirstPaintMS = millis(coldPaint)
	rep.WarmFirstPaintMS = millis(warmPaint)

	if cpuEnd, err := win.CPUTime(); err == nil {
		rep.IdleCPUPercent = float64(cpuEnd-cpuStart) / float64(elapsed) * 100
	} else {
		fail(err)
	}
	if mem, err := win.CurrentMemory(); err == nil {
		rep.FinalWorkingSet = megabytes(mem.WorkingSet)
		rep.PeakWorkingSetMB = megabytes(mem.PeakWorkingSet)
	} else {
		fail(err)
	}
	rep.GDIObjects, rep.USERObjects = win.GUIResources()
	rep.KernelHandles = win.HandleCount()

	emit()
}

func millis(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000.0
}

func megabytes(b uint64) float64 {
	return float64(b) / (1 << 20)
}
