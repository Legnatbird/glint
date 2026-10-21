package ui

// textField is one line of user text with a caret and a selection.
//
// It is pure state: no font, no window, no Win32. That is deliberate — the
// editing rules are the part worth testing, and a launcher's search box has to
// behave like every other text box the user types in, which means a caret that
// moves, a selection that replaces what it covers, and Home/End.
type textField struct {
	text   []rune
	caret  int // insertion point, 0..len(text)
	anchor int // the other end of the selection; equal to caret when empty
}

func newField(text string) textField {
	runes := []rune(text)
	return textField{text: runes, caret: len(runes), anchor: len(runes)}
}

func (f *textField) String() string { return string(f.text) }

func (f *textField) Len() int { return len(f.text) }

func (f *textField) Empty() bool { return len(f.text) == 0 }

// setText replaces the content and puts the caret at the end, which is what
// programming a value into the field (a saved name, a pasted path) means.
func (f *textField) setText(s string) {
	f.text = []rune(s)
	f.caret = len(f.text)
	f.anchor = f.caret
}

func (f *textField) clear() { f.setText("") }

// selectionStart and selectionEnd return the selection in text order.
func (f *textField) selectionStart() int {
	if f.caret < f.anchor {
		return f.caret
	}
	return f.anchor
}

func (f *textField) selectionEnd() int {
	if f.caret < f.anchor {
		return f.anchor
	}
	return f.caret
}

func (f *textField) hasSelection() bool { return f.caret != f.anchor }

func (f *textField) selected() string {
	return string(f.text[f.selectionStart():f.selectionEnd()])
}

// selectAll covers the whole line, which is the gesture the user reaches for
// before replacing or deleting a query.
func (f *textField) selectAll() {
	f.anchor = 0
	f.caret = len(f.text)
}

// deleteSelection removes what is selected and reports whether it did anything.
func (f *textField) deleteSelection() bool {
	if !f.hasSelection() {
		return false
	}
	start, end := f.selectionStart(), f.selectionEnd()
	f.text = append(f.text[:start], f.text[end:]...)
	f.caret, f.anchor = start, start
	return true
}

// insert replaces the selection with s.
func (f *textField) insert(s string) {
	if s == "" {
		return
	}
	f.deleteSelection()
	runes := []rune(s)
	out := make([]rune, 0, len(f.text)+len(runes))
	out = append(out, f.text[:f.caret]...)
	out = append(out, runes...)
	out = append(out, f.text[f.caret:]...)
	f.text = out
	f.caret += len(runes)
	f.anchor = f.caret
}

func (f *textField) insertRune(r rune) { f.insert(string(r)) }

// backspace deletes the selection, or the rune before the caret.
func (f *textField) backspace() bool {
	if f.deleteSelection() {
		return true
	}
	if f.caret == 0 {
		return false
	}
	f.text = append(f.text[:f.caret-1], f.text[f.caret:]...)
	f.caret--
	f.anchor = f.caret
	return true
}

// deleteForward deletes the selection, or the rune after the caret.
func (f *textField) deleteForward() bool {
	if f.deleteSelection() {
		return true
	}
	if f.caret >= len(f.text) {
		return false
	}
	f.text = append(f.text[:f.caret], f.text[f.caret+1:]...)
	f.anchor = f.caret
	return true
}

// backspaceWord deletes the selection, or the run before the caret together with
// any whitespace it trails. A run ends at a space or at a change of kind, so
// "C:\Program Files\steam" loses "steam" and then the whole folder name in two
// presses.
func (f *textField) backspaceWord() bool {
	if f.deleteSelection() {
		return true
	}
	end := f.caret
	if end == 0 {
		return false
	}
	start := end
	for start > 0 && isSpace(f.text[start-1]) {
		start--
	}
	if start == 0 {
		f.text = f.text[end:]
		f.caret, f.anchor = 0, 0
		return true
	}
	kind := runeKind(f.text[start-1])
	for start > 0 && !isSpace(f.text[start-1]) && runeKind(f.text[start-1]) == kind {
		start--
	}
	f.text = append(f.text[:start], f.text[end:]...)
	f.caret, f.anchor = start, start
	return true
}

