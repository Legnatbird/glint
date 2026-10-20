package win

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Tray icon and its context menu.
//
// Shell_NotifyIconW wants a NOTIFYICONDATAW whose layout is version-sensitive.
// The struct below is the Windows Vista+ (V3) shape; NewTray refuses to run if
// the Go layout does not land exactly where the C one does, because a silently
// misaligned struct means an icon that never appears.

const (
	trayMessage = 0x800A // WM_APP + 10

	nimAdd    = 0
	nimDelete = 2

	nifMessage = 0x01
	nifIcon    = 0x02
	nifTip     = 0x04

	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100

	mfString    = 0x0000
	mfSeparator = 0x0800
	mfChecked   = 0x0008

	idiApplication = 32512
	idcArrow       = 32512

	wmLButtonUp = 0x0202
	wmRButtonUp = 0x0205
	wmNull      = 0x0000

	notifyIconDataSize = 976
)

var (
	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	procLoadIconW        = user32.NewProc("LoadIconW")
	procCreatePopupMenu  = user32.NewProc("CreatePopupMenu")
	procAppendMenuW      = user32.NewProc("AppendMenuW")
	procDestroyMenu      = user32.NewProc("DestroyMenu")
	procTrackPopupMenu   = user32.NewProc("TrackPopupMenu")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
)

type notifyIconData struct {
	cbSize       uint32
	hwnd         windows.Handle
	uID          uint32
	uFlags       uint32
	uCallbackMsg uint32
	hIcon        windows.Handle
	szTip        [128]uint16
	dwState      uint32
	dwStateMask  uint32
	szInfo       [256]uint16
	uVersion     uint32
	szInfoTitle  [64]uint16
	dwInfoFlags  uint32
	guidItem     guid
	hBalloonIcon windows.Handle
}

// Exported message codes the window hands to OnTray.
const (
	WmLButtonUp = wmLButtonUp
	WmRButtonUp = wmRButtonUp
)

// MenuItem is one row of the tray menu.
type MenuItem struct {
	ID        uint32
	Label     string
	Checked   bool
	Separator bool
}

// Tray is a notification-area icon owned by a Window.
type Tray struct {
	hwnd uintptr
	data notifyIconData
}

// NewTray adds the icon. tooltip is truncated to what the shell accepts.
func NewTray(hwnd uintptr, tooltip string) (*Tray, error) {
	if size := unsafe.Sizeof(notifyIconData{}); size != notifyIconDataSize {
		return nil, fmt.Errorf("win: NOTIFYICONDATAW layout is %d bytes, expected %d", size, notifyIconDataSize)
	}

	icon := uintptr(0)
	if own, err := AppIcon(32); err == nil {
		icon = uintptr(own.h)
	}
	if icon == 0 {
		icon, _, _ = procLoadIconW.Call(0, idiApplication)
	}

	t := &Tray{hwnd: hwnd}
	t.data.cbSize = uint32(unsafe.Sizeof(t.data))
	t.data.hwnd = windows.Handle(hwnd)
	t.data.uID = 1
	t.data.uFlags = nifMessage | nifIcon | nifTip
	t.data.uCallbackMsg = trayMessage
	t.data.hIcon = windows.Handle(icon)

	tip := windows.StringToUTF16(tooltip)
	if len(tip) > len(t.data.szTip) {
		tip = tip[:len(t.data.szTip)-1]
	}
	copy(t.data.szTip[:], tip)

	ok, _, callErr := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&t.data)))
	if ok == 0 {
		return nil, fmt.Errorf("win: Shell_NotifyIcon(NIM_ADD): %w", callErr)
	}
	return t, nil
}

// Remove deletes the icon. It is safe to call twice.
func (t *Tray) Remove() {
	if t == nil {
		return
	}
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&t.data)))
}

// Popup shows the context menu at the cursor and returns the chosen command.
//
// TPM_RETURNCMD makes TrackPopupMenu return the id instead of posting
// WM_COMMAND, which keeps the menu a plain function call from the window thread.
func (t *Tray) Popup(items []MenuItem) uint32 {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return 0
	}
	defer procDestroyMenu.Call(menu)

	for _, item := range items {
		if item.Separator {
			procAppendMenuW.Call(menu, mfSeparator, 0, 0)
			continue
		}
		label, err := windows.UTF16PtrFromString(item.Label)
		if err != nil {
			continue
		}
		flags := uintptr(mfString)
		if item.Checked {
			flags |= mfChecked
		}
		procAppendMenuW.Call(menu, flags, uintptr(item.ID), uintptr(unsafe.Pointer(label)))
	}

	var pt struct{ x, y int32 }
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	// Without the window in the foreground the menu ignores clicks outside it.
	procSetForegroundWindow.Call(t.hwnd)

	cmd, _, _ := procTrackPopupMenu.Call(
		menu,
		tpmRightButton|tpmReturnCmd,
		uintptr(uint32(pt.x)),
		uintptr(uint32(pt.y)),
		0,
		t.hwnd,
		0,
	)
	procPostMessageW.Call(t.hwnd, wmNull, 0, 0)
	return uint32(cmd)
}
