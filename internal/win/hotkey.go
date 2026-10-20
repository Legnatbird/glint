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
