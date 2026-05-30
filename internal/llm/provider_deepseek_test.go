package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestDeepSeekProviderCapabilities(t *testing.T) {
	prov, err := NewProvider("deepseek", ProviderConfig{BaseURL: "http://x", APIKey: "k"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if prov == nil {
		t.Fatal("provider is nil")
	}
	caps := prov.Capabilities()
	if !caps.Thinking || !caps.PrefixCache || !caps.JSONMode {
		t.Fatalf("capabilities = %#v, want thinking/prefix/json true", caps)
	}
	if caps.MaxContextTokens != 1_000_000 {
		t.Errorf("MaxContextTokens = %d, want 1000000", caps.MaxContextTokens)
	}
	if caps.MaxOutputTokens != 384_000 {
		t.Errorf("MaxOutputTokens = %d, want 384000", caps.MaxOutputTokens)
	}
	if len(caps.ReasoningEfforts) != 4 {
		t.Errorf("ReasoningEfforts len = %d, want 4", len(caps.ReasoningEfforts))
	}
	if !caps.FIM {
		t.Error("FIM should be true for DeepSeek")
	}
	if !caps.FIMRequiresThinkingOff {
		t.Error("FIMRequiresThinkingOff should be true for DeepSeek")
	}
	if !caps.ChatPrefixCompletion {
		t.Error("ChatPrefixCompletion should be true for DeepSeek")
	}
	if prov.BaseClient() == nil {
		t.Fatal("BaseClient() is nil")
	}
}

func TestDeepSeekProviderStreamSmoke(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	req := Request{Model: "deepseek-v4", Messages: []Message{{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "hi"}}}}}
	client := NewClient("k", srv.URL)
	prov, err := NewProvider("deepseek", ProviderConfig{BaseURL: srv.URL, APIKey: "k"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	want, err := collectEvents(client.Stream(context.Background(), req))
	if err != nil {
		t.Fatalf("client.Stream: %v", err)
	}
	got, err := collectEvents(prov.Stream(context.Background(), req))
	if err != nil {
		t.Fatalf("prov.Stream: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("provider events = %#v, want %#v", got, want)
	}
}

func collectEvents(ch <-chan Event, err error) ([]Event, error) {
	if err != nil {
		return nil, err
	}
	var out []Event
	for ev := range ch {
		out = append(out, ev)
	}
	return out, nil
}
