package agent

import (
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// TestAgentBusSecondConsumerParity verifies that a consumer subscribed
// via a.Bus().Subscribe receives every event the agent emits, in the
// same order as the legacy a.Events() consumer. This pins the
// bus-migration invariant (T-2503 AC #5).
func TestAgentBusSecondConsumerParity(t *testing.T) {
	a := New(nil, tools.New(), permissions.New(permissions.ModeReadOnly, "", nil, nil, nil), "x")
	sub := a.Bus().Subscribe(256)
	defer a.bus.Close()

	a.EmitInfo("a")
	a.EmitInfo("b")
	a.EmitInfo("c")

	for _, want := range []string{"a", "b", "c"} {
		select {
		case ev := <-a.Events():
			if info, ok := ev.(EventInfo); !ok || info.Text != want {
				t.Fatalf("Events(): got %T %v, want EventInfo{%q}", ev, ev, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("Events(): timed out waiting for %q", want)
		}
		select {
		case env := <-sub.C:
			if env.Seq == 0 {
				t.Errorf("Bus subscriber: Seq should be > 0")
			}
			if info, ok := env.Event.(EventInfo); !ok || info.Text != want {
				t.Errorf("Bus subscriber: got %T %v, want EventInfo{%q}", env.Event, env.Event, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("Bus subscriber: timed out waiting for %q", want)
		}
	}
}
