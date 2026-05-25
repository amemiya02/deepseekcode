package tools

import (
	"context"
	"encoding/json"
)

// SpawnRequest describes a sub-agent task.
type SpawnRequest struct {
	Agent       string   // .deepseek/agent/<name>; empty = default generic sub-agent
	Description string   // task description for the sub-agent
	Tools       []string // optional: further restrict beyond agent def whitelist
}

// SpawnResult is the outcome of a completed sub-agent run.
type SpawnResult struct {
	Summary    string
	StepCount  int
	TokenCount int
}

// Spawner is implemented by the agent layer (e.g. agent.LoopSpawner).
type Spawner interface {
	Spawn(ctx context.Context, req SpawnRequest) (SpawnResult, error)
}

// SubagentTool delegates a subtask to a sub-agent.
type SubagentTool struct{ s Spawner }

// NewSubagentTool creates a SubagentTool backed by the given Spawner.
func NewSubagentTool(s Spawner) *SubagentTool { return &SubagentTool{s: s} }

func (*SubagentTool) Name() string { return "task" }

func (*SubagentTool) Description() string {
	return "Delegate a self-contained subtask to a sub-agent with a restricted tool set. " +
		"The sub-agent runs to completion and returns a summary. " +
		"Use for parallel exploration or scoped work."
}

func (*SubagentTool) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent":       map[string]any{"type": "string"},
			"description": map[string]any{"type": "string"},
			"tools": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
		"required": []string{"description"},
	})
}

func (t *SubagentTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Agent       string   `json:"agent"`
		Description string   `json:"description"`
		Tools       []string `json:"tools"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("task: invalid args: %v", err), nil
	}
	if p.Description == "" {
		return Errf("task: description is required"), nil
	}
	if t.s == nil {
		return Errf("task: no spawner configured"), nil
	}
	res, err := t.s.Spawn(ctx, SpawnRequest{
		Agent:       p.Agent,
		Description: p.Description,
		Tools:       p.Tools,
	})
	if err != nil {
		return Errf("task failed: %v", err), nil
	}
	if res.Summary == "" {
		return Result{Content: "(subagent returned no summary)"}, nil
	}
	return Result{Content: res.Summary}, nil
}

func (*SubagentTool) IsReadOnly() bool { return false }
