package llm

import (
	"testing"
	"time"
)

func TestPricingFreshnessAt(t *testing.T) {
	now := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	f, err := PricingFreshnessAt(now)
	if err != nil {
		t.Fatalf("PricingFreshnessAt: %v", err)
	}
	if f.AgeDays != 6 {
		t.Errorf("AgeDays = %d, want 6", f.AgeDays)
	}
	if f.Stale {
		t.Error("expected Stale=false for 6 days")
	}
	if f.SourceURL != PricingSourceURL {
		t.Errorf("SourceURL = %q, want %q", f.SourceURL, PricingSourceURL)
	}
}

func TestPricingFreshnessStale(t *testing.T) {
	now := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	f, err := PricingFreshnessAt(now)
	if err != nil {
		t.Fatalf("PricingFreshnessAt: %v", err)
	}
	if f.AgeDays != 36 {
		t.Errorf("AgeDays = %d, want 36", f.AgeDays)
	}
	if !f.Stale {
		t.Error("expected Stale=true for 36 days")
	}
}

func TestPricingFreshnessBeforeChecked(t *testing.T) {
	now := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	f, err := PricingFreshnessAt(now)
	if err != nil {
		t.Fatalf("PricingFreshnessAt: %v", err)
	}
	if f.AgeDays != 0 {
		t.Errorf("AgeDays = %d, want 0 (before checked date)", f.AgeDays)
	}
	if f.Stale {
		t.Error("expected Stale=false when before checked date")
	}
}
