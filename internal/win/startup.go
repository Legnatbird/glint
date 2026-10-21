package win

import (
	"errors"
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	runKey       = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValueName = "Glint"
	// legacyRunValueName is what the launcher wrote before the rename. Leaving it
	// behind would start a second copy at login, pointing at a binary that no
	// longer exists.
	legacyRunValueName = "app-launcher"
)

// SetAutostart adds or removes the launcher from the per-user Run key.
//
// HKCU (not HKLM) on purpose: no elevation, no admin prompt, and the setting
// belongs to the person using the launcher.
func SetAutostart(enabled bool) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()

	// The entry under the old name is dropped whether autostart is being turned
	// on or off: it is a leftover from before the rename.
	if err := key.DeleteValue(legacyRunValueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}

	if !enabled {
		err := key.DeleteValue(runValueName)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return key.SetStringValue(runValueName, quotePath(exe))
}

// AutostartEnabled reports whether the Run key currently has a value.
func AutostartEnabled() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()

	value, _, err := key.GetStringValue(runValueName)
	return err == nil && value != ""
}

// quotePath wraps a path in quotes so the Run key parses paths with spaces.
func quotePath(path string) string {
	path = strings.Trim(path, `"`)
	return `"` + path + `"`
}
