package tools

import (
	"strings"
	"testing"
)

// TestApplyReplaceStrategies proves each strategy fires and — critically —
// splices the ORIGINAL bytes of the matched region, never the normalized form.
// Each case asserts the FULL post-edit string so any offset/Join bug that
// re-emits normalized bytes (the #1 porting trap) is caught.
func TestApplyReplaceStrategies(t *testing.T) {
	tests := []struct {
		name    string
		content string
		old     string
		new     string
		want    string // full expected content after splice
	}{
		{
			name:    "exact single line",
			content: "alpha\nbeta\ngamma\n",
			old:     "beta",
			new:     "BETA",
			want:    "alpha\nBETA\ngamma\n",
		},
		{
			name:    "exact multi line",
			content: "func f() {\n\treturn 1\n}\n",
			old:     "func f() {\n\treturn 1\n}",
			new:     "func f() {\n\treturn 2\n}",
			want:    "func f() {\n\treturn 2\n}\n",
		},
		{
			// old has trailing spaces the file doesn't; line-trimmed matches and
			// splices the ORIGINAL "\treturn 1" (tab-indented, no trailing space).
			name:    "line-trimmed trailing space drift preserves original indent",
			content: "func f() {\n\treturn 1\n}\n",
			old:     "return 1   ",
			new:     "\treturn 2",
			want:    "func f() {\n\treturn 2\n}\n",
		},
		{
			// old is a bare substring of an indented line; the exact strategy
			// finds it verbatim (unique) and splices only that substring,
			// preserving the surrounding indentation bytes untouched.
			name:    "exact substring inside indented line",
			content: "if x {\n        doThing()\n}\n",
			old:     "doThing()",
			new:     "doOther()",
			want:    "if x {\n        doOther()\n}\n",
		},
		{
			// Multi-line block where EVERY line carries trailing-space drift, so
			// the exact block is absent but the trimmed block matches. The splice
			// must restore the ORIGINAL bytes (no trailing spaces, original
			// indentation), proving byte-offset math over original lines.
			name:    "line-trimmed multiline trailing-space drift",
			content: "func f() {\n\tx := 1\n\ty := 2\n}\n",
			old:     "x := 1  \n\ty := 2  ",
			new:     "\tx := 10\n\ty := 20",
			want:    "func f() {\n\tx := 10\n\ty := 20\n}\n",
		},
		{
			// Collapsed internal run: file has "a  =  b", old has "a = b".
			// whitespace-normalized matches; spliced bytes are the ORIGINAL
			// "let a  =  b" with the double spaces preserved outside? No — the
			// whole matched line is replaced, so we assert the new line.
			name:    "whitespace-normalized collapsed run",
			content: "let a  =  b\nlet c = d\n",
			old:     "let a = b",
			new:     "let a = B",
			want:    "let a = B\nlet c = d\n",
		},
		{
			// Tab vs spaces mid-line, multi-line block.
			name:    "whitespace-normalized multiline tab vs space",
			content: "x\nfoo\tbar baz\ny\n",
			old:     "foo bar  baz",
			new:     "REPLACED",
			want:    "x\nREPLACED\ny\n",
		},
		{
			// Whole block re-indented by the model: file uses 4-space indent,
			// old uses 2-space indent. indentation-flexible matches; spliced
			// bytes are the ORIGINAL 4-space-indented block.
			name:    "indentation-flexible whole block reindented",
			content: "def f():\n    a = 1\n    b = 2\n    return a + b\n",
			old:     "  a = 1\n  b = 2\n  return a + b",
			new:     "    a = 10\n    b = 20\n    return a + b",
			want:    "def f():\n    a = 10\n    b = 20\n    return a + b\n",
		},
		{
			// Block anchor: first/last lines match exactly, middle drifted.
			name:    "block-anchor drifted middle",
			content: "function g() {\n  const z = compute();\n  return z;\n}\n",
			old:     "function g() {\n  const z = computeValue();\n  return z;\n}",
			new:     "function g() {\n  return 0;\n}",
			want:    "function g() {\n  return 0;\n}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, matched, ambiguous := applyReplace(tt.content, tt.old, tt.new, false)
			if !matched {
				t.Fatalf("expected match (ambiguous=%v); got none", ambiguous)
			}
			if got != tt.want {
				t.Fatalf("spliced result mismatch:\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}

// TestApplyReplaceYieldsOriginalBytes directly asserts the byte-fidelity
// invariant: every FUZZY strategy must yield candidates that already exist
// verbatim in content (computed by byte-offset/Join of the original lines),
// never the normalized form. This guards against a future strategy returning a
// reconstructed/normalized string that would corrupt the splice.
//
// exactReplacer is excluded on purpose: by design it echoes `find` verbatim
// (which may NOT be in content); the cascade re-verifies it via strings.Index.
func TestApplyReplaceYieldsOriginalBytes(t *testing.T) {
	content := "def f():\n    a = 1\n    b = 2\n    return a + b\n"
	finds := []string{
		"  a = 1\n  b = 2\n  return a + b", // indentation drift
		"a = 1  ",                          // line-trimmed trailing drift
		"a = 1\n  b = 2\n  return a + b",   // block-anchor / whitespace
	}
	fuzzy := []replacer{
		lineTrimmedReplacer,
		whitespaceNormalizedReplacer,
		indentationFlexibleReplacer,
		blockAnchorReplacer,
	}
	for _, r := range fuzzy {
		for _, find := range finds {
			for _, cand := range r(content, find) {
				if !strings.Contains(content, cand) {
					t.Fatalf("fuzzy strategy yielded non-original candidate %q (not a substring of content)", cand)
				}
			}
		}
	}
}

// TestApplyReplaceAmbiguous: old has trailing-space drift so the EXACT form is
// absent (exact yields a candidate not present in content and is skipped); two
// indentically-indented lines are trim-equal to old, so line-trimmed yields the
// same original candidate twice — non-unique — and no later strategy resolves
// it. Result: matched=false, ambiguous=true (ask the model for more context).
func TestApplyReplaceAmbiguous(t *testing.T) {
	content := "  a = 1\nmid\n  a = 1\n"
	old := "a = 1  " // trailing spaces: not a verbatim substring of content
	got, matched, ambiguous := applyReplace(content, old, "a = 2", false)
	if matched {
		t.Fatalf("unexpected match for trim-ambiguous content; got %q", got)
	}
	if !ambiguous {
		t.Fatalf("expected ambiguous=true; got matched=%v ambiguous=%v", matched, ambiguous)
	}
}

// TestApplyReplaceAmbiguousExact: old appears exactly twice verbatim and
// replace_all is false → exact strategy yields a non-unique candidate, no later
// strategy resolves it, so ambiguous=true.
func TestApplyReplaceAmbiguousExact(t *testing.T) {
	content := "x = 1\nmid\nx = 1\n"
	got, matched, ambiguous := applyReplace(content, "x = 1", "x = 2", false)
	if matched {
		t.Fatalf("unexpected match; got %q", got)
	}
	if !ambiguous {
		t.Fatalf("expected ambiguous=true; got matched=%v ambiguous=%v", matched, ambiguous)
	}
}

// TestApplyReplaceNotFound: nothing in any strategy yields a candidate present
// in content → matched=false, ambiguous=false (genuine not-found).
func TestApplyReplaceNotFound(t *testing.T) {
	content := "alpha\nbeta\n"
	_, matched, ambiguous := applyReplace(content, "totally\nabsent\nblock", "x", false)
	if matched {
		t.Fatal("unexpected match for absent block")
	}
	if ambiguous {
		t.Fatal("expected ambiguous=false for genuine not-found")
	}
}

// TestApplyReplacePrefersUniqueOverAmbiguous: an earlier strategy is ambiguous
// (line-trimmed matches two trim-equal regions) but a UNIQUE exact match exists
// elsewhere — exact runs first, so it wins. This verifies cascade ordering: a
// unique exact match is never shadowed by a looser ambiguous one.
func TestApplyReplacePrefersUniqueOverAmbiguous(t *testing.T) {
	// "  target  " appears once verbatim (line 2). "target" (trimmed) is also a
	// trim-equal of line 4 "target". Exact match of the full old "  target  "
	// is unique, so exact wins and we splice that one line.
	content := "head\n  target  \nmid\ntarget\ntail\n"
	old := "  target  "
	got, matched, ambiguous := applyReplace(content, old, "  REPLACED  ", false)
	if !matched {
		t.Fatalf("expected unique exact match to win; ambiguous=%v", ambiguous)
	}
	want := "head\n  REPLACED  \nmid\ntarget\ntail\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestApplyReplaceExactReplaceAll: replace_all replaces every verbatim
// occurrence of the matched candidate.
func TestApplyReplaceExactReplaceAll(t *testing.T) {
	content := "x\nx\ny\nx\n"
	got, matched, ambiguous := applyReplace(content, "x", "z", true)
	if !matched || ambiguous {
		t.Fatalf("expected match; matched=%v ambiguous=%v", matched, ambiguous)
	}
	if got != "z\nz\ny\nz\n" {
		t.Fatalf("got %q", got)
	}
}

// TestApplyReplaceFuzzyReplaceAll: when a normalizing strategy matches, the
// replace_all path replaces exact repeats of the matched ORIGINAL candidate
// only (consistent with opencode), not every trim-equal region.
func TestApplyReplaceFuzzyReplaceAll(t *testing.T) {
	// old has a trailing-space drift; line-trimmed yields the original
	// "  a = 1" twice (both lines are identical originals), so replace_all on
	// that candidate replaces both.
	content := "  a = 1\nmid\n  a = 1\n"
	got, matched, _ := applyReplace(content, "a = 1  ", "  a = 2", true)
	if !matched {
		t.Fatal("expected match")
	}
	if got != "  a = 2\nmid\n  a = 2\n" {
		t.Fatalf("got %q", got)
	}
}

// TestBlockAnchorRequiresThreeLines: a 2-line find must not trigger
// block-anchor (and must not panic).
func TestBlockAnchorRequiresThreeLines(t *testing.T) {
	content := "first\nsecond\n"
	if out := blockAnchorReplacer(content, "first\nsecond"); out != nil {
		t.Fatalf("expected nil for <3-line find; got %v", out)
	}
}

// TestBlockAnchorSingleCandidateLowSimilarity: a single anchored candidate is
// accepted even with a very different middle line (single-candidate threshold
// is 0.0), matching opencode.
func TestBlockAnchorSingleCandidateLowSimilarity(t *testing.T) {
	content := "BEGIN\nthe quick brown fox jumps\nEND\n"
	old := "BEGIN\nx\nEND"
	out := blockAnchorReplacer(content, old)
	if len(out) != 1 {
		t.Fatalf("expected one candidate; got %v", out)
	}
	if out[0] != "BEGIN\nthe quick brown fox jumps\nEND" {
		t.Fatalf("expected original block; got %q", out[0])
	}
}

// TestLevenshteinChars sanity-checks the char-level distance helper.
func TestLevenshteinChars(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"kitten", "sitting", 3},
		{"flaw", "lawn", 2},
		{"héllo", "hello", 1}, // multibyte rune substitution counts as 1
	}
	for _, c := range cases {
		if got := levenshteinChars(c.a, c.b); got != c.want {
			t.Errorf("levenshteinChars(%q,%q)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}
