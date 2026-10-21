package win

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const settingsClassName = "AppLauncherSettings"

const (
	wmSize        = 0x0005
	wmSysKeyDown  = 0x0104
	wmMouseMove   = 0x0200
	wmMouseDblClk = 0x0203
	wmMouseWheel  = 0x020A
	wmMouseLeave  = 0x02A3

	csHDraw   = 0x0002
	csVDraw   = 0x0001
	csDblClks = 0x0008

	tmeLeave = 0x00000002
)

// trackMouseEvent mirrors TRACKMOUSEEVENT. Asking for one WM_MOUSELEAVE is the
// only way to know the pointer left, and a hover highlight that outlives the
// pointer looks like a stuck selection.
type trackMouseEvent struct {
	cbSize      uint32
	dwFlags     uint32
	hwndTrack   windows.Handle
	dwHoverTime uint32
}

// wantLeave arms a single WM_MOUSELEAVE for hwnd.
func wantLeave(hwnd uintptr) {
	tme := trackMouseEvent{
		cbSize:    uint32(unsafe.Sizeof(trackMouseEvent{})),
		dwFlags:   tmeLeave,
		hwndTrack: windows.Handle(hwnd),
	}
	procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
}

var (
	settingsWindow *SettingsWindow
	settingsProc   = syscall.NewCallback(settingsWndProc)
)

// SettingsWindow is an ordinary captioned window, separate from the launcher.
//
// It has no child controls: the form paints itself and reads the keyboard
// directly, which keeps one painting model in the program and costs no extra
// window handles per control.
type SettingsWindow struct {
	hwnd    uintptr
	visible bool

	onPaint  func(*Canvas)
	onKey    func(KeyEvent) bool
	onChar   func(rune)
	onClick  func(x, y int32)
	onWheel  func(delta int32)
	onMove   func(x, y int32)
	onLeave  func()
	onDblClk func(x, y int32)
	onBlur   func()
	onClose  func()

	trackingLeave bool
}

func settingsWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	w := settingsWindow
	switch uint32(msg) {
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if w != nil && w.onPaint != nil && hdc != 0 {
			w.onPaint(&Canvas{hdc: hdc})
		}
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0

	case wmClose:
		if w != nil {
			w.Hide()
			if w.onClose != nil {
				w.onClose()
			}
		}
		return 0

	case wmSize:
		// Mark the client dirty and let the next paint pass run: forcing an
		// update here would repaint inside every mouse move of a resize drag.
		procInvalidateRect.Call(hwnd, 0, 0)
		return 0

	case wmActivate:
		if uint32(wParam) == 0 && w != nil && w.onBlur != nil {
			w.onBlur()
		}

	case wmKeyDown, wmSysKeyDown:
		// Alt combinations arrive as WM_SYSKEYDOWN, which is exactly how a
		// hotkey such as ctrl+alt+space is typed.
		if w != nil && w.onKey != nil && w.onKey(currentKeyEvent(wParam)) {
			return 0
		}

	case wmChar:
		if w != nil && w.onChar != nil {
			w.onChar(rune(wParam))
			return 0
		}

	case wmMouseWheel:
		if w != nil && w.onWheel != nil {
			// The delta is a signed multiple of WHEEL_DELTA (120) in the high word.
			w.onWheel(int32(int16(uint16(wParam >> 16))))
			return 0
		}

	case wmLButtonUp:
		if w != nil && w.onClick != nil {
			w.onClick(loword(lParam), hiword(lParam))
			return 0
		}

	case wmMouseMove:
		if w != nil {
			if !w.trackingLeave {
				wantLeave(hwnd)
				w.trackingLeave = true
			}
			if w.onMove != nil {
				w.onMove(loword(lParam), hiword(lParam))
				return 0
			}
		}

	case wmMouseDblClk:
		if w != nil && w.onDblClk != nil {
			w.onDblClk(loword(lParam), hiword(lParam))
			return 0
		}

	case wmMouseLeave:
		if w != nil {
			w.trackingLeave = false
			if w.onLeave != nil {
				w.onLeave()
				return 0
			}
		}
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return ret
}

