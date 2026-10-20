package win

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Packaged application activation.
//
// A UWP app cannot be started with ShellExecute or CreateProcess: it is
// activated by AppUserModelID through IApplicationActivationManager.

var (
	clsidAppActivationManager = &guid{0x45BA127D, 0x10A8, 0x46EA, [8]byte{0x8A, 0xB7, 0x56, 0xEA, 0x90, 0x78, 0x94, 0x3C}}
	iidAppActivationManager   = &guid{0x2E941141, 0x7F97, 0x4756, [8]byte{0xBA, 0x1D, 0x9D, 0xEC, 0xDE, 0x89, 0x4A, 0x3D}}
)

// ActivateApp starts a packaged application by its AppUserModelID.
func ActivateApp(aumid string) error {
	manager, err := coCreateInstance(clsidAppActivationManager, iidAppActivationManager)
	if err != nil {
		return err
	}
	defer manager.release()

	id, err := windows.UTF16PtrFromString(aumid)
	if err != nil {
		return err
	}

	var pid uint32
	// IApplicationActivationManager::ActivateApplication is vtable slot 3.
	if _, err := manager.call(3, uintptr(unsafe.Pointer(id)), 0, 0, uintptr(unsafe.Pointer(&pid))); err != nil {
		return err
	}
	return nil
}
