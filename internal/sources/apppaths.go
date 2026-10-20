package sources

import (
	"strings"

	"golang.org/x/sys/windows/registry"

	"applauncher/internal/core"
)

// appPathsKey is where Windows keeps the "type chrome and it runs" mapping.
const appPathsKey = `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths`

func scanAppPaths(rep *Report) []core.Entry {
	var out []core.Entry

	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		key, err := registry.OpenKey(root, appPathsKey, registry.READ)
		if err != nil {
			continue
		}
		names, err := key.ReadSubKeyNames(-1)
		key.Close()
		if err != nil {
			continue
		}

		for _, name := range names {
			path := appPathTarget(root, name)
			if path == "" {
				continue
			}
			out = append(out, exeEntry(path, core.SourceAppPaths))
		}
	}
	return out
}

// appPathTarget returns the default value of one App Paths subkey, or "" when
// the key points at something that is not on disk anymore.
func appPathTarget(root registry.Key, name string) string {
	key, err := registry.OpenKey(root, appPathsKey+`\`+name, registry.READ)
	if err != nil {
		return ""
	}
	defer key.Close()

	path, _, err := key.GetStringValue("")
	if err != nil {
		return ""
	}
	path = strings.Trim(strings.TrimSpace(path), `"`)
	if !fileExists(path) {
		return ""
	}
	return path
}
