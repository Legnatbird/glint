package win

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// shellExecuteInfo mirrors SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         windows.Handle
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     windows.Handle
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    windows.Handle
	dwHotKey     uint32
	hIcon        windows.Handle
	hProcess     windows.Handle
}

// ShellExecute opens a file, shortcut or executable the way the shell would.
//
// It is used for every Win32 launch because the shell resolves advertised MSI
// shortcuts, App Paths and file associations that a raw CreateProcess call
// would miss. Must be called from a COM-initialized thread.
func ShellExecute(target, args, workdir string, runAs bool) error {
	if target == "" {
		return errors.New("win: empty launch target")
	}

	file, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}

	var verb, params, dir *uint16
	if runAs {
		if verb, err = windows.UTF16PtrFromString("runas"); err != nil {
			return err
		}
	}
	if args != "" {
		if params, err = windows.UTF16PtrFromString(args); err != nil {
			return err
		}
	}
	if workdir != "" {
		if dir, err = windows.UTF16PtrFromString(workdir); err != nil {
			return err
		}
	}

	info := shellExecuteInfo{
		cbSize:       uint32(unsafe.Sizeof(shellExecuteInfo{})),
		fMask:        seeMaskNoCloseProcess | seeMaskNoAsync | seeMaskFlagNoUI,
		lpVerb:       verb,
		lpFile:       file,
		lpParameters: params,
		lpDirectory:  dir,
		nShow:        swShowNormal,
	}

	ok, _, callErr := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		if callErr == syscall.Errno(1223) { // ERROR_CANCELLED, the user said no to UAC
			return errors.New("win: launch cancelled at the elevation prompt")
		}
		return fmt.Errorf("win: ShellExecuteExW(%s): %w", target, callErr)
	}

	if info.hProcess != 0 {
		procCloseHandle.Call(uintptr(info.hProcess))
	}
	return nil
}

// RevealInExplorer opens the folder that contains path, with the file selected.
func RevealInExplorer(path string) error {
	if path == "" {
		return errors.New("win: empty path")
	}
	explorer, err := windows.UTF16PtrFromString("explorer.exe")
	if err != nil {
		return err
	}
	args, err := windows.UTF16PtrFromString(`/select,"` + path + `"`)
	if err != nil {
		return err
	}
	info := shellExecuteInfo{
		cbSize:       uint32(unsafe.Sizeof(shellExecuteInfo{})),
		fMask:        seeMaskNoCloseProcess | seeMaskNoAsync | seeMaskFlagNoUI,
		lpFile:       explorer,
		lpParameters: args,
		nShow:        swShowNormal,
	}
	ok, _, callErr := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return fmt.Errorf("win: reveal %s: %w", path, callErr)
	}
	if info.hProcess != 0 {
		procCloseHandle.Call(uintptr(info.hProcess))
	}
	return nil
}
