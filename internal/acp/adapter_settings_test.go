package acp

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// ApplySettings must mutate the wrapped *agent.Agent: model, reasoning effort,
// and the permission mode mapped from the GUI vocabulary (ask / auto-edit /
// plan / yolo). It is only called between turns (gateway handlePrompt applies
// it before launching the run goroutine), so plain field writes are safe.
func TestAdapterApplySettings(t *testing.T) {
	pol := permissions.New(permissions.ModeDefault, t.TempDir(), nil, nil, nil)
	a := agent.New(nil, tools.New(), pol, "deepseek-v4-flash")
	ad := NewAgentAdapter(a)

	ad.ApplySettings(TurnSettings{Model: "deepseek-v4-pro", Effort: "low", PermissionMode: "yolo"})

	if a.Model != "deepseek-v4-pro" {
		t.Errorf("Model = %q, want deepseek-v4-pro", a.Model)
	}
	if a.ReasoningEffort != llm.ReasoningEffortLow {
		t.Errorf("ReasoningEffort = %q, want low", a.ReasoningEffort)
	}
	if pol.Mode != permissions.ModeYolo {
		t.Errorf("Permissions.Mode = %v, want ModeYolo", pol.Mode)
	}

	// Empty fields keep current values; unknown effort/mode strings are ignored.
	ad.ApplySettings(TurnSettings{Model: "", Effort: "bogus", PermissionMode: ""})
	if a.Model != "deepseek-v4-pro" || a.ReasoningEffort != llm.ReasoningEffortLow || pol.Mode != permissions.ModeYolo {
		t.Errorf("empty/bogus settings must be no-ops: model=%q effort=%q mode=%v", a.Model, a.ReasoningEffort, pol.Mode)
	}
}

// The four GUI modes map to distinct, honest policy semantics.
func TestAdapterApplySettingsModeMapping(t *testing.T) {
	cases := []struct {
		ui           string
		mode         permissions.Mode
		confirmEdits bool
	}{
		{"ask", permissions.ModeDefault, true},
		{"auto-edit", permissions.ModeDefault, false},
		{"plan", permissions.ModePlan, false},
		{"yolo", permissions.ModeYolo, false},
	}
	for _, c := range cases {
		pol := permissions.New(permissions.ModeAskAll, t.TempDir(), nil, nil, nil)
		a := agent.New(nil, tools.New(), pol, "m")
		NewAgentAdapter(a).ApplySettings(TurnSettings{PermissionMode: c.ui})
		if pol.Mode != c.mode || pol.ConfirmEdits != c.confirmEdits {
			t.Errorf("%s → mode=%v confirmEdits=%v, want mode=%v confirmEdits=%v",
				c.ui, pol.Mode, pol.ConfirmEdits, c.mode, c.confirmEdits)
		}
	}
}
