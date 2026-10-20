package sources

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"applauncher/internal/core"
	"applauncher/internal/win"
)

// startMenuRoots returns the machine-wide and per-user Start Menu programs
// folders. Both are plain environment paths, so no shell folder API is needed.
func startMenuRoots() []string {
	var roots []string
	if p := os.Getenv("ProgramData"); p != "" {
		roots = append(roots, filepath.Join(p, "Microsoft", "Windows", "Start Menu", "Programs"))
	}
	if p := os.Getenv("APPDATA"); p != "" {
		roots = append(roots, filepath.Join(p, "Microsoft", "Windows", "Start Menu", "Programs"))
	}
	return roots
}

func scanStartMenu(rep *Report) []core.Entry {
	var out []core.Entry

	for _, root := range startMenuRoots() {
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			rep.warn("start menu folder not found: " + root)
			continue
		}
		walkFiles(root, map[string]bool{".lnk": true}, func(path string) {
			out = append(out, shortcutEntry(path, core.SourceStartMenu))
		})
	}
	return out
}

// shortcutEntry builds an entry from a .lnk. Resolution is best effort: when it
// fails the entry is still launchable through the shortcut itself.
func shortcutEntry(path string, src core.Source) core.Entry {
	e := core.Entry{
		Key:      "lnk:" + strings.ToLower(path),
		Name:     shortcutName(path),
		Target:   path,
		IconPath: path,
		Source:   src,
	}

	if sc, err := win.ResolveShortcut(path); err == nil && sc.Path != "" {
		// The resolved executable is the identity: this is what lets a Start
		// Menu shortcut and a PATH entry for the same program collapse into a
		// single row.
		e.Resolved = sc.Path
		e.Key = "exe:" + strings.ToLower(sc.Path)
	}
	return e
}

// exeEntry builds an entry from a bare executable.
func exeEntry(path string, src core.Source) core.Entry {
	name := friendlyName(path)
	return core.Entry{
		Key:      "exe:" + strings.ToLower(path),
		Name:     name,
		Target:   path,
		Resolved: path,
		IconPath: path,
		WorkDir:  filepath.Dir(path),
		Source:   src,
	}
}

// shortcutName is the file name without the .lnk extension, which is exactly
// the label Windows shows in the Start Menu.
func shortcutName(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// friendlyName turns "chrome.exe" into "Chrome" without mangling names that
// already carry their own casing ("VSCode", "7-Zip").
func friendlyName(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	base = strings.ReplaceAll(base, "_", " ")
	base = strings.TrimSpace(base)
	if base == "" {
		return filepath.Base(path)
	}
	if base == strings.ToLower(base) {
		runes := []rune(base)
		runes[0] = unicode.ToUpper(runes[0])
		base = string(runes)
	}
	return base
}
