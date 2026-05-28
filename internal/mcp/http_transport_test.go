package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPTransportRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`))
	}))
	defer srv.Close()

	tr := NewHTTPTransport(srv.URL)
	defer tr.Close()

	resp, err := tr.Send(context.Background(), "ping", map[string]any{"value": true})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if !strings.Contains(string(resp), `"ok":true`) || !json.Valid(resp) {
		t.Fatalf("response = %s", resp)
	}
}
