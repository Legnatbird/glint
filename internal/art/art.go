// Package art draws the launcher's own mark.
//
// It is pure Go and knows nothing about Win32: internal/win turns the buffer
// into an HICON and tools/icongen turns the same buffer into the .ico that is
// embedded in the executable. One shape, three consumers, no drift.
package art

// Accent colour of the panel, as straight RGB.
const (
	AccentR = 122
	AccentG = 162
	AccentB = 255
)

// The gloss is the light the name is about: a band of it across the top of the
// rounded panel, wider on the left, the way light falling from the upper-left
// looks on a raised surface.
//
// A band with a soft edge, not a gradient over the whole panel: a gradient reads
// as a vignette and vanishes at icon sizes, while an edge survives being drawn at
// 16px. It is also the same shape at every size, so nothing is redrawn per scale.
const (
	glossPeak    = 0.32 // how much white the lit band mixes in, 0..1
	glossEdge    = 0.30 // where the band ends at the left edge, in unit coordinates
	glossSlope   = 0.10 // how much that boundary drops towards the right edge
	glossFeather = 0.06 // width of the soft edge, so it is not a cut-out
)

// supersampling is the coverage grid per pixel. 4x4 is enough for the sizes the
// shell asks for and keeps the whole render under a millisecond.
const supersampling = 4

// chevron is the ">" glyph in unit coordinates.
var chevron = [][2]float64{
	{0.36, 0.24},
	{0.62, 0.50},
	{0.36, 0.76},
	{0.28, 0.68},
	{0.46, 0.50},
	{0.28, 0.32},
}

// RenderBGRA rasterizes the mark at size into a top-down BGRA buffer
// (size*size*4 bytes), which is both the layout of a 32bpp DIB section and the
// layout of an .ico entry. Alpha is straight, not premultiplied.
func RenderBGRA(size int32) []byte {
	buf := make([]byte, size*size*4)

	radius := float64(size) * 0.22
	margin := float64(size) * 0.04
	steps := float64(supersampling * supersampling)

	for y := int32(0); y < size; y++ {
		for x := int32(0); x < size; x++ {
			var inPanel, inGlyph int
			for sy := 0; sy < supersampling; sy++ {
				for sx := 0; sx < supersampling; sx++ {
					px := float64(x) + (float64(sx)+0.5)/supersampling
					py := float64(y) + (float64(sy)+0.5)/supersampling

					if !InsideRoundedRect(px, py, margin, float64(size)-margin, radius) {
						continue
					}
					inPanel++
					if PointInPolygon(px/float64(size), py/float64(size), chevron) {
						inGlyph++
					}
				}
			}
			if inPanel == 0 {
				continue
			}

			// The panel is lit first and the glyph sits on top of it, which is
			// what a raised white shape over a shining surface looks like.
			shine := gloss((float64(x)+0.5)/float64(size), (float64(y)+0.5)/float64(size))
			panelR := AccentR + (255-AccentR)*shine
			panelG := AccentG + (255-AccentG)*shine
			panelB := AccentB + (255-AccentB)*shine

			// Blend the white glyph over the lit panel, then scale alpha by the
			// panel coverage so the corners stay smooth.
			mix := float64(inGlyph) / float64(inPanel)
			r := panelR + (255-panelR)*mix
			g := panelG + (255-panelG)*mix
			b := panelB + (255-panelB)*mix
			a := 255.0 * float64(inPanel) / steps

			offset := (y*size + x) * 4
			buf[offset+0] = byte(b)
			buf[offset+1] = byte(g)
			buf[offset+2] = byte(r)
			buf[offset+3] = byte(a)
		}
	}
	return buf
}

// gloss is the strength of the highlight at unit coordinates (u, v).
func gloss(u, v float64) float64 {
	edge := glossEdge - glossSlope*u
	if v >= edge {
		return 0
	}
	if v <= edge-glossFeather {
		return glossPeak
	}
	t := (edge - v) / glossFeather
	return glossPeak * t * t * (3 - 2*t)
}

// InsideRoundedRect reports whether (x, y) falls inside the axis-aligned square
// [min, max] with rounded corners of the given radius.
func InsideRoundedRect(x, y, min, max, radius float64) bool {
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

// PointInPolygon is the even-odd rule test.
func PointInPolygon(x, y float64, poly [][2]float64) bool {
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
