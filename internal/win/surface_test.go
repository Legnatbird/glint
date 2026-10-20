package win

import "testing"

// solid returns a width x height BGRA buffer painted with one colour.
func solid(width, height int32, b, g, r, a byte) []byte {
	buf := make([]byte, width*height*4)
	for i := 0; i < len(buf); i += 4 {
		buf[i+0], buf[i+1], buf[i+2], buf[i+3] = b, g, r, a
	}
	return buf
}

// icon wraps a buffer the way the shell path does, measuring the visible box.
func icon(width, height int32, bgra []byte) *iconPixels {
	p := &iconPixels{width: width, height: height, bgra: bgra}
	p.content = contentBounds(bgra, width, height)
	return p
}

// A packaged app icon is composited over the row colour. If this math breaks,
// every Store app draws as a dark or transparent block.
func TestCompositeOverBackground(t *testing.T) {
	const dst = 4

	for _, tc := range []struct {
		name                string
		src                 []byte
		bg                  uint32
		wantB, wantG, wantR byte
	}{
		{
			name:  "opaque source replaces the background",
			src:   solid(dst, dst, 0, 0, 255, 255),
			bg:    0x1F1A1A,
			wantB: 0, wantG: 0, wantR: 255,
		},
		{
			name: "transparent source keeps the background",
			src:  solid(dst, dst, 0, 0, 255, 0),
			// COLORREF 0x00BBGGRR for rgb(0x1A, 0x1A, 0x1F) = #1A1A1F.
			bg:    0x1F1A1A,
			wantB: 0x1F, wantG: 0x1A, wantR: 0x1A,
		},
		{
			name:  "half transparent white over black",
			src:   solid(dst, dst, 255, 255, 255, 128),
			bg:    0x000000,
			wantB: 128, wantG: 128, wantR: 128,
		},
	} {
		out := make([]byte, dst*dst*4)
		icon(dst, dst, tc.src).composite(out, dst, tc.bg)

		centre := (dst/2*dst + dst/2) * 4
		if out[centre+0] != tc.wantB || out[centre+1] != tc.wantG || out[centre+2] != tc.wantR {
			t.Errorf("%s: got bgra(%d,%d,%d), want (%d,%d,%d)",
				tc.name, out[centre+0], out[centre+1], out[centre+2], tc.wantB, tc.wantG, tc.wantR)
		}
	}
}

// The composite must cover every pixel, or the row shows through in bands.
func TestCompositeFillsEveryPixel(t *testing.T) {
	const dst = 8
	out := make([]byte, dst*dst*4)
	icon(32, 32, solid(32, 32, 9, 9, 9, 255)).composite(out, dst, 0xFFFFFF)

	for i := 0; i < len(out); i += 4 {
		if out[i+3] != 255 {
			t.Fatalf("pixel %d has alpha %d, want the opaque buffer a DIB blit needs", i/4, out[i+3])
		}
		if out[i] != 9 || out[i+1] != 9 || out[i+2] != 9 {
			t.Fatalf("pixel %d = %d,%d,%d, want the source colour", i/4, out[i], out[i+1], out[i+2])
		}
	}
}

// A fully transparent icon still has to erase the slot: the row colour has to
// reach every pixel, or the previous frame shows through.
func TestCompositeOfEmptyIconPaintsTheRowColour(t *testing.T) {
	const dst = 6
	out := make([]byte, dst*dst*4)
	icon(dst, dst, solid(dst, dst, 0, 0, 0, 0)).composite(out, dst, 0x1F1A1A)

	for i := 0; i < len(out); i += 4 {
		if out[i+0] != 0x1F || out[i+1] != 0x1A || out[i+2] != 0x1A || out[i+3] != 255 {
			t.Fatalf("pixel %d = %d,%d,%d,%d, want the row colour", i/4, out[i], out[i+1], out[i+2], out[i+3])
		}
	}
}

// Packaged-app logos ship with uneven transparent padding. Fitting the visible
// box - not the whole canvas - is what gives every icon the same optical weight.
func TestCompositeFitsTheVisibleBox(t *testing.T) {
	const size = 32

	// A 16x16 square in the middle of the canvas, like a tile with padding.
	padded := solid(size, size, 0, 0, 0, 0)
	for y := int32(8); y < 24; y++ {
		for x := int32(8); x < 24; x++ {
			offset := (y*size + x) * 4
			padded[offset], padded[offset+1], padded[offset+2], padded[offset+3] = 200, 100, 50, 255
		}
	}

	out := make([]byte, size*size*4)
	icon(size, size, padded).composite(out, size, 0x1F1A1A)

	box := contentBounds(out, size, size)
	if box.w != size || box.h != size {
		t.Fatalf("artwork covers %dx%d, want it to fill the %dpx slot", box.w, box.h, size)
	}
}

