package traceinspect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectTraceSummarizesEpochUsageAndSubagents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	writeFile(t, path, strings.Join([]string{
		`{"type":"prefix.snapshot","turn":1,"epoch_id":"e1","static_prefix_hash":"abcdef123456","agent_role":"root"}`,
		`{"type":"usage","turn":1,"epoch_id":"e1","cache_hit_tokens":0,"cache_miss_tokens":1000,"output_tokens":12,"cost_cny":0.001,"agent_role":"root"}`,
		`{"type":"prefix.snapshot","turn":2,"epoch_id":"e1","static_prefix_hash":"abcdef123456","agent_role":"root"}`,
		`{"type":"usage","turn":2,"epoch_id":"e1","cache_hit_tokens":950,"cache_miss_tokens":50,"output_tokens":10,"cost_cny":0.0002,"agent_role":"root"}`,
		`{"type":"prefix.snapshot","turn":1,"epoch_id":"c1","static_prefix_hash":"childhash","agent_role":"subagent","parent_epoch_id":"e1"}`,
		`{"type":"agent.done","epoch_id":"c1","agent_role":"subagent","parent_epoch_id":"e1","reason":"model_done"}`,
	}, "\n"))

	report, err := InspectFile(path)
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}
	if report.TotalUsageTurns != 2 {
		t.Fatalf("TotalUsageTurns = %d, want 2", report.TotalUsageTurns)
	}
	if report.CacheHitRate < 0.47 || report.CacheHitRate > 0.48 {
		t.Fatalf("CacheHitRate = %.4f, want about 0.475", report.CacheHitRate)
	}
	if report.SubagentEpochs != 1 {
		t.Fatalf("SubagentEpochs = %d, want 1", report.SubagentEpochs)
	}
	if got := RenderText(report); !strings.Contains(got, "cache 47.5%") || !strings.Contains(got, "subagents 1") {
		t.Fatalf("RenderText missing expected summary:\n%s", got)
	}
}

// TestInspectSurfacesLifecycleAndProjectedCost pins the T6.4 additions: the
// inspector now counts compaction/drift/pending records (previously ignored,
// so the summary looked clean even when the gate would fail) and sums the
// budget gate's projected cost for a realized-vs-projected view.
func TestInspectSurfacesLifecycleAndProjectedCost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	writeFile(t, path, strings.Join([]string{
		`{"type":"prefix.snapshot","epoch_id":"e1","static_prefix_hash":"h","agent_role":"root"}`,
		`{"type":"usage","epoch_id":"e1","cache_hit_tokens":900,"cache_miss_tokens":100,"output_tokens":20,"cost_cny":0.002,"agent_role":"root"}`,
		`{"type":"budget.warning","model":"deepseek-v4-flash","projected_cny":0.05}`,
		`{"type":"compaction","before_static_prefix_hash":"h","after_static_prefix_hash":"h"}`,
		`{"type":"drift.blocked","epoch_id":"e1","which":"tools"}`,
		`{"type":"pending_change","epoch_id":"e1","kind":"skill_body_changed"}`,
		`{"type":"agent.done","epoch_id":"e1","agent_role":"root","reason":"model_done"}`,
	}, "\n"))

	rep, err := InspectFile(path)
	if err != nil {
		t.Fatalf("InspectFile: %v", err)
	}
	if rep.CompactionCount != 1 || rep.DriftBlockedCount != 1 || rep.PendingChangeCount != 1 {
		t.Errorf("lifecycle counts = compaction %d / drift %d / pending %d, want 1/1/1",
			rep.CompactionCount, rep.DriftBlockedCount, rep.PendingChangeCount)
	}
	if rep.BudgetEvents != 1 || rep.ProjectedCNY < 0.0499 || rep.ProjectedCNY > 0.0501 {
		t.Errorf("projected = ¥%.4f over %d events, want ¥0.05 over 1", rep.ProjectedCNY, rep.BudgetEvents)
	}
	out := RenderText(rep)
	for _, want := range []string{
		"lifecycle: compaction 1 | drift.blocked 1 | pending 1",
		"cost realized ¥0.002000 vs projected ¥0.050000",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderText missing %q:\n%s", want, out)
		}
	}
}

