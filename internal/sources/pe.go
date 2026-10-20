package sources

import (
	"encoding/binary"
	"os"
)

// Windows PE subsystem values.
const (
	subsystemWindowsGUI = 2
	subsystemWindowsCUI = 3
)

// isGUIExecutable reports whether path is a Windows GUI binary, i.e. one that
// can show a window on its own.
//
// This is how the launcher tells "Chrome" apart from "clusterdb": the PE header
// declares the subsystem, so the filter is a fact about the binary instead of a
// guess about its name. Console binaries are dropped from PATH by default
// because they are 60% of the entries and bury the applications people launch.
func isGUIExecutable(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	// DOS header: "MZ" plus a pointer to the PE header at offset 0x3C.
	var dos [0x40]byte
	if _, err := file.ReadAt(dos[:], 0); err != nil {
		return false
	}
	if dos[0] != 'M' || dos[1] != 'Z' {
		return false
	}

	peOffset := int64(binary.LittleEndian.Uint32(dos[0x3C:]))
	if peOffset <= 0 || peOffset > 1<<20 {
		return false
	}

	// PE signature (4 bytes) + COFF header (20 bytes) + optional header, whose
	// Subsystem field sits at a fixed offset 68 in both PE32 and PE32+.
	header := make([]byte, 24+70)
	if _, err := file.ReadAt(header, peOffset); err != nil {
		return false
	}
	if header[0] != 'P' || header[1] != 'E' || header[2] != 0 || header[3] != 0 {
		return false
	}

	const subsystemOffset = 24 + 68
	subsystem := binary.LittleEndian.Uint16(header[subsystemOffset:])
	return subsystem == subsystemWindowsGUI
}
