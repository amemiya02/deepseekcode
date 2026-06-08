package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/acp"
)

func TestDeriveTitle(t *testing.T) {
	cases := map[string]string{
		"":                       "New session",
		"   ":                    "New session",
		"Refactor the parser":    "Refactor the parser",
		"line one\nline two":     "line one",
		"a very long prompt that should be truncated to keep the rail tidy and readable": "a very long prompt that should be truncated to",
	}
	for in, want := range cases {
		if got := deriveTitle(in); got != want {
			t.Errorf("deriveTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

// Regression: a long multi-byte (CJK) prompt used to PANIC deriveTitle —
// LastIndexByte's BYTE index was used to slice the RUNE slice. A long Chinese
// first message then 500'd POST /v1/prompt.
func TestDeriveTitleUTF8NoPanic(t *testing.T) {
	in := strings.Repeat("你好", 15) + " " + strings.Repeat("世界", 15) // 61 runes, space at rune 30
	got := deriveTitle(in)
	if got == "" {
		t.Fatal("deriveTitle returned empty for CJK prompt")
	}
	if want := strings.Repeat("你好", 15); got != want {
		t.Errorf("deriveTitle(CJK) = %q, want word-truncated %q", got, want)
	}
	// Pure-CJK overflow (no spaces at all) hard-truncates at the rune cap.
	if got := deriveTitle(strings.Repeat("测", 60)); got != strings.Repeat("测", 46) {
		t.Errorf("deriveTitle(pure CJK) = %q, want 46-rune cut", got)
	}
}

func TestSessionStore_RegistersPromptSession(t *testing.T) {
	s := newSessionStore()
	now := int64(1000)
	if _, ok := s.get("s1"); !ok {
		s.put(&sessionMeta{ID: "s1", Title: deriveTitle("Refactor the parser"), Turns: 1, CreatedAt: now, UpdatedAt: now})
	}
	got := s.list()
	if len(got) != 1 || got[0].ID != "s1" || got[0].Title != "Refactor the parser" || got[0].Turns != 1 {
		t.Fatalf("expected one registered session with title+turns, got %+v", got)
	}
}

// stubAgentFactoryInternal is the same stub as in gateway_test.go but declared
// here for the internal test package (cannot cross-reference gateway_test symbols).
func stubAgentFactoryInternal(workingDir string) (acp.AgentRunner, error) {
	return &stubAgentInternal{}, nil
}

type stubAgentInternal struct{}

func (s *stubAgentInternal) Run(ctx context.Context, userPrompt string, onEvent func(acp.AgentEvent)) error {
	onEvent(acp.AgentEvent{Kind: acp.EventKindTextDelta, Text: "hello"})
	onEvent(acp.AgentEvent{Kind: acp.EventKindDone, StopReason: "end_turn"})
	return nil
}

func (s *stubAgentInternal) Steer(_ string) {}

func TestHandlePrompt_PopulatesSessionList(t *testing.T) {
	sm := acp.NewSessionManager(stubAgentFactoryInternal)
	h := NewHandler(sm, "")

	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"prompt":"Refactor the parser"}`)
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/prompt", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("prompt status = %d", rec.Code)
	}
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/v1/sessions", nil))
	var out struct {
		Sessions []sessionMeta `json:"sessions"`
	}
	if err := json.NewDecoder(rec2.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Sessions) != 1 || out.Sessions[0].Title != "Refactor the parser" {
		t.Fatalf("rail did not pick up prompt session: %+v", out.Sessions)
	}
}
