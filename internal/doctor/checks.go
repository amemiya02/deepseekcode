package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/onboarding"
	"github.com/amemiya02/deepseekcode/internal/sandbox"
)

// CheckKeyPresent verifies that the active provider has a resolvable API key.
func CheckKeyPresent(_ context.Context, cfg config.Config, _ *http.Client) CheckResult {
	p, ok := cfg.Providers[cfg.Active.Provider]
	if !ok {
		return CheckResult{Name: "key-present", OK: false, Detail: fmt.Sprintf("provider %q not found in config", cfg.Active.Provider)}
	}
	key, err := config.ResolveSecret(p)
	if err != nil || key == "" {
		return CheckResult{Name: "key-present", OK: false, Detail: "no API key found — set DEEPSEEK_API_KEY or run `dsc init`"}
	}
	masked := key
	if len(key) > 8 {
		masked = key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
	}
	return CheckResult{Name: "key-present", OK: true, Detail: "key found (" + masked + ")"}
}

// CheckKeyValid sends a minimal probe request and checks for HTTP 200.
func CheckKeyValid(ctx context.Context, cfg config.Config, hc *http.Client) CheckResult {
	p, ok := cfg.Providers[cfg.Active.Provider]
	if !ok {
		return CheckResult{Name: "key-valid", OK: false, Detail: "provider not found"}
	}
	key, err := config.ResolveSecret(p)
	if err != nil || key == "" {
		return CheckResult{Name: "key-valid", OK: false, Detail: "no key to validate"}
	}
	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = cfg.API.BaseURL
	}
	if err := onboarding.ValidateKey(ctx, baseURL, key, hc); err != nil {
		return CheckResult{Name: "key-valid", OK: false, Detail: err.Error()}
	}
	return CheckResult{Name: "key-valid", OK: true, Detail: "API key accepted by server"}
}

// CheckBaseURLReachable performs a HEAD request to the base URL to confirm
// basic network reachability (no auth required).
// expects a Finalize()d config — the provider-BaseURL → cfg.API.BaseURL fallback
// mirrors what config.Finalize() does and must stay in sync with it.
func CheckBaseURLReachable(ctx context.Context, cfg config.Config, hc *http.Client) CheckResult {
	p, ok := cfg.Providers[cfg.Active.Provider]
	if !ok {
		p = config.ProviderConfigTOML{}
	}
	base := p.BaseURL
	if base == "" {
		base = cfg.API.BaseURL
	}
	if base == "" {
		return CheckResult{Name: "base-url-reachable", OK: false, Detail: "api.base_url is empty"}
	}
	// HEAD the base URL root; ignore auth errors (401 means we reached it).
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, base, nil)
	if err != nil {
		return CheckResult{Name: "base-url-reachable", OK: false, Detail: fmt.Sprintf("bad URL: %v", err)}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return CheckResult{Name: "base-url-reachable", OK: false, Detail: fmt.Sprintf("connection failed: %v", err)}
	}
	resp.Body.Close()
	return CheckResult{Name: "base-url-reachable", OK: true, Detail: fmt.Sprintf("reached %s (HTTP %d)", base, resp.StatusCode)}
}

// CheckProxyConfigured reports whether any standard HTTP proxy env vars are set.
// This is informational — OK = proxy is configured, not-OK = no proxy (not an error per se).
func CheckProxyConfigured(_ context.Context, _ config.Config, _ *http.Client) CheckResult {
	for _, env := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		if v := os.Getenv(env); v != "" {
			detail := v
			if u, err := url.Parse(v); err == nil {
				detail = u.Redacted()
			}
			detail = fmt.Sprintf("%s=%s", env, detail)
			// Warn when NO_PROXY/no_proxy is also set: a wildcard or matching entry
			// effectively disables the proxy and the doctor should not mislead.
			for _, npEnv := range []string{"NO_PROXY", "no_proxy"} {
				if np := os.Getenv(npEnv); np != "" {
					detail += fmt.Sprintf(" (NO_PROXY also set: %s)", np)
				}
			}
			return CheckResult{Name: "proxy-configured", OK: true, Detail: detail}
		}
	}
	return CheckResult{Name: "proxy-configured", OK: false, Detail: "no HTTP(S) proxy env vars set (OK if direct access)"}
}

// CheckCacheFieldsInProbe fires a minimal chat-completions request and checks
// that the response Usage block contains prompt_cache_hit_tokens and
// prompt_cache_miss_tokens — verifying that the server is returning cache
// accounting fields that the cost tracker depends on.
func CheckCacheFieldsInProbe(ctx context.Context, cfg config.Config, hc *http.Client) CheckResult {
	const name = "cache-fields-in-probe"
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	p, ok := cfg.Providers[cfg.Active.Provider]
	if !ok {
		return CheckResult{Name: name, OK: false, Detail: "active provider not found"}
	}
	key, err := config.ResolveSecret(p)
	if err != nil || key == "" {
		return CheckResult{Name: name, OK: false, Detail: "no API key to probe with"}
	}
	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = cfg.API.BaseURL
	}

	endpoint := strings.TrimRight(baseURL, "/") + "/chat/completions"
	body, _ := json.Marshal(map[string]any{
		"model":      "deepseek-v4-flash",
		"max_tokens": 1,
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return CheckResult{Name: name, OK: false, Detail: fmt.Sprintf("build request: %v", err)}
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := hc.Do(req)
	if err != nil {
		return CheckResult{Name: name, OK: false, Detail: fmt.Sprintf("request failed: %v", err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return CheckResult{Name: name, OK: false, Detail: fmt.Sprintf("HTTP %d from %s", resp.StatusCode, endpoint)}
	}

	var payload struct {
		Usage struct {
			PromptCacheHitTokens  *int `json:"prompt_cache_hit_tokens"`
			PromptCacheMissTokens *int `json:"prompt_cache_miss_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return CheckResult{Name: name, OK: false, Detail: fmt.Sprintf("decode response: %v", err)}
	}
	if payload.Usage.PromptCacheHitTokens == nil || payload.Usage.PromptCacheMissTokens == nil {
		return CheckResult{Name: name, OK: false, Detail: "usage block missing prompt_cache_hit_tokens / prompt_cache_miss_tokens — cache accounting unavailable"}
	}
	hit := *payload.Usage.PromptCacheHitTokens
	miss := *payload.Usage.PromptCacheMissTokens
	return CheckResult{Name: name, OK: true, Detail: fmt.Sprintf("cache fields present (hit=%d miss=%d)", hit, miss)}
}

// CheckSandboxAvailable reports whether OS-native sandboxing is supported on
// the current platform. It delegates to sandbox.Detect().Available() which
// encodes the correct per-OS detection logic (seatbelt on macOS, Landlock on
// Linux, noop elsewhere).
func CheckSandboxAvailable(_ context.Context, _ config.Config, _ *http.Client) CheckResult {
	const name = "sandbox-available"
	sb := sandbox.Detect()
	if sb.Available() {
		return CheckResult{Name: name, OK: true, Detail: fmt.Sprintf("%s sandbox available", sb.Name())}
	}
	return CheckResult{Name: name, OK: false, Detail: fmt.Sprintf("%s sandbox not available on this system", sb.Name())}
}
