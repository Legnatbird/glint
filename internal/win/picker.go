package win

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Shell picker.
//
// IFileOpenDialog replaces both SHBrowseForFolder and the ANSI-only
// GetOpenFileName: it is Unicode, DPI aware, modal to its owner window and
// returns a filesystem path through the same vtable plumbing IShellLink already
// uses.
//
// IFileDialog and IFileOpenDialog share the first 27 slots; everything up to
// SetFilter comes from IFileDialog.

const (
	fosPickFolders     = 0x00000020
	fosForceFileSystem = 0x00000040
	fosPathMustExist   = 0x00000800
	fosFileMustExist   = 0x00001000

	sigdnFileSysPath = uint32(0x80058000)

	// HRESULT_FROM_WIN32(ERROR_CANCELLED): the user closed the dialog.
	errorCancelled = uint32(0x800704C7)
)

var (
	clsidFileOpenDialog = &guid{0xDC1C5A9C, 0xE88A, 0x4DDE, [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	iidFileOpenDialog   = &guid{0xD57C7288, 0xD4AD, 0x4768, [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}
)

// PickFolder shows the shell's folder chooser. A cancelled dialog returns an
// empty path and no error.
func PickFolder(owner uintptr, title string) (string, error) {
	return pick(owner, title, true)
}

// PickFile shows the shell's file chooser. A cancelled dialog returns an empty
// path and no error.
func PickFile(owner uintptr, title string) (string, error) {
	return pick(owner, title, false)
}

func pick(owner uintptr, title string, folder bool) (string, error) {
	dialog, err := coCreateInstance(clsidFileOpenDialog, iidFileOpenDialog)
	if err != nil {
		return "", err
	}
	defer dialog.release()

	var options uint32
	if _, err := dialog.call(10, uintptr(unsafe.Pointer(&options))); err != nil {
		return "", err
	}
	options |= fosForceFileSystem
	if folder {
		options |= fosPickFolders | fosPathMustExist
	} else {
		options |= fosFileMustExist
	}
	if _, err := dialog.call(9, uintptr(options)); err != nil {
		return "", err
	}

	if title != "" {
		ptr, err := windows.UTF16PtrFromString(title)
		if err != nil {
			return "", err
		}
		if _, err := dialog.call(17, uintptr(unsafe.Pointer(ptr))); err != nil {
			return "", err
		}
	}

	if r, err := dialog.call(3, owner); err != nil { // Show
		if uint32(r) == errorCancelled {
			return "", nil
		}
		return "", err
	}

	var item unsafe.Pointer
	if _, err := dialog.call(20, uintptr(unsafe.Pointer(&item))); err != nil { // GetResult
		return "", err
	}
	shellItem := comPtr{p: item}
	defer shellItem.release()

	var name *uint16
	if _, err := shellItem.call(5, uintptr(sigdnFileSysPath), uintptr(unsafe.Pointer(&name))); err != nil {
		return "", err
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(name)))

	return windows.UTF16PtrToString(name), nil
}
