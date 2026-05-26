package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWebSearchTool_Name(t *testing.T) {
	tool := NewWebSearchTool(NewDuckDuckGoHTML())
	if tool.Name() != "web_search" {
		t.Errorf("expected name 'web_search', got %q", tool.Name())
	}
}

func TestWebSearchTool_IsReadOnly(t *testing.T) {
	tool := NewWebSearchTool(NewDuckDuckGoHTML())
	if !tool.IsReadOnly() {
		t.Error("web_search should be read-only")
	}
}

func TestWebSearchTool_Parameters(t *testing.T) {
	tool := NewWebSearchTool(NewDuckDuckGoHTML())
	params := tool.Parameters()

	var schema map[string]any
	if err := json.Unmarshal(params, &schema); err != nil {
		t.Fatalf("failed to parse parameters: %v", err)
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties in schema")
	}

	if _, ok := props["q"]; !ok {
		t.Error("expected 'q' property")
	}
	if _, ok := props["limit"]; !ok {
		t.Error("expected 'limit' property")
	}
}

func TestWebSearchTool_MissingQuery(t *testing.T) {
	tool := NewWebSearchTool(NewDuckDuckGoHTML())
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for missing query")
	}
	if !strings.Contains(result.Content, "query") {
		t.Errorf("expected 'query' error, got %q", result.Content)
	}
}

func TestWebSearchTool_NilProvider(t *testing.T) {
	tool := NewWebSearchTool(nil)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"q":"test"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for nil provider")
	}
}

func TestWebSearchTool_LimitDefaults(t *testing.T) {
	// Test limit 0 defaults to 10
	tool := NewWebSearchTool(&mockSearchProvider{hits: makeHits(15)})
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"q":"test","limit":0}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Count results
	lines := strings.Count(result.Content, "\n\n") + 1
	if lines != 10 {
		t.Errorf("expected 10 results with limit=0, got %d", lines)
	}
}

func TestWebSearchTool_LimitCap(t *testing.T) {
	// Test limit > 25 caps to 25
	tool := NewWebSearchTool(&mockSearchProvider{hits: makeHits(30)})
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"q":"test","limit":50}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Count results
	lines := strings.Count(result.Content, "\n\n") + 1
	if lines != 25 {
		t.Errorf("expected 25 results with limit=50, got %d", lines)
	}
}

func TestWebSearchTool_NoResults(t *testing.T) {
	tool := NewWebSearchTool(&mockSearchProvider{hits: nil})
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"q":"test"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Content, "no results") {
		t.Errorf("expected 'no results' message, got %q", result.Content)
	}
}

func TestWebSearchTool_SearXNG(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Errorf("expected /search path, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("format") != "json" {
			t.Errorf("expected format=json, got %s", r.URL.Query().Get("format"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{
			"results": [
				{"title": "Result A", "url": "https://a.com", "content": "Snippet A"},
				{"title": "Result B", "url": "https://b.com", "content": "Snippet B"}
			]
		}`))
	}))
	defer srv.Close()

	provider := NewSearXNG(srv.URL)
	tool := NewWebSearchTool(provider)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"q":"test","limit":5}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "1. Result A") {
		t.Errorf("expected '1. Result A' in content, got %q", result.Content)
	}
	if !strings.Contains(result.Content, "https://a.com") {
		t.Errorf("expected URL in content, got %q", result.Content)
	}
}

func TestWebSearchTool_SearXNG_MissingBaseURL(t *testing.T) {
	provider := NewSearXNG("")
	hits, err := provider.Search(context.Background(), "test", 5)
	if err == nil {
		t.Error("expected error for missing base URL")
	}
	if !strings.Contains(err.Error(), "base_url") {
		t.Errorf("expected 'base_url' error, got %v", err)
	}
	if hits != nil {
		t.Errorf("expected nil hits, got %v", hits)
	}
}

func TestDuckDuckGoHTML_Name(t *testing.T) {
	provider := NewDuckDuckGoHTML()
	if provider.Name() != "duckduckgo" {
		t.Errorf("expected name 'duckduckgo', got %q", provider.Name())
	}
}

func TestDuckDuckGoHTML_ParseHTML(t *testing.T) {
	// Mock DDG HTML response
	ddgHTML := `<!DOCTYPE html>
<html>
<body>
<div class="result">
	<a class="result__a" href="/l/?uddg=https%3A%2F%2Fexample.com%2F1">Result One</a>
	<a class="result__snippet">Snippet for result one</a>
</div>
<div class="result">
	<a class="result__a" href="/l/?uddg=https%3A%2F%2Fexample.com%2F2">Result Two</a>
	<a class="result__snippet">Snippet for result two</a>
</div>
</body>
</html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(200)
		w.Write([]byte(ddgHTML))
	}))
	defer srv.Close()

	provider := &DuckDuckGoHTML{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		BaseURL:    srv.URL,
	}

	hits, err := provider.Search(context.Background(), "test", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hits) < 2 {
		t.Fatalf("expected at least 2 hits, got %d", len(hits))
	}
	if hits[0].Title != "Result One" {
		t.Errorf("expected title 'Result One', got %q", hits[0].Title)
	}
	if hits[0].URL != "https://example.com/1" {
		t.Errorf("expected URL 'https://example.com/1', got %q", hits[0].URL)
	}
	if hits[0].Snippet != "Snippet for result one" {
		t.Errorf("expected snippet 'Snippet for result one', got %q", hits[0].Snippet)
	}
}

func TestSearXNG_Name(t *testing.T) {
	provider := NewSearXNG("https://example.com")
	if provider.Name() != "searxng" {
		t.Errorf("expected name 'searxng', got %q", provider.Name())
	}
}

// mockSearchProvider implements WebSearchProvider for testing
type mockSearchProvider struct {
	hits []SearchHit
	err  error
	name string
}

func (m *mockSearchProvider) Name() string {
	if m.name != "" {
		return m.name
	}
	return "mock"
}

func (m *mockSearchProvider) Search(ctx context.Context, query string, limit int) ([]SearchHit, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(m.hits) <= limit {
		return m.hits, nil
	}
	return m.hits[:limit], nil
}

func makeHits(n int) []SearchHit {
	hits := make([]SearchHit, n)
	for i := 0; i < n; i++ {
		hits[i] = SearchHit{
			Title:   fmt.Sprintf("Result %d", i+1),
			URL:     fmt.Sprintf("https://example.com/%d", i+1),
			Snippet: fmt.Sprintf("Snippet %d", i+1),
		}
	}
	return hits
}
