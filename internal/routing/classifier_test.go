// internal/routing/classifier_test.go
package routing

import "testing"

func cfg() Config {
	return Config{FlashModel: "deepseek-v4-flash", ProModel: "deepseek-v4-pro", StickyTurns: 2}
}

func TestMechanicalTurnStaysFlashNoThink(t *testing.T) {
	d := Classify(Signals{UserText: "read internal/llm/client.go"}, cfg(), Decision{})
	if d.Model != "deepseek-v4-flash" || d.Thinking {
		t.Fatalf("mechanical turn should be flash + no-think, got %+v", d)
	}
}

func TestHardReasoningEscalatesToProMax(t *testing.T) {
	d := Classify(Signals{UserText: "Why does the cache fingerprint diverge from the wire bytes? Prove it and redesign the epoch boundary."}, cfg(), Decision{})
	if d.Model != "deepseek-v4-pro" || d.Effort != "max" {
		t.Fatalf("hard reasoning should be pro + max, got %+v", d)
	}
}

func TestRepairErrorsForceEscalation(t *testing.T) {
	d := Classify(Signals{UserText: "edit the file", RepairErrorsLastTurn: 3}, cfg(), Decision{})
	if d.Model != "deepseek-v4-pro" {
		t.Fatalf("repeated repair errors should escalate, got %+v", d)
	}
}

func TestStickinessDoesNotFlapBackImmediately(t *testing.T) {
	prev := Decision{Model: "deepseek-v4-pro", StickyLeft: 2}
	d := Classify(Signals{UserText: "ok thanks"}, cfg(), prev)
	if d.Model != "deepseek-v4-pro" {
		t.Fatalf("should stay on pro while sticky, got %+v", d)
	}
	if d.StickyLeft != 1 {
		t.Fatalf("StickyLeft should decrement to 1, got %d", d.StickyLeft)
	}
}
