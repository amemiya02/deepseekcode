package hooks

import (
	"context"
	"fmt"

	"github.com/amemiya02/deepseekcode/internal/memory"
)

// MemoryCapture is a hook handler that auto-captures tool outputs and
// session summaries into the long-term memory store.
type MemoryCapture struct {
	store memory.Store
}

// NewMemoryCapture returns a MemoryCapture wired to store.
func NewMemoryCapture(store memory.Store) *MemoryCapture {
	return &MemoryCapture{store: store}
}

// Handle processes HookInput for EventPostToolUse and EventSessionEnd.
// It extracts meaningful text, strips secrets, and calls store.Remember
// (which handles SHA dedup internally).
func (mc *MemoryCapture) Handle(ctx context.Context, input HookInput) error {
	switch input.Event {
	case EventPostToolUse:
		return mc.captureToolOutput(input)
	case EventSessionEnd:
		return mc.captureSessionSummary(input)
	}
	return nil
}

func (mc *MemoryCapture) captureToolOutput(input HookInput) error {
	output, _ := input.Extra["output"].(string)
	if output == "" {
		return nil
	}
	toolName, _ := input.Extra["tool"].(string)
	fact := fmt.Sprintf("[%s] %s", toolName, output)
	_, err := mc.store.Remember(fact, []string{"auto", "tool:" + toolName, "session:" + input.SessionID})
	return err
}

func (mc *MemoryCapture) captureSessionSummary(input HookInput) error {
	summary, _ := input.Extra["summary"].(string)
	if summary == "" {
		return nil
	}
	_, err := mc.store.Remember(summary, []string{"auto", "session-end", "session:" + input.SessionID})
	return err
}
