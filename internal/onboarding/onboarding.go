// Package onboarding provides helpers that detect whether the user needs
// first-run configuration before the agent can make API calls.
package onboarding

import (
	"errors"

	"github.com/amemiya02/deepseekcode/internal/config"
)

// NeedsOnboarding reports whether the active provider has no resolvable API
// key or the base URL is empty — both are required to make any call.
//
// Resolution order for the key:
//  1. cfg.API.Key (legacy / direct field populated by Load or CLI flag)
//  2. cfg.Providers[cfg.Active.Provider] via config.ResolveSecret
//
// The second return value is non-nil only for unexpected I/O errors (e.g. bad
// secrets-file permissions, TOML parse failure). A missing key is not an
// error: it is expressed as (true, nil). Callers should surface a non-nil
// error as a diagnostic rather than treating it as "needs onboarding".
func NeedsOnboarding(cfg config.Config) (bool, error) {
	if cfg.API.BaseURL == "" {
		return true, nil
	}
	// Fast path: legacy API key field is already populated.
	if cfg.API.Key != "" {
		return false, nil
	}
	// Providers-map path (used when config was loaded via Load() with
	// applyLegacyAPICompat, or when explicit [providers] table is set).
	p, ok := cfg.Providers[cfg.Active.Provider]
	if !ok {
		return true, nil
	}
	_, err := config.ResolveSecret(p)
	if err != nil {
		if errors.Is(err, config.ErrNoAPIKey) {
			// Key is simply absent — that is the onboarding condition.
			return true, nil
		}
		// Real I/O error (bad permissions, TOML parse failure, etc.).
		// Do not silently report "needs onboarding"; propagate so the
		// caller can show a diagnostic.
		return false, err
	}
	// ResolveSecret always returns ErrNoAPIKey (wrapped) for a missing key,
	// so a non-error result here guarantees key != "". Returning false
	// documents that invariant; using key == "" would silently pass if the
	// invariant were ever violated.
	return false, nil
}
