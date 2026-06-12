package agent

// BudgetPolicy configures session cost thresholds. Zero values disable
// the corresponding gate.
type BudgetPolicy struct {
	WarnCNY float64 // emit warning when projected spend >= WarnCNY
	HardCNY float64 // block turn when projected spend >= HardCNY
}

// Active reports whether any cost gate is configured. When neither threshold is
// set there is nothing to cost-gate, so warnings about an unpriceable model
// (e.g. an openai-compat provider with no pricing table) are pure noise and
// must be suppressed.
func (p BudgetPolicy) Active() bool {
	return p.WarnCNY > 0 || p.HardCNY > 0
}

// BudgetState tracks cumulative spend and whether a warning has already
// been emitted for this session.
type BudgetState struct {
	SpentCNY float64
	Warned   bool

	// Rolling cache-hit accounting over every billed turn this session. Used
	// to discount the pre-stream cost projection by the realized cache-hit
	// rate (T4.2). Only frames with input tokens (hit+miss>0) fold in, so an
	// empty/absent usage frame can't skew the rate.
	CacheHitTokens  int
	CacheMissTokens int

	// UnknownModelWarned makes the "model has no known pricing, so the budget
	// gate can't cost-gate it" warning fire at most once per session.
	UnknownModelWarned bool
}

// SessionCacheHitRate is the rolling cache-hit fraction in [0,1] over all input
// tokens billed this session. It returns 0 before any usage is observed (cold
// start), so a projection discounted by it floors to all-miss — never looser
// than the conservative default. The result is clamped defensively.
func (s BudgetState) SessionCacheHitRate() float64 {
	total := s.CacheHitTokens + s.CacheMissTokens
	if total <= 0 {
		return 0
	}
	r := float64(s.CacheHitTokens) / float64(total)
	if r < 0 {
		return 0
	}
	if r > 1 {
		return 1
	}
	return r
}

// FoldCacheUsage adds one turn's realized cache hit/miss token counts into the
// rolling session accounting. Turns with no input tokens are ignored so they
// can't move the rate.
func (s *BudgetState) FoldCacheUsage(hitTokens, missTokens int) {
	if hitTokens < 0 {
		hitTokens = 0
	}
	if missTokens < 0 {
		missTokens = 0
	}
	if hitTokens+missTokens == 0 {
		return
	}
	s.CacheHitTokens += hitTokens
	s.CacheMissTokens += missTokens
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
