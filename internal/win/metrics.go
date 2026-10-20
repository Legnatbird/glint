package win

import (
	"fmt"
	"time"
	"unsafe"
)

// Memory is a snapshot of the process working set.
type Memory struct {
	WorkingSet     uint64
	Private        uint64
	PeakWorkingSet uint64
}

// CurrentMemory reads PROCESS_MEMORY_COUNTERS_EX for the calling process.
func CurrentMemory() (Memory, error) {
	var c processMemoryCountersEx
	c.cb = uint32(unsafe.Sizeof(c))

	proc, _, _ := procGetCurrentProcess.Call()
	ok, _, callErr := procGetProcessMemoryInfo.Call(
		proc,
		uintptr(unsafe.Pointer(&c)),
		uintptr(c.cb),
	)
	if ok == 0 {
		return Memory{}, fmt.Errorf("win: GetProcessMemoryInfo: %w", callErr)
	}
	return Memory{
		WorkingSet:     uint64(c.workingSetSize),
		Private:        uint64(c.privateUsage),
		PeakWorkingSet: uint64(c.peakWorkingSetSize),
	}, nil
}

// CPUTime returns the total user+kernel time consumed so far.
func CPUTime() (time.Duration, error) {
	var creation, exit, kernel, user filetime
	proc, _, _ := procGetCurrentProcess.Call()
	ok, _, callErr := procGetProcessTimes.Call(
		proc,
		uintptr(unsafe.Pointer(&creation)),
		uintptr(unsafe.Pointer(&exit)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	)
	if ok == 0 {
		return 0, fmt.Errorf("win: GetProcessTimes: %w", callErr)
	}
	return kernel.duration() + user.duration(), nil
}

func (f filetime) duration() time.Duration {
	ticks := uint64(f.high)<<32 | uint64(f.low)
	return time.Duration(ticks) * 100 * time.Nanosecond
}

// GUIResources returns the GDI and USER object counts of the process. These
// are the numbers that grow when a UI leaks handles.
func GUIResources() (gdi, user uint32) {
	proc, _, _ := procGetCurrentProcess.Call()
	g, _, _ := procGetGuiResources.Call(proc, 0)
	u, _, _ := procGetGuiResources.Call(proc, 1)
	return uint32(g), uint32(u)
}

// HandleCount returns the number of kernel handles held by the process.
func HandleCount() uint32 {
	var n uint32
	proc, _, _ := procGetCurrentProcess.Call()
	procGetProcessHandleCnt.Call(proc, uintptr(unsafe.Pointer(&n)))
	return n
}

// EnableDPIAwareness switches the process to per-monitor v2 when available.
// Without this the shell window is bitmap-stretched on scaled monitors.
func EnableDPIAwareness() string {
	if ok, _, _ := procSetDPIAwarenessCtx.Call(dpiContextPerMonitorAwareV2); ok != 0 {
		return "per-monitor-v2"
	}
	if ok, _, _ := procSetProcessDPIAware.Call(); ok != 0 {
		return "system"
	}
	return "none"
}
