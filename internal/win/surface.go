package win

import (
	"fmt"
	"math"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Icon pixels, and why they are blended in Go.
//
// Most icons arrive as an HICON and are drawn with DrawIconEx, which takes them
// as they are. Packaged apps are different: the shell hands back a 32bpp bitmap
// with antialiased edges and transparent corners, and DrawIconEx decides
// transparency from a 1bpp AND mask, not from the alpha channel — feeding it one
// turns the corners into a hard, ragged frame.
//
// AlphaBlend is the obvious answer, but on this Windows build it rejects these
// surfaces with ERROR_INVALID_PARAMETER even for a plain DDB-to-DDB copy, so the
// pixels stay in Go: resample, composite over the row background (784 pixels, a
// few microseconds) and stretch the result to the screen in one StretchDIBits
// call. Deterministic, and nothing depends on a blending path that cannot be
// debugged from here.

var (
	procStretchDIBits  = gdi32.NewProc("StretchDIBits")
	procGetDIBits      = gdi32.NewProc("GetDIBits")
	procCreateCompatDC = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC       = gdi32.NewProc("DeleteDC")
)

const srcCopy = 0x00CC0020

// iconPixels is a decoded icon: size x size straight-alpha BGRA.
type iconPixels struct {
	size int32
	bgra []byte
}

// readBitmapPixels copies an HBITMAP into a top-down BGRA buffer.
func readBitmapPixels(bitmap windows.Handle, size int32) ([]byte, error) {
	info := bitmapInfo{}
	info.header = bitmapInfoHeader{
		biSize:        uint32(unsafe.Sizeof(info.header)),
		biWidth:       size,
		biHeight:      -size,
		biPlanes:      1,
		biBitCount:    iconDIBLayout,
		biCompression: biRGB,
	}

	pixels := make([]byte, size*size*4)
	dc, _, dcErr := procCreateCompatDC.Call(0)
	if dc == 0 {
		return nil, fmt.Errorf("win: CreateCompatibleDC: %w", dcErr)
	}
	defer procDeleteDC.Call(dc)

	rows, _, _ := procGetDIBits.Call(
		dc,
		uintptr(bitmap),
		0,
		uintptr(size),
		uintptr(unsafe.Pointer(&pixels[0])),
		uintptr(unsafe.Pointer(&info)),
		0, // DIB_RGB_COLORS
	)
	if rows == 0 {
		return nil, fmt.Errorf("win: GetDIBits read no rows")
	}
	return pixels, nil
}

// drawIconPixels composites the icon over bg and blits the result at size.
func (p *iconPixels) draw(c *Canvas, x, y, size int32, bg uint32) {
	if p == nil || size <= 0 || p.size <= 0 {
		return
	}
	dst := int(size)
	buf := c.scratch(dst * dst * 4)
	p.composite(buf, dst, bg)
	blitDIB(c, buf, dst, x, y)
}

// composite resamples the icon to dst x dst over a solid background.
func (p *iconPixels) composite(out []byte, dst int, bg uint32) {
	bgB := byte(bg)
	bgG := byte(bg >> 8)
	bgR := byte(bg >> 16)

	src := int(p.size)
	scale := float64(src) / float64(dst)

	for dy := 0; dy < dst; dy++ {
		// Bilinear sampling: the shell hands out 32 px and a row draws 28;
		// nearest-neighbour bites chunks out of icon art at that ratio.
		syf := (float64(dy)+0.5)*scale - 0.5
		y0 := int(math.Floor(syf))
		fy := syf - float64(y0)
		if y0 < 0 {
			y0, fy = 0, 0
		}
		y1 := y0 + 1
		if y1 >= src {
			y1 = src - 1
		}

		for dx := 0; dx < dst; dx++ {
			sxf := (float64(dx)+0.5)*scale - 0.5
			x0 := int(math.Floor(sxf))
			fx := sxf - float64(x0)
			if x0 < 0 {
				x0, fx = 0, 0
			}
			x1 := x0 + 1
			if x1 >= src {
				x1 = src - 1
			}

			top := (y0*src + x0) * 4
			topRight := (y0*src + x1) * 4
			bottom := (y1*src + x0) * 4
			bottomRight := (y1*src + x1) * 4

			var sample [4]float64
			for ch := 0; ch < 4; ch++ {
				topMix := float64(p.bgra[top+ch])*(1-fx) + float64(p.bgra[topRight+ch])*fx
				bottomMix := float64(p.bgra[bottom+ch])*(1-fx) + float64(p.bgra[bottomRight+ch])*fx
				sample[ch] = topMix*(1-fy) + bottomMix*fy
			}

			// Straight alpha over the background; the result is opaque, so
			// StretchDIBits can copy it without touching the alpha channel.
			a := sample[3] / 255
			offset := (dy*dst + dx) * 4
			out[offset+0] = clampByte(sample[0]*a + float64(bgB)*(1-a))
			out[offset+1] = clampByte(sample[1]*a + float64(bgG)*(1-a))
			out[offset+2] = clampByte(sample[2]*a + float64(bgR)*(1-a))
			out[offset+3] = 255
		}
	}
}

func clampByte(v float64) byte {
	switch {
	case v <= 0:
		return 0
	case v >= 255:
		return 255
	default:
		return byte(v + 0.5)
	}
}

// blitDIB copies an opaque BGRA buffer onto the canvas at (x, y).
func blitDIB(c *Canvas, pixels []byte, size int, x, y int32) {
	info := bitmapInfo{}
	info.header = bitmapInfoHeader{
		biSize:        uint32(unsafe.Sizeof(info.header)),
		biWidth:       int32(size),
		biHeight:      -int32(size), // top-down, matching the buffer
		biPlanes:      1,
		biBitCount:    iconDIBLayout,
		biCompression: biRGB,
	}
	procStretchDIBits.Call(c.hdc,
		uintptr(x), uintptr(y), uintptr(size), uintptr(size),
		0, 0, uintptr(size), uintptr(size),
		uintptr(unsafe.Pointer(&pixels[0])),
		uintptr(unsafe.Pointer(&info)),
		dibRGBColors, srcCopy,
	)
}

// scratch returns a reusable buffer of at least n bytes on the canvas, so a
// paint pass does not allocate once per icon.
func (c *Canvas) scratch(n int) []byte {
	if cap(c.scratchBuf) < n {
		c.scratchBuf = make([]byte, n)
	}
	buf := c.scratchBuf[:n]
	for i := range buf {
		buf[i] = 0
	}
	return buf
}
