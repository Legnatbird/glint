package win

// FreeConsole detaches the process from any console it inherited or had
// allocated.
//
// The released build is a GUI-subsystem executable, so it never owns a console.
// But a plain `go build ./cmd/launcher` produces a console-subsystem binary,
// and double-clicking that one leaves a black window on screen for the whole
// life of the launcher. Detaching turns that into a momentary flash at worst.
func FreeConsole() {
	procFreeConsole.Call()
}

var procFreeConsole = kernel32.NewProc("FreeConsole")
