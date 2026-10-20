package win

import (
	"fmt"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// AppsFolder enumeration.
//
// Store and MSIX applications do not have .lnk files: they live in the shell's
// AppsFolder namespace, and the only stable identity they expose is an
// AppUserModelID. Missing them means missing Edge, Windows Terminal, Settings
// and every app installed from the Store.
//
// The namespace enumerates through IShellFolder (bound with BHID_SFObject), not
// through the IEnumShellItems handler: requesting the latter returns
// E_NOINTERFACE on Windows 10.

const (
	shcontfFolders    = 0x20
	shcontfNonFolders = 0x40

	shgdnNormal     = 0x0000
	shgdnForParsing = 0x8000
)

// strret mirrors STRRET: a 32-bit discriminator followed by a 260-byte union.
// The explicit padding keeps the union at its native offset of 8.
type strret struct {
	uType uint32
	_     uint32
	data  [264]byte
}

var (
	procStrRetToBufW = shlwapi.NewProc("StrRetToBufW")

	procSHCreateItemWithParent = shell32.NewProc("SHCreateItemWithParent")
	procCreateIconIndirect     = user32.NewProc("CreateIconIndirect")
	procCreateBitmap           = gdi32.NewProc("CreateBitmap")

	iidShellItem    = &guid{0x43826D1E, 0xE718, 0x42EE, [8]byte{0xBC, 0x55, 0xA1, 0xE2, 0x61, 0xC3, 0x7B, 0xFE}}
	iidShellFolder  = &guid{0x000214E6, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidShellItem2   = &guid{0x7E9FB0D3, 0x919F, 0x4307, [8]byte{0xAB, 0x2E, 0x9B, 0x18, 0x60, 0x31, 0x0C, 0x93}}
	iidImageFactory = &guid{0xBCC18B79, 0xBA16, 0x442F, [8]byte{0x80, 0xC4, 0x8A, 0x59, 0xC3, 0x0C, 0x46, 0x3B}}

	bhidSFObject = &guid{0x3981E224, 0xF559, 0x11D3, [8]byte{0x8E, 0x3A, 0x00, 0xC0, 0x4F, 0x68, 0x37, 0xD5}}
)

// ShellApp is one entry of the AppsFolder namespace.
type ShellApp struct {
	Name  string
	AUMID string
}

// EnumerateAppsFolder lists the applications the Start Menu knows about,
// including packaged (UWP/MSIX) ones.
//
// Only entries whose parsing name is an AppUserModelID of the form
// "PackageFamily!AppId" are returned: plain Win32 apps also appear in this
// namespace, but they cannot be activated this way and their .lnk entry already
// covers them.
func EnumerateAppsFolder() ([]ShellApp, error) {
	item, err := shCreateItemFromParsingName("shell:AppsFolder")
	if err != nil {
		return nil, err
	}
	defer item.release()

	var rawFolder unsafe.Pointer
	// IShellItem::BindToHandler is vtable slot 3.
	if _, err := item.call(3, 0,
		uintptr(unsafe.Pointer(bhidSFObject)),
		uintptr(unsafe.Pointer(iidShellFolder)),
		uintptr(unsafe.Pointer(&rawFolder)),
	); err != nil {
		return nil, fmt.Errorf("win: bind AppsFolder: %w", err)
	}
	folder := comPtr{p: rawFolder}
	defer folder.release()

	var rawEnum unsafe.Pointer
	// IShellFolder::EnumObjects is vtable slot 4.
	if _, err := folder.call(4, 0,
		shcontfFolders|shcontfNonFolders,
		uintptr(unsafe.Pointer(&rawEnum)),
	); err != nil {
		return nil, fmt.Errorf("win: enumerate AppsFolder: %w", err)
	}
	enum := comPtr{p: rawEnum}
	defer enum.release()

	var apps []ShellApp
	for {
		var pidl unsafe.Pointer
		var fetched uint32
		// IEnumIDList::Next is vtable slot 3.
		if _, err := enum.call(3, 1,
			uintptr(unsafe.Pointer(&pidl)),
			uintptr(unsafe.Pointer(&fetched)),
		); err != nil || fetched == 0 || pidl == nil {
			break
		}

		name := shellFolderName(folder, pidl, shgdnNormal)
		aumid := shellFolderName(folder, pidl, shgdnForParsing)
		icon, iconErr := extractItemImage(folder, pidl)
		procCoTaskMemFree.Call(uintptr(pidl))

		if name != "" && strings.Contains(aumid, "!") {
			apps = append(apps, ShellApp{Name: name, AUMID: aumid})
			if iconErr == nil {
				packagedIcons.Store(packagedIconKey(aumid), icon)
			}
		}
	}
	return apps, nil
}

func shCreateItemFromParsingName(name string) (comPtr, error) {
	ptr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return comPtr{}, err
	}

	var out unsafe.Pointer
	r, _, _ := procSHCreateItemFromParsingName.Call(
		uintptr(unsafe.Pointer(ptr)),
		0,
		uintptr(unsafe.Pointer(iidShellItem)),
		uintptr(unsafe.Pointer(&out)),
	)
	if int32(r) < 0 {
		return comPtr{}, fmt.Errorf("win: SHCreateItemFromParsingName(%s): 0x%08X", name, uint32(r))
	}
	return comPtr{p: out}, nil
}

// shellFolderName reads IShellFolder::GetDisplayNameOf (vtable slot 11) and
// converts the STRRET through StrRetToBufW, which also frees the allocation.
func shellFolderName(folder comPtr, pidl unsafe.Pointer, flags uint32) string {
	var name strret
	if _, err := folder.call(11,
		uintptr(pidl),
		uintptr(flags),
		uintptr(unsafe.Pointer(&name)),
	); err != nil {
		return ""
	}

	buf := make([]uint16, 512)
	r, _, _ := procStrRetToBufW.Call(
		uintptr(unsafe.Pointer(&name)),
		uintptr(pidl),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if int32(r) < 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

// packagedIcons keeps the HICON of every packaged app found during the scan.
//
// A packaged app has no file path, so its icon can only be extracted while its
// PIDL is in hand, that is, during the AppsFolder enumeration. The handle is
// then handed to the UI through its AUMID.
var packagedIcons sync.Map // lowercase AUMID -> Icon

const (
	// GetImage is called with no flags on purpose. SIIGBF_ICONONLY returns the
	// shell's *plated* tile: the app's art composited over a solid accent-colour
	// square (#0078D7). The Start menu shows the app's own logo, which is
	// transparent around its edges, and that is what a list of rows wants: a
	// blue plate per Store app would be a wall of squares.
	siigbfNoFlags  = 0
	iconSizePixels = 32
)

// packagedIconKey is the IconPath form the UI uses for packaged apps.
func packagedIconKey(aumid string) string { return "aumid:" + strings.ToLower(aumid) }

// loadPackagedIcon returns an icon cached by the last scan.
func loadPackagedIcon(aumid string) (Icon, error) {
	if v, ok := packagedIcons.Load(packagedIconKey(aumid)); ok {
		return v.(Icon), nil
	}
	return Icon{}, fmt.Errorf("win: no icon cached for %s", aumid)
}

// extractItemImage asks IShellItemImageFactory for the item's icon and turns
// the resulting HBITMAP into an HICON, so the rest of the code keeps drawing
// with DrawIconEx.
func extractItemImage(folder comPtr, pidl unsafe.Pointer) (Icon, error) {
	var rawItem unsafe.Pointer
	r, _, _ := procSHCreateItemWithParent.Call(
		0,
		uintptr(folder.p),
		uintptr(pidl),
		uintptr(unsafe.Pointer(iidShellItem2)),
		uintptr(unsafe.Pointer(&rawItem)),
	)
	if int32(r) < 0 || rawItem == nil {
		return Icon{}, fmt.Errorf("win: SHCreateItemWithParent: 0x%08X", uint32(r))
	}
	item := comPtr{p: rawItem}
	defer item.release()

	factory, err := item.queryInterface(iidImageFactory)
	if err != nil {
		return Icon{}, err
	}
	defer factory.release()

	// SIZE is two LONGs packed into one register on x64.
	sizeArg := uintptr(iconSizePixels) | uintptr(iconSizePixels)<<32
	var bitmap windows.Handle
	// IShellItemImageFactory::GetImage is vtable slot 3.
	if _, err := factory.call(3, sizeArg, siigbfNoFlags, uintptr(unsafe.Pointer(&bitmap))); err != nil || bitmap == 0 {
		return Icon{}, fmt.Errorf("win: GetImage: %w", err)
	}
	defer procDeleteObject.Call(uintptr(bitmap))

	pixels, err := readBitmapPixels(bitmap, iconSizePixels)
	if err != nil {
		return Icon{}, err
	}
	return Icon{pixels: &iconPixels{size: iconSizePixels, bgra: pixels}}, nil
}

type iconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  windows.Handle
	hbmColor windows.Handle
}
