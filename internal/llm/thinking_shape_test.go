package llm

import (
	"bytes"
	"strings"
	"testing"
)

func TestThinkingSerializesAsStruct(t *testing.T) {
	req := Request{
		Model:    "deepseek-v4-flash",
		Messages: []Message{{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "hi"}}}},
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

// TestReasoningContentRoundtrips pins the fix for DeepSeek's 400
// "reasoning_content in the thinking mode must be passed back to the
// API." The assistant message's reasoning_content must serialize when
// non-empty so the model can continue the next turn coherently. When
// empty (thinking off), omitempty drops it.
func TestReasoningContentRoundtrips(t *testing.T) {
	req := Request{
		Model: "deepseek-v4-flash",
		Messages: []Message{
			{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "hi"}}},
			{Role: "assistant", Blocks: []ContentBlock{
				ThinkingBlock{Text: "let me think"},
				TextBlock{Text: "ok"},
			}},
		},
	}
	b, err := req.MarshalCacheStable()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(b, []byte(`"reasoning_content":"let me think"`)) {
		t.Fatalf("reasoning_content missing from serialization: %s", b)
	}
}

func TestReasoningContentOmittedWhenEmpty(t *testing.T) {
	req := Request{
		Model: "deepseek-v4-flash",
		Messages: []Message{
			{Role: "assistant", Blocks: []ContentBlock{TextBlock{Text: "ok"}}},
		},
	}
	b, err := req.MarshalCacheStable()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), `"reasoning_content"`) {
		t.Fatalf("expected reasoning_content omitted when empty; got: %s", b)
	}
}

func TestThinkingNilOmitted(t *testing.T) {
	req := Request{
		Model:    "deepseek-v4-flash",
		Messages: []Message{{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "hi"}}}},
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

func TestReasoningEffortMaxSerializes(t *testing.T) {
	req := Request{
		Model:           "deepseek-v4-flash",
		Messages:        []Message{{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "hi"}}}},
		Thinking:        ThinkingEnabled(true),
		ReasoningEffort: ReasoningEffortMax,
	}
	b, err := req.MarshalCacheStable()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(b, []byte(`"reasoning_effort":"max"`)) {
		t.Fatalf("expected reasoning_effort:max; got: %s", b)
	}
	if !bytes.Contains(b, []byte(`"thinking":{"type":"enabled"}`)) {
		t.Fatalf("expected thinking enabled; got: %s", b)
	}
}

func TestReasoningEffortEmptyOmitted(t *testing.T) {
	req := Request{
		Model:    "deepseek-v4-flash",
		Messages: []Message{{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "hi"}}}},
		Thinking: ThinkingEnabled(true),
	}
	b, err := req.MarshalCacheStable()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), `"reasoning_effort"`) {
		t.Fatalf("expected reasoning_effort omitted when empty; got: %s", b)
	}
}

func TestParseReasoningEffort(t *testing.T) {
	tests := []struct {
		input string
		want  ReasoningEffort
		ok    bool
	}{
		{"low", ReasoningEffortLow, true},
		{"medium", ReasoningEffortMedium, true},
		{"high", ReasoningEffortHigh, true},
		{"max", ReasoningEffortMax, true},
		{" low ", ReasoningEffortLow, true},
		{"  high  ", ReasoningEffortHigh, true},
		{"", "", false},
		{"  ", "", false},
		{"MAX", "", false},
		{"High", "", false},
		{"extreme", "", false},
		{"low\n", ReasoningEffortLow, true},
	}
	for _, tt := range tests {
		got, ok := ParseReasoningEffort(tt.input)
		if ok != tt.ok || got != tt.want {
			t.Errorf("ParseReasoningEffort(%q) = (%q, %v); want (%q, %v)", tt.input, got, ok, tt.want, tt.ok)
		}
	}
}

func TestReasoningEffortValid(t *testing.T) {
	if !ReasoningEffortMax.Valid() {
		t.Error("expected max to be valid")
	}
	if ReasoningEffort("invalid").Valid() {
		t.Error("expected invalid to be invalid")
	}
	if ReasoningEffort("").Valid() {
		t.Error("expected empty to be invalid")
	}
}

func TestReasoningEffortString(t *testing.T) {
	if got := ReasoningEffortMax.String(); got != "max" {
		t.Errorf("expected max, got %q", got)
	}
	if got := ReasoningEffort("").String(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestUserIDSerializesWhenSet(t *testing.T) {
	req := Request{
		Model:    "deepseek-v4-flash",
		Messages: []Message{{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "hi"}}}},
		UserID:   "u-123",
	}
	b, err := req.MarshalCacheStable()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(b, []byte(`"user_id":"u-123"`)) {
		t.Fatalf("expected user_id:u-123; got: %s", b)
	}
}

func TestUserIDOmittedWhenEmpty(t *testing.T) {
	req := Request{
		Model:    "deepseek-v4-flash",
		Messages: []Message{{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "hi"}}}},
	}
	b, err := req.MarshalCacheStable()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), `"user_id"`) {
		t.Fatalf("expected user_id omitted when empty; got: %s", b)
	}
}
