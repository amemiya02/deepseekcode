package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestWebFetchTool_Name(t *testing.T) {
	tool := NewWebFetchTool(false)
	if tool.Name() != "web_fetch" {
		t.Errorf("expected name 'web_fetch', got %q", tool.Name())
	}
}

func TestWebFetchTool_IsReadOnly(t *testing.T) {
	tool := NewWebFetchTool(false)
	if !tool.IsReadOnly() {
		t.Error("web_fetch should be read-only")
	}
}

func TestWebFetchTool_Parameters(t *testing.T) {
	tool := NewWebFetchTool(false)
	params := tool.Parameters()

	var schema map[string]any
	if err := json.Unmarshal(params, &schema); err != nil {
		t.Fatalf("failed to parse parameters: %v", err)
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties in schema")
	}

	if _, ok := props["url"]; !ok {
		t.Error("expected 'url' property")
	}
	if _, ok := props["max_bytes"]; !ok {
		t.Error("expected 'max_bytes' property")
	}
	if _, ok := props["accept"]; !ok {
		t.Error("expected 'accept' property")
	}
}

func TestWebFetchTool_MissingURL(t *testing.T) {
	tool := NewWebFetchTool(false)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for missing URL")
	}
	if !strings.Contains(result.Content, "url is required") {
		t.Errorf("expected 'url is required' error, got %q", result.Content)
	}
}

func TestWebFetchTool_InvalidScheme(t *testing.T) {
	tool := NewWebFetchTool(false)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"url":"file:///etc/passwd"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for file:// scheme")
	}
	if !strings.Contains(result.Content, "only http and https") {
		t.Errorf("expected scheme error, got %q", result.Content)
	}
}

func TestWebFetchTool_PrivateIPBlocked(t *testing.T) {
	tool := NewWebFetchTool(false)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"url":"http://10.0.0.1/test"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for private IP")
	}
	if !strings.Contains(result.Content, "private") {
		t.Errorf("expected 'private' error, got %q", result.Content)
	}
}

func TestWebFetchTool_PrivateIPAllowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("hello"))
	}))
	defer srv.Close()

	tool := NewWebFetchTool(true)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"url":"`+srv.URL+`"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "hello") {
		t.Errorf("expected 'hello' in content, got %q", result.Content)
	}
}

func TestWebFetchTool_DNSFailure(t *testing.T) {
	tool := NewWebFetchTool(false)
	// Use a domain that should not resolve
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"url":"http://this-domain-does-not-exist-12345.invalid/test"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for DNS failure")
	}
	if !strings.Contains(result.Content, "dns:") {
		t.Errorf("expected 'dns:' error, got %q", result.Content)
	}
}

func TestWebFetchTool_RedirectToPrivateIP(t *testing.T) {
	// Create a target server on loopback
	targetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("secret"))
	}))
	defer targetSrv.Close()

	// Extract the host:port from target server
	targetURL, _ := url.Parse(targetSrv.URL)
	targetHost := targetURL.Host

	// Create a redirect server that redirects to the target
	redirectSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Redirect to the loopback target
		http.Redirect(w, r, "http://"+targetHost+"/secret", http.StatusFound)
	}))
	defer redirectSrv.Close()

	// Tool with AllowPrivate=false should block the redirect
	tool := NewWebFetchTool(false)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"url":"`+redirectSrv.URL+`"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for redirect to private IP")
	}
	if !strings.Contains(result.Content, "private") {
		t.Errorf("expected 'private' error, got %q", result.Content)
	}
}

func TestWebFetchTool_HTMLToMarkdown(t *testing.T) {
	html := `<html><head><title>Test Page</title></head><body><h1>Heading</h1><p>Hello <a href="https://example.com">link</a></p></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(200)
		w.Write([]byte(html))
	}))
	defer srv.Close()

	tool := NewWebFetchTool(true) // Allow private for test server
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"url":"`+srv.URL+`"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}
	// Check for title and link conversion
	if !strings.Contains(result.Content, "# Test Page") {
		t.Errorf("expected '# Test Page' in content, got %q", result.Content)
	}
	if !strings.Contains(result.Content, "[link](https://example.com)") {
		t.Errorf("expected markdown link in content, got %q", result.Content)
	}
}

func TestWebFetchTool_Truncation(t *testing.T) {
	// Create a server that returns > 1MB
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(200)
		// Write 1.5MB of data
		data := strings.Repeat("x", 1500*1024)
		w.Write([]byte(data))
	}))
	defer srv.Close()

	tool := NewWebFetchTool(true) // Allow private for test server
	tool.MaxBytes = 1024 * 1024   // 1MB
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"url":"`+srv.URL+`"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Assert content is truncated (should be <= 1.1MB including status line and truncation marker)
	if len(result.Content) >= 1100*1024 {
		t.Fatalf("content too large: %d bytes (should be truncated to ~1MB)", len(result.Content))
	}
	if !strings.Contains(result.Content, "(truncated") {
		t.Fatalf("missing truncation marker in content: %q", result.Content)
	}
}

func TestWebFetchTool_4xxResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte("Not Found"))
	}))
	defer srv.Close()

	tool := NewWebFetchTool(true) // Allow private for test server
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"url":"`+srv.URL+`"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for 404 response")
	}
	if !strings.Contains(result.Content, "[status 404]") {
		t.Errorf("expected '[status 404]' in content, got %q", result.Content)
	}
}

func TestWebFetchTool_CustomMaxBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(strings.Repeat("x", 5000)))
	}))
	defer srv.Close()

	tool := NewWebFetchTool(true) // Allow private for test server
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"url":"`+srv.URL+`","max_bytes":100}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Content, "truncated") {
		t.Errorf("expected truncation with custom max_bytes, got %q", result.Content)
	}
}
