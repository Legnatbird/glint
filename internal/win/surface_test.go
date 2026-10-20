package win

import "testing"

// solid returns a size x size BGRA buffer painted with one colour.
func solid(size int32, b, g, r, a byte) []byte {
	buf := make([]byte, size*size*4)
	for i := 0; i < len(buf); i += 4 {
		buf[i+0], buf[i+1], buf[i+2], buf[i+3] = b, g, r, a
	}
	return buf
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
		wantA               byte
	}{
		{
			name:  "opaque source replaces the background",
			src:   solid(dst, 0, 0, 255, 255),
			bg:    0x1F1A1A,
			wantB: 0, wantG: 0, wantR: 255, wantA: 255,
		},
		{
			name: "transparent source keeps the background and stays transparent",
			src:  solid(dst, 0, 0, 255, 0),
			// COLORREF 0x00BBGGRR for rgb(0x1A, 0x1A, 0x1F) = #1A1A1F.
			bg:    0x1F1A1A,
			wantB: 0x1F, wantG: 0x1A, wantR: 0x1A, wantA: 0,
		},
		{
			name:  "half transparent white over black",
			src:   solid(dst, 255, 255, 255, 128),
			bg:    0x000000,
			wantB: 128, wantG: 128, wantR: 128, wantA: 128,
		},
	} {
		out := make([]byte, dst*dst*4)
		pixels := &iconPixels{size: dst, bgra: tc.src}
		pixels.composite(out, dst, tc.bg)

		centre := (dst/2*dst + dst/2) * 4
		if out[centre+0] != tc.wantB || out[centre+1] != tc.wantG || out[centre+2] != tc.wantR {
			t.Errorf("%s: got bgra(%d,%d,%d), want (%d,%d,%d)",
				tc.name, out[centre+0], out[centre+1], out[centre+2], tc.wantB, tc.wantG, tc.wantR)
		}
		// The alpha channel rides along with the pre-blended colour.
		if out[centre+3] != tc.wantA {
			t.Errorf("%s: output alpha = %d, want %d", tc.name, out[centre+3], tc.wantA)
		}
	}
}

// The composite must cover every pixel, or the row shows through in bands.
func TestCompositeFillsEveryPixel(t *testing.T) {
	const dst = 8
	out := make([]byte, dst*dst*4)
	pixels := &iconPixels{size: 32, bgra: solid(32, 9, 9, 9, 255)}
	pixels.composite(out, dst, 0xFFFFFF)

	for i := 0; i < len(out); i += 4 {
		if out[i+3] != 255 {
			t.Fatalf("pixel %d has alpha %d, want the opaque source", i/4, out[i+3])
		}
		if out[i] != 9 || out[i+1] != 9 || out[i+2] != 9 {
			t.Fatalf("pixel %d = %d,%d,%d, want the source colour", i/4, out[i], out[i+1], out[i+2])
		}
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

// UWP logos arrive with uneven transparent padding; trimming gives every icon
// the same optical weight in a row.
func TestTrimToContentFillsTheFrame(t *testing.T) {
	const size = 32

	// A 16x16 square in the middle of a 32x32 canvas, like a tile with padding.
	padded := solid(size, 0, 0, 0, 0)
	for y := int32(8); y < 24; y++ {
		for x := int32(8); x < 24; x++ {
			offset := (y*size + x) * 4
			padded[offset], padded[offset+1], padded[offset+2], padded[offset+3] = 200, 100, 50, 255
		}
	}

	trimmed := trimToContent(padded, size)
	if &trimmed[0] == &padded[0] {
		t.Fatal("padded artwork was not trimmed")
	}

	// The artwork now reaches the edges: the old 1px border was transparent and
	// the middle of the old 8px padding is now inside the tile.
	for _, p := range [][2]int32{{1, 1}, {size - 2, size - 2}, {size / 2, 1}, {1, size / 2}} {
		if a := trimmed[(p[1]*size+p[0])*4+3]; a < 200 {
			t.Errorf("(%d,%d) alpha = %d, want the artwork to fill the frame", p[0], p[1], a)
		}
	}
}

func TestTrimToContentLeavesFullArtworkAlone(t *testing.T) {
	const size = 16
	full := solid(size, 30, 30, 30, 255)
	if got := trimToContent(full, size); &got[0] != &full[0] {
		t.Error("artwork that already fills the frame was resampled")
	}
}

// Straight-alpha interpolation drags transparent black into glyph edges; the
// sampler must interpolate premultiplied.
func TestSampleBilinearDoesNotDarkenEdges(t *testing.T) {
	const size = 4
	pixels := solid(size, 0, 0, 0, 0)
	// Column 1 is opaque white, the rest transparent.
	for y := int32(0); y < size; y++ {
		offset := (y*size + 1) * 4
		pixels[offset], pixels[offset+1], pixels[offset+2], pixels[offset+3] = 255, 255, 255, 255
	}

	// Halfway between the white column and an empty one.
	sample := sampleBilinear(pixels, size, 1.5, 2.0)
	if sample[0] < 240 || sample[1] < 240 || sample[2] < 240 {
		t.Fatalf("edge sample = %v, want a white colour rather than grey", sample[:3])
	}
	if sample[3] < 100 || sample[3] > 160 {
		t.Fatalf("edge alpha = %v, want about half", sample[3])
	}
}
