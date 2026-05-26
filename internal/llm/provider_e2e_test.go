package llm

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestProviderE2EDeepSeekOpenAIWireDifference(t *testing.T) {
	var mu sync.Mutex
	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		mu.Lock()
		bodies = append(bodies, append([]byte(nil), body...))
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	req := Request{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "hi"}}}},
		Thinking: ThinkingEnabled(true),
	}
	deepseek, err := NewProvider("deepseek", ProviderConfig{BaseURL: srv.URL, APIKey: "k"})
	if err != nil {
		t.Fatalf("deepseek provider: %v", err)
	}
	openai, err := NewProvider("openai-compat", ProviderConfig{BaseURL: srv.URL, APIKey: "k", DefaultModel: "test-model"})
	if err != nil {
		t.Fatalf("openai provider: %v", err)
	}

	for _, prov := range []Provider{deepseek, openai} {
		ch, err := prov.Stream(context.Background(), req)
		if err != nil {
			t.Fatalf("%s Stream: %v", prov.Name(), err)
		}
		for range ch {
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("captured %d request bodies, want 2", len(bodies))
	}
	if !bytes.Contains(bodies[0], []byte(`"thinking"`)) {
		t.Fatalf("deepseek body missing thinking: %s", bodies[0])
	}
	if bytes.Contains(bodies[1], []byte(`"thinking"`)) {
		t.Fatalf("openai body contained thinking: %s", bodies[1])
	}
}
