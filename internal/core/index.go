package core

import (
	"strings"
	"time"
)

const (
	// DefaultLimit is how many rows the launcher shows.
	DefaultLimit = 8

	// Ranking weights. Text match is the primary signal; frecency breaks ties
	// in favour of the app the user actually opens.
	matchWeight    = 0.6
	frecencyWeight = 0.4
)

// Query is one search request.
type Query struct {
	Text    string
	Limit   int
	History *History
	Now     time.Time // zero means time.Now()
}

// Result is one ranked row.
type Result struct {
	Entry     Entry
	Score     float64 // blended rank, 0..1
	Match     float64 // textual match quality, 0..1
	Frecency  float64 // usage weight, 0..1
	Positions []int   // rune indices of Entry.Name to highlight

	cand int
}

// Index is an immutable snapshot of the applications. Rebuild it and swap the
// pointer instead of mutating it, so a search never sees a half-built list.
type Index struct {
	entries []Entry
	cands   []candidate
}

// NewIndex deduplicates entries by key, keeping the highest-priority source.
func NewIndex(entries []Entry) *Index {
	idx := &Index{
		entries: make([]Entry, 0, len(entries)),
		cands:   make([]candidate, 0, len(entries)),
	}
	seen := make(map[string]int, len(entries))

	for _, e := range entries {
		if e.Key == "" || e.Name == "" {
			continue
		}
		if at, ok := seen[e.Key]; ok {
			if e.Source.Priority() < idx.entries[at].Source.Priority() {
				idx.entries[at] = e
				idx.cands[at] = newCandidate(e)
			}
			continue
		}
		seen[e.Key] = len(idx.entries)
		idx.entries = append(idx.entries, e)
		idx.cands = append(idx.cands, newCandidate(e))
	}
	return idx
}

// Len reports how many applications the index holds.
func (i *Index) Len() int { return len(i.entries) }

// Entries returns the deduplicated entries in index order.
func (i *Index) Entries() []Entry { return i.entries }

// Search ranks the index against q.
//
// It never materializes more than limit results: a bounded insertion keeps the
// per-keystroke allocation at two slices instead of one per match (a one-letter
// query matches all 3000 entries).
func (i *Index) Search(q Query) []Result {
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	now := q.Now
	if now.IsZero() {
		now = time.Now()
	}

	qn := normalize(strings.TrimSpace(q.Text))
	qc, _ := compactRunes(qn)

	out := make([]Result, 0, limit+1)
	if len(qn) == 0 {
		for c := range i.cands {
			f := q.History.Score(i.cands[c].entry.Key, now)
			out = insertTop(out, Result{
				Entry:    i.cands[c].entry,
				Frecency: f,
				Score:    frecencyWeight * f,
				cand:     c,
			}, limit)
		}
	} else {
		for c := range i.cands {
			m, ok := i.cands[c].score(qn, qc)
			if !ok {
				continue
			}
			f := q.History.Score(i.cands[c].entry.Key, now)
			out = insertTop(out, Result{
				Entry:    i.cands[c].entry,
				Match:    m,
				Frecency: f,
				Score:    matchWeight*m + frecencyWeight*f,
				cand:     c,
			}, limit)
		}
	}

	if len(qn) > 0 {
		for k := range out {
			out[k].Positions = i.cands[out[k].cand].positions(qn, qc)
		}
	}
	return out
}

// insertTop keeps out sorted best-first and never longer than limit.
func insertTop(out []Result, r Result, limit int) []Result {
	if len(out) == limit && !better(r, out[len(out)-1]) {
		return out
	}

	pos := len(out)
	for i := range out {
		if better(r, out[i]) {
			pos = i
			break
		}
	}

	out = append(out, Result{})
	copy(out[pos+1:], out[pos:])
	out[pos] = r
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// better is the single ordering rule: higher score, then shorter name, then
// alphabetical. Shorter wins ties: "Steam" before "Steam Service Helper".
func better(a, b Result) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if la, lb := len(a.Entry.Name), len(b.Entry.Name); la != lb {
		return la < lb
	}
	return a.Entry.Name < b.Entry.Name
}
