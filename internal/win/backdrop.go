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
)

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

// RoundCorners clips the window to a rounded rectangle.
//
// A window region is binary: the corners are antialiased by nothing, which is
// why the radius stays small. Windows 11 could do this through DWM instead.
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
