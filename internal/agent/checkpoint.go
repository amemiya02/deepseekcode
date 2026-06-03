package agent

import (
	"sort"
	"sync"
)

// NamedCheckpoint records a human-readable name for a step index so sessions
// can be resumed or branched from a known point.
type NamedCheckpoint struct {
	Name    string
	StepIdx int
}

// CheckpointIndex is a concurrency-safe name→stepIdx registry.
// It is owned by Agent and reset on each Run.
type CheckpointIndex struct {
	mu    sync.RWMutex
	index map[string]int
}

func newCheckpointIndex() *CheckpointIndex {
	return &CheckpointIndex{index: make(map[string]int)}
}

// Record associates name with stepIdx, overwriting any prior association.
func (c *CheckpointIndex) Record(name string, stepIdx int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.index[name] = stepIdx
}

// Lookup returns the step index for name. ok is false if name is unknown.
func (c *CheckpointIndex) Lookup(name string) (stepIdx int, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	si, ok := c.index[name]
	return si, ok
}

// Names returns all recorded checkpoint names in sorted order.
func (c *CheckpointIndex) Names() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, 0, len(c.index))
	for n := range c.index {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
