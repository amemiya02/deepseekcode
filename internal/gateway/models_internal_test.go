package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleModelsBareFallback covers the path where buildDescriptors yielded no
// descriptors (e.g. a config-less boot or an unknown provider). handleModels must
// then advertise the bare ms.models id list. This test asserts only the Go-side
// JSON the gateway emits in the fallback branch (a bare []string for "models");
// it does NOT exercise the TypeScript web parser, so it makes no claim about how
// the web client tolerates that shape — that back-compat lives in web/src tests.
func TestHandleModelsBareFallback(t *testing.T) {
	h := &Handler{models: &modelState{
		models:      []string{"deepseek-v4-flash", "deepseek-v4-pro"},
		descriptors: nil, // force the fallback branch
		active:      "deepseek-v4-flash",
		effort:      "medium",
	}}

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	h.handleModels(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	// The fallback emits bare strings, so models must decode as a []string.
	var bare struct {
		Active string   `json:"active"`
		Effort string   `json:"effort"`
		Models []string `json:"models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &bare); err != nil {
		t.Fatalf("decode bare models: %v", err)
	}
	if len(bare.Models) != 2 || bare.Models[0] != "deepseek-v4-flash" {
		t.Fatalf("bare models = %v, want [deepseek-v4-flash deepseek-v4-pro]", bare.Models)
	}
	if bare.Active != "deepseek-v4-flash" || bare.Effort != "medium" {
		t.Fatalf("active/effort = %q/%q, want flash/medium", bare.Active, bare.Effort)
	}
}

// TestHandleEffortValidatesFromDescriptor proves the effort validator is derived
// from the active model's capability descriptor rather than a hardcoded level
// set. The active model here advertises only {low, high}, so "medium" — which
// the old hardcoded switch accepted — must now be rejected, while a level inside
// the descriptor set is accepted.
func TestHandleEffortValidatesFromDescriptor(t *testing.T) {
	ms := &modelState{
		models: []string{"custom-model"},
		active: "custom-model",
		effort: "low",
		descriptors: []modelDescriptor{{
			ID:     "custom-model",
			Effort: modelEffort{Kind: "levels", Levels: []string{"low", "high"}, Default: "low"},
		}},
	}
	h := &Handler{models: ms}

	post := func(effort string) int {
		req := httptest.NewRequest(http.MethodPost, "/v1/effort",
			strings.NewReader(`{"effort":"`+effort+`"}`))
		rec := httptest.NewRecorder()
		h.handleEffort(rec, req)
		return rec.Code
	}

	if code := post("high"); code != http.StatusOK {
		t.Errorf(`effort "high" = %d, want 200 (in descriptor levels)`, code)
	}
	if code := post("medium"); code != http.StatusBadRequest {
		t.Errorf(`effort "medium" = %d, want 400 (NOT in this model's descriptor levels)`, code)
	}
}

// TestHandleEffortFallbackLevels covers a descriptor-less modelState (config-less
// boot). With no descriptor for the active model, handleEffort falls back to the
// DeepSeek V4 baseline set {low, medium, high, max} so the endpoint still works.
func TestHandleEffortFallbackLevels(t *testing.T) {
	h := &Handler{models: &modelState{
		models: []string{"deepseek-v4-flash"},
		active: "deepseek-v4-flash",
		effort: "medium",
	}}
	for _, lvl := range []string{"low", "medium", "high", "max"} {
		req := httptest.NewRequest(http.MethodPost, "/v1/effort",
			strings.NewReader(`{"effort":"`+lvl+`"}`))
		rec := httptest.NewRecorder()
		h.handleEffort(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("fallback effort %q = %d, want 200", lvl, rec.Code)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/effort",
		strings.NewReader(`{"effort":"ultra"}`))
	rec := httptest.NewRecorder()
	h.handleEffort(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf(`fallback effort "ultra" = %d, want 400`, rec.Code)
	}
}
