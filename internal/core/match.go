package core

import "math"

// Scoring weights. The score answers one question: how close is this match to
// the ideal (the query, contiguous, at the start of a word)?
const (
	weightContiguity = 0.5
	weightBoundary   = 0.3
	weightPosition   = 0.2

	// exactCompactBonus rewards a compact equality ("vscode" against "VS Code")
	// without letting it beat a real exact-name match.
	exactCompactBonus = 0.2
)

// candidate is an Entry with its normalized forms precomputed. Building them at
// index time keeps the per-keystroke path free of string work.
type candidate struct {
	entry     Entry
	norm      []rune
	compact   []rune
	compactAt []int32
}

func newCandidate(e Entry) candidate {
	norm := normalize(e.Name)
	compact, at := compactRunes(norm)
	return candidate{entry: e, norm: norm, compact: compact, compactAt: at}
}

// score ranks the candidate against an already-normalized query. It allocates
// nothing: positions are computed later, only for the rows that get shown.
func (c *candidate) score(q, qCompact []rune) (float64, bool) {
	best := -1.0
	if s, ok := scoreSubsequence(c.norm, q); ok {
		best = s
	}
	if len(c.compact) > 0 {
		// A separated name ("7-Zip" -> "7zip") can only match a compact query
		// ("7z") contiguously through its compact form.
		if s, ok := scoreSubsequence(c.compact, qCompact); ok && s > best {
			best = s
		}
	}
	if best < 0 {
		return 0, false
	}

	switch {
	case equalRunes(c.norm, q):
		best = 1
	case equalRunes(c.compact, qCompact):
		best = math.Min(1, best+exactCompactBonus)
	}
	return best, true
}

// scoreSubsequence runs a greedy ordered match and scores it in one pass.
func scoreSubsequence(s, q []rune) (float64, bool) {
	if len(q) == 0 || len(s) < len(q) {
		return 0, false
	}

	var (
		j          int
		pairs      int
		boundaries int
		first      = -1
		prev       = -1
	)
	for i := 0; i < len(s) && j < len(q); i++ {
		if s[i] != q[j] {
			continue
		}
		if first < 0 {
			first = i
		}
		if prev >= 0 && i == prev+1 {
			pairs++
		}
		if i == 0 || isSeparator(s[i-1]) {
			boundaries++
		}
		prev = i
		j++
	}
	if j < len(q) {
		return 0, false
	}

	contiguity := 1.0
	if n := len(q); n > 1 {
		contiguity = float64(pairs) / float64(n-1)
	}
	boundary := float64(boundaries) / float64(len(q))

	position := 0.2
	switch {
	case first == 0:
		position = 1.0
	case isSeparator(s[first-1]):
		position = 0.7
	}

	return weightContiguity*contiguity + weightBoundary*boundary + weightPosition*position, true
}

// positions recomputes the winning match and maps it to indices in the display
// name. Only called for results that survive the limit.
func (c *candidate) positions(q, qCompact []rune) []int {
	normScore, normOK := scoreSubsequence(c.norm, q)
	compactScore, compactOK := 0.0, false
	if len(c.compact) > 0 {
		compactScore, compactOK = scoreSubsequence(c.compact, qCompact)
	}

	if normOK && (!compactOK || normScore >= compactScore) {
		return greedyPositions(c.norm, q)
	}
	if compactOK {
		pos := greedyPositions(c.compact, qCompact)
		for i, p := range pos {
			pos[i] = int(c.compactAt[p])
		}
		return pos
	}
	return nil
}

func greedyPositions(s, q []rune) []int {
	if len(q) == 0 {
		return nil
	}
	pos := make([]int, 0, len(q))
	for i, j := 0, 0; i < len(s) && j < len(q); i++ {
		if s[i] == q[j] {
			pos = append(pos, i)
			j++
			if j == len(q) {
				break
			}
		}
	}
	if len(pos) != len(q) {
		return nil
	}
	return pos
}

// ScoreMatch reports how well query matches name and which runes of name should
// be highlighted. Exposed for tests and for the settings preview.
func ScoreMatch(name, query string) (score float64, positions []int, ok bool) {
	q := normalize(query)
	if len(q) == 0 {
		return 0, nil, false
	}
	qCompact, _ := compactRunes(q)

	c := newCandidate(Entry{Name: name})
	s, ok := c.score(q, qCompact)
	if !ok {
		return 0, nil, false
	}
	return s, c.positions(q, qCompact), true
}
