package win

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestSettingsWindowCreation(t *testing.T) {
	w, err := NewSettingsWindow("probe", 400, 300)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Logf("hwnd=%d", w.hwnd)

	class, _ := windows.UTF16PtrFromString(settingsClassName)
	found, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(class)), 0)
	t.Logf("FindWindow=%d", found)
	if found == 0 {
		t.Fatal("class not found after creation")
	}
}