// deleteWordForward is backspaceWord in the other direction (Ctrl+Delete).
func (f *textField) deleteWordForward() bool {
	if f.deleteSelection() {
		return true
	}
	if f.caret >= len(f.text) {
		return false
	}
	end := f.caret
	for end < len(f.text) && isSpace(f.text[end]) {
		end++
	}
	if end == len(f.text) {
		f.text = f.text[:f.caret]
		f.anchor = f.caret
		return true
	}
	kind := runeKind(f.text[end])
	for end < len(f.text) && !isSpace(f.text[end]) && runeKind(f.text[end]) == kind {
		end++
	}
	f.text = append(f.text[:f.caret], f.text[end:]...)
	f.anchor = f.caret
	return true
}

// left and right move the caret; extend keeps the anchor so Shift+arrow grows a
// selection.
func (f *textField) left(extend bool) {
	if !extend && f.hasSelection() {
		// Collapsing goes to the start of the selection, like every text box.
		f.caret = f.selectionStart()
		f.anchor = f.caret
		return
	}
	if f.caret > 0 {
		f.caret--
	}
	if !extend {
		f.anchor = f.caret
	}
}

func (f *textField) right(extend bool) {
	if !extend && f.hasSelection() {
		f.caret = f.selectionEnd()
		f.anchor = f.caret
		return
	}
	if f.caret < len(f.text) {
		f.caret++
	}
	if !extend {
		f.anchor = f.caret
	}
}

// wordLeft and wordRight move a whole run at a time.
func (f *textField) wordLeft(extend bool) {
	if !extend && f.hasSelection() {
		f.caret = f.selectionStart()
		f.anchor = f.caret
		return
	}
	for f.caret > 0 && isSpace(f.text[f.caret-1]) {
		f.caret--
	}
	if f.caret > 0 {
		kind := runeKind(f.text[f.caret-1])
		for f.caret > 0 && !isSpace(f.text[f.caret-1]) && runeKind(f.text[f.caret-1]) == kind {
			f.caret--
		}
	}
	if !extend {
		f.anchor = f.caret
	}
}

func (f *textField) wordRight(extend bool) {
	if !extend && f.hasSelection() {
		f.caret = f.selectionEnd()
		f.anchor = f.caret
		return
	}
	for f.caret < len(f.text) && isSpace(f.text[f.caret]) {
		f.caret++
	}
	if f.caret < len(f.text) {
		kind := runeKind(f.text[f.caret])
		for f.caret < len(f.text) && !isSpace(f.text[f.caret]) && runeKind(f.text[f.caret]) == kind {
			f.caret++
		}
	}
	if !extend {
		f.anchor = f.caret
	}
}

func (f *textField) home(extend bool) {
	f.caret = 0
	if !extend {
		f.anchor = 0
	}
}

func (f *textField) end(extend bool) {
	f.caret = len(f.text)
	if !extend {
		f.anchor = f.caret
	}
}

func isSpace(r rune) bool { return r == ' ' || r == '\t' }

// runeKind groups runes so a Ctrl+Backspace stops at a separator instead of
// eating a whole path. Letters in any alphabet count as one kind, so a name in
// Spanish or Japanese is one word.
type rk int

const (
	rkSpace rk = iota
	rkLetter
	rkDigit
	rkOther
)

func runeKind(r rune) rk {
	switch {
	case isSpace(r):
		return rkSpace
	case r >= '0' && r <= '9':
		return rkDigit
	case r == '_' || r == '-' || r == '.':
		return rkLetter // punctuation that belongs to names such as steam_app
	case r < 128:
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return rkLetter
		}
		return rkOther
	default:
		// Anything non-ASCII is treated as a letter: it is how every non-Latin
		// name the user might search for stays one word.
		return rkLetter
	}
}
