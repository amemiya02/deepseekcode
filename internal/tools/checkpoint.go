package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// CheckpointRecorder is the narrow callback the CheckpointTool uses.
// Implemented by *agent.Agent via a thin wrapper to avoid import cycles.
type CheckpointRecorder interface {
	// RecordCheckpoint names the current step. Returns the step index recorded.
	RecordCheckpoint(name string) int
}

// CheckpointTool lets the model name the current agent step so sessions
// can be resumed or branched from that point by name.
type CheckpointTool struct{ rec CheckpointRecorder }

// NewCheckpointTool creates a CheckpointTool backed by rec.
func NewCheckpointTool(rec CheckpointRecorder) *CheckpointTool {
	return &CheckpointTool{rec: rec}
}

func (*CheckpointTool) Name() string { return "checkpoint" }

func (*CheckpointTool) Description() string {
	return "Name the current agent step so it can be resumed or branched later. " +
		"Call before a risky operation to create a named restore point. " +
		"Use /branch <name> to fork from this checkpoint."
}

func (*CheckpointTool) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Human-readable checkpoint name (e.g. \"before-refactor\")",
			},
		},
		"required": []string{"name"},
	})
}

// IsReadOnly returns false: recording a checkpoint writes to the agent's
// CheckpointIndex, which is an agent-state mutation even though no files
// are touched. The repair module's ToolMutating default applies here.
func (*CheckpointTool) IsReadOnly() bool { return false }

func (t *CheckpointTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("checkpoint: invalid args: %v", err), nil
	}
	if p.Name == "" {
		return Errf("checkpoint: name is required"), nil
	}
	if t.rec == nil {
		return Errf("checkpoint: no recorder configured"), nil
	}
	step := t.rec.RecordCheckpoint(p.Name)
	return Result{Content: fmt.Sprintf("checkpoint %q recorded at step %d", p.Name, step)}, nil
}
