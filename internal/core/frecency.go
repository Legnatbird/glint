package core

import (
	"math"
	"sort"
	"sync"
	"time"
)

// FrecencyHalfLife is how long a use takes to lose half of its weight. A week
// keeps muscle memory relevant without pinning yesterday's mistake to the top.
const FrecencyHalfLife = 7 * 24 * time.Hour

// Record is the usage of one application.
type Record struct {
	Count    int   `json:"count"`
	LastUsed int64 `json:"last_used"` // unix seconds
}

// Frecency turns a use count into a 0..1 weight that decays with age.
//
// raw = count * 0.5^(age/halfLife), then squashed through x/(1+x) so a single
// use cannot dominate a real text match and 100 uses cannot reach 1.
func Frecency(count int, lastUsed, now time.Time) float64 {
	if count <= 0 {
		return 0
	}
	age := now.Sub(lastUsed)
	if age < 0 {
		age = 0
	}
	decay := math.Pow(0.5, age.Hours()/FrecencyHalfLife.Hours())
	raw := float64(count) * decay
	return raw / (1 + raw)
}

// History tracks how often and how recently each application was launched.
// It is safe for concurrent use: the UI reads it while the indexer writes.
type History struct {
	mu   sync.Mutex
	recs map[string]Record
	max  int
}

// NewHistory copies records and keeps at most maxEntries of them.
func NewHistory(records map[string]Record, maxEntries int) *History {
	if maxEntries <= 0 {
		maxEntries = 2000
	}
	h := &History{recs: make(map[string]Record, len(records)), max: maxEntries}
	for k, v := range records {
		h.recs[k] = v
	}
	return h
}

// Touch records one launch of key at time at.
func (h *History) Touch(key string, at time.Time) {
	if h == nil || key == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	rec := h.recs[key]
	rec.Count++
	rec.LastUsed = at.Unix()
	h.recs[key] = rec

	if len(h.recs) > h.max {
		h.pruneLocked()
	}
}

// Score returns the frecency of key, or zero when it was never used.
func (h *History) Score(key string, now time.Time) float64 {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	rec, ok := h.recs[key]
	h.mu.Unlock()
	if !ok {
		return 0
	}
	return Frecency(rec.Count, time.Unix(rec.LastUsed, 0), now)
}

// Snapshot returns a copy of the records for persistence.
func (h *History) Snapshot() map[string]Record {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	out := make(map[string]Record, len(h.recs))
	for k, v := range h.recs {
		out[k] = v
	}
	return out
}

// Len reports how many applications are tracked.
func (h *History) Len() int {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.recs)
}

// Clear forgets every recorded launch. The caller is responsible for persisting
// the now-empty history.
func (h *History) Clear() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.recs = make(map[string]Record)
}

func (h *History) pruneLocked() {
	keys := make([]string, 0, len(h.recs))
	for k := range h.recs {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return h.recs[keys[i]].LastUsed < h.recs[keys[j]].LastUsed
	})
	for i := 0; i < len(keys)-h.max; i++ {
		delete(h.recs, keys[i])
	}
}
