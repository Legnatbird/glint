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

// Icon is the caller's to destroy. It holds either an HICON, or decoded pixels
// when the shell handed back a 32bpp bitmap (packaged apps).
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
	return Icon{h: info.hIcon}, nil
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
	return Icon{h: windows.Handle(h)}
}
