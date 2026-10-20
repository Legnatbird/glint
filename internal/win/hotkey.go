package win

import (
	"errors"
	"fmt"
	"strings"
)

// Hotkey is a parsed global hotkey combination.
type Hotkey struct {
	Mods uint32
	VK   uint32
	Text string
}

var keyCodes = func() map[string]uint32 {
	m := map[string]uint32{
		"space": 0x20,
		"tab":   0x09,
		"enter": 0x0D,
		"esc":   0x1B,
		"`":     0xC0,
	}
	for c := 'a'; c <= 'z'; c++ {
		m[string(c)] = uint32(c - 'a' + 0x41)
	}
	for c := '0'; c <= '9'; c++ {
		m[string(c)] = uint32(c - '0' + 0x30)
	}
	for i := 1; i <= 24; i++ {
		m[fmt.Sprintf("f%d", i)] = uint32(0x6F + i)
	}
	return m
}()

// keyNames is the inverse of keyCodes, used to render a captured key press.
var keyNames = func() map[uint32]string {
	m := make(map[uint32]string, len(keyCodes))
	for name, vk := range keyCodes {
		m[vk] = name
	}
	return m
}()

// Modifier virtual-key codes. The left/right variants are listed because
// GetKeyState reports those while a combination is being typed.
const (
	vkLShift   = 0xA0
	vkRShift   = 0xA1
	vkLControl = 0xA2
	vkRControl = 0xA3
	vkLMenu    = 0xA4
	vkRMenu    = 0xA5
	vkLWin     = 0x5B
	vkRWin     = 0x5C
)

// IsModifierKey reports whether vk is a key that cannot end a combination.
func IsModifierKey(vk uint32) bool {
	switch vk {
	case vkShift, vkControl, vkMenu, vkLShift, vkRShift, vkLControl, vkRControl,
		vkLMenu, vkRMenu, vkLWin, vkRWin:
		return true
	}
	return false
}

// HotkeyFor describes one key press as a hotkey. ok is false while the press is
// still incomplete: a modifier on its own, an unnamed key, or a bare key with no
// modifier held.
func HotkeyFor(ctrl, alt, shift bool, vk uint32) (Hotkey, bool) {
	name, named := keyNames[vk]
	if !named || IsModifierKey(vk) {
		return Hotkey{}, false
	}

	var mods uint32
	parts := make([]string, 0, 3)
	if ctrl {
		mods |= modControl
		parts = append(parts, "ctrl")
	}
	if alt {
		mods |= modAlt
		parts = append(parts, "alt")
	}
	if shift {
		mods |= modShift
		parts = append(parts, "shift")
	}
	if mods == 0 {
		return Hotkey{}, false
	}
	parts = append(parts, name)
	return Hotkey{Mods: mods, VK: vk, Text: strings.Join(parts, "+")}, true
}

// ParseHotkey turns "ctrl+alt+space" into modifiers and a virtual-key code.
func ParseHotkey(s string) (Hotkey, error) {
	var mods, vk uint32
	parts := strings.Split(strings.ToLower(strings.TrimSpace(s)), "+")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch part {
		case "":
			continue
		case "ctrl", "control":
			mods |= modControl
		case "alt":
			mods |= modAlt
		case "shift":
			mods |= modShift
		case "win", "super", "meta":
			mods |= modWin
		default:
			if vk != 0 {
				return Hotkey{}, fmt.Errorf("win: hotkey %q has more than one key", s)
			}
			code, ok := keyCodes[part]
			if !ok {
				return Hotkey{}, fmt.Errorf("win: unknown key %q", part)
			}
			vk = code
		}
	}
	if vk == 0 {
		return Hotkey{}, errors.New("win: hotkey needs a non-modifier key")
	}
	if mods == 0 {
		return Hotkey{}, errors.New("win: hotkey needs at least one modifier")
	}
	return Hotkey{Mods: mods, VK: vk, Text: s}, nil
}

// RegisterHotkey installs a system-wide hotkey that posts WM_HOTKEY to the
// window. Registration fails when another process already owns the combo.
func (w *Window) RegisterHotkey(hk Hotkey) error {
	ok, _, callErr := procRegisterHotKey.Call(w.hwnd, 1, uintptr(hk.Mods), uintptr(hk.VK))
	if ok == 0 {
		return fmt.Errorf("win: RegisterHotKey(%s): %w", hk.Text, callErr)
	}
	return nil
}

// UnregisterHotkey releases the hotkey installed by RegisterHotkey.
func (w *Window) UnregisterHotkey() error {
	ok, _, callErr := procUnregisterHotKey.Call(w.hwnd, 1)
	if ok == 0 {
		return fmt.Errorf("win: UnregisterHotKey: %w", callErr)
	}
	return nil
}
