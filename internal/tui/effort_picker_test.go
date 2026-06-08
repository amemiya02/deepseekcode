package tui

import "testing"

func TestOverlay_EffortPicker(t *testing.T) {
	var o Overlay
	o.OpenEffort("medium")
	if !o.IsOpen() {
		t.Fatal("effort picker should be open")
	}
	if got := o.SelectedEffort(); got != "medium" {
		t.Fatalf("cursor should start on current effort, got %q", got)
	}
}