// TestInspectCacheEvidenceStablePrefix verifies that a stable-prefix run
// reports exactly 1 unique prefix hash and 1 expected cache miss.
func TestInspectCacheEvidenceStablePrefix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	writeFile(t, path, strings.Join([]string{
		`{"type":"prefix.snapshot","epoch_id":"e1","static_prefix_hash":"abc123","agent_role":"root"}`,
		`{"type":"usage","epoch_id":"e1","model":"deepseek-v4-flash","cache_hit_tokens":0,"cache_miss_tokens":1000,"output_tokens":10,"cost_cny":0.001}`,
		`{"type":"prefix.snapshot","epoch_id":"e1","static_prefix_hash":"abc123","agent_role":"root"}`,
		`{"type":"usage","epoch_id":"e1","model":"deepseek-v4-flash","cache_hit_tokens":950,"cache_miss_tokens":50,"output_tokens":10,"cost_cny":0.0002}`,
		`{"type":"agent.done","epoch_id":"e1","reason":"model_done"}`,
	}, "\n"))

	rep, err := InspectFile(path)
	if err != nil {
		t.Fatalf("InspectFile: %v", err)
	}
	if rep.UniquePrefixHashes != 1 {
		t.Errorf("UniquePrefixHashes = %d, want 1", rep.UniquePrefixHashes)
	}
	if rep.ExpectedCacheMisses != 1 {
		t.Errorf("ExpectedCacheMisses = %d, want 1", rep.ExpectedCacheMisses)
	}
	out := RenderText(rep)
	if !strings.Contains(out, "prefixes 1") {
		t.Errorf("RenderText missing 'prefixes 1':\n%s", out)
	}
	if !strings.Contains(out, "expected_miss 1") {
		t.Errorf("RenderText missing 'expected_miss 1':\n%s", out)
	}
}

// TestInspectCacheEvidenceDriftedPrefix verifies that a drifted-prefix run
// reports 2+ unique prefix hashes.
func TestInspectCacheEvidenceDriftedPrefix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	writeFile(t, path, strings.Join([]string{
		`{"type":"prefix.snapshot","epoch_id":"e1","static_prefix_hash":"abc123","agent_role":"root"}`,
		`{"type":"usage","epoch_id":"e1","model":"deepseek-v4-flash","cache_hit_tokens":0,"cache_miss_tokens":1000,"output_tokens":10,"cost_cny":0.001}`,
		`{"type":"prefix.snapshot","epoch_id":"e1","static_prefix_hash":"def456","agent_role":"root"}`,
		`{"type":"usage","epoch_id":"e1","model":"deepseek-v4-flash","cache_hit_tokens":0,"cache_miss_tokens":1000,"output_tokens":10,"cost_cny":0.001}`,
		`{"type":"agent.done","epoch_id":"e1","reason":"model_done"}`,
	}, "\n"))

	rep, err := InspectFile(path)
	if err != nil {
		t.Fatalf("InspectFile: %v", err)
	}
	if rep.UniquePrefixHashes != 2 {
		t.Errorf("UniquePrefixHashes = %d, want 2", rep.UniquePrefixHashes)
	}
	out := RenderText(rep)
	if !strings.Contains(out, "prefixes 2") {
		t.Errorf("RenderText missing 'prefixes 2':\n%s", out)
	}
}

// TestInspectCacheSavings verifies that cache savings are computed from
// the usage hit/miss token split.
func TestInspectCacheSavings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	writeFile(t, path, strings.Join([]string{
		`{"type":"prefix.snapshot","epoch_id":"e1","static_prefix_hash":"abc","agent_role":"root"}`,
		`{"type":"usage","epoch_id":"e1","model":"deepseek-v4-flash","cache_hit_tokens":48700000,"cache_miss_tokens":190000,"output_tokens":100,"cost_cny":0.01}`,
		`{"type":"agent.done","epoch_id":"e1","reason":"model_done"}`,
	}, "\n"))

	rep, err := InspectFile(path)
	if err != nil {
		t.Fatalf("InspectFile: %v", err)
	}
	if rep.CacheSavingsCNY <= 0 {
		t.Errorf("CacheSavingsCNY = %.4f, want > 0", rep.CacheSavingsCNY)
	}
	out := RenderText(rep)
	if !strings.Contains(out, "saved CNY") {
		t.Errorf("RenderText missing 'saved CNY':\n%s", out)
	}
}

