package win

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// The launcher's own icon, drawn at runtime.
//
// Keeping it in code avoids a resource compiler step (windres/.syso) in the
// build. The shape is a rounded square with a white chevron because that is
// what survives at 16x16 on a taskbar: geometric forms antialias, text does not.
//
// The bitmap is rasterized in Go rather than with GDI calls: GDI leaves the
// alpha channel at zero on a 32bpp DIB, and repairing it afterwards is more
// work than computing coverage in the first place.

const (
	biRGB         = 0
	dibRGBColors  = 0
	iconAccentR   = 122
	iconAccentG   = 162
	iconAccentB   = 255
	iconSupersamp = 4
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

// chevron is the polygon of the ">" glyph in unit coordinates.
var chevron = [][2]float64{
	{0.36, 0.24},
	{0.62, 0.50},
	{0.36, 0.76},
	{0.28, 0.68},
	{0.46, 0.50},
	{0.28, 0.32},
}

// AppIcon builds a square icon of the given size.
func AppIcon(size int32) (Icon, error) {
	info := bitmapInfo{}
	info.header = bitmapInfoHeader{
		biSize:        uint32(unsafe.Sizeof(info.header)),
		biWidth:       size,
		biHeight:      -size, // top-down rows
		biPlanes:      1,
		biBitCount:    32,
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

	drawIconPixels((*[1 << 20]byte)(pixels)[:size*size*4], size)

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

// drawIconPixels fills a BGRA buffer with 4x4 supersampled coverage.
func drawIconPixels(buf []byte, size int32) {
	radius := float64(size) * 0.22
	margin := float64(size) * 0.04
	steps := iconSupersamp

	for y := int32(0); y < size; y++ {
		for x := int32(0); x < size; x++ {
			var panel, glyph int
			for sy := 0; sy < steps; sy++ {
				for sx := 0; sx < steps; sx++ {
					px := float64(x) + (float64(sx)+0.5)/float64(steps)
					py := float64(y) + (float64(sy)+0.5)/float64(steps)

					if !insideRoundedRect(px, py, margin, float64(size)-margin, radius) {
						continue
					}
					panel++
					if pointInPolygon(px/float64(size), py/float64(size), chevron) {
						glyph++
					}
				}
			}
			if panel == 0 {
				continue
			}

			// Blend the white glyph over the accent panel, then scale alpha by
			// the panel coverage so the corners stay smooth.
			mix := float64(glyph) / float64(panel)
			r := iconAccentR + (255-iconAccentR)*mix
			g := iconAccentG + (255-iconAccentG)*mix
			b := iconAccentB + (255-iconAccentB)*mix
			a := 255.0 * float64(panel) / float64(steps*steps)

			offset := (y*size + x) * 4
			buf[offset+0] = byte(b)
			buf[offset+1] = byte(g)
			buf[offset+2] = byte(r)
			buf[offset+3] = byte(a)
		}
	}
}

func insideRoundedRect(x, y, min, max, radius float64) bool {
	// Clamp to the inner rectangle; the distance to that point must stay within
	// the corner radius for the pixel to be inside.
	innerX := x
	if innerX < min+radius {
		innerX = min + radius
	}
	if innerX > max-radius {
		innerX = max - radius
	}
	innerY := y
	if innerY < min+radius {
		innerY = min + radius
	}
	if innerY > max-radius {
		innerY = max - radius
	}

	dx := x - innerX
	dy := y - innerY
	return dx*dx+dy*dy <= radius*radius
}

func pointInPolygon(x, y float64, poly [][2]float64) bool {
	inside := false
	j := len(poly) - 1
	for i := 0; i < len(poly); i++ {
		xi, yi := poly[i][0], poly[i][1]
		xj, yj := poly[j][0], poly[j][1]
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			inside = !inside
		}
		j = i
	}
	return inside
}
