package win

import "testing"

func TestAppIconIsDrawn(t *testing.T) {
	icon, err := AppIcon(32)
	if err != nil || icon.h == 0 {
		t.Fatalf("AppIcon: %v", err)
	}

	buf := make([]byte, 32*32*4)
	drawIconPixels(buf, 32)

	var opaque, white int
	for i := 0; i < len(buf); i += 4 {
		if buf[i+3] > 200 {
			opaque++
		}
		if buf[i] > 240 && buf[i+1] > 240 && buf[i+2] > 240 && buf[i+3] > 200 {
			white++
		}
	}
	t.Logf("opaque=%d white=%d of %d", opaque, white, 32*32)
	if opaque < 300 {
		t.Fatalf("icon is mostly empty: %d opaque pixels", opaque)
	}
	if white < 20 {
		t.Fatalf("chevron is missing: %d white pixels", white)
	}
}