func loword(v uintptr) int32 { return int32(int16(uint16(v & 0xFFFF))) }
func hiword(v uintptr) int32 { return int32(int16(uint16((v >> 16) & 0xFFFF))) }

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
	// The executable carries the launcher's icon as a resource, so the title bar
	// and Alt+Tab show the same mark as the tray and the settings row.
	large, small := SelfIcon()
	wc := wndClassEx{
		// CS_DBLCLKS is what makes WM_LBUTTONDBLCLK arrive at all, and it lets
		// Windows decide what a double click is instead of us timing clicks.
		style:         csHDraw | csVDraw | csDblClks,
		lpfnWndProc:   settingsProc,
		hInstance:     windows.Handle(hinst),
		hIcon:         windows.Handle(large),
		hIconSm:       windows.Handle(small),
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

// Show brings the window to the front and gives it the keyboard.
func (w *SettingsWindow) Show() {
	w.visible = true
	procShowWindow.Call(w.hwnd, swShow)
	forceForeground(w.hwnd)
}

// Hide dismisses it without destroying it.
func (w *SettingsWindow) Hide() {
	w.visible = false
	procShowWindow.Call(w.hwnd, swHide)
}

// Invalidate marks the whole client area for repaint.
func (w *SettingsWindow) Invalidate() {
	procInvalidateRect.Call(w.hwnd, 0, 0)
	procUpdateWindow.Call(w.hwnd)
}

// ClientSize reports the client area in pixels.
func (w *SettingsWindow) ClientSize() (int32, int32) {
	var r nativeRect
	procGetClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&r)))
	return r.right - r.left, r.bottom - r.top
}

// DPI returns the window's dots per inch (96 at 100% scaling).
func (w *SettingsWindow) DPI() uint32 {
	dpi, _, _ := procGetDpiForWindow.Call(w.hwnd)
	if dpi == 0 {
		return 96
	}
	return uint32(dpi)
}

// OnPaint installs the paint callback. The canvas is only valid during the call.
func (w *SettingsWindow) OnPaint(fn func(*Canvas)) { w.onPaint = fn }

// OnKey installs the WM_KEYDOWN/WM_SYSKEYDOWN handler. Returning true marks the
// key as handled.
func (w *SettingsWindow) OnKey(fn func(KeyEvent) bool) { w.onKey = fn }

// OnChar installs the WM_CHAR handler.
func (w *SettingsWindow) OnChar(fn func(rune)) { w.onChar = fn }

// OnClick installs the left-button-up handler, in client coordinates.
func (w *SettingsWindow) OnClick(fn func(x, y int32)) { w.onClick = fn }

// OnWheel installs the mouse-wheel handler with the raw delta (multiples of 120).
func (w *SettingsWindow) OnWheel(fn func(delta int32)) { w.onWheel = fn }

// OnMove installs the pointer-move handler, in client coordinates.
func (w *SettingsWindow) OnMove(fn func(x, y int32)) { w.onMove = fn }

// OnLeave installs the handler for the pointer leaving the client area.
func (w *SettingsWindow) OnLeave(fn func()) { w.onLeave = fn }

// OnDoubleClick installs the double-click handler, in client coordinates. It
// runs in addition to the pair of button-up events that make the click.
func (w *SettingsWindow) OnDoubleClick(fn func(x, y int32)) { w.onDblClk = fn }

// OnBlur runs when the window loses the foreground.
func (w *SettingsWindow) OnBlur(fn func()) { w.onBlur = fn }

// OnClose runs after the window hides itself.
func (w *SettingsWindow) OnClose(fn func()) { w.onClose = fn }

// HWND exposes the handle for logging and tests.
func (w *SettingsWindow) HWND() uintptr { return w.hwnd }

// Visible reports whether it is on screen.
func (w *SettingsWindow) Visible() bool { return w != nil && w.visible }
