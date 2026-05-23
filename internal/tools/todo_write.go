package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// TodoWrite manages the session's structured plan. The TUI renders the
// active list inline; the agent reads it back to keep itself on track.
//
// Plan state is per-process (session) only. We don't persist across
// resume in v0.1 — the model can regenerate the plan from the
// conversation if it needs to.
type TodoWrite struct {
	mu    sync.Mutex
	items []TodoItem
}

// TodoItem mirrors the schema we expose to the model.
type TodoItem struct {
	Subject    string `json:"subject"`
	Status     string `json:"status"` // pending | in_progress | completed
	ActiveForm string `json:"active_form,omitempty"`
}

func (t *TodoWrite) Name() string { return "todo_write" }

func (t *TodoWrite) Description() string {
	return "Replace the current TODO list with a new one. The TUI renders it inline " +
		"and you (the model) can re-read it by calling this with no todos to see the current state " +
		"(actually, this tool always replaces; to inspect, just look at your prior tool result). " +
		"Use exactly one in_progress item at a time. Statuses: pending, in_progress, completed."
}

func (t *TodoWrite) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"todos": map[string]any{
				"type":        "array",
				"description": "Full replacement list.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"subject": map[string]any{
							"type":        "string",
							"description": "Imperative form: 'Fix auth bug' (not 'Fixing auth bug').",
						},
						"status": map[string]any{
							"type": "string",
							"enum": []string{"pending", "in_progress", "completed"},
						},
						"active_form": map[string]any{
							"type":        "string",
							"description": "Present-continuous: 'Fixing auth bug'. Shown in spinners when in_progress.",
						},
					},
					"required": []string{"subject", "status"},
				},
			},
		},
		"required": []string{"todos"},
	})
}

func (t *TodoWrite) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Todos []TodoItem `json:"todos"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("invalid args: %v", err), nil
	}
	// Validate.
	inProgress := 0
	for i, item := range p.Todos {
		if item.Subject == "" {
			return Errf("todos[%d].subject is empty", i), nil
		}
		switch item.Status {
		case "pending", "in_progress", "completed":
		default:
			return Errf("todos[%d].status=%q invalid (use pending|in_progress|completed)",
				i, item.Status), nil
		}
		if item.Status == "in_progress" {
			inProgress++
		}
	}
	if inProgress > 1 {
		return Errf("only one item may be in_progress at a time (found %d)", inProgress), nil
	}

	t.mu.Lock()
	t.items = p.Todos
	t.mu.Unlock()

	return Result{Content: renderTodos(p.Todos)}, nil
}

// Snapshot returns a copy of the current todo list. TUI uses this.
func (t *TodoWrite) Snapshot() []TodoItem {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]TodoItem, len(t.items))
	copy(out, t.items)
	return out
}

func renderTodos(items []TodoItem) string {
	if len(items) == 0 {
		return "(plan cleared)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "plan (%d item%s):\n", len(items), plural(len(items)))
	for _, it := range items {
		var glyph string
		switch it.Status {
		case "completed":
			glyph = "[x]"
		case "in_progress":
			glyph = "[*]"
		default:
			glyph = "[ ]"
		}
		fmt.Fprintf(&b, "  %s %s\n", glyph, it.Subject)
	}
	return b.String()
}
