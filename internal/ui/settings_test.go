package ui

import (
	"errors"
	"testing"

	"applauncher/internal/core"
	"applauncher/internal/win"
)

var errTest = errors.New("test failure")

// harness drives the form without a window: every effect it would ask the
// running launcher for is recorded instead of executed.
type harness struct {
	form *Settings
	cfg  core.Config

	saved      int
	hotkeys    []string
	hotkeyLock []bool
	backdrops  []string
	autostart  []bool
	maxResults []int
	rescans    int
	clears     int
	entries    int
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{cfg: core.DefaultConfig(), entries: 3}
	cfg := &h.cfg
	h.form = NewSettings(nil, cfg, nil, SettingsHooks{
		Save:       func() error { h.saved++; return nil },
		Hotkey:     func(text string) error { h.hotkeys = append(h.hotkeys, text); return nil },
		HotkeyLock: func(capturing bool) { h.hotkeyLock = append(h.hotkeyLock, capturing) },
		Backdrop:   func(mode string) error { h.backdrops = append(h.backdrops, mode); return nil },
		Autostart:  func(on bool) error { h.autostart = append(h.autostart, on); return nil },
		MaxResults: func(n int) { h.maxResults = append(h.maxResults, n) },
		Rescan:     func() { h.rescans++ },
		ClearLog:   func() error { h.clears++; h.entries = 0; return nil },
		Entries:    func() int { return h.entries },
	})
	return h
}

// selectRow puts the keyboard selection on the row whose label matches.
func (h *harness) selectRow(t *testing.T, label string) {
	t.Helper()
	rows, _ := h.form.build()
	for i, r := range rows {
		if r.label == label {
			h.form.sel = i
			return
		}
	}
	t.Fatalf("no row labelled %q", label)
}

// selectedLabel reports the label the selection is on.
func (h *harness) selectedLabel() string {
	rows, _ := h.form.build()
	if h.form.sel < 0 || h.form.sel >= len(rows) {
		return ""
	}
	return rows[h.form.sel].label
}

func (h *harness) press(vk uint32) { h.form.HandleKey(win.KeyEvent{VK: vk}) }

func TestNavigationSkipsHeaders(t *testing.T) {
	h := newHarness(t)
	h.selectRow(t, "Hotkey")

	h.press(win.KeyDown)
	if got := h.selectedLabel(); got != "Max results" {
		t.Fatalf("after one down: %q, want Max results", got)
	}

	// PATH scope follows the SOURCES header, which must be stepped over.
	h.press(win.KeyDown)
	h.press(win.KeyDown)
	h.press(win.KeyDown)
	if got := h.selectedLabel(); got != "PATH scope" {
		t.Fatalf("after four downs: %q, want PATH scope", got)
	}

	// The first row is a header, so going up from the first focusable row stays.
	h.selectRow(t, "Hotkey")
	h.press(win.KeyUp)
	if got := h.selectedLabel(); got != "Hotkey" {
		t.Fatalf("up from the first row: %q, want Hotkey", got)
	}
}

func TestMaxResultsStepsAndClamps(t *testing.T) {
	h := newHarness(t)
	h.selectRow(t, "Max results")

	for i := 0; i < 30; i++ {
		h.press(win.KeyRight)
	}
	if h.cfg.MaxResults != 20 {
		t.Fatalf("max_results = %d, want the 20 cap", h.cfg.MaxResults)
	}
	if len(h.maxResults) == 0 || h.maxResults[len(h.maxResults)-1] > 20 {
		t.Fatalf("hook saw %v, want values capped at 20", h.maxResults)
	}

	for i := 0; i < 30; i++ {
		h.press(win.KeyLeft)
	}
	if h.cfg.MaxResults != 1 {
		t.Fatalf("max_results = %d, want the floor of 1", h.cfg.MaxResults)
	}
	if h.saved == 0 {
		t.Fatal("no save on a value change")
	}
}

