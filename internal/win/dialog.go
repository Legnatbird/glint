package win

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Confirmation dialogs.
//
// A backdrop change needs the launcher to restart, which is not something to do
// behind the user's back. MessageBoxW is the one dialog that needs no resources,
// no custom window class and no theme work, which is the right size for a
// question with two answers.

const (
	mbOKCancel      = 0x00000001
	mbIconWarning   = 0x00000030
	mbDefButton2    = 0x00000100 // Cancel is the default
	mbSetForeground = 0x00010000
	idOK            = 1
)

var procMessageBoxW = user32.NewProc("MessageBoxW")

// Confirm asks a yes/no question and reports whether the user accepted. The
// safe answer is the default: Enter does not accept.
func Confirm(owner uintptr, title, text string) bool {
	titlePtr, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return false
	}
	textPtr, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return false
	}

	result, _, _ := procMessageBoxW.Call(
		owner,
		uintptr(unsafe.Pointer(textPtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		mbOKCancel|mbIconWarning|mbDefButton2|mbSetForeground,
	)
	return int32(result) == idOK
}
