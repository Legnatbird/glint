// Package sources discovers the applications installed on this machine and
// turns them into core.Entry values.
//
// It is the only package allowed to touch both the filesystem and Win32. Every
// entry carries a stable Key, which is what makes deduplication and usage
// tracking survive a change of source.
package sources

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"applauncher/internal/core"
	"applauncher/internal/win"
)

// Options selects which sources to scan and what to leave out.
type Options struct {
	CustomFolders   []string
	ExcludeFolders  []string
	IncludePath     bool
	IncludeAppPaths bool
}

// Report is the outcome of a scan.
type Report struct {
	Entries  []core.Entry
	Warnings []string
	Counts   map[string]int
	Duration time.Duration

	seen map[string]int
}

const (
	maxWarnings     = 20
	maxPerRoot      = 5000
	sourceStartMenu = "start menu"
	sourceStoreApps = "store apps"
	sourceCustom    = "custom folders"
	sourceAppPaths  = "app paths"
	sourcePath      = "path"
)

// Scan walks every enabled source. It never fails as a whole: a missing folder
// or an unreadable registry key becomes a warning and the rest still works.
func Scan(opts Options) Report {
	release := win.CoInitialize()
	defer release()

	rep := Report{Counts: map[string]int{}, seen: map[string]int{}}
	started := time.Now()

	rep.add(scanStartMenu(&rep), sourceStartMenu)
	rep.add(scanAppsFolder(&rep), sourceStoreApps)
	if opts.IncludeAppPaths {
		rep.add(scanAppPaths(&rep), sourceAppPaths)
	}
	if opts.IncludePath {
		rep.add(scanPath(&rep, opts.ExcludeFolders), sourcePath)
	}
	rep.add(scanCustomFolders(&rep, opts.CustomFolders, opts.ExcludeFolders), sourceCustom)

	sort.Slice(rep.Entries, func(i, j int) bool { return rep.Entries[i].Name < rep.Entries[j].Name })
	rep.Duration = time.Since(started)
	return rep
}

// add merges entries, keeping the highest-priority source for duplicated keys.
func (r *Report) add(entries []core.Entry, source string) {
	for _, e := range entries {
		r.Counts[source]++
		if prev, ok := r.seen[e.Key]; ok {
			if e.Source.Priority() < r.Entries[prev].Source.Priority() {
				r.Entries[prev] = e
			}
			continue
		}
		r.seen[e.Key] = len(r.Entries)
		r.Entries = append(r.Entries, e)
	}
}

func (r *Report) warn(msg string) {
	if len(r.Warnings) < maxWarnings {
		r.Warnings = append(r.Warnings, msg)
	}
}

// walkFiles calls fn for every file under root whose extension is in exts.
// Unreadable directories are skipped: a launcher must not die because one
// folder denies access.
func walkFiles(root string, exts map[string]bool, fn func(path string)) int {
	count := 0
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if count >= maxPerRoot {
			return fs.SkipAll
		}
		if !exts[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		count++
		fn(path)
		return nil
	})
	return count
}

func underDir(path, dir string) bool {
	if dir == "" {
		return false
	}
	p := strings.ToLower(strings.TrimRight(path, `\`)) + `\`
	d := strings.ToLower(strings.TrimRight(dir, `\`)) + `\`
	return strings.HasPrefix(p, d)
}

// excluded reports whether path sits under any configured exclude folder.
func excluded(path string, excludes []string) bool {
	for _, ex := range excludes {
		if underDir(path, ex) {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
