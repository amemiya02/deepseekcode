package tools

import (
	"context"
	"encoding/json"
)

// StepEvidence is one evidenced plan step.
type StepEvidence struct {
	Step     string
	Evidence string
}

// StepRecorder is implemented by the agent; complete_step appends to its ledger.
type StepRecorder interface {
	RecordStep(StepEvidence)
}

// CompleteStep records that a plan step is done with evidence.
type CompleteStep struct{ Controller StepRecorder }

func (CompleteStep) Name() string     { return "complete_step" }
func (CompleteStep) IsReadOnly() bool { return true }

func (CompleteStep) Description() string {
	return "Record a plan step as complete with evidence (a command + its output, or a test result). " +
		"Both step and evidence are required. Available in plan mode."
}

func (CompleteStep) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"step": map[string]any{
				"type":        "string",
				"description": "The plan step that was completed.",
			},
			"evidence": map[string]any{
				"type":        "string",
				"description": "Proof the step is done: a command output, test result, or verification.",
			},
		},
		"required": []string{"step", "evidence"},
	})
}

func (c CompleteStep) Execute(_ context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Step     string `json:"step"`
		Evidence string `json:"evidence"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("invalid args: %v", err), nil
	}
	if p.Step == "" || p.Evidence == "" {
		return Errf("both step and evidence are required; evidence is the proof the step is done"), nil
	}
	if c.Controller != nil {
		c.Controller.RecordStep(StepEvidence{Step: p.Step, Evidence: p.Evidence})
	}
	return Result{Content: "recorded step with evidence: " + p.Step}, nil
}
