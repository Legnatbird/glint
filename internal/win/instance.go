package win

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// MsgToggle is posted to an already running launcher to bring it up.
const MsgToggle = 0x8001

var procPostMessageW = user32.NewProc("PostMessageW")

// AcquireSingleInstance creates a named mutex. It returns true for the first
// process and false when another instance already owns the name.
func AcquireSingleInstance(name string) bool {
	ptr, err := windows.UTF16PtrFromString("Local\\" + name)
	if err != nil {
		return true
	}
	h, _, callErr := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(ptr)))
	if h == 0 {
		return true
	}
	// The handle is intentionally never closed: it must outlive the process so
	// the name stays owned.
	return callErr != syscall.ERROR_ALREADY_EXISTS
}

// PostToRunningInstance sends msg to the window of a previous instance.
func PostToRunningInstance(msg uint32) bool {
	class, err := windows.UTF16PtrFromString(windowClassName)
	if err != nil {
		return false
	}
	hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(class)), 0)
	if hwnd == 0 {
		return false
	}
	ok, _, _ := procPostMessageW.Call(hwnd, uintptr(msg), 0, 0)
	return ok != 0
}

// SystemDPI returns the primary monitor's DPI, used to size the window before
// it exists.
func SystemDPI() uint32 {
	if dpi, _, _ := procGetDpiForSystem.Call(); dpi != 0 {
		return uint32(dpi)
	}
	return 96
}
