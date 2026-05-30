package llm

import (
	"fmt"
	"time"
)

// PricingSourceURL is the canonical source for the hardcoded pricing
// table. Doctor surfaces it so the user can verify freshness.
const PricingSourceURL = "https://api-docs.deepseek.com/quick_start/pricing"

// PricingCheckedDate is the date the pricing table was last verified
// against the official source. Update this when Prices changes.
const PricingCheckedDate = "2026-05-25"

// PricingFreshness holds the computed freshness metadata for the
// hardcoded pricing table.
type PricingFreshness struct {
	SourceURL string
	CheckedAt time.Time
	AgeDays   int
	Stale     bool
}

// PricingFreshnessAt computes the freshness of the pricing table
// relative to now. Stale is true when AgeDays > 30.
func PricingFreshnessAt(now time.Time) (PricingFreshness, error) {
	checkedAt, err := time.Parse("2006-01-02", PricingCheckedDate)
	if err != nil {
		return PricingFreshness{}, fmt.Errorf("parse PricingCheckedDate %q: %w", PricingCheckedDate, err)
	}
	age := now.Sub(checkedAt)
	ageDays := int(age.Hours() / 24)
	if ageDays < 0 {
		ageDays = 0
	}
	return PricingFreshness{
		SourceURL: PricingSourceURL,
		CheckedAt: checkedAt,
		AgeDays:   ageDays,
		Stale:     ageDays > 30,
	}, nil
}
