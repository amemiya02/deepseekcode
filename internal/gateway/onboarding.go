package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/amemiya02/deepseekcode/internal/onboarding"
)

// Onboarding seams: tests swap key validation (no network) and persistence (no
// disk writes). Production wires onboarding.ValidateKey and onboarding.PersistConfig.
var (
	obMu        sync.RWMutex
	validateKey = onboarding.ValidateKey
	persistKey  = persistKeyDefault
)

func persistKeyDefault(baseURL, apiKey, model string) error {
	return onboarding.PersistConfig(onboarding.OnboardingResult{
		APIKey: apiKey, BaseURL: baseURL, Model: model,
	})
}

// SetValidateKeySeam overrides the key validator for tests.
func SetValidateKeySeam(f func(context.Context, string, string, *http.Client) error) {
	obMu.Lock()
	defer obMu.Unlock()
	validateKey = f
}

// ResetValidateKeySeam restores the production validator.
func ResetValidateKeySeam() {
	obMu.Lock()
	defer obMu.Unlock()
	validateKey = onboarding.ValidateKey
}

// SetPersistSeam overrides config persistence for tests.
func SetPersistSeam(f func(baseURL, apiKey, model string) error) {
	obMu.Lock()
	defer obMu.Unlock()
	persistKey = f
}

// ResetPersistSeam restores production persistence.
func ResetPersistSeam() {
	obMu.Lock()
	defer obMu.Unlock()
	persistKey = persistKeyDefault
}

// onboardingStatus is the GET /v1/onboarding response.
type onboardingStatus struct {
	NeedsOnboarding bool   `json:"needsOnboarding"`
	BaseURL         string `json:"baseUrl"`
	Model           string `json:"model"`
}

// handleOnboarding implements GET /v1/onboarding. It reports whether first-run
// setup is required and echoes sensible defaults the wizard can pre-fill.
func (h *Handler) handleOnboarding(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	c, err := loadConfig()
	if err != nil {
		http.Error(w, "load config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	needs, err := onboarding.NeedsOnboarding(c)
	if err != nil {
		http.Error(w, "onboarding check: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, onboardingStatus{
		NeedsOnboarding: needs,
		BaseURL:         c.API.BaseURL,
		Model:           c.Defaults.Model,
	})
}

// connectKeyRequest is the POST /v1/connect-key body.
type connectKeyRequest struct {
	APIKey  string `json:"apiKey"`
	BaseURL string `json:"baseUrl"`
	Model   string `json:"model"`
}

// handleConnectKey implements POST /v1/connect-key: validate the key against
// the provider, then persist it. A failing validation is a 400 (the SPA shows
// the message inline); a persistence failure is a 500.
func (h *Handler) handleConnectKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req connectKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.APIKey == "" || req.BaseURL == "" {
		http.Error(w, "apiKey and baseUrl are required", http.StatusBadRequest)
		return
	}
	obMu.RLock()
	vk, pk := validateKey, persistKey
	obMu.RUnlock()
	if err := vk(r.Context(), req.BaseURL, req.APIKey, http.DefaultClient); err != nil {
		http.Error(w, "key validation failed: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := pk(req.BaseURL, req.APIKey, req.Model); err != nil {
		http.Error(w, "persist key: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
