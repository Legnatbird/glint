package win

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Shell icons.
//
// SHGetFileInfoW resolves whichever icon the shell would show for a path: the
// executable's icon, the shortcut's icon, and even shell parsing names such as
// "shell:AppsFolder\<AUMID>" for packaged apps. It is one call, no COM object
// to keep alive, and the HICON it returns is ours to destroy.

const (
	shgfiIcon      = 0x00000100
	shgfiLargeIcon = 0x00000000
	shgfiSmallIcon = 0x00000001

	diNormal = 0x0003
)

var (
	procSHGetFileInfoW = shell32.NewProc("SHGetFileInfoW")
	procDrawIconEx     = user32.NewProc("DrawIconEx")
	procDestroyIcon    = user32.NewProc("DestroyIcon")
)

type shFileInfo struct {
	hIcon         windows.Handle
	iIcon         int32
	dwAttributes  uint32
	szDisplayName [260]uint16
	szTypeName    [80]uint16
}

// nativeBitmap mirrors BITMAP from wingdi.h: what GetObjectW reports for an
// HBITMAP, which is how an icon tells us how big it really is.
type nativeBitmap struct {
	bmType       int32
	bmWidth      int32
	bmHeight     int32
	bmWidthBytes int32
	bmPlanes     uint16
	bmBitsPixel  uint16
	bmBits       uintptr
}

// Icon is the caller's to destroy.
//
// Rows always hold pixels: an icon arrives at 32px and is drawn at 25, and GDI's
// own scaling of an HICON leaves hard, stepped edges, so the handle is decoded
// once and released. The handle survives only where Win32 needs one back, as in
// the notification area.
type Icon struct {
	h      windows.Handle
	pixels *iconPixels
}

// LoadShellIcon asks the shell for the icon of path.
func LoadShellIcon(path string) (Icon, error) {
	// Packaged apps have no file: their icon was captured during the scan.
	if aumid, ok := strings.CutPrefix(path, "aumid:"); ok {
		return loadPackagedIcon(aumid)
	}

	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return Icon{}, err
	}

	var info shFileInfo
	ok, _, callErr := procSHGetFileInfoW.Call(
		uintptr(unsafe.Pointer(ptr)),
		0,
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
		shgfiIcon|shgfiLargeIcon,
	)
	if ok == 0 || info.hIcon == 0 {
		return Icon{}, fmt.Errorf("win: no icon for %s: %w", path, callErr)
	}

	// Decode now and hand the handle back: a row never needs it again, and the
	// process stays a little poorer in GDI objects for the rest of the session.
	pixels, decodeErr := decodeHICON(info.hIcon)
	procDestroyIcon.Call(uintptr(info.hIcon))
	if decodeErr != nil {
		return Icon{}, decodeErr
	}
	return Icon{pixels: pixels}, nil
}

// decodeHICON reads an icon into straight-alpha pixels at its own size.
//
// Older icons carry transparency in a 1bpp AND mask instead of an alpha channel;
// GetDIBits hands the mask back as black and white, which is enough to rebuild
// the alpha the rest of the code expects.
func decodeHICON(h windows.Handle) (*iconPixels, error) {
	var info iconInfo
	if r, _, callErr := procGetIconInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&info))); r == 0 {
		return nil, fmt.Errorf("win: GetIconInfo: %w", callErr)
	}
	if info.hbmColor != 0 {
		defer procDeleteObject.Call(uintptr(info.hbmColor))
	}
	if info.hbmMask != 0 {
		defer procDeleteObject.Call(uintptr(info.hbmMask))
	}
	if info.hbmColor == 0 {
		return nil, fmt.Errorf("win: icon carries no colour bitmap")
	}

	var bm nativeBitmap
	if r, _, _ := procGetObjectW.Call(uintptr(info.hbmColor), unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm))); r == 0 {
		return nil, fmt.Errorf("win: GetObjectW on the icon bitmap failed")
	}

	pixels, err := readBitmapPixels(info.hbmColor, bm.bmWidth, bm.bmHeight)
	if err != nil {
		return nil, err
	}
	if !hasAlpha(pixels) {
		applyIconMask(pixels, info.hbmMask, bm.bmWidth, bm.bmHeight)
	}

	return &iconPixels{
		width:   bm.bmWidth,
		height:  bm.bmHeight,
		bgra:    pixels,
		content: contentBounds(pixels, bm.bmWidth, bm.bmHeight),
	}, nil
}

// hasAlpha reports whether the buffer carries a real alpha channel. Pre-alpha
// icons leave it at zero and are opaque or masked instead.
func hasAlpha(pixels []byte) bool {
	for i := 3; i < len(pixels); i += 4 {
		if pixels[i] != 0 {
			return true
		}
	}
	return false
}

// applyIconMask sets alpha from the AND mask: a set bit means transparent.
func applyIconMask(pixels []byte, mask windows.Handle, width, height int32) {
	var bits []byte
	if mask != 0 {
		// A 1bpp mask read as 32bpp arrives black and white, which is all the
		// transparency information there is.
		bits, _ = readBitmapPixels(mask, width, height)
	}
	maskAlpha(pixels, bits)
}

// maskAlpha copies the mask into the alpha channel. A nil mask means the icon is
// simply opaque.
func maskAlpha(pixels, bits []byte) {
	for o := 0; o+3 < len(pixels); o += 4 {
		if bits != nil && o+3 < len(bits) && bits[o] > 128 {
			pixels[o+3] = 0 // white in the mask: not part of the icon
			continue
		}
		pixels[o+3] = 255
	}
}

// Draw paints the icon at (x, y) with the requested size. bg is the colour the
// row paints behind it, which pixel icons need in order to composite.
func (i Icon) Draw(c *Canvas, x, y, size int32, bg uint32) {
	if i.pixels != nil {
		i.pixels.draw(c, x, y, size, bg)
		return
	}
	if i.h == 0 {
		return
	}
	procDrawIconEx.Call(c.hdc,
		uintptr(x), uintptr(y),
		uintptr(i.h),
		uintptr(size), uintptr(size),
		0, 0, diNormal,
	)
}

// Destroy releases whatever the icon owns.
func (i Icon) Destroy() {
	i.pixels = nil
	if i.h != 0 {
		procDestroyIcon.Call(uintptr(i.h))
	}
}

// DefaultAppIcon is the generic application icon, used when an entry has no
// icon of its own.
func DefaultAppIcon() Icon {
	h, _, _ := procLoadIconW.Call(0, idiApplication)
	// IDI_APPLICATION is shared with the rest of the process: decode it so the
	// caller never ends up holding a handle it does not own.
	if pixels, err := decodeHICON(windows.Handle(h)); err == nil {
		return Icon{pixels: pixels}
	}
	return Icon{h: windows.Handle(h)}
}
