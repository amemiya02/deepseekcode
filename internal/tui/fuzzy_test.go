package tui

import (
	"reflect"
	"sort"
	"testing"
)

func TestFuzzyMatchEmptyQuery(t *testing.T) {
	ok, score, matched := fuzzyMatch("", "anything")
	if !ok {
		t.Fatal("empty query should match")
	}
	if score != 0 {
		t.Fatalf("empty query score = %d, want 0", score)
	}
	if matched != nil {
		t.Fatalf("empty query matched = %v, want nil", matched)
	}
}

func TestFuzzyMatchNonMatch(t *testing.T) {
	// 'z' is not a subsequence of "models".
	if ok, _, _ := fuzzyMatch("xz", "models"); ok {
		t.Fatal("xz should not match models")
	}
	// All query runes present individually but order is wrong.
	if ok, _, _ := fuzzyMatch("lm", "models"); ok {
		t.Fatal("lm (wrong order) should not subsequence-match models")
	}
}

func TestFuzzyMatchCaseInsensitive(t *testing.T) {
	ok, _, matched := fuzzyMatch("MOD", "models")
	if !ok {
		t.Fatal("MOD should match models case-insensitively")
	}
	if !reflect.DeepEqual(matched, []int{0, 1, 2}) {
		t.Fatalf("matched = %v, want [0 1 2]", matched)
	}
}

// TestFuzzyMatchTierOrdering pins the tier boundaries: exact < prefix <
// word-boundary subsequence < anywhere subsequence, regardless of length.
func TestFuzzyMatchTierOrdering(t *testing.T) {
	_, exact, _ := fuzzyMatch("models", "models")     // tier 0
	_, prefix, _ := fuzzyMatch("mod", "models")       // tier 1
	_, boundary, _ := fuzzyMatch("mt", "models-tape") // tier 2: m, t (after '-')
	_, anywhere, _ := fuzzyMatch("oe", "models")      // tier 3: o, e mid-word

	if !(exact < prefix && prefix < boundary && boundary < anywhere) {
		t.Fatalf("tier ordering violated: exact=%d prefix=%d boundary=%d anywhere=%d",
			exact, prefix, boundary, anywhere)
	}
}

// A prefix on a very long candidate must still beat an anywhere-match on a
// short candidate — the tier dominates length.
func TestFuzzyMatchTierDominatesLength(t *testing.T) {
	_, longPrefix, _ := fuzzyMatch("mo", "moooooooooooooooooooooodels")
	_, shortAnywhere, _ := fuzzyMatch("oe", "model")
	if longPrefix >= shortAnywhere {
		t.Fatalf("prefix on long cand (%d) should beat anywhere on short cand (%d)",
			longPrefix, shortAnywhere)
	}
}

// Within a tier, the shorter candidate wins when match positions are identical.
func TestFuzzyMatchShorterWinsWithinTier(t *testing.T) {
	_, short, _ := fuzzyMatch("mo", "model")
	_, long, _ := fuzzyMatch("mo", "models-and-modes")
	if short >= long {
		t.Fatalf("shorter prefix candidate should score lower: short=%d long=%d", short, long)
	}
}

// Within a tier, a denser (more contiguous) match wins over a sparse one.
func TestFuzzyMatchDenserWins(t *testing.T) {
	_, dense, _ := fuzzyMatch("ab", "xabxxxx")  // contiguous a,b
	_, sparse, _ := fuzzyMatch("ab", "xaxxxxb") // spread out a..b, same length
	if dense >= sparse {
		t.Fatalf("denser match should score lower: dense=%d sparse=%d", dense, sparse)
	}
}

func TestFuzzyMatchWordBoundaryIndices(t *testing.T) {
	ok, _, matched := fuzzyMatch("rs", "reload-skills")
	if !ok {
		t.Fatal("rs should match reload-skills")
	}
	// r at 0 (start), s at 7 (just after '-').
	if !reflect.DeepEqual(matched, []int{0, 7}) {
		t.Fatalf("matched = %v, want [0 7]", matched)
	}
}

// TestFuzzyMatchUnicodeIndices verifies indices are rune indices, not byte
// offsets, on a multibyte candidate.
func TestFuzzyMatchUnicodeIndices(t *testing.T) {
	// café-über: 'é'(idx 3) and 'ü'(idx 5) are multibyte. Query "cb" matches
	// 'c'(rune 0) and 'b'(rune 6); 'b' sits at byte offset 8 but rune index 6,
	// so a byte-offset implementation would return the wrong index here.
	cand := "café-über"
	ok, _, matched := fuzzyMatch("cb", cand)
	if !ok {
		t.Fatal("cb should match café-über")
	}
	if !reflect.DeepEqual(matched, []int{0, 6}) {
		t.Fatalf("matched = %v, want [0 6]", matched)
	}
	// Sanity: indexing the rune slice at the returned indices yields the query.
	cr := []rune(cand)
	if cr[matched[0]] != 'c' || cr[matched[1]] != 'b' {
		t.Fatalf("rune indices point at %c%c, want cb", cr[matched[0]], cr[matched[1]])
	}
}

// TestFuzzyRanking sorts a realistic candidate set by score and asserts the
// resulting order, exercising the full tier + tiebreak ladder end to end.
func TestFuzzyRanking(t *testing.T) {
	cands := []string{
		"mode",          // tier 1 prefix, len 4
		"models",        // tier 1 prefix, len 6
		"mod",           // tier 0 exact
		"reload-models", // tier 2: m,o,d contiguous just after '-'
		"command-mode",  // tier 3: m,o,d anywhere (the "mod" inside "-mode")
	}
	type scored struct {
		name  string
		score int
	}
	var got []scored
	for _, c := range cands {
		ok, s, _ := fuzzyMatch("mod", c)
		if !ok {
			t.Fatalf("mod should match %q", c)
		}
		got = append(got, scored{c, s})
	}
	sort.SliceStable(got, func(i, j int) bool { return got[i].score < got[j].score })

	want := []string{"mod", "mode", "models", "reload-models", "command-mode"}
	var order []string
	for _, g := range got {
		order = append(order, g.name)
	}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("ranking = %v, want %v (scores: %+v)", order, want, got)
	}
}
