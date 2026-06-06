package gateway

import (
	"context"
	"net/http"
	"sync"

	"github.com/amemiya02/deepseekcode/internal/version"
)

// Update seams: tests swap the release lookup (no GitHub call) and the current
// version string. Production wires version.LatestRelease and version.String.
var (
	updMu         sync.RWMutex
	latestRelease = version.LatestRelease
	currentVer    = version.String
)

// SetUpdateSeam overrides the release lookup + current-version funcs for tests.
func SetUpdateSeam(latest func(context.Context) (string, string, error), current func() string) {
	updMu.Lock()
	defer updMu.Unlock()
	if latest != nil {
		latestRelease = latest
	}
	if current != nil {
		currentVer = current
	}
}

// ResetUpdateSeam restores production funcs.
func ResetUpdateSeam() {
	updMu.Lock()
	defer updMu.Unlock()
	latestRelease = version.LatestRelease
	currentVer = version.String
}

// updateInfo is the GET /v1/update response. The SPA's UpdateBanner shows the
// URL; on macOS the Wails bridge opens it in the browser (signature-verified
// installer lives behind that page — no in-app binary swap in v1).
type updateInfo struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"updateAvailable"`
	URL             string `json:"url"`
}

// handleUpdate implements GET /v1/update. A lookup failure (offline, rate-limit)
// is NOT a 500: it returns updateAvailable=false with the current version so the
// UI degrades gracefully to "you're up to date / couldn't check".
func (h *Handler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	updMu.RLock()
	lr, cv := latestRelease, currentVer
	updMu.RUnlock()
	cur := cv()
	tag, url, err := lr(r.Context())
	if err != nil {
		writeJSON(w, updateInfo{Current: cur, Latest: cur, UpdateAvailable: false})
		return
	}
	avail := version.CompareVersions(cur, tag) < 0
	writeJSON(w, updateInfo{Current: cur, Latest: tag, UpdateAvailable: avail, URL: url})
}