// Fitting must never stretch: a 2:1 logo stays 2:1 inside a square slot.
func TestCompositeKeepsAspect(t *testing.T) {
	const size = 32

	wide := solid(size, size, 0, 0, 0, 0)
	for y := int32(12); y < 20; y++ {
		for x := int32(8); x < 24; x++ {
			offset := (y*size + x) * 4
			wide[offset], wide[offset+1], wide[offset+2], wide[offset+3] = 0, 200, 0, 255
		}
	}

	out := make([]byte, size*size*4)
	icon(size, size, wide).composite(out, size, 0x1F1A1A)

	// The output is opaque, so the box is measured against the row colour. The
	// edges of the artwork blend into the margin, which is worth one pixel.
	box := paintedBox(out, size, size, 0x1F1A1A)
	if box.w != 32 {
		t.Fatalf("2:1 artwork is %d wide, want the full 32", box.w)
	}
	if box.h < 15 || box.h > 18 {
		t.Fatalf("2:1 artwork is %d tall, want about 16 (a stretch would be 32)", box.h)
	}
}

// paintedBox is contentBounds for an already opaque buffer: it looks for pixels
// that differ from the colour the slot was filled with.
func paintedBox(bgra []byte, width, height int32, bg uint32) contentBox {
	bgB := byte(bg >> 16)
	bgG := byte(bg >> 8)
	bgR := byte(bg)

	minX, minY := width, height
	maxX, maxY := int32(-1), int32(-1)
	for y := int32(0); y < height; y++ {
		for x := int32(0); x < width; x++ {
			o := (y*width + x) * 4
			if near(bgra[o+0], bgB) && near(bgra[o+1], bgG) && near(bgra[o+2], bgR) {
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

func near(a, b byte) bool {
	d := int(a) - int(b)
	if d < 0 {
		d = -d
	}
	return d <= 3
}

// Shrinking has to average the source footprint. Point sampling a checkerboard
// keeps the extremes - that is exactly the stepped, ragged edge that made the
// list look badly rendered - while averaging lands on the mean.
func TestCompositeAveragesWhenShrinking(t *testing.T) {
	const src = 32
	const dst = 8

	checker := make([]byte, src*src*4)
	for y := int32(0); y < src; y++ {
		for x := int32(0); x < src; x++ {
			offset := (y*src + x) * 4
			v := byte(0)
			if (x+y)%2 == 0 {
				v = 255
			}
			checker[offset], checker[offset+1], checker[offset+2], checker[offset+3] = v, v, v, 255
		}
	}

	out := make([]byte, dst*dst*4)
	icon(src, src, checker).composite(out, dst, 0x000000)

	for i := 0; i < len(out); i += 4 {
		if v := out[i]; v < 120 || v > 136 {
			t.Fatalf("pixel %d = %d, want the 50%% grey of an averaged checkerboard", i/4, v)
		}
	}
}

// Straight-alpha interpolation drags transparent black into glyph edges; the
// sampler must interpolate premultiplied.
func TestBilinearDoesNotDarkenEdges(t *testing.T) {
	const size = 4
	pixels := solid(size, size, 0, 0, 0, 0)
	// Column 1 is opaque white, the rest transparent.
	for y := int32(0); y < size; y++ {
		offset := (y*size + 1) * 4
		pixels[offset], pixels[offset+1], pixels[offset+2], pixels[offset+3] = 255, 255, 255, 255
	}

	// Halfway between the white column and an empty one.
	b, g, r, a := icon(size, size, pixels).bilinear(1.5, 2.0).straight()
	if b < 240 || g < 240 || r < 240 {
		t.Fatalf("edge sample = %v,%v,%v, want a white colour rather than grey", b, g, r)
	}
	if a < 100 || a > 160 {
		t.Fatalf("edge alpha = %v, want about half", a)
	}
}

// BenchmarkComposite is the cost of one icon in a paint pass: a list of nine
// rows must not turn a keystroke into a frame drop.
func BenchmarkComposite(b *testing.B) {
	const src = 32
	const dst = 25

	art := make([]byte, src*src*4)
	for i := 0; i < len(art); i += 4 {
		art[i+0], art[i+1], art[i+2], art[i+3] = byte(i), byte(i>>8), 0x80, byte(i>>4)
	}
	p := icon(src, src, art)
	out := make([]byte, dst*dst*4)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.composite(out, dst, 0x332A2A)
	}
}

func TestClampByte(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		want byte
	}{
		{-5, 0},
		{0, 0},
		{127.4, 127},
		{127.6, 128},
		{255, 255},
		{400, 255},
	} {
		if got := clampByte(tc.in); got != tc.want {
			t.Errorf("clampByte(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
