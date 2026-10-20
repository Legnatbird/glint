package win

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TestRenderIcons is a debugging aid: it paints every icon the launcher would
// show, at 96px, through the production Icon.Draw path, into an offscreen DIB.
// No window and no screen capture are involved, so whatever shape appears is the
// shape the code draws.
//
//	RENDER_ICONS=1 RENDER_OUT=D:\tmp go test ./internal/win -run RenderIcons -v
func TestRenderIcons(t *testing.T) {
	if os.Getenv("RENDER_ICONS") == "" {
		t.Skip("set RENDER_ICONS=1 to render the icons at 96px")
	}
	outDir := os.Getenv("RENDER_OUT")
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

	var targets []iconTarget
	for _, app := range apps {
		targets = append(targets, iconTarget{app.Name, packagedIconKey(app.AUMID)})
	}
	for _, p := range extraIconPaths() {
		targets = append(targets, iconTarget{filepath.Base(p), p})
	}

	const size = 96
	const cell = size + 8
	cols := 6
	rows := (len(targets) + cols - 1) / cols
	sheet := image.NewNRGBA(image.Rect(0, 0, cols*cell, rows*cell))

	for i, tg := range targets {
		icon, err := LoadShellIcon(tg.path)
		if err != nil {
			t.Logf("%-38s %v", tg.name, err)
			continue
		}

		pixels, err := drawIconAt(icon, size)
		icon.Destroy()
		if err != nil {
			t.Logf("%-38s draw: %v", tg.name, err)
			continue
		}

		label := tg.name
		if icon.pixels != nil {
			minX, minY, maxX, maxY := contentBounds(icon.pixels.bgra, icon.pixels.size)
			label += fmt.Sprintf(" [px %dx%d]", maxX-minX+1, maxY-minY+1)
		} else if icon.h != 0 {
			var bm struct {
				bmType, bmWidth, bmHeight, bmWidthBytes int32
				bmPlanes, bmBitsPixel                   uint16
				bmBits                                  uintptr
			}
			var ii iconInfo
			if r, _, _ := procGetIconInfo.Call(uintptr(icon.h), uintptr(unsafe.Pointer(&ii))); r != 0 {
				procGetObject.Call(uintptr(ii.hbmColor), unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm)))
				label += fmt.Sprintf(" [hicon %dx%d %dbpp]", bm.bmWidth, bm.bmHeight, bm.bmBitsPixel)
				procDeleteObject.Call(uintptr(ii.hbmColor))
				procDeleteObject.Call(uintptr(ii.hbmMask))
			}
		}

		// Bounds of what was actually painted, at 96px.
		minX, minY, maxX, maxY := contentBounds(pixels, size)
		t.Logf("%-38s drawn %dx%d (ratio %.2f) from %s",
			label, maxX-minX+1, maxY-minY+1,
			float64(maxX-minX+1)/float64(maxY-minY+1), tg.path)

		writeIconPNG(t, filepath.Join(outDir, "r-"+sanitize(tg.name)+".png"), pixels, size)

		ox, oy := (i%cols)*cell+4, (i/cols)*cell+4
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				src := (y*size + x) * 4
				sheet.SetNRGBA(ox+x, oy+y, color.NRGBA{
					R: pixels[src+2], G: pixels[src+1], B: pixels[src], A: 255,
				})
			}
		}
	}

	path := filepath.Join(outDir, "icons-96.png")
	writeIconSheet(t, path, sheet)
	t.Logf("wrote %s (%d targets)", path, len(targets))

	// The same icons at the size a row actually uses, then blown up with
	// nearest neighbour, which is the only honest way to look at 28px.
	writeRowSheet(t, filepath.Join(outDir, "icons-28-zoom.png"), targets)
}

// iconTarget is one entry to render.
type iconTarget struct {
	name string
	path string
}

