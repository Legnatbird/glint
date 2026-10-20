package sources

import (
	"os"
	"path/filepath"
	"strings"

	"applauncher/internal/core"
)

// scanPath walks the PATH directories.
//
// Windows system folders are skipped on purpose: system32 alone holds hundreds
// of executables that nobody launches from a launcher and would bury the real
// applications. When guiOnly is set, console binaries are skipped as well; the
// PE subsystem decides, not the file name.
func scanPath(rep *Report, excludes []string, guiOnly bool) []core.Entry {
	systemRoot := os.Getenv("SystemRoot")

	var out []core.Entry
	seen := make(map[string]bool)

	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		dir = strings.TrimSpace(strings.Trim(dir, `"`))
		if dir == "" {
			continue
		}

		key := strings.ToLower(strings.TrimRight(dir, `\`))
		if seen[key] {
			continue
		}
		seen[key] = true

		if underDir(dir, systemRoot) {
			continue
		}
		// UWP execution aliases are zero-byte stubs, useless as entries.
		if strings.Contains(key, `\windowsapps`) {
			continue
		}
		if excluded(dir, excludes) {
			continue
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, de := range entries {
			if de.IsDir() || !strings.EqualFold(filepath.Ext(de.Name()), ".exe") {
				continue
			}
			full := filepath.Join(dir, de.Name())
			if strings.HasPrefix(strings.ToLower(de.Name()), "unins") {
				continue
			}
			if guiOnly && !isGUIExecutable(full) {
				continue
			}
			out = append(out, exeEntry(full, core.SourcePath))
		}
	}
	return out
}
