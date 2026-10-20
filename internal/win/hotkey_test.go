package win

import "testing"

func TestHotkeyFor(t *testing.T) {
	tests := []struct {
		name             string
		ctrl, alt, shift bool
		vk               uint32
		want             string
		wantOK           bool
	}{
		{name: "ctrl alt letter", ctrl: true, alt: true, vk: 'G', want: "ctrl+alt+g", wantOK: true},
		{name: "shift function key", shift: true, vk: 0x74, want: "shift+f5", wantOK: true},
		{name: "ctrl space", ctrl: true, vk: 0x20, want: "ctrl+space", wantOK: true},
		{name: "alt digit", alt: true, vk: '7', want: "alt+7", wantOK: true},
		{name: "all three modifiers", ctrl: true, alt: true, shift: true, vk: 'Q', want: "ctrl+alt+shift+q", wantOK: true},
		{name: "no modifier", vk: 'G'},
		{name: "modifier alone", ctrl: true, vk: vkControl},
		{name: "right shift alone", shift: true, vk: vkRShift},
		{name: "unnamed key", ctrl: true, vk: 0x07},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := HotkeyFor(tt.ctrl, tt.alt, tt.shift, tt.vk)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if got.Text != tt.want {
				t.Fatalf("text = %q, want %q", got.Text, tt.want)
			}
			if got.VK != tt.vk {
				t.Fatalf("vk = %d, want %d", got.VK, tt.vk)
			}
		})
	}
}

// The text a captured combination produces must be readable by the parser: the
// config file stores it verbatim and the next launch depends on it.
func TestHotkeyForRoundTripsThroughParse(t *testing.T) {
	cases := []struct {
		ctrl, alt, shift bool
		vk               uint32
	}{
		{ctrl: true, alt: true, vk: 'G'},
		{shift: true, vk: 0x74},
		{ctrl: true, vk: 0x20},
		{ctrl: true, alt: true, shift: true, vk: 'Q'},
	}

	for _, c := range cases {
		captured, ok := HotkeyFor(c.ctrl, c.alt, c.shift, c.vk)
		if !ok {
			t.Fatalf("HotkeyFor(%v,%v,%v,%d) not ok", c.ctrl, c.alt, c.shift, c.vk)
		}
		parsed, err := ParseHotkey(captured.Text)
		if err != nil {
			t.Fatalf("ParseHotkey(%q): %v", captured.Text, err)
		}
		if parsed.VK != captured.VK || parsed.Mods != captured.Mods {
			t.Fatalf("round trip of %q = vk %d mods %d, want vk %d mods %d",
				captured.Text, parsed.VK, parsed.Mods, captured.VK, captured.Mods)
		}
	}
}

func TestIsModifierKey(t *testing.T) {
	for _, vk := range []uint32{vkShift, vkControl, vkMenu, vkLShift, vkRShift, vkLControl, vkRControl, vkLMenu, vkRMenu, vkLWin, vkRWin} {
		if !IsModifierKey(vk) {
			t.Errorf("IsModifierKey(0x%X) = false, want true", vk)
		}
	}
	for _, vk := range []uint32{'A', 0x20, 0x74} {
		if IsModifierKey(vk) {
			t.Errorf("IsModifierKey(0x%X) = true, want false", vk)
		}
	}
}
