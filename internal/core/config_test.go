package core_test

import (
	"os"
	"path/filepath"
	"testing"

	"applauncher/internal/core"
)

func TestDefaultConfig(t *testing.T) {
	cfg := core.DefaultConfig()

	if cfg.Hotkey != "ctrl+alt+space" {
		t.Errorf("hotkey = %q", cfg.Hotkey)
	}
	if cfg.MaxResults != 8 {
		t.Errorf("max results = %d", cfg.MaxResults)
	}
	if !cfg.IncludePath || !cfg.IncludeAppPaths {
		t.Error("path and app paths should be enabled by default")
	}
	if cfg.Theme != "dark" {
		t.Errorf("theme = %q", cfg.Theme)
	}
	if !cfg.History.Enabled || cfg.History.MaxEntries != 2000 {
		t.Errorf("history defaults = %+v", cfg.History)
	}
}

func TestLoadConfigMissingFileReturnsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	cfg, err := core.LoadConfig(path)
	if err != nil {
		t.Fatalf("a missing config is not an error for a portable app: %v", err)
	}
	if cfg.MaxResults != core.DefaultConfig().MaxResults {
		t.Fatalf("max results = %d, want defaults", cfg.MaxResults)
	}
}

func TestLoadConfigRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{ not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := core.LoadConfig(path)
	if err == nil {
		t.Fatal("invalid JSON must return an error")
	}
	if cfg.MaxResults != core.DefaultConfig().MaxResults {
		t.Fatal("invalid JSON must still yield usable defaults")
	}
}

func TestConfigIgnoresUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := `{"hotkey":"ctrl+shift+space","future_option":42}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := core.LoadConfig(path)
	if err != nil {
		t.Fatalf("forward compatibility: %v", err)
	}
	if cfg.Hotkey != "ctrl+shift+space" {
		t.Fatalf("hotkey = %q", cfg.Hotkey)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	want := core.DefaultConfig()
	want.Hotkey = "ctrl+shift+space"
	want.MaxResults = 12
	want.CustomFolders = []string{`D:\Portables`, `D:\Games`}
	want.Theme = "light"

	if err := want.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := core.LoadConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if got.Hotkey != want.Hotkey || got.MaxResults != want.MaxResults || got.Theme != want.Theme {
		t.Fatalf("round trip mismatch: got %+v, want %+v", got, want)
	}
	if len(got.CustomFolders) != 2 || got.CustomFolders[1] != `D:\Games` {
		t.Fatalf("custom folders = %v", got.CustomFolders)
	}
}

func TestConfigNormalizeClampsInvalidValues(t *testing.T) {
	cfg := core.Config{
		Hotkey:        "   ",
		MaxResults:    999,
		Theme:         "neon",
		CustomFolders: []string{"  ", `D:\Portables`, `D:\Portables`, ""},
		History:       core.HistoryConfig{MaxEntries: 0},
	}

	cfg.Normalize()

	if cfg.Hotkey != "ctrl+alt+space" {
		t.Errorf("empty hotkey should fall back to the default, got %q", cfg.Hotkey)
	}
	if cfg.MaxResults != 20 {
		t.Errorf("max results = %d, want the cap of 20", cfg.MaxResults)
	}
	if cfg.Theme != "dark" {
		t.Errorf("unknown theme should fall back to dark, got %q", cfg.Theme)
	}
	if len(cfg.CustomFolders) != 1 || cfg.CustomFolders[0] != `D:\Portables` {
		t.Errorf("custom folders should be trimmed and deduplicated, got %v", cfg.CustomFolders)
	}
	if cfg.History.MaxEntries != 2000 {
		t.Errorf("history max entries = %d", cfg.History.MaxEntries)
	}
}

func TestConfigNormalizeLowerBounds(t *testing.T) {
	cfg := core.Config{MaxResults: 0, History: core.HistoryConfig{Enabled: true, MaxEntries: 1}}
	cfg.Normalize()

	if cfg.MaxResults != 8 {
		t.Errorf("zero max results should reset to the default, got %d", cfg.MaxResults)
	}
	if cfg.History.MaxEntries != 1 {
		t.Errorf("an explicit small history must be respected, got %d", cfg.History.MaxEntries)
	}
}
