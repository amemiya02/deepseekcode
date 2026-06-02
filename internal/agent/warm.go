// internal/agent/warm.go
package agent

import (
	"fmt"
	"time"
)

// IsLikelyWarm reports whether the current static-prefix fingerprint probably
// still has a hot DeepSeek disk cache from a prior session: same fingerprint and
// last used within ttl. DeepSeek clears unused prefix caches in hours-to-days,
// so this is a best-effort hint (used to message "first turn was a cache hit"),
// never a guarantee.
// WarmthNotice returns a one-line human notice when warm==true, or "" when cold.
// sinceLastUse is rounded to minutes (if <1h) or hours.
func WarmthNotice(warm bool, sinceLastUse time.Duration) string {
	if !warm {
		return ""
	}
	if sinceLastUse < time.Hour {
		mins := int(sinceLastUse.Round(time.Minute).Minutes())
		if mins <= 0 {
			mins = 1
		}
		return fmt.Sprintf("Prefix likely still warm (last used ~%d minutes ago) — your first turn should hit the DeepSeek cache.", mins)
	}
	hrs := int(sinceLastUse.Round(time.Hour).Hours())
	if hrs <= 0 {
		hrs = 1
	}
	return fmt.Sprintf("Prefix likely still warm (last used ~%d hours ago) — your first turn should hit the DeepSeek cache.", hrs)
}

func IsLikelyWarm(lastFingerprint, curFingerprint string, sinceLastUse, ttl time.Duration) bool {
	if lastFingerprint == "" || lastFingerprint != curFingerprint {
		return false
	}
	if ttl <= 0 {
		return false
	}
	return sinceLastUse >= 0 && sinceLastUse <= ttl
}
