package sandbox

import "testing"

func TestDetectNonNil(t *testing.T) {
	if Detect() == nil {
		t.Fatal("Detect() returned nil")
	}
}

func TestAvailableAlwaysCheckable(t *testing.T) {
	_ = Detect().Available()
}

func TestWasDeniedEmptyFalse(t *testing.T) {
	if Detect().WasDenied("") {
		t.Fatal("WasDenied(\"\") = true, want false")
	}
}
