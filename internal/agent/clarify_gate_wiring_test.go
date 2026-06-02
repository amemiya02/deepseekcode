package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// TestClarifyGateInterceptsVaguePrompt verifies the AutoClarify gate yields the
// turn with a clarifying question and NEVER calls the model on a vague prompt.
func TestClarifyGateInterceptsVaguePrompt(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		emitSSE(w, `{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"total_tokens":1}}`)
	}))
	defer srv.Close()

	a := New(llm.NewClient("k", srv.URL), tools.New(),
		permissions.New(permissions.ModeYolo, t.TempDir(), nil, nil, nil), "deepseek-v4-flash")
	a.AutoClarify = true
	a.StopWhen = []StopCondition{MaxSteps(1)}

	reason, err := a.Run(context.Background(), "fix it")
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("clarify gate must NOT call the model for a vague prompt")
	}
	if reason != StopModelDone {
		t.Fatalf("reason = %v, want StopModelDone", reason)
	}
	last := a.Messages[len(a.Messages)-1]
	if last.Role != "assistant" {
		t.Fatalf("last message role = %q, want an assistant clarifying question", last.Role)
	}
}

// TestClarifyGateOffByDefaultRunsModel verifies the default (AutoClarify=false)
// path is unchanged: even a vague prompt goes to the model.
func TestClarifyGateOffByDefaultRunsModel(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		emitSSE(w,
			`{"choices":[{"index":0,"delta":{"content":"ok"}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"total_tokens":2}}`,
		)
	}))
	defer srv.Close()

	a := New(llm.NewClient("k", srv.URL), tools.New(),
		permissions.New(permissions.ModeYolo, t.TempDir(), nil, nil, nil), "deepseek-v4-flash")
	a.StopWhen = []StopCondition{MaxSteps(1)}

	if _, err := a.Run(context.Background(), "fix it"); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("with AutoClarify off, the model must be called even for a vague prompt")
	}
}
