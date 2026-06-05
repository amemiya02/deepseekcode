package agent

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// Steer enqueues; drainSteer pops the queue, appends each as a user message,
// and reports whether anything was drained. Empty steer is a no-op.
func TestSteerDrainAppendsUserMessages(t *testing.T) {
	a := &Agent{}
	if a.drainSteer() {
		t.Fatal("drainSteer on empty queue should report false")
	}
	a.Steer("focus on the parser")
	a.Steer("") // ignored
	a.Steer("and add a test")
	before := len(a.Messages)
	if !a.drainSteer() {
		t.Fatal("drainSteer with queued items should report true")
	}
	if got := len(a.Messages) - before; got != 2 {
		t.Fatalf("expected 2 appended user messages, got %d", got)
	}
	last := a.Messages[len(a.Messages)-1]
	if last.Role != "user" {
		t.Fatalf("expected role user, got %q", last.Role)
	}
	tb, ok := last.Blocks[0].(llm.TextBlock)
	if !ok || tb.Text != "and add a test" {
		t.Fatalf("expected steer text, got %#v", last.Blocks[0])
	}
	// Queue is emptied after a drain.
	if a.drainSteer() {
		t.Fatal("second drainSteer should report false (queue emptied)")
	}
}