func TestBackdropSwitchAppliesLive(t *testing.T) {
	h := newHarness(t)
	h.selectRow(t, "Backdrop")

	h.press(win.KeyRight)
	if h.cfg.Backdrop != core.BackdropSolid {
		t.Fatalf("backdrop = %q, want solid", h.cfg.Backdrop)
	}
	if len(h.backdrops) != 1 || h.backdrops[0] != core.BackdropSolid {
		t.Fatalf("backdrop hook saw %v, want [solid]", h.backdrops)
	}

	h.press(win.KeyRight)
	if h.cfg.Backdrop != core.BackdropAcrylic {
		t.Fatalf("backdrop = %q, want acrylic", h.cfg.Backdrop)
	}
}

func TestAutostartToggles(t *testing.T) {
	h := newHarness(t)
	h.selectRow(t, "Start with Windows")

	h.press(win.KeyReturn)
	if !h.cfg.Autostart {
		t.Fatal("autostart not enabled")
	}
	if len(h.autostart) != 1 || !h.autostart[0] {
		t.Fatalf("autostart hook saw %v, want [true]", h.autostart)
	}

	h.press(win.KeySpace)
	if h.cfg.Autostart {
		t.Fatal("autostart not disabled")
	}
}

func TestHotkeyCaptureAppliesCombination(t *testing.T) {
	h := newHarness(t)
	h.selectRow(t, "Hotkey")

	h.press(win.KeyReturn)
	if !h.form.capture {
		t.Fatal("Enter did not start capturing")
	}
	if len(h.hotkeyLock) != 1 || !h.hotkeyLock[0] {
		t.Fatalf("hotkey lock saw %v, want [true] (released while capturing)", h.hotkeyLock)
	}

	h.form.HandleKey(win.KeyEvent{VK: 'G', Ctrl: true})
	if h.form.capture {
		t.Fatal("still capturing after a complete combination")
	}
	if h.cfg.Hotkey != "ctrl+g" {
		t.Fatalf("hotkey = %q, want ctrl+g", h.cfg.Hotkey)
	}
	if len(h.hotkeys) != 1 || h.hotkeys[0] != "ctrl+g" {
		t.Fatalf("hotkey hook saw %v, want [ctrl+g]", h.hotkeys)
	}
	if h.saved == 0 {
		t.Fatal("captured hotkey was not saved")
	}
}

func TestHotkeyCaptureRejectsIncompletePress(t *testing.T) {
	h := newHarness(t)
	h.selectRow(t, "Hotkey")
	h.press(win.KeyReturn)

	h.form.HandleKey(win.KeyEvent{VK: 'G'}) // no modifier
	if !h.form.capture {
		t.Fatal("a bare key ended the capture")
	}
	if !h.form.failed {
		t.Fatal("the rejection was not reported to the user")
	}
	if h.cfg.Hotkey != "ctrl+alt+space" {
		t.Fatalf("hotkey changed to %q", h.cfg.Hotkey)
	}

	h.form.HandleKey(win.KeyEvent{VK: win.KeyEscape})
	if h.form.capture {
		t.Fatal("Escape did not cancel the capture")
	}
	if len(h.hotkeyLock) != 2 || h.hotkeyLock[1] {
		t.Fatalf("hotkey lock saw %v, want it released and re-acquired", h.hotkeyLock)
	}
	if h.cfg.Hotkey != "ctrl+alt+space" {
		t.Fatalf("cancel changed the hotkey to %q", h.cfg.Hotkey)
	}
}

func TestRemovingEntryNeedsConfirmation(t *testing.T) {
	h := newHarness(t)
	h.cfg.CustomFolders = []string{`C:\Tools`, `D:\Games`}

	h.selectRow(t, "Games")
	h.press(win.KeyReturn)
	if len(h.cfg.CustomFolders) != 2 {
		t.Fatal("the first Enter already removed the folder")
	}
	if h.form.armed < 0 {
		t.Fatal("the row was not armed")
	}

	h.press(win.KeyReturn)
	if len(h.cfg.CustomFolders) != 1 || h.cfg.CustomFolders[0] != `C:\Tools` {
		t.Fatalf("folders = %v, want only C:\\Tools", h.cfg.CustomFolders)
	}
	if h.rescans != 1 {
		t.Fatalf("rescans = %d, want 1", h.rescans)
	}
}

