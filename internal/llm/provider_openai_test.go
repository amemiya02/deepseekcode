package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAICompatCapabilities(t *testing.T) {
	prov, err := NewProvider("openai-compat", ProviderConfig{BaseURL: "http://x", APIKey: "k", DefaultModel: "gpt-4o"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	caps := prov.Capabilities()
	if caps.Thinking || caps.PrefixCache || !caps.JSONMode {
		t.Fatalf("capabilities = %#v, want thinking/prefix false and json true", caps)
	}
	if caps.FIM {
		t.Error("FIM should be false for OpenAI-compat")
	}
	if caps.ReasoningEfforts != nil {
		t.Errorf("ReasoningEfforts should be nil for OpenAI-compat, got %v", caps.ReasoningEfforts)
	}
	if caps.ChatPrefixCompletion {
		t.Error("ChatPrefixCompletion should be false for OpenAI-compat")
	}
}

func TestOpenAICompatStreamDropsThinking(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		body, err = ioReadAll(r)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	prov, err := NewProvider("openai-compat", ProviderConfig{BaseURL: srv.URL, APIKey: "k", DefaultModel: "gpt-4o"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ch, err := prov.Stream(context.Background(), Request{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "hi"}}}},
		Thinking: ThinkingEnabled(true),
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	for range ch {
	}
	if bytes.Contains(body, []byte(`"thinking"`)) {
		t.Fatalf("OpenAI compat request contained thinking: %s", body)
	}
}

func TestOpenAICompatValidatePro(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"approve":true,"reasoning":"ok"}`}},
			},
		})
	}))
	defer srv.Close()

	prov, err := NewProvider("openai-compat", ProviderConfig{BaseURL: srv.URL, APIKey: "k", DefaultModel: "gpt-4o"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	approve, reasoning, err := prov.ValidatePro(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("ValidatePro: %v", err)
	}
	if !approve || reasoning != "ok" {
		t.Fatalf("ValidatePro = (%v, %q), want (true, ok)", approve, reasoning)
	}
}

func ioReadAll(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r.Body)
	return buf.Bytes(), err
}
