package sources

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/windows/registry"

	"applauncher/internal/core"
)

// Steam libraries.
//
// A Steam game has no Start Menu shortcut, so Windows never lists it. The
// manifests do: every installed game leaves an appmanifest_<id>.acf with its
// commercial name, and Steam registers the steam:// protocol for launching.
// That means the launcher never has to guess which executable inside the
// install folder is the game.

var (
	reName       = regexp.MustCompile(`"name"\s+"([^"]+)"`)
	reInstallDir = regexp.MustCompile(`"installdir"\s+"([^"]+)"`)
	reSteamPath  = regexp.MustCompile(`"path"\s+"([^"]+)"`)
)

// steamLibraries returns every library root, including the extra drives the
// user added in Steam.
func steamLibraries() []string {
	var roots []string

	if key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam`, registry.QUERY_VALUE); err == nil {
		path, _, valueErr := key.GetStringValue("SteamPath")
		key.Close()
		if valueErr == nil && path != "" {
			path = filepath.FromSlash(path)
			roots = append(roots, path)

			if data, readErr := os.ReadFile(filepath.Join(path, "steamapps", "libraryfolders.vdf")); readErr == nil {
				for _, match := range reSteamPath.FindAllStringSubmatch(string(data), -1) {
					folder := filepath.FromSlash(match[1])
					if folder != "" {
						roots = append(roots, folder)
					}
				}
			}
		}
	}

	seen := map[string]bool{}
	unique := roots[:0]
	for _, root := range roots {
		key := strings.ToLower(root)
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, root)
	}
	return unique
}

func scanSteam(rep *Report) []core.Entry {
	var out []core.Entry

	for _, library := range steamLibraries() {
		manifests, err := filepath.Glob(filepath.Join(library, "steamapps", "appmanifest_*.acf"))
		if err != nil {
			continue
		}

		for _, manifest := range manifests {
			data, err := os.ReadFile(manifest)
			if err != nil {
				continue
			}
			content := string(data)

			name := firstMatch(reName, content)
			if isSteamTool(name) {
				continue
			}
			installDir := firstMatch(reInstallDir, content)
			if name == "" || installDir == "" {
				continue
			}

			appID := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(manifest), "appmanifest_"), ".acf")
			exe := findGameExecutable(filepath.Join(library, "steamapps", "common", installDir), installDir)

			out = append(out, core.Entry{
				Key: "steam:" + appID,
				// Launching through the protocol lets Steam handle overlays,
				// updates and per-game arguments.
				Target:   "steam://rungameid/" + appID,
				Name:     name,
				Resolved: exe,
				IconPath: exe,
				Source:   core.SourceGame,
			})
		}
	}
	return out
}

func firstMatch(re *regexp.Regexp, content string) string {
	match := re.FindStringSubmatch(content)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

// findGameExecutable looks for the executable of a game, preferring one that
// matches the install folder over installers and crash handlers.
func findGameExecutable(dir, installDir string) string {
	var candidates []string
	_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			if entry != nil && entry.IsDir() && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".exe") {
			return nil
		}
		base := strings.ToLower(filepath.Base(path))
		for _, noise := range []string{"unins", "vcredist", "dxsetup", "dxwebsetup", "crashhandler", "crashreport", "launcher_installer"} {
			if strings.HasPrefix(base, noise) {
				return nil
			}
		}
		candidates = append(candidates, path)
		return nil
	})

	want := strings.ToLower(installDir) + ".exe"
	for _, path := range candidates {
		if strings.ToLower(filepath.Base(path)) == want {
			return path
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return ""
}

// isSteamTool filters the manifests that are runtime packages rather than
// games: nobody opens "Steamworks Common Redistributables" from a launcher.
func isSteamTool(name string) bool {
	lower := strings.ToLower(name)
	for _, tool := range []string{"redistributable", "steamworks common", "steam linux runtime", "proton", "steamvr"} {
		if strings.Contains(lower, tool) {
			return true
		}
	}
	return false
}
