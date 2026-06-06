package onboarding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ValidateKey sends a minimal single-token chat-completions request to baseURL
// using the provided apiKey. hc is injected so tests can supply httptest clients.
// Returns nil on HTTP 200, a descriptive error otherwise.
func ValidateKey(ctx context.Context, baseURL, apiKey string, hc *http.Client) error {
	if hc == nil {
		hc = http.DefaultClient
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/v1/chat/completions"
	body, err := json.Marshal(map[string]any{
		"model":      "deepseek-v4-flash",
		"max_tokens": 1,
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
	})
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("key validation request: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("API key rejected (HTTP 401) — check the key and try again")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, endpoint)
	}
	return nil
}