// writeRowSheet renders every target at the row size and enlarges the result
// without resampling, so pixels can be counted instead of squinted at.
func writeRowSheet(t *testing.T, path string, targets []iconTarget) {
	const size = 28
	const pad = 4
	const scale = 12
	const cols = 5

	cells := make([][]byte, 0, len(targets))
	for _, tg := range targets {
		icon, err := LoadShellIcon(tg.path)
		if err != nil {
			continue
		}
		pixels, err := drawIconAt(icon, size)
		icon.Destroy()
		if err != nil {
			continue
		}
		cells = append(cells, pixels)
	}

	cell := size + pad
	rows := (len(cells) + cols - 1) / cols
	img := image.NewNRGBA(image.Rect(0, 0, cols*cell*scale, rows*cell*scale))
	for y := range img.Pix {
		img.Pix[y] = 0x1F // background of the row, in BGRA order below
	}
	for i := range img.Pix {
		img.Pix[i] = 0x1A
		if i%4 == 0 {
			img.Pix[i] = 0x1F
		}
		if i%4 == 3 {
			img.Pix[i] = 0xFF
		}
	}

	for i, px := range cells {
		ox := (i % cols) * cell * scale
		oy := (i / cols) * cell * scale
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				src := (y*size + x) * 4
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						img.SetNRGBA(ox+x*scale+dx+pad*scale/2, oy+y*scale+dy+pad*scale/2, color.NRGBA{
							R: px[src+2], G: px[src+1], B: px[src], A: 255,
						})
					}
				}
			}
		}
	}
	writeIconSheet(t, path, img)
	t.Logf("wrote %s (%d icons at %dpx, %dx)", path, len(cells), size, scale)
}

// drawIconAt paints icon at size x size over the row colour and returns the
// pixels, which is the same call the window makes.
func drawIconAt(icon Icon, size int32) ([]byte, error) {
	info := bitmapInfo{}
	info.header = bitmapInfoHeader{
		biSize:        uint32(unsafe.Sizeof(info.header)),
		biWidth:       size,
		biHeight:      -size,
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
		return nil, fmt.Errorf("CreateDIBSection: %w", callErr)
	}
	defer procDeleteObject.Call(uintptr(bitmap))

	buf := unsafe.Slice((*byte)(bits), size*size*4)
	for i := 0; i < len(buf); i += 4 {
		buf[i], buf[i+1], buf[i+2], buf[i+3] = 0x33, 0x2A, 0x2A, 255
	}

	dc, _, dcErr := procCreateCompatDC.Call(0)
	if dc == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC: %w", dcErr)
	}
	defer procDeleteDC.Call(dc)

	old, _, _ := procSelectObject.Call(dc, bitmap)
	icon.Draw(&Canvas{hdc: dc}, 0, 0, size, 0x332A2A)
	procSelectObject.Call(dc, old)

	out := make([]byte, len(buf))
	copy(out, buf)
	return out, nil
}

func writeIconPNG(t *testing.T, path string, pixels []byte, size int32) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, int(size), int(size)))
	for i := int32(0); i < size*size; i++ {
		img.SetNRGBA(int(i%size), int(i/size), color.NRGBA{
			R: pixels[i*4+2], G: pixels[i*4+1], B: pixels[i*4], A: 255,
		})
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		t.Fatalf("encode %s: %v", path, err)
	}
}

func writeIconSheet(t *testing.T, path string, sheet image.Image) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer file.Close()
	if err := png.Encode(file, sheet); err != nil {
		t.Fatalf("encode %s: %v", path, err)
	}
}

var procGetIconInfo = user32.NewProc("GetIconInfo")

func sanitize(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}

// extraIconPaths is a few desktop executables, which take the HICON path.
func extraIconPaths() []string {
	var out []string
	for _, p := range []string{
		`C:\Windows\System32\notepad.exe`,
		`C:\Windows\System32\mspaint.exe`,
		`C:\Windows\System32\cmd.exe`,
		`D:\ArcTracker\ARCTracker.exe`,
		`d:\steam\steamapps\common\Arc Raiders\PioneerGame.exe`,
	} {
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

var _ = windows.ERROR_SUCCESS
