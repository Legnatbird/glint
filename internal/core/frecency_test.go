package core_test

import (
	"testing"
	"time"

	"glint/internal/core"
)

func TestFrecencyDecay(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour

	cases := []struct {
		name  string
		count int
		age   time.Duration
		want  float64
	}{
		{"never used", 0, 0, 0},
		{"once today", 1, 0, 0.5},
		{"ten times today", 10, 0, 10.0 / 11.0},
		{"once a week ago", 1, 7 * day, 0.5 / 1.5},
		{"once two weeks ago", 1, 14 * day, 0.25 / 1.25},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := core.Frecency(tc.count, now.Add(-tc.age), now)
			if diff := got - tc.want; diff > 1e-9 || diff < -1e-9 {
				t.Fatalf("Frecency(%d, now-%v) = %.6f, want %.6f", tc.count, tc.age, got, tc.want)
			}
		})
	}
}

func TestFrecencyIgnoresFutureTimestamps(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	if got, want := core.Frecency(1, now.Add(time.Hour), now), 0.5; got != want {
		t.Fatalf("a clock skew into the future must decay as if it were now: got %.4f, want %.4f", got, want)
	}
}

func TestHistoryTouchAndSize(t *testing.T) {
	h := core.NewHistory(nil, 2)
	now := time.Now()

	h.Touch("a", now)
	h.Touch("b", now.Add(time.Second))
	h.Touch("c", now.Add(2*time.Second)) // evicts "a", the least recently used

	if got := len(h.Snapshot()); got != 2 {
		t.Fatalf("history holds %d entries, want 2", got)
	}
	if got := h.Score("a", now.Add(3*time.Second)); got != 0 {
		t.Fatalf("evicted key still scores %.3f", got)
	}
	if got := h.Score("c", now.Add(3*time.Second)); got == 0 {
		t.Fatal("newest key should be tracked")
	}
}

func TestHistoryKeepsActiveKeys(t *testing.T) {
	h := core.NewHistory(nil, 2)
	now := time.Now()

	h.Touch("a", now)
	h.Touch("b", now.Add(time.Second))
	h.Touch("a", now.Add(2*time.Second))
	h.Touch("c", now.Add(3*time.Second)) // "b" is the oldest, not "a"

	if got := h.Score("b", now.Add(4*time.Second)); got != 0 {
		t.Fatalf("least recently used key should have been evicted, scored %.3f", got)
	}
	if got := h.Score("a", now.Add(4*time.Second)); got == 0 {
		t.Fatal("recently touched key should survive eviction")
	}
}
