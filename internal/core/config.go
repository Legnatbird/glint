package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// HistoryConfig controls usage tracking.
type HistoryConfig struct {
	Enabled    bool `json:"enabled"`
	MaxEntries int  `json:"max_entries"`
}

// Path scopes for scanning the PATH environment variable.
const (
	PathOff = "off" // ignore PATH entirely
	PathGUI = "gui" // only executables that can show a window (default)
	PathAll = "all" // every executable, including console tools
)

// Config is the portable configuration file. It lives next to the executable so
// the whole app is a folder that can be copied or deleted.
type Config struct {
	Hotkey          string        `json:"hotkey"`
	MaxResults      int           `json:"max_results"`
	Theme           string        `json:"theme"`
	PathScope       string        `json:"path_scope"`
	Autostart       bool          `json:"autostart"`
	CustomFolders   []string      `json:"custom_folders"`
	ExcludeFolders  []string      `json:"exclude_folders"`
	IncludeAppPaths bool          `json:"include_app_paths"`
	History         HistoryConfig `json:"history"`
}

// DefaultConfig is also the fallback for every invalid field.
func DefaultConfig() Config {
	return Config{
		Hotkey:          "ctrl+alt+space",
		MaxResults:      8,
		Theme:           "dark",
		PathScope:       PathGUI,
		IncludeAppPaths: true, History: HistoryConfig{Enabled: true, MaxEntries: 2000},
	}
}

// LoadConfig reads path over the defaults. A missing file is not an error: a
// fresh portable copy must start working immediately.
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("core: read config: %w", err)
	}
	// Unmarshalling into the defaults keeps absent fields at their default
	// value, which is what makes partial config files safe to hand-write.
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), fmt.Errorf("core: parse config %s: %w", path, err)
	}
	cfg.Normalize()
	return cfg, nil
}

// Save writes the config atomically next to path.
func (c Config) Save(path string) error {
	c.Normalize()

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("core: encode config: %w", err)
	}
	data = append(data, '\n')

	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("core: create config dir: %w", err)
		}
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("core: write config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("core: replace config: %w", err)
	}
	return nil
}

// Normalize clamps every field to a value the launcher can actually run with.
func (c *Config) Normalize() {
	defaults := DefaultConfig()

	c.Hotkey = strings.TrimSpace(c.Hotkey)
	if c.Hotkey == "" {
		c.Hotkey = defaults.Hotkey
	}

	switch {
	case c.MaxResults <= 0:
		c.MaxResults = defaults.MaxResults
	case c.MaxResults > 20:
		c.MaxResults = 20
	}

	switch c.Theme {
	case "dark", "light", "system":
	default:
		c.Theme = defaults.Theme
	}

	switch c.PathScope {
	case PathOff, PathGUI, PathAll:
	default:
		c.PathScope = defaults.PathScope
	}

	c.CustomFolders = cleanPaths(c.CustomFolders)
	c.ExcludeFolders = cleanPaths(c.ExcludeFolders)

	switch {
	case c.History.MaxEntries <= 0:
		c.History.MaxEntries = defaults.History.MaxEntries
	case c.History.MaxEntries > 20000:
		c.History.MaxEntries = 20000
	}
}

// cleanPaths trims, drops empty entries and removes case-insensitive duplicates.
func cleanPaths(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		key := strings.ToLower(p)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
