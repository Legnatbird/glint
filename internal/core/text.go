package core

import (
	"strings"
	"unicode"
)

// separators end a word. A matched character right after one of these gets the
// word-start bonus, which is what separates "Google Chrome" from "Chromium"
// only when the query starts at the beginning.
const separators = " -_./\\()[]:,+"

func isSeparator(r rune) bool { return strings.ContainsRune(separators, r) }

// fold strips the accents that Spanish, Portuguese and French app names carry.
// It is a 1:1 mapping so rune offsets survive normalization and can be used for
// highlighting.
func fold(r rune) rune {
	switch r {
	case 'á', 'à', 'â', 'ä', 'ã', 'å', 'ā':
		return 'a'
	case 'é', 'è', 'ê', 'ë', 'ē':
		return 'e'
	case 'í', 'ì', 'î', 'ï', 'ī':
		return 'i'
	case 'ó', 'ò', 'ô', 'ö', 'õ', 'ø', 'ō':
		return 'o'
	case 'ú', 'ù', 'û', 'ü', 'ū':
		return 'u'
	case 'ñ', 'ń':
		return 'n'
	case 'ç', 'ć':
		return 'c'
	case 'ý', 'ÿ':
		return 'y'
	case 'š', 'ś':
		return 's'
	case 'ž', 'ź', 'ż':
		return 'z'
	}
	return r
}

// normalize lowercases and folds, preserving length.
func normalize(s string) []rune {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		out = append(out, fold(unicode.ToLower(r)))
	}
	return out
}

// compactRunes removes separators and reports, for every compacted rune, the
// index it occupied in the original slice. That mapping is what lets a compact
// match ("7z" against "7zip") still highlight the right characters.
func compactRunes(s []rune) ([]rune, []int32) {
	out := make([]rune, 0, len(s))
	at := make([]int32, 0, len(s))
	for i, r := range s {
		if isSeparator(r) {
			continue
		}
		out = append(out, r)
		at = append(at, int32(i))
	}
	return out, at
}

func equalRunes(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
