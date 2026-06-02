// internal/agent/warm.go
package agent

import "time"

// IsLikelyWarm reports whether the current static-prefix fingerprint probably
// still has a hot DeepSeek disk cache from a prior session: same fingerprint and
// last used within ttl. DeepSeek clears unused prefix caches in hours-to-days,
// so this is a best-effort hint (used to message "first turn was a cache hit"),
// never a guarantee.
func IsLikelyWarm(lastFingerprint, curFingerprint string, sinceLastUse, ttl time.Duration) bool {
	if lastFingerprint == "" || lastFingerprint != curFingerprint {
		return false
	}
	if ttl <= 0 {
		return false
	}
	return sinceLastUse >= 0 && sinceLastUse <= ttl
}
