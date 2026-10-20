package win

import "unsafe"

var (
	procGetClientRect   = user32.NewProc("GetClientRect")
	procGetDpiForSystem = user32.NewProc("GetDpiForSystem")
)

// ClientSize returns the drawable area in pixels.
func (w *Window) ClientSize() (int32, int32) {
	var r nativeRect
	procGetClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&r)))
	return r.right - r.left, r.bottom - r.top
}
