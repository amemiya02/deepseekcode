package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// SpawnBatchTool fans out a list of SpawnRequests concurrently on a shared
// Spawner, capped to maxConcurrency goroutines, then aggregates the results.
// It reuses the same tools.Spawner interface as SubagentTool so LoopSpawner
// backs it without modification.
type SpawnBatchTool struct {
	s              Spawner
	maxConcurrency int
}

// NewSpawnBatchTool creates a SpawnBatchTool. maxConcurrency <= 0 defaults to 4.
func NewSpawnBatchTool(s Spawner, maxConcurrency int) *SpawnBatchTool {
	if maxConcurrency <= 0 {
		maxConcurrency = 4
	}
	return &SpawnBatchTool{s: s, maxConcurrency: maxConcurrency}
}

func (*SpawnBatchTool) Name() string { return "spawn_batch" }

func (*SpawnBatchTool) Description() string {
	return "Fan out multiple subtasks to sub-agents concurrently and collect all results. " +
		"Each task is a SpawnRequest object ({description, agent?, tools?, async?}). " +
		"Tasks run in parallel (capped to avoid overload) and results are returned in order. " +
		"Prefer this over sequential task calls when subtasks are independent."
}

func (*SpawnBatchTool) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tasks": map[string]any{
				"type":        "array",
				"description": "List of subtask requests to run in parallel",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"description": map[string]any{"type": "string"},
						"agent":       map[string]any{"type": "string"},
						"tools": map[string]any{
							"type":  "array",
							"items": map[string]any{"type": "string"},
						},
					},
					"required": []string{"description"},
				},
			},
			"max_concurrency": map[string]any{
				"type":        "integer",
				"description": "Maximum parallel sub-agents (default: tool default)",
				"default":     4,
			},
		},
		"required": []string{"tasks"},
	})
}

func (*SpawnBatchTool) IsReadOnly() bool { return false }

type batchTaskSpec struct {
	Description string   `json:"description"`
	Agent       string   `json:"agent"`
	Tools       []string `json:"tools"`
}

type batchResult struct {
	idx     int
	summary string
	err     error
}

func (t *SpawnBatchTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Tasks          []batchTaskSpec `json:"tasks"`
		MaxConcurrency int             `json:"max_concurrency"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("spawn_batch: invalid args: %v", err), nil
	}
	if len(p.Tasks) == 0 {
		return Result{Content: "spawn_batch: no tasks; nothing to do"}, nil
	}
	if t.s == nil {
		return Errf("spawn_batch: no spawner configured"), nil
	}

	cap := t.maxConcurrency
	if p.MaxConcurrency > 0 {
		cap = p.MaxConcurrency
	}

	sem := make(chan struct{}, cap)
	results := make([]batchResult, len(p.Tasks))
	var wg sync.WaitGroup

	for i, task := range p.Tasks {
		if task.Description == "" {
			results[i] = batchResult{idx: i, summary: "[error: description is required]"}
			continue
		}
		wg.Add(1)
		go func(idx int, req batchTaskSpec) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res, err := t.s.Spawn(ctx, SpawnRequest{
				Agent:       req.Agent,
				Description: req.Description,
				Tools:       req.Tools,
			})
			if err != nil {
				results[idx] = batchResult{idx: idx, summary: fmt.Sprintf("[error: %v]", err)}
				return
			}
			sum := res.Summary
			if sum == "" {
				sum = "(no summary)"
			}
			results[idx] = batchResult{idx: idx, summary: sum}
		}(i, task)
	}
	wg.Wait()

	var sb strings.Builder
	for i, r := range results {
		fmt.Fprintf(&sb, "task[%d]: %s\n", i, r.summary)
	}
	return Result{Content: sb.String()}, nil
}
