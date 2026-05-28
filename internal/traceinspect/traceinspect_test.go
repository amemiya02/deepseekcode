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

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
