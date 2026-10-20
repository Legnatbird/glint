package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CacheSchema is bumped whenever the meaning of a cached Entry changes. An old
// cache is ignored and rebuilt, never migrated.
const CacheSchema = 1

// Cache is the on-disk index, so the first hotkey of the day does not wait for
// a full scan of the Start Menu.
type Cache struct {
	Schema  int       `json:"schema"`
	Built   time.Time `json:"built"`
	Entries []Entry   `json:"entries"`
}

// Valid reports whether the cache can be trusted as-is.
func (c Cache) Valid() bool {
	return c.Schema == CacheSchema && len(c.Entries) > 0
}

// LoadCache reads a cache file. A missing file is a cold start, not an error.
func LoadCache(path string) (Cache, error) {
	var c Cache

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("core: read cache: %w", err)
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return Cache{}, fmt.Errorf("core: parse cache %s: %w", path, err)
	}
	return c, nil
}

// Save writes the cache atomically.
func (c Cache) Save(path string) error {
	c.Schema = CacheSchema
	if c.Built.IsZero() {
		c.Built = time.Now().UTC()
	}

	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("core: encode cache: %w", err)
	}

	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("core: create cache dir: %w", err)
		}
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("core: write cache: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("core: replace cache: %w", err)
	}
	return nil
}
