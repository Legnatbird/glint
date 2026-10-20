package win

import "testing"

// Icons shipped before Windows 2000-style alpha channels carry their
// transparency in an AND mask. Reading those as if they had alpha would paint
// them as solid black rectangles.
func TestMaskAlpha(t *testing.T) {
	const pixels = 4
	bgra := make([]byte, pixels*4)
	bits := make([]byte, pixels*4)
	bits[0] = 255 // first pixel is masked out
	bits[1*4] = 255

	maskAlpha(bgra, bits)

	want := []byte{0, 0, 255, 255}
	for i, w := range want {
		if got := bgra[i*4+3]; got != w {
			t.Fatalf("pixel %d alpha = %d, want %d", i, got, w)
		}
	}
}

func TestMaskAlphaWithoutMask(t *testing.T) {
	bgra := make([]byte, 4*4)
	maskAlpha(bgra, nil)
	for i := 0; i < len(bgra); i += 4 {
		if bgra[i+3] != 255 {
			t.Fatalf("pixel %d alpha = %d, want an opaque icon", i/4, bgra[i+3])
		}
	}
}

func TestHasAlpha(t *testing.T) {
	if hasAlpha(make([]byte, 4*4)) {
		t.Error("a buffer with a zero alpha channel was reported as having alpha")
	}
	withAlpha := make([]byte, 4*4)
	withAlpha[3] = 1
	if !hasAlpha(withAlpha) {
		t.Error("a buffer with one non-zero alpha was reported as having none")
	}
}

// The visible box drives how big the artwork is drawn, so it has to ignore the
// transparent padding and nothing else.
func TestContentBounds(t *testing.T) {
	const size = 8
	bgra := make([]byte, size*size*4)
	for y := int32(2); y < 6; y++ {
		for x := int32(3); x < 7; x++ {
			bgra[(y*size+x)*4+3] = 255
		}
	}

	box := contentBounds(bgra, size, size)
	if box.x != 3 || box.y != 2 || box.w != 4 || box.h != 4 {
		t.Fatalf("box = %+v, want x=3 y=2 w=4 h=4", box)
	}
}

func TestContentBoundsOfEmptyBuffer(t *testing.T) {
	box := contentBounds(make([]byte, 8*8*4), 8, 8)
	if box.w != 0 || box.h != 0 {
		t.Fatalf("box = %+v, want an empty box", box)
	}
}
