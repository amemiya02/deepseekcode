package doctor

import (
	"context"
	"net/http"

	"github.com/amemiya02/deepseekcode/internal/config"
)

// CheckKeyPresent verifies that an API key is configured.
func CheckKeyPresent(_ context.Context, _ config.Config, _ *http.Client) CheckResult {
	return CheckResult{Name: "key-present", OK: false, Detail: "not implemented"}
}

// CheckKeyValid probes the API with the configured key.
func CheckKeyValid(_ context.Context, _ config.Config, _ *http.Client) CheckResult {
	return CheckResult{Name: "key-valid", OK: false, Detail: "not implemented"}
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
