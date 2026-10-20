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
