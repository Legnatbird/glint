package win

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Custom messages, posted to the launcher's own window. Both exist because the
// things that raise them (a second process, a background scan) cannot touch the
// UI thread directly.
const (
	// MsgShow asks the running instance to come up. It is what a second launch
	// posts instead of opening a second window.
	MsgShow = 0x8001
	// MsgIndexReady reports that a scan swapped in a new index.
	MsgIndexReady = 0x8002
)

var procPostMessageW = user32.NewProc("PostMessageW")

// PostToWindow sends msg to hwnd and returns immediately. It is safe from any
// goroutine: PostMessageW only touches the target thread's queue.
func PostToWindow(hwnd uintptr, msg uint32) {
	if hwnd != 0 {
		procPostMessageW.Call(hwnd, uintptr(msg), 0, 0)
	}
}

// instanceMutex is held for the life of the process so the name stays owned.
var instanceMutex windows.Handle

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
	instanceMutex = windows.Handle(h)
	return callErr != syscall.ERROR_ALREADY_EXISTS
}

// ReleaseSingleInstance gives the name up so a successor process can take it.
// Only the restart path needs this: normally the mutex dies with the process.
func ReleaseSingleInstance() {
	if instanceMutex != 0 {
		procCloseHandle.Call(uintptr(instanceMutex))
		instanceMutex = 0
	}
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

// spiGetWorkArea is SPI_GETWORKAREA: the desktop minus the taskbar and any
// appbar docked to an edge.
const spiGetWorkArea = 0x0030

// WorkArea returns the usable desktop rectangle. A window centred in it is
// centred for the user; a window centred on the full screen sits under the
// taskbar by half the taskbar's height.
func WorkArea() (x, y, width, height int32) {
	var r nativeRect
	ok, _, _ := procSystemParameters.Call(
		spiGetWorkArea, 0, uintptr(unsafe.Pointer(&r)), 0)
	if ok == 0 || r.right <= r.left || r.bottom <= r.top {
		screenW, _, _ := procGetSystemMetrics.Call(smCXScreen)
		screenH, _, _ := procGetSystemMetrics.Call(smCYScreen)
		return 0, 0, int32(screenW), int32(screenH)
	}
	return r.left, r.top, r.right - r.left, r.bottom - r.top
}
