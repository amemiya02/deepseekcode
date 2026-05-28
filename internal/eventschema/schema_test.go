package eventschema

import "testing"

func TestKnownEventNames(t *testing.T) {
	for _, name := range []string{
		ModelTurnStarted,
		ModelTurnFinished,
		PrefixEpochCreated,
		PrefixEpochFrozen,
		PrefixPendingChange,
		PrefixDriftBlocked,
		SessionCompacted,
		ToolStarted,
		ToolFinished,
		SubagentStarted,
		SubagentFinished,
	} {
		if !Known(name) {
			t.Fatalf("Known(%q) = false, want true", name)
		}
	}
}

func TestUnknownEventName(t *testing.T) {
	if Known("prefix.made_up") {
		t.Fatal("unknown event name reported as known")
	}
}
