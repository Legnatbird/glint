package win

import (
	"fmt"
	"unsafe"
)

// Acrylic backdrop and rounded corners.
//
// On Windows 10 the acrylic material is reachable through
// SetWindowCompositionAttribute, an API Microsoft never documented and has
// changed between releases. Two details decide whether it is visible at all:
//
//   - the window must be layered, so the desktop (and the blurred backdrop DWM
//     renders behind it) composites with the content instead of being covered;
//   - the tint is carried by the accent policy, not by the painted background,
//     which stays opaque and dark.
//
// If any call fails the launcher keeps its solid background: this is polish,
// never a startup dependency.

const (
	wcaAccentPolicy = 19

	accentDisabled         = 0
	accentEnableBlurBehind = 3
	accentEnableAcrylic    = 4

	lwaAlpha = 0x00000002
)

type accentPolicy struct {
	state         uint32
	flags         uint32
	gradientColor uint32
	animationID   uint32
}

type windowCompositionAttributeData struct {
	attribute  uint32
	data       unsafe.Pointer
	sizeOfData uint32
}

var (
	procSetWindowCompositionAttribute = user32.NewProc("SetWindowCompositionAttribute")
	procSetLayeredWindowAttributes    = user32.NewProc("SetLayeredWindowAttributes")
	procCreateRoundRectRgn            = gdi32.NewProc("CreateRoundRectRgn")
	procSetWindowRgn                  = user32.NewProc("SetWindowRgn")
	procGetWindowLongPtrW             = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW             = user32.NewProc("SetWindowLongPtrW")
)

// GWL_EXSTYLE as an unsigned value: -20 in two's complement, because a negative
// constant cannot be converted to uintptr directly.
const gwlExStyle = ^uintptr(19)

// SetLayered toggles WS_EX_LAYERED on a window that already exists. Acrylic
// needs it, and switching the backdrop at runtime needs it applied after the
// fact: the style cannot be changed from outside the creation call.
func SetLayered(hwnd uintptr, on bool) error {
	ex, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlExStyle)
	if on {
		ex |= wsExLayered
	} else {
		ex &^= wsExLayered
	}
	procSetWindowLongPtrW.Call(hwnd, gwlExStyle, ex)

	got, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlExStyle)
	if layered := got&wsExLayered != 0; layered != on {
		return fmt.Errorf("win: SetWindowLongPtrW(GWL_EXSTYLE) did not stick")
	}

	// The style change only takes effect on the next frame pass. Without this the
	// window keeps its old composition and a window that just became layered
	// renders as a fully transparent rectangle.
	procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0,
		swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate|swpFrameChanged)
	return nil
}

// EnableAcrylic applies the acrylic tint and the global opacity that lets it
// show through the painted content. tint is AABBGGRR.
func EnableAcrylic(hwnd uintptr, tint uint32, opacity byte) error {
	if ok, _, _ := procSetLayeredWindowAttributes.Call(hwnd, 0, uintptr(opacity), lwaAlpha); ok == 0 {
		return fmt.Errorf("win: SetLayeredWindowAttributes failed")
	}

	policy := accentPolicy{
		state:         accentEnableAcrylic,
		gradientColor: tint,
	}
	data := windowCompositionAttributeData{
		attribute:  wcaAccentPolicy,
		data:       unsafe.Pointer(&policy),
		sizeOfData: uint32(unsafe.Sizeof(policy)),
	}
	if ok, _, err := procSetWindowCompositionAttribute.Call(hwnd, uintptr(unsafe.Pointer(&data))); ok == 0 {
		return fmt.Errorf("win: SetWindowCompositionAttribute failed: %w", err)
	}
	return nil
}

// DisableAcrylic restores an opaque window.
func DisableAcrylic(hwnd uintptr) {
	policy := accentPolicy{state: accentDisabled}
	data := windowCompositionAttributeData{
		attribute:  wcaAccentPolicy,
		data:       unsafe.Pointer(&policy),
		sizeOfData: uint32(unsafe.Sizeof(policy)),
	}
	procSetWindowCompositionAttribute.Call(hwnd, uintptr(unsafe.Pointer(&data)))
	procSetLayeredWindowAttributes.Call(hwnd, 0, 255, lwaAlpha)
}

// Round corners, two ways.
//
// A window region clips the window to a rounded rectangle, but the clip is
// binary: the arc is a staircase of full pixels, which at a small radius reads as
// a diagonal chamfer rather than a corner. Windows 11 can round a window through
// DWM instead, with antialiasing, so that is tried first and the region stays as
// the fallback for Windows 10.

const (
	dwmwaWindowCornerPreference = 33

	// CornerRound is DWMWCP_ROUND.
	CornerRound = 2
)

var procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")

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
