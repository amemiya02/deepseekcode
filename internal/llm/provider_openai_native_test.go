package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func fixedOpenAINativeRequest() Request {
	temp := 0.0
	return Request{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: "system", Blocks: []ContentBlock{TextBlock{Text: "You are a helpful assistant."}}},
			{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "What is 2+2?"}}},
		},
		MaxTokens:   256,
		Temperature: &temp,
		Stream:      true,
	}
}

func TestOpenAINativeMarshalShape(t *testing.T) {
	req := fixedOpenAINativeRequest()
	b, err := openaiNativeMarshal(req)
	if err != nil {
		t.Fatalf("openaiNativeMarshal: %v", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(b, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// must contain stream:true
	if s, _ := wire["stream"].(bool); !s {
		t.Errorf("expected stream:true, got %v", wire["stream"])
	}

	// must NOT contain reasoning_effort, cache_control, prefix
	for _, forbidden := range []string{"reasoning_effort", "cache_control", "prefix"} {
		if _, has := wire[forbidden]; has {
			t.Errorf("wire must not contain %q", forbidden)
		}
	}

	// must contain messages array
	msgs, ok := wire["messages"].([]any)
	if !ok || len(msgs) == 0 {
		t.Fatalf("expected non-empty messages array")
	}

	// system message stays inside messages (OpenAI style)
	first := msgs[0].(map[string]any)
	if first["role"] != "system" {
		t.Errorf("expected first message role=system, got %v", first["role"])
	}

	// max_tokens present
	if wire["max_tokens"] == nil {
		t.Errorf("expected max_tokens")
	}
}

func TestOpenAINativeMarshalGolden(t *testing.T) {
	golden, err := os.ReadFile("testdata/openai_native_marshal_golden.json")
	if err != nil {
		t.Fatalf("golden file missing: %v", err)
	}

	req := fixedOpenAINativeRequest()
	got, err := openaiNativeMarshal(req)
	if err != nil {
		t.Fatalf("openaiNativeMarshal: %v", err)
	}

	// Strip optional trailing newline from the golden file (POSIX convention)
	// so that adding a newline to the file does not cause spurious failures.
	golden = bytes.TrimRight(golden, "\n")
	if !bytes.Equal(got, golden) {
		t.Fatalf("OpenAI Native wire bytes changed!\ngot  %d bytes\nwant %d bytes\nfirst diff at byte %d",
			len(got), len(golden), firstDiff(got, golden))
	}
}

func TestOpenAINativeGenGolden(t *testing.T) {
	if os.Getenv("GEN_GOLDEN") == "" {
		t.Skip("set GEN_GOLDEN=1 to regenerate")
	}
	req := fixedOpenAINativeRequest()
	b, err := openaiNativeMarshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("testdata/openai_native_marshal_golden.json", append(b, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d bytes", len(b))
}

func TestOpenAINativeProviderStream(t *testing.T) {
	// OpenAI SSE: data: {"choices":[{"delta":{"content":"4"}}]}
	sse := strings.Join([]string{
		`data: {"id":"x","object":"chat.completion.chunk","choices":[{"delta":{"content":"4"},"finish_reason":null}]}`,
		``,
		`data: {"id":"x","object":"chat.completion.chunk","choices":[{"delta":{},"finish_reason":"stop"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("missing Authorization header")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sse)
	}))
	defer srv.Close()

	p, err := NewProvider("openai", ProviderConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	ctx := context.Background()
	ch, err := p.Stream(ctx, Request{
		Model:     "gpt-4o",
		Messages:  []Message{{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "2+2?"}}}},
		MaxTokens: 16,
		Stream:    true,
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	// Collect all events by type.
	byType := map[EventType][]Event{}
	for ev := range ch {
		byType[ev.Type] = append(byType[ev.Type], ev)
	}

	// Must have at least one text delta.
	if len(byType[EventTextDelta]) == 0 {
		t.Error("expected at least one EventTextDelta")
	}

	// Must have exactly one EventFinish with FinishReason=="stop".
	finishes := byType[EventFinish]
	if len(finishes) != 1 {
		t.Errorf("expected exactly 1 EventFinish, got %d", len(finishes))
	} else if finishes[0].FinishReason != "stop" {
		t.Errorf("expected EventFinish.FinishReason=stop, got %q", finishes[0].FinishReason)
	}

	// Must have no EventError.
	if errs := byType[EventError]; len(errs) != 0 {
		t.Errorf("expected no EventError, got %d: first=%v", len(errs), errs[0].Err)
	}
}
