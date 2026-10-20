package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// StateSchema is bumped when the usage file layout changes.
const StateSchema = 1

// state is the on-disk shape of the usage history.
type state struct {
	Schema int               `json:"schema"`
	Usage  map[string]Record `json:"usage"`
}

// LoadHistory reads the usage file. A missing file, an old schema or corrupt
// JSON all degrade to an empty history: losing ranking is never fatal.
func LoadHistory(path string, maxEntries int) (*History, error) {
	empty := NewHistory(nil, maxEntries)

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return empty, fmt.Errorf("core: read history: %w", err)
	}

	var st state
	if err := json.Unmarshal(data, &st); err != nil {
		return empty, fmt.Errorf("core: parse history %s: %w", path, err)
	}
	if st.Schema != StateSchema {
		return empty, nil
	}
	return NewHistory(st.Usage, maxEntries), nil
}

// Save writes the usage file atomically.
func (h *History) Save(path string) error {
	st := state{Schema: StateSchema, Usage: h.Snapshot()}

	data, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("core: encode history: %w", err)
	}

	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("core: create history dir: %w", err)
		}
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("core: write history: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("core: replace history: %w", err)
	}
	return nil
}
