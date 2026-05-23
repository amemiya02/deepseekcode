package llm

import (
	"bytes"
	"strings"
	"testing"
)

func TestThinkingSerializesAsStruct(t *testing.T) {
	req := Request{
		Model:    "deepseek-v4-flash",
		Messages: []Message{{Role: "user", Content: "hi"}},
		Thinking: ThinkingEnabled(true),
	}
	b, err := req.MarshalCacheStable()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(b, []byte(`"thinking":{"type":"enabled"}`)) {
		t.Fatalf("thinking shape wrong; got: %s", b)
	}
}

func TestThinkingNilOmitted(t *testing.T) {
	req := Request{
		Model:    "deepseek-v4-flash",
		Messages: []Message{{Role: "user", Content: "hi"}},
		Thinking: ThinkingEnabled(false), // nil
	}
	b, err := req.MarshalCacheStable()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), `"thinking"`) {
		t.Fatalf("expected no thinking field when disabled; got: %s", b)
	}
}
