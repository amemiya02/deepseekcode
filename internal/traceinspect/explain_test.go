package traceinspect_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/traceinspect"
)

const minimalTrace = `{"type":"prefix.snapshot","epoch_id":"e1","static_prefix_hash":"aabbccdd","schema_version":2}
{"type":"usage","turn":1,"epoch_id":"e1","cache_hit_tokens":0,"cache_miss_tokens":5000,"output_tokens":200,"cost_cny":0.002,"schema_version":2}
{"type":"usage","turn":2,"epoch_id":"e1","cache_hit_tokens":12000,"cache_miss_tokens":200,"output_tokens":150,"cost_cny":0.0005,"schema_version":2}
{"type":"compaction","epoch_id":"e1","schema_version":2}
{"type":"usage","turn":3,"epoch_id":"e1","cache_hit_tokens":100,"cache_miss_tokens":4900,"output_tokens":180,"cost_cny":0.0019,"schema_version":2}
`

func writeTmp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "trace*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

func TestExplainFile_TurnCount(t *testing.T) {
	path := writeTmp(t, minimalTrace)
	ledger, err := traceinspect.ExplainFile(path)
	if err != nil {
		t.Fatalf("ExplainFile: %v", err)
	}
	if len(ledger) != 3 {
		t.Fatalf("want 3 turns, got %d", len(ledger))
	}
}

func TestExplainFile_Turn1_ExpectedMiss(t *testing.T) {
	path := writeTmp(t, minimalTrace)
	ledger, _ := traceinspect.ExplainFile(path)
	if ledger[0].Turn != 1 {
		t.Fatalf("want turn 1, got %d", ledger[0].Turn)
	}
	if ledger[0].Why != traceinspect.WhyExpectedMiss {
		t.Fatalf("want WhyExpectedMiss, got %q", ledger[0].Why)
	}
	if ledger[0].Evicted {
		t.Fatal("turn 1 should not be flagged evicted (it is an expected miss)")
	}
}

func TestExplainFile_Turn3_EvictionAfterCompaction(t *testing.T) {
	path := writeTmp(t, minimalTrace)
	ledger, _ := traceinspect.ExplainFile(path)
	row := ledger[2]
	if !row.Evicted {
		t.Fatalf("turn 3 (hit=%d) should be flagged evicted", row.HitTokens)
	}
	if row.Why != traceinspect.WhyCompaction {
		t.Fatalf("want WhyCompaction, got %q", row.Why)
	}
}

func TestExplainFile_Turn2_HitOK(t *testing.T) {
	path := writeTmp(t, minimalTrace)
	ledger, _ := traceinspect.ExplainFile(path)
	if ledger[1].Evicted {
		t.Fatal("turn 2 has good cache hit and should not be evicted")
	}
	if ledger[1].Why != "" {
		t.Fatalf("turn 2 should have no why label, got %q", ledger[1].Why)
	}
}

func TestRenderExplainText_Headers(t *testing.T) {
	path := writeTmp(t, minimalTrace)
	ledger, _ := traceinspect.ExplainFile(path)
	out := traceinspect.RenderExplainText(ledger)
	for _, want := range []string{"TURN", "HIT", "MISS", "EVICT", "COST", "WHY"} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderExplainText: missing header %q\noutput:\n%s", want, out)
		}
	}
}

func TestRenderExplainText_EvictMark(t *testing.T) {
	path := writeTmp(t, minimalTrace)
	ledger, _ := traceinspect.ExplainFile(path)
	out := traceinspect.RenderExplainText(ledger)
	if !strings.Contains(out, "Y") {
		t.Errorf("RenderExplainText: expected at least one eviction mark 'Y'\noutput:\n%s", out)
	}
}

