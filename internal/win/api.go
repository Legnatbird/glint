// Package win is the only package in this program that talks to Win32.
//
// Every syscall, struct layout and magic constant lives here. The rest of the
// code consumes small, typed helpers so it stays testable and free of unsafe
// pointer arithmetic.
package win

import (
	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	psapi    = windows.NewLazySystemDLL("psapi.dll")

	procRegisterClassExW     = user32.NewProc("RegisterClassExW")
	procCreateWindowExW      = user32.NewProc("CreateWindowExW")
	procDefWindowProcW       = user32.NewProc("DefWindowProcW")
	procDestroyWindow        = user32.NewProc("DestroyWindow")
	procGetMessageW          = user32.NewProc("GetMessageW")
	procTranslateMessage     = user32.NewProc("TranslateMessage")
	procDispatchMessageW     = user32.NewProc("DispatchMessageW")
	procPostQuitMessage      = user32.NewProc("PostQuitMessage")
	procPostThreadMessageW   = user32.NewProc("PostThreadMessageW")
	procShowWindow           = user32.NewProc("ShowWindow")
	procGetForegroundWindow  = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadPID   = user32.NewProc("GetWindowThreadProcessId")
	procSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
	procBringWindowToTop     = user32.NewProc("BringWindowToTop")
	procRegisterHotKey       = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey     = user32.NewProc("UnregisterHotKey")
	procGetGuiResources      = user32.NewProc("GetGuiResources")
	procGetSystemMetrics     = user32.NewProc("GetSystemMetrics")
	procSetTimer             = user32.NewProc("SetTimer")
	procKillTimer            = user32.NewProc("KillTimer")
	procSetProcessDPIAware   = user32.NewProc("SetProcessDPIAware")
	procSetDPIAwarenessCtx   = user32.NewProc("SetProcessDpiAwarenessContext")
	procCreateSolidBrush     = gdi32.NewProc("CreateSolidBrush")
	procDeleteObject         = gdi32.NewProc("DeleteObject")
	procGetModuleHandleW     = kernel32.NewProc("GetModuleHandleW")
	procGetCurrentThreadID   = kernel32.NewProc("GetCurrentThreadId")
	procGetCurrentProcess    = kernel32.NewProc("GetCurrentProcess")
	procGetProcessHandleCnt  = kernel32.NewProc("GetProcessHandleCount")
	procGetProcessTimes      = kernel32.NewProc("GetProcessTimes")
	procGetProcessMemoryInfo = psapi.NewProc("GetProcessMemoryInfo")
)

// Window styles, messages and flags. Values come from winuser.h.
const (
	wsPopup        = 0x80000000
	wsClipChildren = 0x02000000

	wsExToolWindow = 0x00000080
	wsExTopmost    = 0x00000008

	wmDestroy    = 0x0002
	wmPaint      = 0x000F
	wmTimer      = 0x0113
	wmHotkey     = 0x0312
	wmQuit       = 0x0012
	wmEraseBkgnd = 0x0014

	swHide = 0
	swShow = 5

	modAlt     = 0x0001
	modControl = 0x0002
	modShift   = 0x0004
	modWin     = 0x0008

	smCXScreen = 0
	smCYScreen = 1

	// (DPI_AWARENESS_CONTEXT)-4
	dpiContextPerMonitorAwareV2 = ^uintptr(3)
)

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     windows.Handle
	hIcon         windows.Handle
	hCursor       windows.Handle
	hbrBackground windows.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       windows.Handle
}

type msgStruct struct {
	hwnd     windows.Handle
	message  uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	ptX      int32
	ptY      int32
	lPrivate uint32
}

type processMemoryCountersEx struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
	privateUsage               uintptr
}

type filetime struct {
	low  uint32
	high uint32
}
