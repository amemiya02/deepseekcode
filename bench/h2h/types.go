// Package h2h is the dsc-vs-Reasonix head-to-head cache benchmark
// harness (spec: docs/superpowers/specs/2026-06-10-cache-supremacy-design.md §3).
package h2h

// TaskSpec is one pinned real-repo editing task.
type TaskSpec struct {
	ID        string `json:"id"`
	Repo      string `json:"repo"`
	Commit    string `json:"commit"`     // buggy commit (checkout this)
	FixCommit string `json:"fix_commit"` // fixing commit (checkout tests from here)
	Prompt    string `json:"prompt"`
	// FailToPass test names; "Test/Name" means a grpctest-style wrapper
	// suite subtest. Each slash segment is anchored ^...$ at run time,
	// so the leaf name must be unique package-wide for goldcheck's
	// failure attribution to work.
	FailToPass      []string `json:"fail_to_pass"`
	TestDir         string   `json:"test_dir"`
	TurnCap         int      `json:"turn_cap"`
	WallclockCapMin int      `json:"wallclock_cap_min"`
}

// TurnUsage is one model turn's provider usage counters.
type TurnUsage struct {
	HitTokens  int `json:"prompt_cache_hit_tokens"`
	MissTokens int `json:"prompt_cache_miss_tokens"`
	OutTokens  int `json:"completion_tokens"`
}

// ArmResult is one arm's outcome on one task run.
type ArmResult struct {
	Arm      string      `json:"arm"` // "dsc" | "reasonix"
	TaskID   string      `json:"task_id"`
	Repeat   int         `json:"repeat"`
	Resolved bool        `json:"resolved"`
	DNF      bool        `json:"dnf"` // crashed / hit cap
	Turns    []TurnUsage `json:"turns"`
	Err      string      `json:"err,omitempty"`
}

// HitRate returns hit/(hit+miss); 0 when no input tokens recorded.
func (r ArmResult) HitRate() float64 {
	var hit, miss int
	for _, t := range r.Turns {
		hit += t.HitTokens
		miss += t.MissTokens
	}
	if hit+miss == 0 {
		return 0
	}
	return float64(hit) / float64(hit+miss)
}

// Billable returns miss+output token total (the cost guard metric).
func (r ArmResult) Billable() int {
	var n int
	for _, t := range r.Turns {
		n += t.MissTokens + t.OutTokens
	}
	return n
}

// RunResult is the whole benchmark output, serialized to JSON.
type RunResult struct {
	Date           string      `json:"date"`
	Model          string      `json:"model"`
	ReasonixSHA256 string      `json:"reasonix_sha256"`
	DscCommit      string      `json:"dsc_commit"`
	Results        []ArmResult `json:"results"`
}
