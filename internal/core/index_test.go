package core_test

import (
	"fmt"
	"testing"
	"time"

	"glint/internal/core"
)

func testEntries() []core.Entry {
	return []core.Entry{
		{Key: "lnk:chrome", Name: "Google Chrome", Target: `C:\ProgramData\Microsoft\Windows\Start Menu\Programs\Chrome.lnk`, Source: core.SourceStartMenu},
		{Key: "exe:chromium", Name: "Chromium", Target: `C:\Tools\chromium\chrome.exe`, Source: core.SourceCustomFolder},
		{Key: "lnk:vscode", Name: "Visual Studio Code", Target: `C:\ProgramData\Microsoft\Windows\Start Menu\Programs\Code.lnk`, Source: core.SourceStartMenu},
		{Key: "exe:settings", Name: "Configuración", Target: `C:\Windows\ImmersiveControlPanel\SystemSettings.exe`, Source: core.SourceStartMenu},
	}
}

func mustSearch(t *testing.T, idx *core.Index, q core.Query) []core.Result {
	t.Helper()
	return idx.Search(q)
}

func TestSearchTextRanking(t *testing.T) {
	idx := core.NewIndex(testEntries())

	got := mustSearch(t, idx, core.Query{Text: "chr", Limit: 8})
	if len(got) == 0 {
		t.Fatal("no results for chr")
	}
	if got[0].Entry.Name != "Chromium" {
		t.Fatalf("first result = %q, want Chromium (prefix match)", got[0].Entry.Name)
	}
}

func TestSearchFrecencyOutranksSlightlyWorseMatch(t *testing.T) {
	idx := core.NewIndex(testEntries())
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

	history := core.NewHistory(nil, 100)
	for i := 0; i < 10; i++ {
		history.Touch("lnk:chrome", now)
	}

	got := mustSearch(t, idx, core.Query{Text: "chr", Limit: 8, History: history, Now: now})
	if got[0].Entry.Name != "Google Chrome" {
		t.Fatalf("first result = %q, want Google Chrome: recent use must beat a marginally better text match", got[0].Entry.Name)
	}
}

func TestSearchMatchesWithoutAccents(t *testing.T) {
	idx := core.NewIndex(testEntries())

	got := mustSearch(t, idx, core.Query{Text: "configuracion", Limit: 8})
	if len(got) == 0 || got[0].Entry.Name != "Configuración" {
		t.Fatalf("results = %v, want Configuración first", got)
	}
}

func TestSearchRespectsLimit(t *testing.T) {
	idx := core.NewIndex(testEntries())

	if got := mustSearch(t, idx, core.Query{Text: "c", Limit: 2}); len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	if got := mustSearch(t, idx, core.Query{Text: "c", Limit: 1}); len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
}

func TestSearchWithoutMatchReturnsNothing(t *testing.T) {
	idx := core.NewIndex(testEntries())

	if got := mustSearch(t, idx, core.Query{Text: "zzzz", Limit: 8}); len(got) != 0 {
		t.Fatalf("got %d results for a non-matching query", len(got))
	}
}

func TestSearchEmptyQueryWithoutHistoryReturnsNothing(t *testing.T) {
	idx := core.NewIndex(testEntries())

	// A fresh install must not advertise eight random applications.
	if got := mustSearch(t, idx, core.Query{Text: "", Limit: 3}); len(got) != 0 {
		t.Fatalf("got %d results on an empty query with no history", len(got))
	}
}

func TestSearchEmptyQueryIsDeterministic(t *testing.T) {
	idx := core.NewIndex(testEntries())
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

	history := core.NewHistory(nil, 100)
	history.Touch("lnk:chrome", now)
	history.Touch("lnk:vscode", now)

	q := core.Query{Text: "", Limit: 3, History: history, Now: now}
	first := mustSearch(t, idx, q)
	second := mustSearch(t, idx, q)

	if len(first) != 2 {
		t.Fatalf("got %d results, want the 2 used apps", len(first))
	}
	for i := range first {
		if first[i].Entry.Key != second[i].Entry.Key {
			t.Fatalf("empty query is not deterministic: %q vs %q", first[i].Entry.Key, second[i].Entry.Key)
		}
	}
}

func TestSearchEmptyQueryUsesHistory(t *testing.T) {
	idx := core.NewIndex(testEntries())
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

	history := core.NewHistory(nil, 100)
	history.Touch("lnk:vscode", now)

	got := mustSearch(t, idx, core.Query{Text: "", Limit: 3, History: history, Now: now})
	if len(got) == 0 || got[0].Entry.Key != "lnk:vscode" {
		t.Fatalf("first result = %q, want the only used app", got[0].Entry.Key)
	}
}

func TestNewIndexDeduplicatesByKeyKeepingBestSource(t *testing.T) {
	idx := core.NewIndex([]core.Entry{
		{Key: "exe:code", Name: "Visual Studio Code", Target: `C:\Tools\Code.exe`, Source: core.SourcePath},
		{Key: "exe:code", Name: "Visual Studio Code", Target: `C:\Tools\Code.exe`, Source: core.SourceAppPaths},
		{Key: "exe:code", Name: "Visual Studio Code", Target: `C:\Tools\Code.exe`, Source: core.SourceStartMenu},
	})

	if got := idx.Len(); got != 1 {
		t.Fatalf("index holds %d entries, want 1 after dedup", got)
	}
	if got := idx.Entries()[0].Source; got != core.SourceStartMenu {
		t.Fatalf("kept source %v, want SourceStartMenu", got)
	}
}

func TestNewIndexSkipsIncompleteEntries(t *testing.T) {
	idx := core.NewIndex([]core.Entry{
		{Key: "", Name: "No key"},
		{Key: "k1", Name: ""},
		{Key: "k2", Name: "Valid"},
	})

	if got := idx.Len(); got != 1 {
		t.Fatalf("index holds %d entries, want only the valid one", got)
	}
}

func TestSearchOn3000EntriesStaysFast(t *testing.T) {
	entries := make([]core.Entry, 3000)
	for i := range entries {
		entries[i] = core.Entry{
			Key:    fmt.Sprintf("k%d", i),
			Name:   fmt.Sprintf("Application %d", i),
			Source: core.SourceStartMenu,
		}
	}
	idx := core.NewIndex(entries)

	start := time.Now()
	got := idx.Search(core.Query{Text: "app", Limit: 8})
	elapsed := time.Since(start)

	if len(got) != 8 {
		t.Fatalf("got %d results, want 8", len(got))
	}
	if elapsed > 5*time.Millisecond {
		t.Fatalf("searching 3000 entries took %v, the budget is 5ms", elapsed)
	}
	t.Logf("3000 entries searched in %v", elapsed)
}

func BenchmarkSearch3000(b *testing.B) {
	entries := make([]core.Entry, 3000)
	for i := range entries {
		entries[i] = core.Entry{
			Key:    fmt.Sprintf("k%d", i),
			Name:   fmt.Sprintf("Application %d", i),
			Source: core.SourceStartMenu,
		}
	}
	idx := core.NewIndex(entries)
	q := core.Query{Text: "app", Limit: 8}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Search(q)
	}
}
