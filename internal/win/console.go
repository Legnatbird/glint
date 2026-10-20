package win

import "os"

var (
	procAttachConsole = kernel32.NewProc("AttachConsole")
	procGetStdHandle  = kernel32.NewProc("GetStdHandle")
)

const stdOutputHandle = ^uintptr(0) - 10 // STD_OUTPUT_HANDLE (-11)

// AttachParentConsole makes -dump work from a GUI-subsystem binary.
//
// The executable is linked with -H=windowsgui so double-clicking it never
// flashes a black console. The cost is that a fresh process has no console to
// write to. Two cases matter:
//
//   - the caller redirected stdout (a pipe or a file): the handle is inherited
//     and valid, so it must be left alone;
//   - the caller is a terminal: attach to its console and open CONOUT$.
func AttachParentConsole() {
	if h, _, _ := procGetStdHandle.Call(stdOutputHandle); h != 0 && h != ^uintptr(0) {
		return
	}

	const attachParentProcess = ^uintptr(0) // ATTACH_PARENT_PROCESS (-1)
	if ok, _, _ := procAttachConsole.Call(attachParentProcess); ok == 0 {
		return
	}

	if out, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout = out
	}
	if errOut, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stderr = errOut
	}
}
