package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/prompt"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// TestAgentSystemPromptUsesPromptBuilder verifies that a Run with a
// non-nil PromptBuilder overrides the System field with the
// builder's output. The BasePromptV1 header (identity sentence) is
// the cheapest stable marker to look for.
func TestAgentSystemPromptUsesPromptBuilder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	a := New(llm.NewClient("k", srv.URL), tools.New(),
		permissions.New(permissions.ModeYolo, "", nil, nil, nil), "m")
	a.System = "old-prompt-do-not-use"
	a.PromptBuilder = &prompt.SystemPromptBuilder{
		StaticBase: prompt.BasePromptV1,
		Project:    &prompt.ProjectContext{CWD: "/tmp", CurrentDate: "2026-05-24"},
	}
	a.StopWhen = []StopCondition{MaxSteps(1)}

	if _, err := a.Run(context.Background(), "hi"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasPrefix(a.System, "You are deepseekcode") {
		t.Errorf("a.System did not start with BasePromptV1 head; got: %q", trimMid(a.System, 200))
	}
	if !strings.Contains(a.System, prompt.DynamicContextBoundary) {
		t.Errorf("a.System missing dynamic boundary marker")
	}
	if strings.Contains(a.System, "old-prompt-do-not-use") {
		t.Errorf("a.System still contains the pre-builder string")
	}
}

func trimMid(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
