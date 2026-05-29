package tools

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// editCorpusCase is one near-miss edit scenario loaded from
// testdata/edit_corpus.jsonl. `content` is the original file; `old` is the
// model's drifted old_string; `new` the replacement; `want` the FULL expected
// file after the edit (a full-file assertion catches any offset/Join bug that
// re-emits normalized bytes); `exact_hits` records whether plain
// strings.Contains(content, old) is true (used to compute the exact-only
// baseline rate).
type editCorpusCase struct {
	Name      string `json:"name"`
	Strategy  string `json:"strategy"`
	Content   string `json:"content"`
	Old       string `json:"old"`
	New       string `json:"new"`
	Want      string `json:"want"`
	ExactHits bool   `json:"exact_hits"`
}

func loadEditCorpus(t *testing.T) []editCorpusCase {
	t.Helper()
	path := filepath.Join("testdata", "edit_corpus.jsonl")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading corpus %s: %v", path, err)
	}
	var cases []editCorpusCase
	sc := bufio.NewScanner(bytes.NewReader(b))
	// Lines can be long (multi-line snippets are JSON-escaped onto one line).
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		var c editCorpusCase
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			t.Fatalf("corpus line %d: %v", line, err)
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanning corpus: %v", err)
	}
	return cases
}

// TestEditCorpusSuccessRate is the acceptance test for the fuzzy edit cascade.
// It computes two rates over the same near-miss corpus:
//
//   - exactOnly baseline: the OLD behavior (strings.Count==1 then strings.Replace).
//     This is pinned to an explicit fraction (the "before" number).
//   - cascade: applyReplace's success rate (the "after" number), asserted >= 0.95.
//
// It also asserts the delta (cascade > baseline), which is the roadmap's
// before/after requirement, and t.Logf prints both numbers.
func TestEditCorpusSuccessRate(t *testing.T) {
	cases := loadEditCorpus(t)

	const minCases = 20
	if len(cases) < minCases {
		t.Fatalf("corpus has %d cases, need >= %d realistic near-miss cases", len(cases), minCases)
	}

	total := len(cases)
	exactOnly := 0
	cascadeOK := 0
	exactHitsDeclared := 0

	for _, c := range cases {
		c := c
		// Sanity: the declared exact_hits flag must agree with reality, so the
		// pinned baseline below is computed from the corpus, not guessed.
		realExactHit := strings.Contains(c.Content, c.Old)
		if realExactHit != c.ExactHits {
			t.Errorf("%s: exact_hits flag=%v but strings.Contains=%v", c.Name, c.ExactHits, realExactHit)
		}
		if c.ExactHits {
			exactHitsDeclared++
		}

		// Exact-only baseline: replicate the OLD edit_file behavior precisely.
		if strings.Count(c.Content, c.Old) == 1 {
			got := strings.Replace(c.Content, c.Old, c.New, 1)
			if got == c.Want {
				exactOnly++
			}
		}

		// Cascade behavior.
		t.Run(c.Name, func(t *testing.T) {
			updated, matched, ambiguous := applyReplace(c.Content, c.Old, c.New, false)
			if !matched {
				t.Fatalf("cascade did not match (ambiguous=%v) for strategy %q\ncontent=%q\nold=%q",
					ambiguous, c.Strategy, c.Content, c.Old)
			}
			if updated != c.Want {
				t.Fatalf("byte-fidelity failure for %q:\n got: %q\nwant: %q", c.Strategy, updated, c.Want)
			}
		})
		// Re-run outside the subtest to count successes for the rate assertions
		// regardless of subtest failures being fatal to the goroutine.
		if updated, matched, _ := applyReplace(c.Content, c.Old, c.New, false); matched && updated == c.Want {
			cascadeOK++
		}
	}

	exactBaseline := float64(exactOnly) / float64(total)
	cascadeRate := float64(cascadeOK) / float64(total)

	t.Logf("edit corpus: %d cases", total)
	t.Logf("  exact-only baseline (before): %d/%d = %.3f", exactOnly, total, exactBaseline)
	t.Logf("  cascade success (after):      %d/%d = %.3f", cascadeOK, total, cascadeRate)
	t.Logf("  delta (after - before):       %.3f", cascadeRate-exactBaseline)

	// (b) Pin the exact-only baseline as an explicit fraction. The exact-only
	// path succeeds exactly on the cases whose old_string is a UNIQUE verbatim
	// substring of content AND reproduces want — i.e. the declared exact_hits
	// cases. Pinning this makes the "before" number a regression-guarded
	// constant: if the corpus drifts, this assertion forces a deliberate update.
	if exactOnly != exactHitsDeclared {
		t.Errorf("exact-only baseline=%d but %d cases declared exact_hits; "+
			"baseline must equal the count of unique verbatim-substring cases",
			exactOnly, exactHitsDeclared)
	}
	wantBaseline := float64(exactHitsDeclared) / float64(total)
	if exactBaseline != wantBaseline {
		t.Errorf("exact baseline fraction = %.4f, pinned want %.4f (%d/%d)",
			exactBaseline, wantBaseline, exactHitsDeclared, total)
	}

	// (c) Cascade success rate must be >= 0.95.
	const minCascadeRate = 0.95
	if cascadeRate < minCascadeRate {
		t.Errorf("cascade success rate %.3f < required %.2f", cascadeRate, minCascadeRate)
	}

	// (d) The before/after delta: the cascade must strictly beat the baseline.
	if !(cascadeRate > exactBaseline) {
		t.Errorf("cascade rate %.3f must exceed exact-only baseline %.3f (no improvement)",
			cascadeRate, exactBaseline)
	}
}