// TestInspectRegradesGoldenFixtures is the offline re-grade: the inspector
// reproduces a summary for every committed golden fixture without re-running
// the loop or burning tokens (T6.4). A fixture that fails to parse is itself a
// reproducible (fail) grade, not a test failure.
func TestInspectRegradesGoldenFixtures(t *testing.T) {
	dir := filepath.Join("..", "..", "bench", "golden-traces")
	fixtures := []string{
		"pass-complete-subagent.jsonl",
		"fail-compaction-moved-prefix.jsonl",
		"fail-malformed-usage.jsonl",
		"fail-split-subagent.jsonl",
	}
	graded := 0
	for _, f := range fixtures {
		t.Run(f, func(t *testing.T) {
			rep, err := InspectFile(filepath.Join(dir, f))
			if err != nil {
				t.Logf("%s: parse-fail grade reproduced offline: %v", f, err)
				return
			}
			graded++
			if got := RenderText(rep); !strings.Contains(got, "lifecycle:") {
				t.Errorf("%s: RenderText missing lifecycle line:\n%s", f, got)
			}
		})
	}
	if graded == 0 {
		t.Fatal("no golden fixture re-graded successfully; expected at least the pass fixture")
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

// TestInspectFilePrefixReasons verifies that prefix.snapshot Reason values
// are aggregated and rendered in the output.
func TestInspectFilePrefixReasons(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	writeFile(t, path, strings.Join([]string{
		`{"type":"prefix.snapshot","epoch_id":"e1","static_prefix_hash":"h","reason":"session_start"}`,
		`{"type":"prefix.snapshot","epoch_id":"e1","static_prefix_hash":"h","reason":"epoch_switched"}`,
		`{"type":"prefix.snapshot","epoch_id":"e1","static_prefix_hash":"h","reason":"epoch_switched"}`,
		`{"type":"usage","epoch_id":"e1","cache_hit_tokens":0,"cache_miss_tokens":100,"output_tokens":5,"cost_cny":0.0001}`,
		`{"type":"agent.done","epoch_id":"e1","reason":"model_done"}`,
	}, "\n"))

	rep, err := InspectFile(path)
	if err != nil {
		t.Fatalf("InspectFile: %v", err)
	}
	if len(rep.PrefixReasons) != 2 {
		t.Fatalf("PrefixReasons len = %d, want 2", len(rep.PrefixReasons))
	}
	// epoch_switched=2 should come first (descending count).
	if rep.PrefixReasons[0].Reason != "epoch_switched" || rep.PrefixReasons[0].Count != 2 {
		t.Errorf("PrefixReasons[0] = %+v, want epoch_switched=2", rep.PrefixReasons[0])
	}
	if rep.PrefixReasons[1].Reason != "session_start" || rep.PrefixReasons[1].Count != 1 {
		t.Errorf("PrefixReasons[1] = %+v, want session_start=1", rep.PrefixReasons[1])
	}
	out := RenderText(rep)
	if !strings.Contains(out, "cache reasons: epoch_switched=2 session_start=1") {
		t.Errorf("RenderText missing prefix reasons line:\n%s", out)
	}
}

// TestInspectFileRepairSummary verifies that repair records are aggregated
// by kind and by tool.
func TestInspectFileRepairSummary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	writeFile(t, path, strings.Join([]string{
		`{"type":"prefix.snapshot","epoch_id":"e1","static_prefix_hash":"h"}`,
		`{"type":"usage","epoch_id":"e1","cache_hit_tokens":0,"cache_miss_tokens":100,"output_tokens":5,"cost_cny":0.0001}`,
		`{"type":"repair","kind":"args_completed","tool":"read_file","call_id":"c1"}`,
		`{"type":"repair","kind":"args_completed","tool":"read_file","call_id":"c2"}`,
		`{"type":"repair","kind":"recovered","tool":"grep","call_id":"c3"}`,
		`{"type":"agent.done","epoch_id":"e1","reason":"model_done"}`,
	}, "\n"))

	rep, err := InspectFile(path)
	if err != nil {
		t.Fatalf("InspectFile: %v", err)
	}
	if len(rep.RepairKinds) != 2 {
		t.Fatalf("RepairKinds len = %d, want 2", len(rep.RepairKinds))
	}
	// args_completed=2 first (descending count).
	if rep.RepairKinds[0].Name != "args_completed" || rep.RepairKinds[0].Count != 2 {
		t.Errorf("RepairKinds[0] = %+v, want args_completed=2", rep.RepairKinds[0])
	}
	if rep.RepairKinds[1].Name != "recovered" || rep.RepairKinds[1].Count != 1 {
		t.Errorf("RepairKinds[1] = %+v, want recovered=1", rep.RepairKinds[1])
	}
	if len(rep.RepairTools) != 2 {
		t.Fatalf("RepairTools len = %d, want 2", len(rep.RepairTools))
	}
	if rep.RepairTools[0].Name != "read_file" || rep.RepairTools[0].Count != 2 {
		t.Errorf("RepairTools[0] = %+v, want read_file=2", rep.RepairTools[0])
	}
	out := RenderText(rep)
	if !strings.Contains(out, "repairs: args_completed=2 recovered=1") {
		t.Errorf("RenderText missing repairs line:\n%s", out)
	}
	if !strings.Contains(out, "repair tools: read_file=2 grep=1") {
		t.Errorf("RenderText missing repair tools line:\n%s", out)
	}
}
