package sources

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFriendlyName(t *testing.T) {
	cases := map[string]string{
		`C:\Tools\chrome.exe`:           "Chrome",
		`C:\Tools\VSCode.exe`:           "VSCode",
		`C:\Tools\7-Zip.exe`:            "7-Zip",
		`C:\Tools\my_app.exe`:           "My app",
		`D:\vscode\bin\code-tunnel.exe`: "Code-tunnel",
	}
	for path, want := range cases {
		if got := friendlyName(path); got != want {
			t.Errorf("friendlyName(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestShortcutName(t *testing.T) {
	got := shortcutName(`C:\ProgramData\Microsoft\Windows\Start Menu\Programs\Visual Studio Code.lnk`)
	if want := "Visual Studio Code"; got != want {
		t.Fatalf("shortcutName = %q, want %q", got, want)
	}
}

func TestUnderDir(t *testing.T) {
	cases := []struct {
		path, dir string
		want      bool
	}{
		{`C:\Foo\Bar`, `C:\Foo`, true},
		{`C:\Foo`, `C:\Foo`, true},
		{`C:\Foobar`, `C:\Foo`, false}, // prefix must end on a separator
		{`C:\Foo\Bar`, ``, false},
		{`c:\foo\bar`, `C:\FOO`, true}, // Windows paths are case-insensitive
	}
	for _, tc := range cases {
		if got := underDir(tc.path, tc.dir); got != tc.want {
			t.Errorf("underDir(%q, %q) = %v, want %v", tc.path, tc.dir, got, tc.want)
		}
	}
}

func TestScanCustomFoldersFindsExecutablesAndSkipsNoise(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"myapp.exe", "unins000.exe", "readme.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(sub, "tool.exe"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	rep := &Report{seen: map[string]int{}}
	entries := scanCustomFolders(rep, []string{dir}, nil)

	got := map[string]bool{}
	for _, e := range entries {
		got[e.Name] = true
	}
	if !got["Myapp"] || !got["Tool"] {
		t.Fatalf("executables not found, got %v", got)
	}
	if got["Unins000"] {
		t.Fatal("uninstallers must never become entries")
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2: %v", len(entries), got)
	}
}

func TestScanCustomFoldersHonorsExcludes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.exe"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	rep := &Report{seen: map[string]int{}}
	if entries := scanCustomFolders(rep, []string{dir}, []string{dir}); len(entries) != 0 {
		t.Fatalf("excluded folder produced %d entries", len(entries))
	}
}

func TestScanCustomFolderMissingIsAWarning(t *testing.T) {
	rep := &Report{seen: map[string]int{}}
	scanCustomFolders(rep, []string{filepath.Join(t.TempDir(), "nope")}, nil)

	if len(rep.Warnings) != 1 {
		t.Fatalf("got %d warnings, want 1", len(rep.Warnings))
	}
}
