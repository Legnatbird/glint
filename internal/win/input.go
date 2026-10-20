package win

// Keyboard constants and state.
var procGetKeyState = user32.NewProc("GetKeyState")

const (
	vkBack    = 0x08
	vkTab     = 0x09
	vkReturn  = 0x0D
	vkEscape  = 0x1B
	vkPrior   = 0x21
	vkNext    = 0x22
	vkEnd     = 0x23
	vkHome    = 0x24
	vkLeft    = 0x25
	vkUp      = 0x26
	vkRight   = 0x27
	vkDown    = 0x28
	vkDelete  = 0x2E
	vkShift   = 0x10
	vkControl = 0x11
	vkMenu    = 0x12
)

// Key constants exported for the UI layer, which must not know Win32 values.
const (
	KeyReturn = vkReturn
	KeyEscape = vkEscape
	KeyUp     = vkUp
	KeyDown   = vkDown
	KeyTab    = vkTab
	KeyPrior  = vkPrior
	KeyF5     = 0x74
	KeyNext   = vkNext
)
