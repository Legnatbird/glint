package win

import (
	"fmt"
	"unsafe"
)

// Rounded corners.
//
// Windows 11 can round a window through DWM, with antialiasing. Windows 10 has no
// such attribute, so the window is clipped to a region and the arc is a staircase
// of whole pixels, which is why the painted border follows the same arc instead
// of cutting across the corner.
//
// The window is opaque: a translucent backdrop was removed because the accent
// policy that provided it paints the whole window rectangle and ignores the
// region, leaving a dark sliver in every corner.

const (
	dwmwaWindowCornerPreference = 33

	// CornerRound is DWMWCP_ROUND.
	CornerRound = 2
)

var (
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
	procCreateRoundRectRgn    = gdi32.NewProc("CreateRoundRectRgn")
	procSetWindowRgn          = user32.NewProc("SetWindowRgn")
)

// RoundCornersDWM asks the compositor to round the window's corners.
func RoundCornersDWM(hwnd uintptr) error {
	preference := int32(CornerRound)
	r, _, callErr := procDwmSetWindowAttribute.Call(
		hwnd,
		uintptr(dwmwaWindowCornerPreference),
		uintptr(unsafe.Pointer(&preference)),
		uintptr(unsafe.Sizeof(preference)),
	)
	if int32(r) < 0 {
		return fmt.Errorf("win: DwmSetWindowAttribute(corners): 0x%08X: %w", uint32(r), callErr)
	}
	return nil
}

// ClearRegion drops a window region, which is what a window rounded by DWM needs:
// the two mechanisms would otherwise fight over the same corners.
func ClearRegion(hwnd uintptr) {
	procSetWindowRgn.Call(hwnd, 0, 1)
}

// RoundCorners clips the window to a rounded rectangle.
//
// A window region is binary: the corners are antialiased by nothing, which is
// why the radius stays small.
func RoundCorners(hwnd uintptr, radius int32) error {
	var r nativeRect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	width := r.right - r.left
	height := r.bottom - r.top
	if width <= 0 || height <= 0 {
		return fmt.Errorf("win: cannot round a %dx%d window", width, height)
	}

	// +1 because CreateRoundRectRgn excludes the right and bottom edges.
	rgn, _, err := procCreateRoundRectRgn.Call(0, 0, uintptr(width+1), uintptr(height+1),
		uintptr(radius*2), uintptr(radius*2))
	if rgn == 0 {
		return fmt.Errorf("win: CreateRoundRectRgn failed: %w", err)
	}
	ok, _, callErr := procSetWindowRgn.Call(hwnd, rgn, 1)
	if ok == 0 {
		return fmt.Errorf("win: SetWindowRgn failed: %w", callErr)
	}
	// SetWindowRgn takes ownership: the region is freed by the system.
	return nil
}
