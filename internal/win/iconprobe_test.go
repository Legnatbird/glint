package win

import (
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

// TestProbePackagedIcon is a debugging aid, not a test: it dumps the bitmap the
// shell returns for a packaged app under several GetImage flags, so the choice of
// flags can be made from evidence instead of from documentation.
//
//	PROBE_AUMID="Microsoft.WindowsTerminal_8wekyb3d8bbwe!App" go test ./internal/win -run Probe -v
func TestProbePackagedIcon(t *testing.T) {
	aumid := os.Getenv("PROBE_AUMID")
	if aumid == "" {
		t.Skip("set PROBE_AUMID to dump the shell image for a packaged app")
	}
	outDir := os.Getenv("PROBE_OUT")
	if outDir == "" {
		outDir = os.TempDir()
	}

	// COM objects are apartment bound: the goroutine must not migrate.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	release := CoInitialize()
	defer release()

	item, err := shCreateItemFromParsingName("shell:AppsFolder\\" + aumid)
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	defer item.release()

	factory, err := item.queryInterface(iidImageFactory)
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	defer factory.release()

	const (
		biggerSizeOK = 0x00000001
		iconOnly     = 0x00000004
	)

	for _, probe := range []struct {
		name  string
		size  uintptr
		flags uintptr
	}{
		{"icononly-32", 32, iconOnly},
		{"default-32", 32, 0},
		{"bigger-32", 32, biggerSizeOK},
		{"default-64", 64, 0},
		{"default-16", 16, 0},
	} {
		var bitmap windows.Handle
		sizeArg := probe.size | probe.size<<32
		if _, err := factory.call(3, sizeArg, probe.flags, uintptr(unsafe.Pointer(&bitmap))); err != nil || bitmap == 0 {
			t.Logf("%-22s GetImage failed: %v", probe.name, err)
			continue
		}

		var bm struct {
			bmType, bmWidth, bmHeight, bmWidthBytes int32
			bmPlanes, bmBitsPixel                   uint16
			bmBits                                  uintptr
		}
		procGetObject.Call(uintptr(bitmap), unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm)))
		t.Logf("%-22s bitmap is %dx%d %dbpp", probe.name, bm.bmWidth, bm.bmHeight, bm.bmBitsPixel)

		pixels, err := readBitmapPixels(bitmap, int32(probe.size))
		procDeleteObject.Call(uintptr(bitmap))
		if err != nil {
			t.Logf("%-22s %v", probe.name, err)
			continue
		}

		img := bgraToImage(pixels, int32(probe.size))
		path := filepath.Join(outDir, "probe-"+probe.name+".png")
		file, err := os.Create(path)
		if err != nil {
			t.Fatalf("create %s: %v", path, err)
		}
		if err := png.Encode(file, img); err != nil {
			file.Close()
			t.Fatalf("encode %s: %v", path, err)
		}
		file.Close()

		r, g, b, a := img.At(int(probe.size)/2, int(probe.size)/2).RGBA()
		cr, cg, cb, ca := img.At(0, 0).RGBA()
		t.Logf("%-22s -> %s centre=(%d,%d,%d,%d) corner=(%d,%d,%d,%d)",
			probe.name, path, r>>8, g>>8, b>>8, a>>8, cr>>8, cg>>8, cb>>8, ca>>8)
	}
}

var procGetObject = gdi32.NewProc("GetObjectW")

func bgraToImage(pixels []byte, size int32) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, int(size), int(size)))
	for i := int32(0); i < size*size; i++ {
		img.SetNRGBA(int(i%size), int(i/size), color.NRGBA{
			R: pixels[i*4+2],
			G: pixels[i*4+1],
			B: pixels[i*4+0],
			A: pixels[i*4+3],
		})
	}
	return img
}
