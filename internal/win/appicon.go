package win

import (
	"unsafe"

	"applauncher/internal/art"

	"golang.org/x/sys/windows"
)

// The launcher's own icon, built from the pixels internal/art rasterizes.
//
// The bitmap is filled in Go rather than with GDI calls: GDI leaves the alpha
// channel at zero on a 32bpp DIB, and repairing it afterwards is more work than
// copying a buffer that already has it right.

const (
	biRGB         = 0
	dibRGBColors  = 0
	iconDIBLayout = 32 // bits per pixel
)

type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

type bitmapInfo struct {
	header bitmapInfoHeader
	colors [1]uint32
}

var procCreateDIBSection = gdi32.NewProc("CreateDIBSection")

// AppIcon builds a square icon of the given size.
func AppIcon(size int32) (Icon, error) {
	if size <= 0 {
		return Icon{}, windows.ERROR_INVALID_PARAMETER
	}

	info := bitmapInfo{}
	info.header = bitmapInfoHeader{
		biSize:        uint32(unsafe.Sizeof(info.header)),
		biWidth:       size,
		biHeight:      -size, // top-down rows
		biPlanes:      1,
		biBitCount:    iconDIBLayout,
		biCompression: biRGB,
	}

	var pixels unsafe.Pointer
	bitmap, _, callErr := procCreateDIBSection.Call(
		0,
		uintptr(unsafe.Pointer(&info)),
		dibRGBColors,
		uintptr(unsafe.Pointer(&pixels)),
		0, 0,
	)
	if bitmap == 0 || pixels == nil {
		return Icon{}, callErr
	}

	// The DIB and internal/art share the top-down BGRA layout, so this is a copy.
	dst := unsafe.Slice((*byte)(pixels), size*size*4)
	copy(dst, art.RenderBGRA(size))

	mask, _, _ := procCreateBitmap.Call(uintptr(size), uintptr(size), 1, 1, 0)
	iconInfo := iconInfo{fIcon: 1, hbmColor: windows.Handle(bitmap), hbmMask: windows.Handle(mask)}

	icon, _, callErr := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&iconInfo)))
	procDeleteObject.Call(bitmap)
	if mask != 0 {
		procDeleteObject.Call(mask)
	}
	if icon == 0 {
		return Icon{}, callErr
	}
	return Icon{h: windows.Handle(icon)}, nil
}
