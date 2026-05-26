package agent

// BudgetPolicy configures session cost thresholds. Zero values disable
// the corresponding gate.
type BudgetPolicy struct {
	WarnCNY float64 // emit warning when projected spend >= WarnCNY
	HardCNY float64 // block turn when projected spend >= HardCNY
}

// BudgetState tracks cumulative spend and whether a warning has already
// been emitted for this session.
type BudgetState struct {
	SpentCNY float64
	Warned   bool
}

// CheckBudget evaluates whether a model turn should proceed given the
// policy, current state, and projected cost for the upcoming turn.
//
// Returns:
//   - allow: true if the turn may proceed; false if hard limit is exceeded
//   - warn: true if this call should emit a warning (first time crossing WarnCNY)
//
// Pure function: does not modify state. The caller updates BudgetState
// based on the returned flags.
func CheckBudget(policy BudgetPolicy, state BudgetState, projectedCNY float64) (allow bool, warn bool) {
	// Treat negative projected cost as zero (defensive)
	if projectedCNY < 0 {
		projectedCNY = 0
	}

	total := state.SpentCNY + projectedCNY

	// Hard block: when HardCNY > 0 and total meets/exceeds threshold
	if policy.HardCNY > 0 && total >= policy.HardCNY {
		// Only warn if WarnCNY is enabled and threshold is met
		warn := policy.WarnCNY > 0 && total >= policy.WarnCNY && !state.Warned
		return false, warn
	}

	// Warning: when WarnCNY > 0, total meets/exceeds threshold, and not already warned
	if policy.WarnCNY > 0 && total >= policy.WarnCNY && !state.Warned {
		return true, true
	}

	return true, false
}
