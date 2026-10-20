package sources

import (
	"os"
	"path/filepath"
	"testing"
)

// notepad and cmd are present on every supported Windows install and are the
// two subsystems the filter has to tell apart.
func TestIsGUIExecutable(t *testing.T) {
	root := os.Getenv("SystemRoot")
	if root == "" {
		t.Skip("SystemRoot is not set")
	}

	gui := filepath.Join(root, "System32", "notepad.exe")
	console := filepath.Join(root, "System32", "cmd.exe")
	for _, path := range []string{gui, console} {
		if _, err := os.Stat(path); err != nil {
			t.Skipf("%s not available", path)
		}
	}

	if !isGUIExecutable(gui) {
		t.Errorf("notepad.exe must be detected as a GUI binary")
	}
	if isGUIExecutable(console) {
		t.Errorf("cmd.exe must be detected as a console binary")
	}
}

func TestIsGUIExecutableRejectsNonPE(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fake.exe")
	if err := os.WriteFile(path, []byte("not a portable executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	if isGUIExecutable(path) {
		t.Fatal("garbage must not be reported as a GUI binary")
	}
}

func TestIsGUIExecutableMissingFile(t *testing.T) {
	if isGUIExecutable(filepath.Join(t.TempDir(), "nope.exe")) {
		t.Fatal("a missing file must not be reported as a GUI binary")
	}
}
