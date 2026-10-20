package win

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestDumpPackagedIcons is a debugging aid: it extracts every packaged app icon
// the way the launcher does and writes a contact sheet, so a change in the
// compositing can be looked at instead of guessed at.
//
//	DUMP_ICONS=1 DUMP_OUT=D:\tmp go test ./internal/win -run DumpIcons -v
func TestDumpPackagedIcons(t *testing.T) {
	if os.Getenv("DUMP_ICONS") == "" {
		t.Skip("set DUMP_ICONS=1 to write a contact sheet of packaged app icons")
	}
	outDir := os.Getenv("DUMP_OUT")
	if outDir == "" {
		outDir = os.TempDir()
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	release := CoInitialize()
	defer release()

	apps, err := EnumerateAppsFolder()
	if err != nil {
		t.Fatalf("enumerate: %v", err)
	}

	const cell = 64 // 32px icon drawn at 2x, on the row colour
	cols := 4
	rows := (len(apps) + cols - 1) / cols
	sheet := image.NewNRGBA(image.Rect(0, 0, cols*cell, rows*cell))
	for i := range sheet.Pix {
		if i%4 == 3 {
			sheet.Pix[i] = 255
		}
	}
	// Row colour #2A2A33.
	for y := 0; y < sheet.Bounds().Dy(); y++ {
		for x := 0; x < sheet.Bounds().Dx(); x++ {
			sheet.SetNRGBA(x, y, color.NRGBA{R: 0x2A, G: 0x2A, B: 0x33, A: 255})
		}
	}

	for i, app := range apps {
		icon, err := loadPackagedIcon(app.AUMID)
		if err != nil || icon.pixels == nil {
			t.Logf("%-40s (no icon)", app.Name)
			continue
		}
		pixels := icon.pixels
		// Draw at 2x so the edges are visible in the sheet.
		buf := make([]byte, 64*64*4)
		pixels.composite(buf, 64, 0x332A2A)
		ox, oy := (i%cols)*cell, (i/cols)*cell
		for y := 0; y < cell; y++ {
			for x := 0; x < cell; x++ {
				src := (y*64 + x) * 4
				sheet.SetNRGBA(ox+x, oy+y, color.NRGBA{
					R: buf[src+2], G: buf[src+1], B: buf[src+0], A: 255,
				})
			}
		}
		minX, minY, maxX, maxY := contentBounds(pixels.bgra, pixels.size)
		t.Logf("%-40s density=%.3f bounds=%d,%d..%d,%d", app.Name, density(pixels), minX, minY, maxX, maxY)
	}

	path := filepath.Join(outDir, "packaged-icons.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer file.Close()
	if err := png.Encode(file, sheet); err != nil {
		t.Fatalf("encode: %v", err)
	}
	t.Logf("wrote %s (%d apps)", path, len(apps))
}

// density is the share of pixels that are visible, which is the number that
// decides whether an icon looks smaller than its neighbours.
func density(p *iconPixels) float64 {
	var visible int
	for i := 0; i < len(p.bgra); i += 4 {
		if p.bgra[i+3] > 8 {
			visible++
		}
	}
	return float64(visible) / float64(p.size*p.size)
}
