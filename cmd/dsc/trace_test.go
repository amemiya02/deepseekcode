package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunTraceInspect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	writeTestFile(t, path, strings.Join([]string{
		`{"type":"prefix.snapshot","turn":1,"epoch_id":"e1","static_prefix_hash":"abcdef123456","agent_role":"root"}`,
		`{"type":"usage","turn":1,"epoch_id":"e1","cache_hit_tokens":0,"cache_miss_tokens":100,"output_tokens":5,"cost_cny":0.001,"agent_role":"root"}`,
	}, "\n"))

	out, err := runTraceInspect([]string{path})
	if err != nil {
		t.Fatalf("runTraceInspect returned error: %v", err)
	}
	if !strings.Contains(out, "cache 0.0%") || !strings.Contains(out, "epochs root 1") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
