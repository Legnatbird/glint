package main

import (
	"os"
	"path/filepath"
)

// Portable layout: everything the launcher owns lives next to the executable,
// so deleting the folder deletes the app.
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func configPath() string  { return filepath.Join(exeDir(), "config.json") }
func cachePath() string   { return filepath.Join(exeDir(), "index-cache.json") }
func historyPath() string { return filepath.Join(exeDir(), "history.json") }
