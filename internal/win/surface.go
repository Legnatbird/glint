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
// pixels stay in Go: trim, resample, composite over the row background and
// stretch the result to the screen in one StretchDIBits call. Deterministic, and
// nothing depends on a blending path that cannot be debugged from here.

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
	// bg is a COLORREF (0x00BBGGRR): the low byte is red, not blue. Reading it
	// the other way round paints the icon's transparent margin in a colour that
	// misses the row by a few levels, which shows up as a box around the icon.
	bgB := byte(bg >> 16)
	bgG := byte(bg >> 8)
	bgR := byte(bg)

	scale := float64(p.size) / float64(dst)
	for dy := 0; dy < dst; dy++ {
		sy := (float64(dy)+0.5)*scale - 0.5
		for dx := 0; dx < dst; dx++ {
			sx := (float64(dx)+0.5)*scale - 0.5
			sample := sampleBilinear(p.bgra, p.size, sx, sy)

			// RGB is pre-blended over the row colour, which is what a solid
			// window blits. The alpha channel is carried through unchanged: on
			// the layered (acrylic) window GDI paint lands at partial opacity and
			// DWM composites per pixel, so an opaque margin would punch a square
			// hole in the row and read as a box around the icon.
			a := sample[3] / 255
			offset := (dy*dst + dx) * 4
			out[offset+0] = clampByte(sample[0]*a + float64(bgB)*(1-a))
			out[offset+1] = clampByte(sample[1]*a + float64(bgG)*(1-a))
			out[offset+2] = clampByte(sample[2]*a + float64(bgR)*(1-a))
			out[offset+3] = clampByte(sample[3])
		}
	}
}

// trimToContent scales the artwork so it fills the icon box.
//
// A packaged app's logo is authored on a 32px canvas with padding, and the
// padding is not uniform: Terminal's tile covers about 28 of those pixels while a
// glyph such as Character Map's fills the frame, so in a row the first reads as a
// smaller icon. The transparent border is trimmed and the content re-scaled to
// fill, which puts every icon on the same optical footing.
func trimToContent(pixels []byte, size int32) []byte {
	minX, minY, maxX, maxY := contentBounds(pixels, size)
	if maxX < minX || maxY < minY {
		return pixels // nothing visible
	}

	w := maxX - minX + 1
	h := maxY - minY + 1
	if w >= size-1 && h >= size-1 {
		return pixels // already fills the frame
	}

	out := make([]byte, len(pixels))
	span := w
	if h > span {
		span = h
	}
	scale := float64(size) / float64(span)
	offX := (float64(size) - float64(w)*scale) / 2
	offY := (float64(size) - float64(h)*scale) / 2

	for dy := int32(0); dy < size; dy++ {
		sy := float64(minY) + (float64(dy)+0.5-offY)/scale - 0.5
		for dx := int32(0); dx < size; dx++ {
			sx := float64(minX) + (float64(dx)+0.5-offX)/scale - 0.5
			sample := sampleBilinear(pixels, size, sx, sy)
			offset := (dy*size + dx) * 4
			out[offset+0] = clampByte(sample[0])
			out[offset+1] = clampByte(sample[1])
			out[offset+2] = clampByte(sample[2])
			out[offset+3] = clampByte(sample[3])
		}
	}
	return out
}

// contentBounds returns the box that holds every pixel that is not fully
// transparent, in exclusive-max coordinates.
func contentBounds(pixels []byte, size int32) (minX, minY, maxX, maxY int32) {
	minX, minY = size, size
	maxX, maxY = -1, -1

	const visible = 8 // ignore the faint antialiasing at the very edge
	for y := int32(0); y < size; y++ {
		for x := int32(0); x < size; x++ {
			if pixels[(y*size+x)*4+3] <= visible {
				continue
			}
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	return minX, minY, maxX, maxY
}

// sampleBilinear reads the colour at fractional pixel (fx, fy).
//
// Interpolation happens in premultiplied space: sampling straight alpha would
// drag the colour of transparent pixels (black) into the edges of a glyph and
// leave a dark fringe around every icon.
func sampleBilinear(pixels []byte, size int32, fx, fy float64) [4]float64 {
	last := float64(size - 1)
	fx = math.Max(0, math.Min(last, fx))
	fy = math.Max(0, math.Min(last, fy))

	x0 := int32(math.Floor(fx))
	y0 := int32(math.Floor(fy))
	fracX := fx - float64(x0)
	fracY := fy - float64(y0)

	x1, y1 := x0+1, y0+1
	if x1 >= size {
		x1 = size - 1
	}
	if y1 >= size {
		y1 = size - 1
	}

	type premul struct{ b, g, r, a float64 }
	load := func(x, y int32) premul {
		offset := (y*size + x) * 4
		a := float64(pixels[offset+3])
		scale := a / 255
		return premul{
			b: float64(pixels[offset+0]) * scale,
			g: float64(pixels[offset+1]) * scale,
			r: float64(pixels[offset+2]) * scale,
			a: a,
		}
	}

	p00, p10 := load(x0, y0), load(x1, y0)
	p01, p11 := load(x0, y1), load(x1, y1)

	mix := func(a, b float64) float64 { return a*(1-fracX) + b*fracX }
	topB, topG, topR, topA := mix(p00.b, p10.b), mix(p00.g, p10.g), mix(p00.r, p10.r), mix(p00.a, p10.a)
	botB, botG, botR, botA := mix(p01.b, p11.b), mix(p01.g, p11.g), mix(p01.r, p11.r), mix(p01.a, p11.a)

	outA := topA*(1-fracY) + botA*fracY
	outB := topB*(1-fracY) + botB*fracY
	outG := topG*(1-fracY) + botG*fracY
	outR := topR*(1-fracY) + botR*fracY

	if outA <= 0 {
		return [4]float64{0, 0, 0, 0}
	}
	// Back to straight alpha for the caller.
	inv := 255 / outA
	return [4]float64{outB * inv, outG * inv, outR * inv, outA}
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

// blitDIB copies a BGRA buffer onto the canvas at (x, y).
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
