package main

import (
	"os"
	"strings"
	"testing"
)

const cacheTestTrace = `{"type":"prefix.snapshot","epoch_id":"e1","static_prefix_hash":"aabb","schema_version":2}
{"type":"usage","turn":1,"epoch_id":"e1","cache_hit_tokens":0,"cache_miss_tokens":4000,"output_tokens":100,"cost_cny":0.001,"schema_version":2}
{"type":"usage","turn":2,"epoch_id":"e1","cache_hit_tokens":3800,"cache_miss_tokens":200,"output_tokens":80,"cost_cny":0.0003,"schema_version":2}
`

func TestRunCacheExplain_Smoke(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "ct*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(cacheTestTrace)
	f.Close()

	out, err := runCacheExplain([]string{f.Name()})
	if err != nil {
		t.Fatalf("runCacheExplain: %v", err)
	}
	for _, want := range []string{"TURN", "HIT", "MISS", "EVICT", "WHY"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}

func TestRunCacheExplain_MissingFile(t *testing.T) {
	_, err := runCacheExplain([]string{"/no/such/file.jsonl"})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestRunCacheExplain_NoArgs(t *testing.T) {
	_, err := runCacheExplain([]string{})
	if err == nil {
		t.Fatal("expected usage error when no args given")
	}
}

func TestRunCache_UnknownSubcmd(t *testing.T) {
	err := runCache([]string{"frob"})
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
}
