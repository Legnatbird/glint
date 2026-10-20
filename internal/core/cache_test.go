package core_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"applauncher/internal/core"
)

func writeFile(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o600)
}

func TestCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index-cache.json")

	want := core.Cache{
		Built: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC),
		Entries: []core.Entry{
			{Key: "lnk:chrome", Name: "Google Chrome", Source: core.SourceStartMenu},
			{Key: "uwp:terminal", Name: "Windows Terminal", AUMID: "Microsoft.WindowsTerminal_8wekyb3d8bbwe", Source: core.SourceStartMenu},
		},
	}
	if err := want.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := core.LoadCache(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !got.Valid() {
		t.Fatal("a freshly written cache must be valid")
	}
	if len(got.Entries) != 2 || got.Entries[1].AUMID != want.Entries[1].AUMID {
		t.Fatalf("entries = %+v", got.Entries)
	}
	if !got.Built.Equal(want.Built) {
		t.Fatalf("built = %v, want %v", got.Built, want.Built)
	}
}

func TestCacheMissingFileIsNotAnError(t *testing.T) {
	got, err := core.LoadCache(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("a missing cache is a cold start, not an error: %v", err)
	}
	if got.Valid() {
		t.Fatal("an empty cache must not claim to be valid")
	}
}

func TestCacheSchemaMismatchIsInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index-cache.json")
	body := `{"schema":0,"built":"2026-10-20T12:00:00Z","entries":[]}`
	if err := writeFile(path, body); err != nil {
		t.Fatal(err)
	}

	got, err := core.LoadCache(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Valid() {
		t.Fatal("a cache from an older schema must be rebuilt")
	}
}
