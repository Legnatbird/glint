package ui

import "testing"

func TestTextFieldTypesAtTheCaret(t *testing.T) {
	f := newField("")
	f.insert("term")
	f.left(false)
	f.insert("X")
	if got := f.String(); got != "terXm" {
		t.Fatalf("text = %q, want terXm", got)
	}
	if f.caret != 4 {
		t.Fatalf("caret = %d, want 4", f.caret)
	}
}

// Select all then type replaces everything: that is the gesture the user asked
// for instead of pressing Escape.
func TestTextFieldSelectAllReplaces(t *testing.T) {
	f := newField("chrome")
	f.selectAll()
	if !f.hasSelection() || f.selected() != "chrome" {
		t.Fatalf("selection = %q", f.selected())
	}
	f.insert("steam")
	if got := f.String(); got != "steam" {
		t.Fatalf("text = %q, want steam", got)
	}
	if f.hasSelection() {
		t.Fatal("typing over a selection left it selected")
	}
}

func TestTextFieldSelectAllThenBackspace(t *testing.T) {
	f := newField("terminal")
	f.selectAll()
	if !f.backspace() {
		t.Fatal("backspace did nothing with everything selected")
	}
	if !f.Empty() {
		t.Fatalf("text = %q, want empty", f.String())
	}
	if f.caret != 0 || f.anchor != 0 {
		t.Fatalf("caret/anchor = %d/%d, want 0/0", f.caret, f.anchor)
	}
}

func TestTextFieldShiftArrowsSelect(t *testing.T) {
	f := newField("abc")
	// Caret is at the end after newField.
	f.left(false) // at 2
	f.left(true)  // selection 1..2
	if got := f.selected(); got != "b" {
		t.Fatalf("selection = %q, want b", got)
	}
	f.insert("B")
	if got := f.String(); got != "aBc" {
		t.Fatalf("text = %q, want aBc", got)
	}
}

func TestTextFieldArrowCollapsesSelection(t *testing.T) {
	f := newField("abcdef")
	f.home(false)
	f.right(true)
	f.right(true) // selection 0..2
	f.left(false)
	if f.caret != 0 || f.hasSelection() {
		t.Fatalf("caret = %d, selection = %v, want collapsed at the start", f.caret, f.hasSelection())
	}

	f.right(true)
	f.right(true)
	f.right(false)
	if f.caret != 2 || f.hasSelection() {
		t.Fatalf("caret = %d, want collapsed at the end of the old selection", f.caret)
	}
}

func TestTextFieldBackspaceWord(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"steam", ""},
		{"arc raiders", "arc "},
		{"C:\\Program Files\\steam", "C:\\Program Files\\"},
		{"trailing   ", ""},
		{"ñandú", ""},
	} {
		f := newField(tc.in)
		f.backspaceWord()
		if got := f.String(); got != tc.want {
			t.Errorf("backspaceWord(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTextFieldDeleteWordForward(t *testing.T) {
	f := newField("arc raiders")
	f.home(false)
	f.deleteWordForward()
	if got := f.String(); got != " raiders" {
		t.Fatalf("text = %q, want the first word removed", got)
	}
}

func TestTextFieldHomeEnd(t *testing.T) {
	f := newField("search")
	f.home(false)
	if f.caret != 0 {
		t.Fatalf("caret = %d, want 0", f.caret)
	}
	f.end(false)
	if f.caret != 6 {
		t.Fatalf("caret = %d, want 6", f.caret)
	}
}

func TestTextFieldWordMovement(t *testing.T) {
	// "arc raiders two": a=0, space=3, raiders=4..10, space=11, two=12..14.
	f := newField("arc raiders two")
	f.wordLeft(false)
	if f.caret != 12 {
		t.Fatalf("caret = %d, want 12 (start of \"two\")", f.caret)
	}
	f.wordLeft(false)
	if f.caret != 4 {
		t.Fatalf("caret = %d, want 4 (start of \"raiders\")", f.caret)
	}
	f.wordRight(false)
	if f.caret != 11 {
		t.Fatalf("caret = %d, want 11 (end of \"raiders\")", f.caret)
	}
}

func TestTextFieldDeleteForwardLeavesTextAloneAtEnd(t *testing.T) {
	f := newField("abc")
	if f.deleteForward() {
		t.Fatal("deleteForward at the end reported a change")
	}
	if f.String() != "abc" {
		t.Fatalf("text = %q", f.String())
	}
}
