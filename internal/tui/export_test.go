package tui

import (
	"context"
	"encoding/json"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// ExportedRenderWelcome exposes renderWelcome for package-external tests.
func ExportedRenderWelcome(t Theme, width int) string {
	return renderWelcome(t, width)
}

// stubTool is a minimal tools.Tool used to populate PermissionFlow for testing.
type stubTool struct{ name string }

func (s *stubTool) Name() string                        { return s.name }
func (s *stubTool) Description() string                 { return "" }
func (s *stubTool) Parameters() json.RawMessage         { return json.RawMessage("{}") }
func (s *stubTool) Execute(_ context.Context, _ json.RawMessage) (tools.Result, error) {
	return tools.Result{}, nil
}
func (s *stubTool) IsReadOnly() bool { return true }

// ExportedRenderPermission opens a synthetic PermissionFlow with a stub pending
// ask and renders it, so integration tests can inspect the button labels.
func ExportedRenderPermission(t Theme, width int) string {
	reply := make(chan agent.PermissionResponse, 1)
	flow := NewPermissionFlow()
	flow.Open(agent.EventPermissionAsk{
		Check: permissions.Check{
			Tool: &stubTool{name: "test_tool"},
			Args: json.RawMessage(`{"path":"/tmp/test"}`),
		},
		Reply: reply,
	})
	return flow.Render(t, width)
}

// ExportedRenderQuestion opens a synthetic QuestionFlow with a stub pending ask
// that has options (triggering the nav-hint branch) and renders it.
func ExportedRenderQuestion(t Theme, width int) string {
	reply := make(chan tools.QuestionResponse, 1)
	flow := NewQuestionFlow()
	flow.Open(agent.EventQuestionAsk{
		Questions: []tools.Question{
			{
				Question: "Pick an option",
				Options: []tools.Option{
					{Label: "option-a"},
					{Label: "option-b"},
				},
			},
		},
		Reply: reply,
	})
	return flow.Render(t, width)
}
