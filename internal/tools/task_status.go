package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// JobStatusController is the narrow interface for status queries.
type JobStatusController interface {
	JobStatus(id string) (Status, error)
	CancelJob(id string) error
}

// Status represents the public status of a background job.
type Status struct {
	ID           string
	Kind         string
	State        string
	StartedAt    time.Time
	FinishedAt   time.Time
	Summary      string
	Tail         string
	DroppedBytes int64
	TotalLines   int
	Truncated    bool
}

// TaskStatusTool queries the status of a background job.
type TaskStatusTool struct {
	c JobStatusController
}

// NewTaskStatusTool creates a TaskStatusTool backed by the given controller.
func NewTaskStatusTool(c JobStatusController) *TaskStatusTool {
	return &TaskStatusTool{c: c}
}

func (*TaskStatusTool) Name() string { return "task_status" }

func (*TaskStatusTool) Description() string {
	return "Query the status of a background job (started by background_bash or task{async:true}). " +
		"Returns current state, summary, and recent output. Use cancel:true to cancel a running job."
}

func (*TaskStatusTool) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"job_id":     map[string]any{"type": "string", "description": "The job ID to query"},
			"tail_lines": map[string]any{"type": "integer", "description": "Number of output lines to return (default 200)", "default": 200},
			"cancel":     map[string]any{"type": "boolean", "description": "Cancel the job if still running", "default": false},
		},
		"required": []string{"job_id"},
	})
}

func (*TaskStatusTool) IsReadOnly() bool { return true }

func (t *TaskStatusTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		JobID     string `json:"job_id"`
		TailLines int    `json:"tail_lines"`
		Cancel    bool   `json:"cancel"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("task_status: invalid args: %v", err), nil
	}
	if p.JobID == "" {
		return Errf("job_id is required"), nil
	}
	if t.c == nil {
		return Errf("task_status: no controller configured"), nil
	}
	if p.TailLines <= 0 {
		p.TailLines = 200
	}

	// Cancel first if requested (errors are ignored - job may already be done)
	if p.Cancel {
		_ = t.c.CancelJob(p.JobID)
	}

	status, err := t.c.JobStatus(p.JobID)
	if err != nil {
		return Errf("job not found: %s", p.JobID), nil
	}

	// Render result
	var content string
	finished := "-"
	if !status.FinishedAt.IsZero() {
		finished = status.FinishedAt.Format(time.RFC3339)
	}
	summary := status.Summary
	if summary == "" {
		summary = "-"
	}

	content = fmt.Sprintf("state: %s\nkind: %s\nstarted: %s  finished: %s\nsummary: %s",
		status.State, status.Kind, status.StartedAt.Format(time.RFC3339), finished, summary)

	if p.Cancel && status.State == "running" {
		content += "\n(cancel requested; state may be running until child exits)"
	}

	content += "\n----\n"

	if status.Tail == "" {
		content += "(no output)"
	} else {
		content += status.Tail
	}

	if status.DroppedBytes > 0 {
		content += fmt.Sprintf("\n(output truncated: %d bytes dropped from head)", status.DroppedBytes)
	}

	return Result{Content: content}, nil
}