func TestMovingDisarms(t *testing.T) {
	h := newHarness(t)
	h.cfg.CustomFolders = []string{`C:\Tools`}

	h.selectRow(t, "Tools")
	h.press(win.KeyReturn)
	h.press(win.KeyDown) // disarm
	if h.form.armed >= 0 {
		t.Fatal("moving the selection left the row armed")
	}
}

func TestClearHistoryNeedsConfirmation(t *testing.T) {
	h := newHarness(t)
	h.selectRow(t, "Clear history")

	h.press(win.KeyDelete)
	if h.clears != 0 {
		t.Fatal("one Delete cleared the history")
	}
	h.press(win.KeyReturn)
	if h.clears != 1 {
		t.Fatalf("clears = %d, want 1", h.clears)
	}
}

func TestPathScopeCyclesAndRescans(t *testing.T) {
	h := newHarness(t)
	h.selectRow(t, "PATH scope")

	h.press(win.KeyRight) // gui -> all
	if h.cfg.PathScope != core.PathAll {
		t.Fatalf("scope = %q, want all", h.cfg.PathScope)
	}
	h.press(win.KeyRight) // all -> off
	if h.cfg.PathScope != core.PathOff {
		t.Fatalf("scope = %q, want off", h.cfg.PathScope)
	}
	if h.rescans != 2 {
		t.Fatalf("rescans = %d, want 2", h.rescans)
	}
}

func TestHotkeyRejectedBySystemReverts(t *testing.T) {
	h := newHarness(t)
	h.selectRow(t, "Hotkey")
	h.press(win.KeyReturn)

	h.form.hooks.Hotkey = func(string) error { return errTest }

	h.form.HandleKey(win.KeyEvent{VK: 'K', Ctrl: true})
	if h.cfg.Hotkey != "ctrl+alt+space" {
		t.Fatalf("hotkey = %q, want the previous combination", h.cfg.Hotkey)
	}
	if !h.form.failed {
		t.Fatal("the failure was not reported")
	}
	if len(h.hotkeyLock) != 2 || h.hotkeyLock[1] {
		t.Fatalf("hotkey lock saw %v, want the previous combination back", h.hotkeyLock)
	}
}

func TestWantsSettings(t *testing.T) {
	cases := map[string]bool{
		"":       false,
		"s":      false, // too short: would shadow every S app
		"se":     false,
		"set":    true,
		"sett":   true,
		"config": true,
		"aju":    true,
		"pref":   true,
		"chr":    false,
		"zzz":    false,
	}
	for query, want := range cases {
		if got := wantsSettings(query); got != want {
			t.Errorf("wantsSettings(%q) = %v, want %v", query, got, want)
		}
	}
}

func TestSettingsRowIsInjectedFirst(t *testing.T) {
	idx := core.NewIndex([]core.Entry{
		{Name: "Chrome", Key: "exe:chrome"},
		{Name: "Settings Tool", Key: "exe:settings-tool"},
	})
	a := &App{
		config:  core.DefaultConfig(),
		index:   func() *core.Index { return idx },
		history: core.NewHistory(nil, 10),
		open:    func() {},
	}

	a.query = []rune("sett")
	a.refresh()
	if len(a.results) == 0 || a.results[0].Entry.Key != settingsKey {
		t.Fatalf("first row = %+v, want the settings row", a.results)
	}
	if len(a.results) > a.config.MaxResults {
		t.Fatalf("results = %d, want at most %d", len(a.results), a.config.MaxResults)
	}

	// A query that does not name it never gets the row.
	a.query = []rune("chr")
	a.refresh()
	for _, r := range a.results {
		if r.Entry.Key == settingsKey {
			t.Fatal("settings row injected for an unrelated query")
		}
	}
}
