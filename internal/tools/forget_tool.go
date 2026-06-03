package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/amemiya02/deepseekcode/internal/memory"
)

// ForgetTool removes a memory by ID.
type ForgetTool struct{ store memory.Store }

// NewForgetTool returns a ForgetTool backed by the given Store.
func NewForgetTool(store memory.Store) *ForgetTool { return &ForgetTool{store: store} }

func (t *ForgetTool) Name() string { return "forget" }

func (t *ForgetTool) IsReadOnly() bool { return false }

func (t *ForgetTool) Description() string {
	return "Remove a fact from long-term memory by its ID."
}

func (t *ForgetTool) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{"type": "string", "description": "Memory ID to forget."},
		},
		"required": []string{"id"},
	})
}

func (t *ForgetTool) Execute(_ context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("forget: invalid args: %v", err), nil
	}
	if p.ID == "" {
		return Errf("forget: id is required"), nil
	}
	if err := t.store.Forget(p.ID); err != nil {
		return Result{}, fmt.Errorf("forget: store error: %w", err)
	}
	return Result{Content: fmt.Sprintf(`{"status":"forgotten","id":%q}`, p.ID)}, nil
}
