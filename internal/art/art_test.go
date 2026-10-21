package art

import "testing"

func TestRenderBGRA(t *testing.T) {
	for _, size := range []int32{16, 32, 256} {
		buf := RenderBGRA(size)
		if len(buf) != int(size*size*4) {
			t.Fatalf("size %d: buffer is %d bytes, want %d", size, len(buf), size*size*4)
		}

		// The corners fall outside the rounded panel, so they must be fully
		// transparent: that is what lets the icon sit on any background.
		for _, corner := range [][2]int32{{0, 0}, {size - 1, 0}, {0, size - 1}, {size - 1, size - 1}} {
			offset := (corner[1]*size + corner[0]) * 4
			if a := buf[offset+3]; a != 0 {
				t.Errorf("size %d: corner (%d,%d) has alpha %d, want 0", size, corner[0], corner[1], a)
			}
		}

		// The exact centre sits inside the chevron: opaque white glyph.
		centre := ((size/2)*size + size/2) * 4
		if a := buf[centre+3]; a != 255 {
			t.Errorf("size %d: centre alpha = %d, want 255", size, a)
		}
		if b, g, r := buf[centre], buf[centre+1], buf[centre+2]; r < 200 || g < 200 || b < 200 {
			t.Errorf("size %d: centre is rgb(%d,%d,%d), want the white glyph", size, r, g, b)
		}

		// Both materials must be present: the accent panel and the glyph.
		var accent, glyph int
		for offset := 0; offset < len(buf); offset += 4 {
			b, g, r, a := buf[offset], buf[offset+1], buf[offset+2], buf[offset+3]
			if a < 250 {
				continue
			}
			switch {
			case r > 240 && g > 240 && b > 240:
				glyph++
			case r < 140 && b > 240:
				accent++
			}
		}
		if accent == 0 || glyph == 0 {
			t.Errorf("size %d: accent=%d glyph=%d pixels, want both", size, accent, glyph)
		}
	}
}

func TestInsideRoundedRect(t *testing.T) {
	const min, max, radius = 0.0, 100.0, 20.0

	if !InsideRoundedRect(50, 50, min, max, radius) {
		t.Error("centre should be inside")
	}
	if InsideRoundedRect(2, 2, min, max, radius) {
		t.Error("a point beyond the corner radius should be outside")
	}
	if !InsideRoundedRect(10, 50, min, max, radius) {
		t.Error("the middle of the left edge should be inside")
	}
}

func TestPointInPolygon(t *testing.T) {
	square := [][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}}

	if !PointInPolygon(0.5, 0.5, square) {
		t.Error("centre should be inside")
	}
	if PointInPolygon(1.5, 0.5, square) {
		t.Error("outside point reported inside")
	}
}

// The mark is named after a glint, so the light has to be a band at the top of
// the panel, wider on the left, with a soft edge.
func TestGlossIsABandAtTheTop(t *testing.T) {
	if got := gloss(0.1, 0.1); got != glossPeak {
		t.Fatalf("top gloss = %v, want the full %v", got, glossPeak)
	}
	if got := gloss(0.1, 0.5); got != 0 {
		t.Fatalf("middle gloss = %v, want none", got)
	}
	// Light from the upper-left: the same height is lit on the left, not on the right.
	if gloss(0.1, 0.24) <= gloss(0.9, 0.24) {
		t.Fatalf("band does not slope: left %v, right %v", gloss(0.1, 0.24), gloss(0.9, 0.24))
	}
	// The edge is soft, not a cut-out.
	if mid := gloss(0.1, 0.26); mid <= 0 || mid >= glossPeak {
		t.Fatalf("edge gloss = %v, want something between 0 and %v", mid, glossPeak)
	}
}

// Rendered pixels, not the helper: the panel's top-left has to come out lighter
// than its bottom-right.
func TestRenderIsLitFromTheTopLeft(t *testing.T) {
	const size = 64
	buf := RenderBGRA(size)

	lum := func(x, y int32) int {
		o := (y*size + x) * 4
		if buf[o+3] == 0 {
			t.Fatalf("(%d,%d) is transparent, cannot compare", x, y)
		}
		return int(buf[o]) + int(buf[o+1]) + int(buf[o+2])
	}

	// Both points sit in the panel margin, away from the white glyph.
	topLeft := lum(size/8, size/8)
	bottomRight := lum(size-size/8-1, size-size/8-1)
	if topLeft <= bottomRight {
		t.Fatalf("top-left luminance %d is not brighter than bottom-right %d", topLeft, bottomRight)
	}
}

// The glyph has to stay white: a shine that reached it would wash out the only
// shape this mark has at 16px.
func TestGlyphStaysWhite(t *testing.T) {
	// A variable, not a constant: int32(0.58*64) would be a constant conversion
	// and Go refuses a non-integer constant.
	size := int32(64)
	buf := RenderBGRA(size)

	// The chevron's tip, from the polygon in this package.
	x, y := int32(0.58*float64(size)), int32(0.50*float64(size))
	o := (y*size + x) * 4
	if buf[o] < 250 || buf[o+1] < 250 || buf[o+2] < 250 {
		t.Fatalf("glyph tip = %d,%d,%d, want white", buf[o], buf[o+1], buf[o+2])
	}
}
