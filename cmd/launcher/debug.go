package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// debugLog writes to launcher-debug.log next to the executable when
// LAUNCHER_DEBUG is set.
//
// The process is a GUI-subsystem app: stderr goes nowhere, so a failure to
// register the hotkey or to add the tray icon would be invisible. This is the
// only channel the user (or a support session) can inspect after the fact.
var debugLog *os.File

func debugf(format string, args ...any) {
	if os.Getenv("LAUNCHER_DEBUG") == "" {
		return
	}
	if debugLog == nil {
		path := filepath.Join(exeDir(), "launcher-debug.log")
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return
		}
		debugLog = file
	}
	fmt.Fprintf(debugLog, "%s %s\n", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
}
