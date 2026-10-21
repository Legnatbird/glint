package sources

import (
	"os"
	"path/filepath"
	"strings"

	"glint/internal/core"
)

// scanCustomFolders walks the user-configured portable folders for executables
// and shortcuts.
func scanCustomFolders(rep *Report, dirs, excludes []string) []core.Entry {
	var out []core.Entry

	for _, dir := range dirs {
		if excluded(dir, excludes) {
			continue
		}
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			rep.warn("custom folder not found: " + dir)
			continue
		}

		walkFiles(dir, map[string]bool{".exe": true, ".lnk": true}, func(path string) {
			base := strings.ToLower(filepath.Base(path))
			// Uninstallers are never a launch target.
			if strings.HasPrefix(base, "unins") {
				return
			}
			if strings.HasSuffix(base, ".lnk") {
				out = append(out, shortcutEntry(path, core.SourceCustomFolder))
				return
			}
			out = append(out, exeEntry(path, core.SourceCustomFolder))
		})
	}
	return out
}
