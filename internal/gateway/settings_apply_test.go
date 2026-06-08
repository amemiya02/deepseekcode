package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/acp"
)

// settingsRecorder is an AgentRunner that records the TurnSettings applied to
// it. It proves the gateway pushes the composer's model/effort selection and
// the prompt's permission mode into the session's agent before each run —
// previously POST /v1/model and /v1/effort only mutated gateway display state
// (models.go: "applying it to a live agent run is the concern of a later
// wave") and the GUI mode dropdown never left the SPA at all, so Yolo still
// asked for approval.
type settingsRecorder struct {
	mu      sync.Mutex
	applied []acp.TurnSettings
}

func (s *settingsRecorder) Run(ctx context.Context, prompt string, onEvent func(acp.AgentEvent)) error {
	onEvent(acp.AgentEvent{Kind: acp.EventKindTextDelta, Text: "ok"})
	onEvent(acp.AgentEvent{Kind: acp.EventKindDone, StopReason: "end_turn"})
	return nil
}

func (s *settingsRecorder) Steer(string) {}

func (s *settingsRecorder) ApplySettings(ts acp.TurnSettings) {
	s.mu.Lock()
	s.applied = append(s.applied, ts)
	s.mu.Unlock()
}

func (s *settingsRecorder) last() (acp.TurnSettings, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.applied) == 0 {
		return acp.TurnSettings{}, false
	}
	return s.applied[len(s.applied)-1], true
}

func TestHandlePromptAppliesComposerSettings(t *testing.T) {
	rec := &settingsRecorder{}
	sm := acp.NewSessionManager(func(workingDir string) (acp.AgentRunner, error) {
		return rec, nil
	})
	h := NewHandler(sm, "")

	// Select model + effort via the composer chip endpoints.
	post := func(path, body string) {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		if w.Code != http.StatusOK {
			t.Fatalf("POST %s = %d: %s", path, w.Code, w.Body.String())
		}
	}
	post("/v1/model", `{"model":"deepseek-v4-pro"}`)
	post("/v1/effort", `{"effort":"low"}`)

	// The prompt carries the GUI permission mode; everything lands on the agent.
	post("/v1/prompt", `{"prompt":"hi","mode":"yolo"}`)

	ts, ok := rec.last()
	if !ok {
		t.Fatal("handlePrompt did not apply any settings to the session agent")
	}
	if ts.Model != "deepseek-v4-pro" || ts.Effort != "low" || ts.PermissionMode != "yolo" {
		t.Errorf("applied = %+v, want {deepseek-v4-pro low yolo}", ts)
	}
}

// A prompt without mode still applies model/effort and leaves the mode empty
// (ApplySettings treats "" as keep-current).
func TestHandlePromptAppliesSettingsWithoutMode(t *testing.T) {
	rec := &settingsRecorder{}
	sm := acp.NewSessionManager(func(workingDir string) (acp.AgentRunner, error) {
		return rec, nil
	})
	h := NewHandler(sm, "")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/prompt", strings.NewReader(`{"prompt":"hi"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("POST /v1/prompt = %d", w.Code)
	}
	ts, ok := rec.last()
	if !ok {
		t.Fatal("handlePrompt did not apply settings")
	}
	if ts.PermissionMode != "" {
		t.Errorf("PermissionMode = %q, want empty (keep current)", ts.PermissionMode)
	}
	if ts.Model == "" || ts.Effort == "" {
		t.Errorf("model/effort must reflect gateway state, got %+v", ts)
	}
}
