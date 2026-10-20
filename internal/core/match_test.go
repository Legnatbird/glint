package core_test

import (
	"math"
	"testing"

	"applauncher/internal/core"
)

// The scorer's job is to rank, not to produce a beautiful number. These cases
// pin the ordering rules that the UI depends on.
func TestScoreMatchOrdering(t *testing.T) {
	cases := []struct {
		name          string
		query         string
		better, worse string
	}{
		{"prefix beats word start", "chr", "Chromium", "Google Chrome"},
		{"word start beats mid word", "code", "Visual Studio Code", "Encode"},
		{"contiguous beats scattered", "term", "Terminal", "The Emulator Room Machine"},
		{"word start beats scattered", "code", "Code::Blocks", "Cursor Dev Environment"},
		{"prefix beats suffix", "edge", "Edge", "Microsoft Edge"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			better, _, ok := core.ScoreMatch(tc.better, tc.query)
			if !ok {
				t.Fatalf("ScoreMatch(%q, %q) did not match", tc.better, tc.query)
			}
			worse, _, ok := core.ScoreMatch(tc.worse, tc.query)
			if !ok {
				t.Fatalf("ScoreMatch(%q, %q) did not match", tc.worse, tc.query)
			}
			if better <= worse {
				t.Fatalf("%q scored %.4f but %q scored %.4f; the first should rank higher",
					tc.better, better, tc.worse, worse)
			}
		})
	}
}

func TestScoreMatchBounds(t *testing.T) {
	for _, tc := range []struct{ name, query string }{
		{"Chrome", "chrome"},
		{"Google Chrome", "chro"},
		{"Encode", "code"},
		{"Visual Studio Code", "vscode"},
		{"7-Zip", "7z"},
		{"Configuración", "configuracion"},
	} {
		got, _, ok := core.ScoreMatch(tc.name, tc.query)
		if !ok {
			t.Fatalf("ScoreMatch(%q, %q) did not match", tc.name, tc.query)
		}
		if got < 0 || got > 1 {
			t.Fatalf("ScoreMatch(%q, %q) = %.4f, want it inside [0,1]", tc.name, tc.query, got)
		}
	}
}

func TestScoreMatchExactName(t *testing.T) {
	got, _, ok := core.ScoreMatch("Chrome", "chrome")
	if !ok {
		t.Fatal("exact name should match")
	}
	if math.Abs(got-1) > 1e-9 {
		t.Fatalf("exact name scored %.4f, want 1.0", got)
	}
}

func TestScoreMatchNoMatch(t *testing.T) {
	if _, _, ok := core.ScoreMatch("Chrome", "zzz"); ok {
		t.Fatal("a query with missing characters must not match")
	}
	if _, _, ok := core.ScoreMatch("Chrome", ""); ok {
		t.Fatal("an empty query is not a match at this layer")
	}
}

func TestScoreMatchPositions(t *testing.T) {
	cases := []struct {
		name, query string
		want        []int
	}{
		{"Google Chrome", "chr", []int{7, 8, 9}},
		{"7-Zip", "7z", []int{0, 2}},
		{"Terminal", "term", []int{0, 1, 2, 3}},
	}

	for _, tc := range cases {
		_, got, ok := core.ScoreMatch(tc.name, tc.query)
		if !ok {
			t.Fatalf("ScoreMatch(%q, %q) did not match", tc.name, tc.query)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("%q positions = %v, want %v", tc.name, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("%q positions = %v, want %v", tc.name, got, tc.want)
			}
		}
	}
}

func TestScoreMatchDiacritics(t *testing.T) {
	got, _, ok := core.ScoreMatch("Configuración", "configuracion")
	if !ok {
		t.Fatal("unaccented query must match an accented name")
	}
	if math.Abs(got-1) > 1e-9 {
		t.Fatalf("accent-folded exact match scored %.4f, want 1.0", got)
	}
}
