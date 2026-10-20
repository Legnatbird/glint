package win

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const settingsClassName = "AppLauncherSettings"

var (
	settingsWindow *SettingsWindow
	settingsProc   = syscall.NewCallback(settingsWndProc)
)

// SettingsWindow is an ordinary captioned window, separate from the launcher.
type SettingsWindow struct {
	hwnd    uintptr
	visible bool
	clsDone bool
	clsErr  error
}

func settingsWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch uint32(msg) {
	case wmEraseBkgnd:
		return 1
	case wmClose:
		procShowWindow.Call(hwnd, swHide)
		if settingsWindow != nil {
			settingsWindow.visible = false
		}
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return ret
}

// NewSettingsWindow creates the hidden settings window.
func NewSettingsWindow(title string, width, height int32) (*SettingsWindow, error) {
	w := &SettingsWindow{}
	settingsWindow = w

	hinst, _, _ := procGetModuleHandleW.Call(0)
	class, err := windows.UTF16PtrFromString(settingsClassName)
	if err != nil {
		return nil, err
	}

	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	brush, _, _ := procCreateSolidBrush.Call(uintptr(0x1F1A1A))
	wc := wndClassEx{
		lpfnWndProc:   settingsProc,
		hInstance:     windows.Handle(hinst),
		hCursor:       windows.Handle(cursor),
		hbrBackground: windows.Handle(brush),
		lpszClassName: class,
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	if atom, _, regErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
		return nil, fmt.Errorf("RegisterClassExW(settings): %w", regErr)
	}

	titlePtr, _ := windows.UTF16PtrFromString(title)
	hwnd, _, createErr := procCreateWindowExW.Call(
		uintptr(wsExAppWindow),
		uintptr(unsafe.Pointer(class)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(wsOverlappedWindow),
		120, 120,
		uintptr(uint32(width)), uintptr(uint32(height)),
		0, 0, hinst, 0,
	)
	if hwnd == 0 {
		return nil, fmt.Errorf("CreateWindowExW(settings): %w", createErr)
	}
	w.hwnd = hwnd
	return w, nil
}

// Show brings the window to the front.
func (w *SettingsWindow) Show() {
	w.visible = true
	procShowWindow.Call(w.hwnd, swShow)
	procSetForegroundWindow.Call(w.hwnd)
}

// Hide dismisses it without destroying it.
func (w *SettingsWindow) Hide() {
	w.visible = false
	procShowWindow.Call(w.hwnd, swHide)
}

// Visible reports whether it is on screen.
func (w *SettingsWindow) Visible() bool { return w != nil && w.visible }
