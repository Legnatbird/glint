package win

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Self icons.
//
// The .ico is embedded in the executable as a resource (see assets/README.md),
// so the shell already knows how to find it. ExtractIconExW is the one call that
// returns it without depending on the resource ids the compiler happened to
// assign: index 0 is the first icon group of the file, whatever its number.

var procExtractIconExW = shell32.NewProc("ExtractIconExW")

// SelfIcon returns the icon embedded in the running executable, at the two sizes
// a window class wants. Zero handles mean the process has no icon resource.
func SelfIcon() (large, small uintptr) {
	exe, err := os.Executable()
	if err != nil {
		return 0, 0
	}
	path, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return 0, 0
	}

	var big, mini uintptr
	n, _, _ := procExtractIconExW.Call(
		uintptr(unsafe.Pointer(path)),
		0,
		uintptr(unsafe.Pointer(&big)),
		uintptr(unsafe.Pointer(&mini)),
		1,
	)
	if n == 0 {
		return 0, 0
	}
	return big, mini
}

// ReleaseIcon frees one handle returned by SelfIcon. Windows keeps the icons a
// window class registered for the life of the process, so this only matters for
// handles that were loaded and then not used.
func ReleaseIcon(h uintptr) {
	if h != 0 {
		procDestroyIcon.Call(h)
	}
}
