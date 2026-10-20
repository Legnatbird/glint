package win

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// COM plumbing.
//
// The launcher needs a single shell object for now: ShellLink, to resolve .lnk
// files into their real executable. Calling vtables by hand keeps the
// dependency list at one module (golang.org/x/sys) instead of pulling a COM
// binding library that would also have to be maintained.
//
// COM pointers are held as unsafe.Pointer, never uintptr: converting an integer
// back into a pointer is exactly what lets the GC lose track of an object, and
// `go vet` rightly refuses it.

const (
	stgmRead    = 0
	slgpRawPath = 4
)

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	clsidShellLink = &guid{0x00021401, 0, 0, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidShellLinkW  = &guid{0x000214F9, 0, 0, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidPersistFile = &guid{0x0000010B, 0, 0, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
)

// CoInitialize prepares the calling thread for COM. Every successful call must
// be paired with the returned release function, on the same OS thread.
func CoInitialize() func() {
	if r, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded); int32(r) >= 0 {
		return func() { procCoUninitialize.Call() }
	}
	// Already initialized with another apartment model: nothing to undo.
	return func() {}
}

type comPtr struct{ p unsafe.Pointer }

// call invokes vtable slot idx. The interface pointer is always the first
// argument in COM, so it is prepended here.
func (c comPtr) call(idx int, args ...uintptr) (uintptr, error) {
	if c.p == nil {
		return 0, errors.New("win: null COM pointer")
	}

	vtbl := *(*unsafe.Pointer)(c.p)
	fn := *(*uintptr)(unsafe.Add(vtbl, uintptr(idx)*unsafe.Sizeof(uintptr(0))))

	callArgs := make([]uintptr, 0, len(args)+1)
	callArgs = append(callArgs, uintptr(c.p))
	callArgs = append(callArgs, args...)

	r, _, _ := syscall.SyscallN(fn, callArgs...)
	// HRESULT failures have the high bit set.
	if int32(r) < 0 {
		return r, fmt.Errorf("win: COM call %d failed: 0x%08X", idx, uint32(r))
	}
	return r, nil
}

// release drops one reference (IUnknown slot 2).
func (c comPtr) release() {
	if c.p != nil {
		_, _ = c.call(2)
	}
}

// queryInterface follows IUnknown slot 0.
func (c comPtr) queryInterface(iid *guid) (comPtr, error) {
	var out unsafe.Pointer
	if _, err := c.call(0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out))); err != nil {
		return comPtr{}, err
	}
	return comPtr{p: out}, nil
}

func coCreateInstance(clsid, iid *guid) (comPtr, error) {
	var out unsafe.Pointer
	r, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(clsid)),
		0,
		clsctxInprocServer,
		uintptr(unsafe.Pointer(iid)),
		uintptr(unsafe.Pointer(&out)),
	)
	if int32(r) < 0 {
		return comPtr{}, fmt.Errorf("win: CoCreateInstance failed: 0x%08X", uint32(r))
	}
	return comPtr{p: out}, nil
}

// Shortcut is the resolved content of a .lnk file.
type Shortcut struct {
	Path    string // target executable
	Args    string // arguments baked into the shortcut
	WorkDir string // start-in directory
}

// ResolveShortcut follows a .lnk to its target using IShellLink.
//
// Advertised MSI shortcuts can resolve to an installer stub instead of the real
// executable, which is why launching always goes through the .lnk itself and
// never through this path.
func ResolveShortcut(path string) (Shortcut, error) {
	var sc Shortcut

	link, err := coCreateInstance(clsidShellLink, iidShellLinkW)
	if err != nil {
		return sc, err
	}
	defer link.release()

	persist, err := link.queryInterface(iidPersistFile)
	if err != nil {
		return sc, err
	}
	defer persist.release()

	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return sc, err
	}
	// IPersistFile::Load is vtable slot 5: after the three IUnknown slots,
	// GetClassID and IsDirty.
	if _, err := persist.call(5, uintptr(unsafe.Pointer(pathPtr)), stgmRead); err != nil {
		return sc, err
	}

	sc.Path = expandEnv(shellLinkString(link, 3, 0))    // GetPath
	sc.Args = shellLinkString(link, 10, 0)              // GetArguments
	sc.WorkDir = expandEnv(shellLinkString(link, 8, 0)) // GetWorkingDirectory
	return sc, nil
}

// expandEnv resolves the %windir%-style variables that some shortcuts store
// verbatim in their target.
func expandEnv(s string) string {
	if s == "" || !strings.Contains(s, "%") {
		return s
	}
	src, err := windows.UTF16PtrFromString(s)
	if err != nil {
		return s
	}
	buf := make([]uint16, 1024)
	n, err := windows.ExpandEnvironmentStrings(src, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || int(n) > len(buf) {
		return s
	}
	return windows.UTF16ToString(buf)
}

// shellLinkString reads one of the string properties of IShellLinkW. A failing
// or empty property is not fatal: many shortcuts only fill some of them.
func shellLinkString(link comPtr, method int, flags uintptr) string {
	buf := make([]uint16, 1024)
	args := []uintptr{uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))}
	if method == 3 {
		// GetPath also takes WIN32_FIND_DATAW* and a flag.
		args = append(args, 0, flags)
	}
	if _, err := link.call(method, args...); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf)
}
