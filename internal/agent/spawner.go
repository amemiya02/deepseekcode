// Package agent is the deepseekcode ReAct loop. It owns turn boundaries,
// tool dispatch, callback fan-out, and stop conditions.
//
// The shape closely mirrors charmbracelet/crush's internal/agent/agent.go
// (callback table + StopWhen []StopCondition). The three load-bearing
// patches from docs/design.md §6.4 are applied here: stream/present
// split, finish-reason override, and two-tier timeout (the timeout is
// applied at the llm.Client layer; this package just configures it).
package agent

import "context"

// Spawner is the v0.2 interface for subagent dispatch. v0.1 leaves it
// unimplemented; reserving the type makes the v0.2 addition additive
// rather than a refactor.
type Spawner interface {
	Spawn(ctx context.Context, task SubTask) (SubResult, error)
}

// SubTask is what a parent agent hands to a subagent.
type SubTask struct {
	Description string
	Tools       []string // names; subset of parent registry
}

// SubResult is the summary a subagent returns.
type SubResult struct {
	Summary    string
	StepCount  int
	TokenCount int
}
