package win

import (
	"strings"
	"testing"
	"unsafe"
)

// offscreenCanvas returns a canvas backed by a 32bpp DIB, so tests can use real
// font metrics instead of guessing at pixel widths.
func offscreenCanvas(t *testing.T, width, height int32) *Canvas {
	t.Helper()

	info := bitmapInfo{}
	info.header = bitmapInfoHeader{
		biSize:        uint32(unsafe.Sizeof(info.header)),
		biWidth:       width,
		biHeight:      -height,
		biPlanes:      1,
		biBitCount:    iconDIBLayout,
		biCompression: biRGB,
	}

	var bits unsafe.Pointer
	bitmap, _, callErr := procCreateDIBSection.Call(
		0, uintptr(unsafe.Pointer(&info)), dibRGBColors,
		uintptr(unsafe.Pointer(&bits)), 0, 0,
	)
	if bitmap == 0 || bits == nil {
		t.Fatalf("CreateDIBSection: %v", callErr)
	}

	dc, _, dcErr := procCreateCompatDC.Call(0)
	if dc == 0 {
		t.Fatalf("CreateCompatibleDC: %v", dcErr)
	}
	procSelectObject.Call(dc, bitmap)

	t.Cleanup(func() {
		procDeleteDC.Call(dc)
		procDeleteObject.Call(bitmap)
	})
	return &Canvas{hdc: dc}
}

func TestShortenLeavesShortTextAlone(t *testing.T) {
	c := offscreenCanvas(t, 200, 40)
	fonts := NewFonts(96)
	defer fonts.Release()
	c.UseFont(fonts.Row)

	if got := c.Shorten("Terminal", 150); got != "Terminal" {
		t.Fatalf("Shorten = %q, want the string untouched", got)
	}
}

// A name that does not fit has to end in an ellipsis and stay inside its box:
// a row whose text runs under the window edge looks broken, not long.
func TestShortenCutsWithEllipsis(t *testing.T) {
	c := offscreenCanvas(t, 400, 40)
	fonts := NewFonts(96)
	defer fonts.Release()
	c.UseFont(fonts.Row)

	const long = "Compañero de la consola Xbox y algo más largo todavía"
	for _, width := range []int32{40, 90, 160, 240} {
		got := c.Shorten(long, width)
		if !strings.HasSuffix(got, "…") {
			t.Fatalf("Shorten(%d) = %q, want an ellipsis", width, got)
		}
		if w := c.Measure(got); w > width {
			t.Fatalf("Shorten(%d) measures %d, want it to fit", width, w)
		}
		if !strings.HasPrefix(long, strings.TrimSuffix(got, "…")) {
			t.Fatalf("Shorten(%d) = %q, want a prefix of the original", width, got)
		}
	}
}

func TestShortenDegradesToJustTheEllipsis(t *testing.T) {
	c := offscreenCanvas(t, 200, 40)
	fonts := NewFonts(96)
	defer fonts.Release()
	c.UseFont(fonts.Row)

	if got := c.Shorten("Terminal", 1); got != "…" {
		t.Fatalf("Shorten(1) = %q, want just an ellipsis", got)
	}
}

// The query field keeps the caret at the end, so a long query shows its tail.
func TestTailKeepsTheEnd(t *testing.T) {
	c := offscreenCanvas(t, 400, 40)
	fonts := NewFonts(96)
	defer fonts.Release()
	c.UseFont(fonts.Query)

	const query = "C:\\Program Files\\WindowsApps\\Microsoft.WindowsTerminal"
	for _, width := range []int32{60, 120, 260} {
		got := c.Tail(query, width)
		if w := c.Measure(got); w > width {
			t.Fatalf("Tail(%d) measures %d, want it to fit", width, w)
		}
		if !strings.HasSuffix(query, got) {
			t.Fatalf("Tail(%d) = %q, want a suffix of the query", width, got)
		}
	}
	if got := c.Tail("term", 200); got != "term" {
		t.Fatalf("Tail = %q, want the string untouched when it fits", got)
	}
}

// A row's text is centred on the font's line height. If that ever regresses the
// labels sit low in their row again, which is what a hand-tuned offset does.
func TestLineHeightFitsInsideARow(t *testing.T) {
	c := offscreenCanvas(t, 200, 40)
	fonts := NewFonts(96)
	defer fonts.Release()

	for name, font := range map[string]uintptr{"query": fonts.Query, "row": fonts.Row, "footer": fonts.Footer} {
		c.UseFont(font)
		h := c.LineHeight()
		if h <= 0 {
			t.Fatalf("%s font reports line height %d", name, h)
		}
		if h > 50 {
			t.Fatalf("%s line height %d does not fit a 50px row", name, h)
		}
	}
}
