package win

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const windowClassName = "AppLauncherShell"

// Config describes the shell window.
//
// Background is a 0x00BBGGRR color used to erase the client area. The launcher
// paints its own content later; the brush only prevents a white flash.
type Config struct {
	Title      string
	Width      int32
	Height     int32
	Background uint32
}

// Window is a borderless, topmost shell window with a message loop.
//
// A Window is bound to the OS thread that created it. Call NewWindow and Run
// from the same goroutine after runtime.LockOSThread.
type Window struct {
	hwnd     uintptr
	threadID uintptr
	cfg      Config

	onHotkey func()
	onTimer  func(id uintptr)
	onClose  func()

	paintStart time.Time
	paintLat   time.Duration
	painted    bool
}

var (
	activeWindow *Window
	classReady   bool
	classErr     error

	// wndProcPtr must stay reachable for the process lifetime: the callback
	// trampoline is not tracked by the Go GC.
	wndProcPtr = syscall.NewCallback(wndProc)
)

func wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	if w := activeWindow; w != nil {
		switch uint32(msg) {
		case wmHotkey:
			if w.onHotkey != nil {
				w.onHotkey()
			}
			return 0
		case wmTimer:
			if w.onTimer != nil {
				w.onTimer(wParam)
			}
			return 0
		case wmPaint:
			if !w.paintStart.IsZero() {
				w.paintLat = time.Since(w.paintStart)
				w.paintStart = time.Time{}
				w.painted = true
			}
		case wmDestroy:
			if w.onClose != nil {
				w.onClose()
			}
		}
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return ret
}

func registerClass(cfg Config) error {
	if classReady {
		return classErr
	}
	classReady = true

	hinst, _, callErr := procGetModuleHandleW.Call(0)
	if hinst == 0 {
		classErr = fmt.Errorf("win: GetModuleHandleW: %w", callErr)
		return classErr
	}

	className, err := windows.UTF16PtrFromString(windowClassName)
	if err != nil {
		classErr = err
		return classErr
	}

	brush, _, _ := procCreateSolidBrush.Call(uintptr(cfg.Background))

	wc := wndClassEx{
		lpfnWndProc:   wndProcPtr,
		hInstance:     windows.Handle(hinst),
		hbrBackground: windows.Handle(brush),
		lpszClassName: className,
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))

	atom, _, regErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		classErr = fmt.Errorf("win: RegisterClassExW: %w", regErr)
		return classErr
	}
	return nil
}

// NewWindow creates the hidden shell window and locks it to the calling OS
// thread, which must be the one that later runs the message loop.
func NewWindow(cfg Config) (*Window, error) {
	runtime.LockOSThread()

	if err := registerClass(cfg); err != nil {
		runtime.UnlockOSThread()
		return nil, err
	}

	hinst, _, callErr := procGetModuleHandleW.Call(0)
	if hinst == 0 {
		runtime.UnlockOSThread()
		return nil, fmt.Errorf("win: GetModuleHandleW: %w", callErr)
	}

	title, err := windows.UTF16PtrFromString(cfg.Title)
	if err != nil {
		runtime.UnlockOSThread()
		return nil, err
	}
	className, err := windows.UTF16PtrFromString(windowClassName)
	if err != nil {
		runtime.UnlockOSThread()
		return nil, err
	}

	screenW, _, _ := procGetSystemMetrics.Call(smCXScreen)
	screenH, _, _ := procGetSystemMetrics.Call(smCYScreen)
	x := (int32(screenW) - cfg.Width) / 2
	y := (int32(screenH) - cfg.Height) / 3

	hwnd, _, createErr := procCreateWindowExW.Call(
		uintptr(wsExToolWindow|wsExTopmost),
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		uintptr(wsPopup|wsClipChildren),
		uintptr(uint32(x)), uintptr(uint32(y)),
		uintptr(uint32(cfg.Width)), uintptr(uint32(cfg.Height)),
		0, 0, hinst, 0,
	)
	if hwnd == 0 {
		runtime.UnlockOSThread()
		return nil, fmt.Errorf("win: CreateWindowExW: %w", createErr)
	}

	tid, _, _ := procGetCurrentThreadID.Call()
	w := &Window{hwnd: hwnd, threadID: tid, cfg: cfg}
	activeWindow = w
	return w, nil
}

// Run dispatches messages until WM_QUIT. It must run on the creating thread.
func (w *Window) Run() error {
	cur, _, _ := procGetCurrentThreadID.Call()
	if cur != w.threadID {
		return errors.New("win: Run called off the window's OS thread")
	}

	var msg msgStruct
	for {
		ret, _, callErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		switch int32(ret) {
		case -1:
			return fmt.Errorf("win: GetMessageW: %w", callErr)
		case 0:
			return nil
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// Show makes the window visible and foreground, returning how long the two
// calls took. It is the hot path of the launcher, so it allocates nothing.
func (w *Window) Show() time.Duration {
	w.paintStart = time.Now()
	w.painted = false

	start := time.Now()
	procShowWindow.Call(w.hwnd, swShow)
	procSetForegroundWindow.Call(w.hwnd)
	return time.Since(start)
}

// Hide removes the window from the screen without destroying it.
func (w *Window) Hide() {
	procShowWindow.Call(w.hwnd, swHide)
}

// PaintLatency reports the time between the last Show and its first WM_PAINT.
func (w *Window) PaintLatency() (time.Duration, bool) {
	return w.paintLat, w.painted
}

// ResetPaint clears the paint measurement before the next Show.
func (w *Window) ResetPaint() {
	w.paintLat = 0
	w.painted = false
}

// SetTimer posts WM_TIMER with the given id after d. Message timers are
// coalesced; they are for coarse scheduling, never for animations.
func (w *Window) SetTimer(id uintptr, d time.Duration) {
	procSetTimer.Call(w.hwnd, id, uintptr(d.Milliseconds()), 0)
}

// KillTimer stops a timer previously set with SetTimer.
func (w *Window) KillTimer(id uintptr) {
	procKillTimer.Call(w.hwnd, id)
}

// PostQuit ends the message loop from any goroutine.
func (w *Window) PostQuit() {
	procPostThreadMessageW.Call(w.threadID, wmQuit, 0, 0)
}

// Destroy frees the native window and detaches the packet-level callbacks.
func (w *Window) Destroy() {
	if w.hwnd != 0 {
		procDestroyWindow.Call(w.hwnd)
		w.hwnd = 0
	}
	activeWindow = nil
}

// OnHotkey sets the WM_HOTKEY handler. It runs on the message thread.
func (w *Window) OnHotkey(fn func()) { w.onHotkey = fn }

// OnTimer sets the WM_TIMER handler. It runs on the message thread.
func (w *Window) OnTimer(fn func(id uintptr)) { w.onTimer = fn }

// OnClose sets the WM_DESTROY handler.
func (w *Window) OnClose(fn func()) { w.onClose = fn }