func TestExplainFile_WhyDrift(t *testing.T) {
	// drift.blocked record followed by a low-hit usage must produce WhyDrift.
	trace := `{"type":"prefix.snapshot","epoch_id":"e2","static_prefix_hash":"aabbccdd","schema_version":2}
{"type":"usage","turn":1,"epoch_id":"e2","cache_hit_tokens":0,"cache_miss_tokens":5000,"output_tokens":200,"cost_cny":0.002,"schema_version":2}
{"type":"drift.blocked","epoch_id":"e2","schema_version":2}
{"type":"usage","turn":2,"epoch_id":"e2","cache_hit_tokens":100,"cache_miss_tokens":4900,"output_tokens":180,"cost_cny":0.0019,"schema_version":2}
`
	path := writeTmp(t, trace)
	ledger, err := traceinspect.ExplainFile(path)
	if err != nil {
		t.Fatalf("ExplainFile: %v", err)
	}
	if len(ledger) != 2 {
		t.Fatalf("want 2 turns, got %d", len(ledger))
	}
	row := ledger[1]
	if !row.Evicted {
		t.Fatalf("turn 2 (hit=%d) should be flagged evicted", row.HitTokens)
	}
	if row.Why != traceinspect.WhyDrift {
		t.Fatalf("want WhyDrift, got %q", row.Why)
	}
}

func TestExplainFile_WhyEviction(t *testing.T) {
	// Bare low-hit turn with no preceding compaction or drift record must produce WhyEviction.
	trace := `{"type":"prefix.snapshot","epoch_id":"e3","static_prefix_hash":"aabbccdd","schema_version":2}
{"type":"usage","turn":1,"epoch_id":"e3","cache_hit_tokens":0,"cache_miss_tokens":5000,"output_tokens":200,"cost_cny":0.002,"schema_version":2}
{"type":"usage","turn":2,"epoch_id":"e3","cache_hit_tokens":50,"cache_miss_tokens":4950,"output_tokens":180,"cost_cny":0.0019,"schema_version":2}
`
	path := writeTmp(t, trace)
	ledger, err := traceinspect.ExplainFile(path)
	if err != nil {
		t.Fatalf("ExplainFile: %v", err)
	}
	if len(ledger) != 2 {
		t.Fatalf("want 2 turns, got %d", len(ledger))
	}
	row := ledger[1]
	if !row.Evicted {
		t.Fatalf("turn 2 (hit=%d) should be flagged evicted", row.HitTokens)
	}
	if row.Why != traceinspect.WhyEviction {
		t.Fatalf("want WhyEviction, got %q", row.Why)
	}
}

func TestExplainFile_EvictionThresholdBoundary(t *testing.T) {
	// A turn with cache_hit_tokens == EvictionThreshold must be flagged Evicted==true.
	// This pins the <= boundary so a change to < would be caught.
	trace := `{"type":"prefix.snapshot","epoch_id":"e4","static_prefix_hash":"aabbccdd","schema_version":2}
{"type":"usage","turn":1,"epoch_id":"e4","cache_hit_tokens":0,"cache_miss_tokens":5000,"output_tokens":200,"cost_cny":0.002,"schema_version":2}
{"type":"usage","turn":2,"epoch_id":"e4","cache_hit_tokens":9000,"cache_miss_tokens":1000,"output_tokens":150,"cost_cny":0.001,"schema_version":2}
`
	path := writeTmp(t, trace)
	ledger, err := traceinspect.ExplainFile(path)
	if err != nil {
		t.Fatalf("ExplainFile: %v", err)
	}
	if len(ledger) != 2 {
		t.Fatalf("want 2 turns, got %d", len(ledger))
	}
	row := ledger[1]
	if row.HitTokens != traceinspect.EvictionThreshold {
		t.Fatalf("expected HitTokens==%d, got %d", traceinspect.EvictionThreshold, row.HitTokens)
	}
	if !row.Evicted {
		t.Fatalf("turn with cache_hit_tokens==%d (==EvictionThreshold) should be flagged Evicted", traceinspect.EvictionThreshold)
	}
}

func TestExplainFile_NotFound(t *testing.T) {
	_, err := traceinspect.ExplainFile(filepath.Join(t.TempDir(), "no.jsonl"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
