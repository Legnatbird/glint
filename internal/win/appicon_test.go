package win

import "testing"

// The pixels themselves are covered by internal/art; here the contract is that
// the buffer becomes a live HICON and that a bad size is refused.
func TestAppIconIsDrawn(t *testing.T) {
	icon, err := AppIcon(32)
	if err != nil || icon.h == 0 {
		t.Fatalf("AppIcon(32): %v", err)
	}
	icon.Destroy()

	if _, err := AppIcon(0); err == nil {
		t.Fatal("AppIcon(0) returned no error")
	}
}
