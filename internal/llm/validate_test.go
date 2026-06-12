package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidatePro(t *testing.T) {
	cases := []struct {
		name       string
		handler    http.HandlerFunc
		wantAppr   bool
		wantReason string
		wantErr    bool
	}{
		{
			name: "approve_true",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{
					"choices": []map[string]any{
						{"message": map[string]any{"content": `{"approve":true,"reasoning":"safe"}`}},
					},
				})
			},
			wantAppr:   true,
			wantReason: "safe",
		},
		{
			name: "approve_false",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{
					"choices": []map[string]any{
						{"message": map[string]any{"content": `{"approve":false,"reasoning":"too dangerous"}`}},
					},
				})
			},
			wantAppr:   false,
			wantReason: "too dangerous",
		},
		{
			name: "malformed_json",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{
					"choices": []map[string]any{
						{"message": map[string]any{"content": "not json at all"}},
					},
				})
			},
			wantAppr:   false,
			wantReason: "malformed validator response",
		},
		{
			name: "empty_content",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{
					"choices": []map[string]any{
						{"message": map[string]any{"content": ""}},
					},
				})
			},
			wantAppr:   false,
			wantReason: "validator returned empty response",
		},
		{
			name: "non_2xx",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(500)
				w.Write([]byte("internal error"))
			},
			wantErr: true,
		},
		{
			name: "fenced_json",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				// Some models wrap JSON in markdown fences; scanner recovers.
				json.NewEncoder(w).Encode(map[string]any{
					"choices": []map[string]any{
						{"message": map[string]any{"content": "```json\n{\"approve\":true,\"reasoning\":\"wrapped\"}\n```"}},
					},
				})
			},
			wantAppr:   true,
			wantReason: "wrapped",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			// Set base URL to point at the test server. NewClient
			// strips trailing slash, so the server URL is fine as-is.
			c := NewClient("k", srv.URL)
			// Override the default /v1/chat/completions path by
			// rewriting base URL — the test server handles any path.
			// ValidatePro builds the versioned chat-completions endpoint from BaseURL,
			// so we need the test server to handle that path. Since
			// httptest.Server ignores the path (all routes go to the
			// same handler), the trailing path is harmless.
			c.BaseURL = srv.URL

			gotAppr, gotReason, err := c.ValidatePro(t.Context(), "test prompt")
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotAppr != tc.wantAppr {
				t.Errorf("approve = %v, want %v", gotAppr, tc.wantAppr)
			}
			if !strings.Contains(gotReason, tc.wantReason) {
				t.Errorf("reason = %q, want substring %q", gotReason, tc.wantReason)
			}
		})
	}
}
