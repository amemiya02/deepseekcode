// internal/mcp/streamable_http_test.go
package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStreamableHTTPSendParsesJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Mcp-Session-Id", "sess-1")
		_ = json.NewEncoder(w).Encode(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"ok":true}`)})
	}))
	defer srv.Close()

	tr := NewStreamableHTTPTransport(srv.URL)
	defer tr.Close()
	res, err := tr.Send(context.Background(), "ping", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if string(res) != `{"ok":true}` {
		t.Fatalf("unexpected result: %s", res)
	}
	if tr.sessionID != "sess-1" {
		t.Fatalf("session id not captured: %q", tr.sessionID)
	}
}
