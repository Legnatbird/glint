package win

import (
	"fmt"
	"math"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Icon pixels, and why every icon is blended in Go.
//
// The shell can hand back an icon as an HICON or as a 32bpp bitmap, and both
// arrive bigger than the row that shows them (32px art in a 25px slot). Letting
// GDI scale them is what made the list look ragged: DrawIconEx resamples with the
// device's raw stretch mode, so a circle arrives with hard, stepped edges, while
// the shell's bitmaps went through two resamples (once to trim their padding and
// once to fit the slot) and arrived soft. Neither is a geometry problem (the
// shapes were always right), but both read as "badly rendered".
//
// So the pixels stay in Go: each icon is decoded once at its native size, its
// visible box is measured once, and drawing fits that box into the slot with a
// single resample that averages area when shrinking and interpolates when
// growing. AlphaBlend is not an option here: on this Windows build it rejects
// these surfaces with ERROR_INVALID_PARAMETER even for a plain DDB-to-DDB copy.

var (
	procStretchDIBits  = gdi32.NewProc("StretchDIBits")
	procGetDIBits      = gdi32.NewProc("GetDIBits")
	procCreateCompatDC = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC       = gdi32.NewProc("DeleteDC")
)

const srcCopy = 0x00CC0020

// iconPixels is a decoded icon: width x height straight-alpha BGRA, plus the box
// that actually holds visible pixels.
type iconPixels struct {
	width, height int32
	bgra          []byte
	content       contentBox
}

// contentBox is the part of a buffer that is not fully transparent.
type contentBox struct {
	x, y, w, h int32
}

// readBitmapPixels copies an HBITMAP into a top-down BGRA buffer.
func readBitmapPixels(bitmap windows.Handle, width, height int32) ([]byte, error) {
	if width <= 0 || height <= 0 || bitmap == 0 {
		return nil, fmt.Errorf("win: bitmap has no pixels")
	}

	info := bitmapInfo{}
	info.header = bitmapInfoHeader{
		biSize:        uint32(unsafe.Sizeof(info.header)),
		biWidth:       width,
		biHeight:      -height,
		biPlanes:      1,
		biBitCount:    iconDIBLayout,
		biCompression: biRGB,
	}

	pixels := make([]byte, int(width)*int(height)*4)
	dc, _, dcErr := procCreateCompatDC.Call(0)
	if dc == 0 {
		return nil, fmt.Errorf("win: CreateCompatibleDC: %w", dcErr)
	}
	defer procDeleteDC.Call(dc)

	rows, _, _ := procGetDIBits.Call(
		dc,
		uintptr(bitmap),
		0,
		uintptr(height),
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
	if p == nil || size <= 0 || p.width <= 0 || p.height <= 0 {
		return
	}
	dst := int(size)
	buf := c.scratch(dst * dst * 4)
	p.composite(buf, size, bg)
	blitDIB(c, buf, dst, x, y)
}

// composite fits the icon's visible box into a size x size square and resamples
// it once, blending each pixel over bg.
func (p *iconPixels) composite(out []byte, size int32, bg uint32) {
	// bg is a COLORREF (0x00BBGGRR): the low byte is red, not blue. Reading it
	// the other way round paints the icon's transparent margin in a colour that
	// misses the row by a few levels, which shows up as a box around the icon.
	bgB := byte(bg >> 16)
	bgG := byte(bg >> 8)
	bgR := byte(bg)

	// The slot is square and the artwork usually is not, so the margin starts as
	// the row colour and the artwork is centred over it.
	for i := 0; i < len(out); i += 4 {
		out[i+0], out[i+1], out[i+2], out[i+3] = bgB, bgG, bgR, 255
	}

	box := p.content
	if box.w <= 0 || box.h <= 0 {
		return
	}

	scale := math.Min(float64(size)/float64(box.w), float64(size)/float64(box.h))
	w := int32(math.Round(float64(box.w) * scale))
	h := int32(math.Round(float64(box.h) * scale))
	if w > size {
		w = size
	}
	if h > size {
		h = size
	}
	if w <= 0 || h <= 0 {
		return
	}
	x0 := (size - w) / 2
	y0 := (size - h) / 2

	// One source texel spans step destination pixels, so step > 1 means the icon
	// is being shrunk and the sampler has to average instead of point-sample.
	step := 1 / scale
	for dy := int32(0); dy < h; dy++ {
		sy := float64(box.y) + (float64(dy)+0.5)*step - 0.5
		row := (int(y0+dy)*int(size) + int(x0)) * 4
		for dx := int32(0); dx < w; dx++ {
			sx := float64(box.x) + (float64(dx)+0.5)*step - 0.5
			b, g, r, a := p.sample(sx, sy, step).straight()

			// RGB is pre-blended over the row colour, which is what an opaque
			// window blits. The alpha byte is carried through anyway: it costs
			// nothing and keeps the buffer meaningful if it is ever drawn on a
			// composited surface.
			cov := a / 255
			offset := row + int(dx)*4
			out[offset+0] = clampByte(b*cov + float64(bgB)*(1-cov))
			out[offset+1] = clampByte(g*cov + float64(bgG)*(1-cov))
			out[offset+2] = clampByte(r*cov + float64(bgR)*(1-cov))
			out[offset+3] = clampByte(a)
		}
	}
}

// contentBounds returns the box that holds every pixel that is not fully
// transparent.
func contentBounds(bgra []byte, width, height int32) contentBox {
	minX, minY := width, height
	maxX, maxY := int32(-1), int32(-1)

	const visible = 8 // ignore the faint antialiasing at the very edge
	for y := int32(0); y < height; y++ {
		for x := int32(0); x < width; x++ {
			if bgra[(y*width+x)*4+3] <= visible {
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
	if maxX < minX || maxY < minY {
		return contentBox{}
	}
	return contentBox{x: minX, y: minY, w: maxX - minX + 1, h: maxY - minY + 1}
}

// sample reads the icon at fractional source position (fx, fy). step is the
// source span of one destination pixel: shrinking averages the whole footprint,
// which is what keeps a small circle round instead of stepped.
func (p *iconPixels) sample(fx, fy, step float64) premul {
	if step <= 1 {
		return p.bilinear(fx, fy)
	}

	n := int(math.Ceil(step))
	if n > 4 {
		n = 4
	}
	// The footprint is centred on fx, so the taps start at its left edge;
	// sampling from fx itself would walk off the end and clamp.
	half := step / 2
	var acc premul
	for i := 0; i < n; i++ {
		sx := fx - half + (float64(i)+0.5)*step/float64(n)
		for j := 0; j < n; j++ {
			sy := fy - half + (float64(j)+0.5)*step/float64(n)
			s := p.bilinear(sx, sy)
			acc.b += s.b
			acc.g += s.g
			acc.r += s.r
			acc.a += s.a
		}
	}
	k := 1 / float64(n*n)
	return premul{b: acc.b * k, g: acc.g * k, r: acc.r * k, a: acc.a * k}
}

// premul is a colour with premultiplied alpha, which is the only space where
// mixing is correct: averaging straight alpha would drag the colour of
// transparent pixels (black) into the edges and leave a dark fringe.
type premul struct{ b, g, r, a float64 }

// straight returns the colour with straight alpha, all channels in 0..255.
func (s premul) straight() (b, g, r, a float64) {
	if s.a <= 0 {
		return 0, 0, 0, 0
	}
	k := 255 / s.a
	return s.b * k, s.g * k, s.r * k, s.a
}

func (p *iconPixels) bilinear(fx, fy float64) premul {
	lastX := float64(p.width - 1)
	lastY := float64(p.height - 1)
	fx = math.Max(0, math.Min(lastX, fx))
	fy = math.Max(0, math.Min(lastY, fy))

	x0 := int32(math.Floor(fx))
	y0 := int32(math.Floor(fy))
	fracX := fx - float64(x0)
	fracY := fy - float64(y0)

	x1, y1 := x0+1, y0+1
	if x1 >= p.width {
		x1 = p.width - 1
	}
	if y1 >= p.height {
		y1 = p.height - 1
	}

	top := mixPremul(p.load(x0, y0), p.load(x1, y0), fracX)
	bot := mixPremul(p.load(x0, y1), p.load(x1, y1), fracX)
	return mixPremul(top, bot, fracY)
}

func (p *iconPixels) load(x, y int32) premul {
	offset := (y*p.width + x) * 4
	a := float64(p.bgra[offset+3])
	k := a / 255
	return premul{
		b: float64(p.bgra[offset+0]) * k,
		g: float64(p.bgra[offset+1]) * k,
		r: float64(p.bgra[offset+2]) * k,
		a: a,
	}
}

func mixPremul(a, b premul, t float64) premul {
	return premul{
		b: a.b*(1-t) + b.b*t,
		g: a.g*(1-t) + b.g*t,
		r: a.r*(1-t) + b.r*t,
		a: a.a*(1-t) + b.a*t,
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
