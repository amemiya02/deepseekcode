// fuzzy.go is a dependency-free tiered subsequence matcher used to rank
// completion candidates (the `/` and `@` menus) and filterable pickers.
// It mirrors crush's tierExactName/tierPrefixName/tierPathSegment/tierFallback
// ordering: the tier is the dominant component of the score (lower is better),
// and within a tier earlier+denser matches on shorter candidates win.
package tui

import (
	"strings"
)

// Score weights. The tier dominates; gap (how spread out the match is) and the
// index of the first matched rune break ties before the candidate length does.
// The multipliers are large enough that no realistic candidate length or gap
// count can climb out of its tier.
const (
	fuzzyTierWeight  = 1 << 20
	fuzzyGapWeight   = 1 << 8
	fuzzyStartWeight = 1 << 2
)

// fuzzyMatch reports whether query subsequence-matches candidate (case-insensitive),
// a score where LOWER is better, and the matched rune indices into candidate (for
// bold highlighting). Empty query => ok=true, score=0, matched=nil.
func fuzzyMatch(query, candidate string) (ok bool, score int, matched []int) {
	if query == "" {
		return true, 0, nil
	}
	cr := []rune(candidate)
	ql := strings.ToLower(query)
	cl := strings.ToLower(candidate)

	// Tier 0/1: exact and prefix are decided on the lowercased strings; the
	// matched runes are simply the leading runes of the candidate.
	tier := -1
	switch {
	case cl == ql:
		tier = 0
	case strings.HasPrefix(cl, ql):
		tier = 1
	}
	if tier >= 0 {
		n := len([]rune(ql))
		matched = make([]int, n)
		for i := range matched {
			matched[i] = i
		}
		return true, fuzzyScore(tier, matched, len(cr)), matched
	}

	// Tier 2/3: greedy left-to-right subsequence walk over runes. We prefer a
	// word-boundary alignment (tier 2) but always fall back to anywhere (tier 3)
	// with the same indices, so a single pass yields both.
	qr := []rune(ql)
	clr := []rune(cl)
	idx := make([]int, 0, len(qr))
	atBoundary := true // every matched rune began at a word boundary so far
	qi := 0
	for ci := 0; ci < len(clr) && qi < len(qr); ci++ {
		if clr[ci] != qr[qi] {
			continue
		}
		if !isWordBoundary(clr, ci) {
			atBoundary = false
		}
		idx = append(idx, ci)
		qi++
	}
	if qi < len(qr) {
		return false, 0, nil
	}
	tier = 3
	if atBoundary {
		tier = 2
	}
	return true, fuzzyScore(tier, idx, len(cr)), idx
}

// fuzzyScore folds the tier, match density, start position, and candidate
// length into a single lower-is-better integer. Density is measured as the
// total gap between consecutive matched runes (0 == contiguous).
func fuzzyScore(tier int, matched []int, candLen int) int {
	gap := 0
	for i := 1; i < len(matched); i++ {
		gap += matched[i] - matched[i-1] - 1
	}
	start := 0
	if len(matched) > 0 {
		start = matched[0]
	}
	return tier*fuzzyTierWeight + gap*fuzzyGapWeight + start*fuzzyStartWeight + candLen
}

// isWordBoundary reports whether the rune at i starts a word: the string start,
// or a rune immediately following a separator (_ - space . /).
func isWordBoundary(rs []rune, i int) bool {
	if i == 0 {
		return true
	}
	switch rs[i-1] {
	case '_', '-', ' ', '.', '/':
		return true
	}
	return false
}
