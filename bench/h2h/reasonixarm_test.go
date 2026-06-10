package h2h

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseReasonixSessionUsageFallback(t *testing.T) {
	jl := `{"role":"assistant","usage":{"prompt_cache_hit_tokens":900,"prompt_cache_miss_tokens":40,"completion_tokens":120}}
{"role":"user"}
{"role":"assistant","usage":{"prompt_cache_hit_tokens":1100,"prompt_cache_miss_tokens":10,"completion_tokens":80}}
`
	p := filepath.Join(t.TempDir(), "x.session.jsonl")
	os.WriteFile(p, []byte(jl), 0o644)
	turns, err := ParseReasonixSession(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || turns[0].HitTokens != 900 || turns[1].OutTokens != 80 {
		t.Fatalf("bad parse: %+v", turns)
	}
}
