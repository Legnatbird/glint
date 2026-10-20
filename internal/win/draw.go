package win

import (
	"math"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Drawing primitives. The launcher paints a few rectangles and some text per
// frame, so plain GDI is enough: no Direct2D context, no GPU device, nothing to
// keep alive between keystrokes.

var procDrawTextW = user32.NewProc("DrawTextW")

const (
	dtLeft       = 0x00000000
	dtTop        = 0x00000000
	dtSingleLine = 0x00000020
	dtNoPrefix   = 0x00000800

	cleartypeQuality = 5
	defaultCharset   = 1
)

// Rect is a device-pixel rectangle, left/top inclusive and right/bottom
// exclusive, exactly like the Win32 RECT.
type Rect struct {
	Left, Top, Right, Bottom int32
}

type nativeRect struct {
	left, top, right, bottom int32
}

type nativeSize struct {
	cx, cy int32
}

type paintStruct struct {
	hdc         windows.Handle
	fErase      int32
	rcPaint     nativeRect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

// Canvas wraps the device context handed out by WM_PAINT. It is only valid
// inside the paint callback.
type Canvas struct{ hdc uintptr }

// Fill paints a solid rectangle. The color is 0x00BBGGRR.
func (c *Canvas) Fill(r Rect, bgr uint32) {
	brush, _, _ := procCreateSolidBrush.Call(uintptr(bgr))
	if brush == 0 {
		return
	}
	rect := nativeRect{r.Left, r.Top, r.Right, r.Bottom}
	procFillRect.Call(c.hdc, uintptr(unsafe.Pointer(&rect)), brush)
	procDeleteObject.Call(brush)
}

// UseFont selects font for the following text calls.
func (c *Canvas) UseFont(font uintptr) {
	if font != 0 {
		procSelectObject.Call(c.hdc, font)
	}
}

// Text draws one line at (x, y).
func (c *Canvas) Text(s string, x, y int32, color uint32) {
	if s == "" {
		return
	}
	ptr, err := windows.UTF16PtrFromString(s)
	if err != nil {
		return
	}

	procSetBkMode.Call(c.hdc, transparentBkMode)
	procSetTextColor.Call(c.hdc, uintptr(color))
	r := nativeRect{x, y, x + 4096, y + 256}
	procDrawTextW.Call(c.hdc,
		uintptr(unsafe.Pointer(ptr)),
		^uintptr(0), // -1: null-terminated
		uintptr(unsafe.Pointer(&r)),
		dtLeft|dtTop|dtSingleLine|dtNoPrefix,
	)
}

// Measure returns the width in pixels of s with the selected font.
func (c *Canvas) Measure(s string) int32 {
	if s == "" {
		return 0
	}
	u, err := windows.UTF16FromString(s)
	if err != nil || len(u) <= 1 {
		return 0
	}
	var sz nativeSize
	procGetTextExtentPointW.Call(c.hdc,
		uintptr(unsafe.Pointer(&u[0])),
		uintptr(len(u)-1),
		uintptr(unsafe.Pointer(&sz)),
	)
	return sz.cx
}

// LineHeight returns the height in pixels of the selected font.
func (c *Canvas) LineHeight() int32 {
	u, _ := windows.UTF16FromString("Ag")
	var sz nativeSize
	procGetTextExtentPointW.Call(c.hdc,
		uintptr(unsafe.Pointer(&u[0])),
		uintptr(len(u)-1),
		uintptr(unsafe.Pointer(&sz)),
	)
	return sz.cy
}

// Fonts are the three text sizes the launcher uses. They are created once per
// DPI and destroyed when the window is destroyed.
type Fonts struct {
	Query  uintptr
	Row    uintptr
	Footer uintptr
}

// NewFonts builds the font set scaled for dpi.
func NewFonts(dpi uint32) *Fonts {
	scale := func(px int32) int32 {
		return int32(math.Round(float64(px) * float64(dpi) / 96.0))
	}
	return &Fonts{
		Query:  createFont(scale(22), fwNormal),
		Row:    createFont(scale(14), fwNormal),
		Footer: createFont(scale(11), fwNormal),
	}
}

// Release destroys the underlying GDI objects.
func (f *Fonts) Release() {
	for _, h := range []uintptr{f.Query, f.Row, f.Footer} {
		if h != 0 {
			procDeleteObject.Call(h)
		}
	}
}

func createFont(height, weight int32) uintptr {
	face, err := windows.UTF16PtrFromString("Segoe UI")
	if err != nil {
		return 0
	}
	h, _, _ := procCreateFontW.Call(
		uintptr(uint32(-height)), // negative: character height, not cell height
		0, 0, 0,
		uintptr(uint32(weight)),
		0, 0, 0,
		defaultCharset,
		0, 0,
		cleartypeQuality,
		0,
		uintptr(unsafe.Pointer(face)),
	)
	return h
}
