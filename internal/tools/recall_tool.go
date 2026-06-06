package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/amemiya02/deepseekcode/internal/memory"
)

// RecallTool retrieves relevant memories by query.
type RecallTool struct{ store memory.Store }

// NewRecallTool returns a RecallTool backed by the given Store.
func NewRecallTool(store memory.Store) *RecallTool { return &RecallTool{store: store} }

func (t *RecallTool) Name() string { return "recall" }

func (t *RecallTool) IsReadOnly() bool { return true }

func (t *RecallTool) Description() string {
	return "Recall relevant facts from long-term memory by semantic query."
}

func (t *RecallTool) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "Search query."},
		},
		"required": []string{"query"},
	})
}

func (t *RecallTool) Execute(_ context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("recall: invalid args: %v", err), nil
	}
	if p.Query == "" {
		return Errf("recall: query is required"), nil
	}
	results, err := t.store.Recall(p.Query)
	if err != nil {
		return Result{}, fmt.Errorf("recall: store error: %w", err)
	}
	b, err := json.Marshal(results)
	if err != nil {
		return Result{}, fmt.Errorf("recall: marshal error: %w", err)
	}
	return Result{Content: string(b)}, nil
}
