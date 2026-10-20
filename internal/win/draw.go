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
type Canvas struct {
	hdc uintptr
	// scratchBuf is reused by icons that must be composited in Go, so a paint
	// pass allocates once instead of once per row.
	scratchBuf []byte
}

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

// RoundBorder outlines a rounded rectangle with a 1px pen. The corners follow
// the same arc the window region clips: drawing four straight lines instead
// leaves the light stroke cut diagonally across the corner, which reads as a
// chamfer rather than a curve.
func (c *Canvas) RoundBorder(r Rect, radius int32, bgr uint32) {
	if radius <= 0 || r.Right <= r.Left || r.Bottom <= r.Top {
		return
	}

	pen, _, _ := procCreatePen.Call(psSolid, 1, uintptr(bgr))
	if pen == 0 {
		return
	}
	brush, _, _ := procGetStockObject.Call(hollowBrush)
	if brush == 0 {
		procDeleteObject.Call(pen)
		return
	}

	oldPen, _, _ := procSelectObject.Call(c.hdc, pen)
	oldBrush, _, _ := procSelectObject.Call(c.hdc, brush)
	// The ellipse size is twice the radius, matching CreateRoundRectRgn.
	procRoundRect.Call(c.hdc,
		uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom),
		uintptr(radius*2), uintptr(radius*2),
	)
	procSelectObject.Call(c.hdc, oldPen)
	procSelectObject.Call(c.hdc, oldBrush)
	procDeleteObject.Call(pen)
}

// Clip intersects the current clip region with r and returns a function that
// restores the previous region. Callers use it to keep a scrolling list from
// painting over the header and footer.
func (c *Canvas) Clip(r Rect) func() {
	saved, _, _ := procSaveDC.Call(c.hdc)
	if saved == 0 {
		return func() {}
	}
	rect := nativeRect{r.Left, r.Top, r.Right, r.Bottom}
	procIntersectClipRect.Call(c.hdc,
		uintptr(rect.left), uintptr(rect.top),
		uintptr(rect.right), uintptr(rect.bottom))
	return func() { procRestoreDC.Call(c.hdc, saved) }
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
		Query:  createFont(scale(24), fwNormal),
		Row:    createFont(scale(18), fwNormal),
		Footer: createFont(scale(12), fwNormal),
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
