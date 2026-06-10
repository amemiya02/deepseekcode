package h2h

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDscTrace(t *testing.T) {
	// Fixture uses the real dsc -trace-jsonl shape: usage counters are
	// flat top-level fields, selected by type=="usage".
	trace := `{"type":"prefix.snapshot","turn":1,"epoch_id":"e1"}
{"type":"usage","turn":1,"epoch_id":"e1","cache_hit_tokens":7800,"cache_miss_tokens":120,"output_tokens":450}
{"type":"prefix.snapshot","turn":2,"epoch_id":"e1"}
{"type":"usage","turn":2,"epoch_id":"e1","cache_hit_tokens":8200,"cache_miss_tokens":25000,"output_tokens":900}
`
	p := filepath.Join(t.TempDir(), "trace.jsonl")
	os.WriteFile(p, []byte(trace), 0o644)
	turns, err := ParseDscTrace(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 {
		t.Fatalf("want 2 usage turns, got %d", len(turns))
	}
	if turns[0].HitTokens != 7800 || turns[0].MissTokens != 120 || turns[0].OutTokens != 450 {
		t.Fatalf("bad parse turn 0: %+v", turns[0])
	}
	if turns[1].HitTokens != 8200 || turns[1].MissTokens != 25000 || turns[1].OutTokens != 900 {
		t.Fatalf("bad parse turn 1: %+v", turns[1])
	}
}

func TestParseDscTraceGoldenPassComplete(t *testing.T) {
	// Exercise against the real golden trace with subagent turns.
	// Only root+subagent usage lines should be extracted (6 total across
	// the three golden files, but this file has 3 usage lines).
	p := filepath.Join("..", "golden-traces", "pass-complete-subagent.jsonl")
	turns, err := ParseDscTrace(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 3 {
		t.Fatalf("want 3 usage turns, got %d", len(turns))
	}
	// First turn: miss=1000, hit=0
	if turns[0].MissTokens != 1000 || turns[0].HitTokens != 0 {
		t.Fatalf("bad turn 0: %+v", turns[0])
	}
	// Second turn: hit=1000, miss=0
	if turns[1].HitTokens != 1000 || turns[1].MissTokens != 0 {
		t.Fatalf("bad turn 1: %+v", turns[1])
	}
	// Third turn (subagent): miss=100
	if turns[2].MissTokens != 100 || turns[2].OutTokens != 3 {
		t.Fatalf("bad turn 2: %+v", turns[2])
	}
}

func TestParseDscTraceGoldenMalformed(t *testing.T) {
	// The malformed-usage golden trace has a missing epoch_id on turn 1;
	// parser should still extract usage (it only checks type=="usage").
	p := filepath.Join("..", "golden-traces", "fail-malformed-usage.jsonl")
	turns, err := ParseDscTrace(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 {
		t.Fatalf("want 2 usage turns, got %d", len(turns))
	}
}

func TestParseDscTraceNonexistent(t *testing.T) {
	_, err := ParseDscTrace("/nonexistent/trace.jsonl")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}
