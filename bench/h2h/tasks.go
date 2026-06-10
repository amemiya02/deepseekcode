package h2h

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadTasks reads and validates tasks.json. Every field except the
// caps is required; caps default to 30 turns / 20 minutes.
func LoadTasks(path string) ([]TaskSpec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tasks []TaskSpec
	if err := json.Unmarshal(raw, &tasks); err != nil {
		return nil, fmt.Errorf("tasks.json: %w", err)
	}
	for i := range tasks {
		t := &tasks[i]
		if t.ID == "" || t.Repo == "" || t.Commit == "" || t.FixCommit == "" || t.Prompt == "" || len(t.FailToPass) == 0 || t.TestDir == "" {
			return nil, fmt.Errorf("task %d (%q): missing required field", i, t.ID)
		}
		if t.TurnCap <= 0 {
			t.TurnCap = 30
		}
		if t.WallclockCapMin <= 0 {
			t.WallclockCapMin = 20
		}
	}
	return tasks, nil
}
