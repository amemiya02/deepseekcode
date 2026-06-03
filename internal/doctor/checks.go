package doctor

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/onboarding"
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

// CheckBaseURLReachable verifies that the configured base URL is reachable.
func CheckBaseURLReachable(_ context.Context, _ config.Config, _ *http.Client) CheckResult {
	return CheckResult{Name: "base-url-reachable", OK: false, Detail: "not implemented"}
}

// CheckProxyConfigured reports whether a proxy is configured.
func CheckProxyConfigured(_ context.Context, _ config.Config, _ *http.Client) CheckResult {
	return CheckResult{Name: "proxy-configured", OK: false, Detail: "not implemented"}
}

// CheckCacheFieldsInProbe verifies that cache fields appear in a probe response.
func CheckCacheFieldsInProbe(_ context.Context, _ config.Config, _ *http.Client) CheckResult {
	return CheckResult{Name: "cache-fields-in-probe", OK: false, Detail: "not implemented"}
}

// CheckSandboxAvailable verifies that the sandbox environment is available.
func CheckSandboxAvailable(_ context.Context, _ config.Config, _ *http.Client) CheckResult {
	return CheckResult{Name: "sandbox-available", OK: false, Detail: "not implemented"}
}
