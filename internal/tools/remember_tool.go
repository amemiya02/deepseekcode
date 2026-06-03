package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/amemiya02/deepseekcode/internal/memory"
)

// RememberTool stores a fact in long-term memory.
type RememberTool struct{ store memory.Store }

// NewRememberTool returns a RememberTool backed by the given Store.
func NewRememberTool(store memory.Store) *RememberTool { return &RememberTool{store: store} }

func (t *RememberTool) Name() string { return "remember" }

func (t *RememberTool) IsReadOnly() bool { return false }

func (t *RememberTool) Description() string {
	return "Store a fact in long-term memory for future sessions."
}

func (t *RememberTool) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{"type": "string", "description": "The fact to remember."},
			"tags": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Optional tags.",
			},
		},
		"required": []string{"content"},
	})
}

func (t *RememberTool) Execute(_ context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Content string   `json:"content"`
		Tags    []string `json:"tags"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("remember: invalid args: %v", err), nil
	}
	if p.Content == "" {
		return Errf("remember: content is required"), nil
	}
	id, err := t.store.Remember(p.Content, p.Tags)
	if err != nil {
		return Result{}, fmt.Errorf("remember: store error: %w", err)
	}
	return Result{Content: fmt.Sprintf(`{"id":%q,"status":"stored"}`, id)}, nil
}
