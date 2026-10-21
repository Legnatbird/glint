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

// WindowConfig describes the shell window.
//
// Background is a 0x00BBGGRR color used to erase the client area. The launcher
// paints its own content; the brush only prevents a white flash while the first
// frame is drawn.
type WindowConfig struct {
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
	cfg      WindowConfig

	onHotkey     func()
	onTimer      func(id uintptr)
	onClose      func()
	onPaint      func(c *Canvas)
	onChar       func(r rune)
	onKey        func(key KeyEvent) bool
	onDeactivate func()
	onTray       func(msg uint32)
	onShow       func()
	onIndexReady func()

	hiding bool

	paintStart time.Time
	paintLat   time.Duration
	painted    bool
}

// KeyEvent is one keyboard event, with the modifiers that matter to a launcher.
type KeyEvent struct {
	VK    uint32
	Ctrl  bool
	Shift bool
	Alt   bool
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
	w := activeWindow
	if w != nil {
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
			var ps paintStruct
			hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			if w.onPaint != nil && hdc != 0 {
				w.onPaint(&Canvas{hdc: hdc})
			}
			procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			return 0
		case wmChar:
			if w.onChar != nil {
				w.onChar(rune(wParam))
			}
			return 0
		case wmKeyDown:
			if w.onKey != nil && w.onKey(currentKeyEvent(wParam)) {
				return 0
			}
		case wmActivate:
			// WA_INACTIVE. The window is about to lose the foreground: hide it,
			// the way every launcher dismisses on click-away.
			if uint32(wParam) == 0 && w.onDeactivate != nil && !w.hiding {
				w.onDeactivate()
			}
			return 0
		case trayMessage:
			if w.onTray != nil {
				w.onTray(uint32(lParam))
			}
			return 0
		case MsgShow:
			// A second process asked this instance to come up.
			if w.onShow != nil {
				w.onShow()
			}
			return 0
		case MsgIndexReady:
			if w.onIndexReady != nil {
				w.onIndexReady()
			}
			return 0
		case wmDestroy:
			if w.onClose != nil {
				w.onClose()
			}
		case wmEraseBkgnd:
			// Painting is fully handled in WM_PAINT.
			return 1
		}
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return ret
}

func currentKeyEvent(vk uintptr) KeyEvent {
	return KeyEvent{
		VK:    uint32(vk),
		Ctrl:  keyDown(vkControl),
		Shift: keyDown(vkShift),
		Alt:   keyDown(vkMenu),
	}
}

func keyDown(vk uintptr) bool {
	r, _, _ := procGetKeyState.Call(vk)
	return int16(r) < 0
}

func registerClass(cfg WindowConfig) error {
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
	// Without a class cursor Windows leaves whatever the previous window had,
	// which is why the launcher could show the app-starting cursor forever.
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)

	wc := wndClassEx{
		lpfnWndProc:   wndProcPtr,
		hInstance:     windows.Handle(hinst),
		hbrBackground: windows.Handle(brush),
		hCursor:       windows.Handle(cursor),
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
func NewWindow(cfg WindowConfig) (*Window, error) {
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

	exStyle := uint32(wsExToolWindow | wsExTopmost)

	hwnd, _, createErr := procCreateWindowExW.Call(
		uintptr(exStyle),
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
	w.hiding = false
	w.paintStart = time.Now()
	w.painted = false
	start := time.Now()

	procShowWindow.Call(w.hwnd, swShow)
	procSetWindowPos.Call(w.hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpShowWindow)
	forceForeground(w.hwnd)

	return time.Since(start)
}

// Hide removes the window from the screen without destroying it.
func (w *Window) Hide() {
	w.hiding = true
	procShowWindow.Call(w.hwnd, swHide)
}

// Invalidate marks the whole client area for repaint.
func (w *Window) Invalidate() {
	procInvalidateRect.Call(w.hwnd, 0, 0)
	procUpdateWindow.Call(w.hwnd)
}

// PaintLatency reports the time between the last Show and its first WM_PAINT.
func (w *Window) PaintLatency() (time.Duration, bool) { return w.paintLat, w.painted }

// ResetPaint clears the paint measurement before the next Show.
func (w *Window) ResetPaint() {
	w.paintLat = 0
	w.painted = false
	w.paintStart = time.Time{}
}

// DPI returns the window's current dots per inch (96 at 100% scaling).
func (w *Window) DPI() uint32 {
	dpi, _, _ := procGetDpiForWindow.Call(w.hwnd)
	if dpi == 0 {
		return 96
	}
	return uint32(dpi)
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

// Destroy frees the native window and detaches the callbacks.
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

// OnPaint sets the WM_PAINT handler. The canvas is only valid during the call.
func (w *Window) OnPaint(fn func(c *Canvas)) { w.onPaint = fn }

// OnChar sets the WM_CHAR handler, which is already IME/dead-key composed.
func (w *Window) OnChar(fn func(r rune)) { w.onChar = fn }

// OnKey sets the WM_KEYDOWN handler. Returning true marks the key as handled.
func (w *Window) OnKey(fn func(KeyEvent) bool) { w.onKey = fn }

// OnTray sets the notification-area callback.
func (w *Window) OnTray(fn func(msg uint32)) { w.onTray = fn }

// OnShow runs when another process asks this instance to come up.
func (w *Window) OnShow(fn func()) { w.onShow = fn }

// OnIndexReady runs when a background scan reports a fresh index.
func (w *Window) OnIndexReady(fn func()) { w.onIndexReady = fn }

// HWND exposes the native handle for the tray icon and shell calls.
func (w *Window) HWND() uintptr { return w.hwnd }

// OnDeactivate runs when the window loses the foreground.
func (w *Window) OnDeactivate(fn func()) { w.onDeactivate = fn }

// forceForeground brings the window to the front even when another process owns
// the foreground. Windows refuses SetForegroundWindow from a background thread;
// attaching to the foreground thread's input queue lifts that restriction.
func forceForeground(hwnd uintptr) {
	fg, _, _ := procGetForegroundWindow.Call()
	if fg != 0 && fg != hwnd {
		fgThread, _, _ := procGetWindowThreadPID.Call(fg, 0)
		curThread, _, _ := procGetCurrentThreadID.Call()
		if fgThread != 0 && fgThread != curThread {
			procAttachThreadInput.Call(curThread, fgThread, 1)
			procBringWindowToTop.Call(hwnd)
			procSetForegroundWindow.Call(hwnd)
			procSetFocus.Call(hwnd)
			procAttachThreadInput.Call(curThread, fgThread, 0)
			return
		}
	}
	procSetForegroundWindow.Call(hwnd)
	procSetFocus.Call(hwnd)
}
