package core_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"glint/internal/core"
)

func TestHistoryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	now := time.Now().Truncate(time.Second)

	h := core.NewHistory(nil, 100)
	h.Touch("lnk:chrome", now.Add(-time.Hour))
	h.Touch("lnk:chrome", now)
	h.Touch("lnk:code", now)

	if err := h.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := core.LoadHistory(path, 100)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	wantChrome := h.Score("lnk:chrome", now)
	if got := loaded.Score("lnk:chrome", now); got != wantChrome {
		t.Fatalf("chrome score after reload = %f, want %f", got, wantChrome)
	}
	if got := loaded.Len(); got != 2 {
		t.Fatalf("reloaded %d entries, want 2", got)
	}
}

func TestLoadHistoryMissingFileIsEmpty(t *testing.T) {
	loaded, err := core.LoadHistory(filepath.Join(t.TempDir(), "nope.json"), 100)
	if err != nil {
		t.Fatalf("a missing history is a first run: %v", err)
	}
	if loaded.Len() != 0 {
		t.Fatalf("expected an empty history, got %d entries", loaded.Len())
	}
}

func TestLoadHistoryRejectsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	if err := os.WriteFile(path, []byte("{ broken"), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := core.LoadHistory(path, 100)
	if err == nil {
		t.Fatal("corrupt JSON must be reported")
	}
	if loaded == nil || loaded.Len() != 0 {
		t.Fatal("must still return a usable empty history")
	}
}

func TestLoadHistoryIgnoresOldSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	body := `{"schema":0,"usage":{"k":{"count":5,"last_used":1}}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := core.LoadHistory(path, 100)
	if err != nil {
		t.Fatalf("an old schema is not an error: %v", err)
	}
	if loaded.Len() != 0 {
		t.Fatal("an old schema must be discarded, not migrated")
	}
}
