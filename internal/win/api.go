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

	wsExToolWindow     = 0x00000080
	wsExTopmost        = 0x00000008
	wsExLayered        = 0x00080000
	wsExAppWindow      = 0x00040000
	wsOverlappedWindow = 0x00CF0000

	wmDestroy    = 0x0002
	wmClose      = 0x0010
	wmActivate   = 0x0006
	wmPaint      = 0x000F
	wmTimer      = 0x0113
	wmChar       = 0x0102
	wmKeyDown    = 0x0100
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

// Shell, COM and GDI entry points. They live here, next to the rest of the
// interop, so no other file has to know a DLL name.
var (
	ole32   = windows.NewLazySystemDLL("ole32.dll")
	shell32 = windows.NewLazySystemDLL("shell32.dll")
	shlwapi = windows.NewLazySystemDLL("shlwapi.dll")
	dwmapi  = windows.NewLazySystemDLL("dwmapi.dll")

	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procCoUninitialize   = ole32.NewProc("CoUninitialize")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	procCoTaskMemFree    = ole32.NewProc("CoTaskMemFree")

	procShellExecuteExW             = shell32.NewProc("ShellExecuteExW")
	procSHCreateItemFromParsingName = shell32.NewProc("SHCreateItemFromParsingName")

	procCloseHandle  = kernel32.NewProc("CloseHandle")
	procCreateMutexW = kernel32.NewProc("CreateMutexW")

	procAttachThreadInput = user32.NewProc("AttachThreadInput")
	procSetFocus          = user32.NewProc("SetFocus")
	procSetActiveWindow   = user32.NewProc("SetActiveWindow")
	procSetWindowPos      = user32.NewProc("SetWindowPos")
	procFindWindowW       = user32.NewProc("FindWindowW")
	procBeginPaint        = user32.NewProc("BeginPaint")
	procEndPaint          = user32.NewProc("EndPaint")
	procInvalidateRect    = user32.NewProc("InvalidateRect")
	procUpdateWindow      = user32.NewProc("UpdateWindow")
	procFillRect          = user32.NewProc("FillRect")
	procGetDpiForWindow   = user32.NewProc("GetDpiForWindow")
	procLoadCursorW       = user32.NewProc("LoadCursorW")

	procCreateFontW         = gdi32.NewProc("CreateFontW")
	procSelectObject        = gdi32.NewProc("SelectObject")
	procSetTextColor        = gdi32.NewProc("SetTextColor")
	procSetBkMode           = gdi32.NewProc("SetBkMode")
	procTextOutW            = gdi32.NewProc("TextOutW")
	procGetTextExtentPointW = gdi32.NewProc("GetTextExtentPoint32W")
	procGetStockObject      = gdi32.NewProc("GetStockObject")
	procSaveDC              = gdi32.NewProc("SaveDC")
	procRestoreDC           = gdi32.NewProc("RestoreDC")
	procIntersectClipRect   = gdi32.NewProc("IntersectClipRect")
	procCreatePen           = gdi32.NewProc("CreatePen")
	procRoundRect           = gdi32.NewProc("RoundRect")
)

// Additional constants used by the shell and painting code.
const (
	swShowNormal = 1

	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
	seeMaskFlagNoUI       = 0x00000400

	coinitApartmentThreaded = 0x2
	clsctxInprocServer      = 0x1

	transparentBkMode = 1

	psSolid     = 0
	hollowBrush = 5 // HOLLOW_BRUSH stock object

	fwNormal = 400

	dwmwaUseImmersiveDarkMode = 20
	hwndTopmost               = ^uintptr(0) // (HWND)-1
	swpNoSize                 = 0x0001
	swpNoMove                 = 0x0002
	swpShowWindow             = 0x0040
)
