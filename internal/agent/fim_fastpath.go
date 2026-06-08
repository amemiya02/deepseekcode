// fim_fastpath.go implements a gated, conservative eligibility check for
// Fill-In-the-Middle (FIM) cheap-edit completions. When enabled (config
// [cache].fim_fastpath = true), small single-hunk edits can be routed
// through a cheaper FIM endpoint instead of the full chat completion path.
//
// THIS IS A PROTOTYPE — default OFF. Do not route real edits through it
// without separate quality evidence (per the honesty guardrail).
package agent

// editRequest describes a pending edit for FIM eligibility screening.
type editRequest struct {
	// LinesChanged is the total number of lines the edit adds + removes.
	LinesChanged int
	// SingleHunk is true when the edit touches one contiguous region.
	SingleHunk bool
}

// fimFastPathEnabled reads the config gate. Returns true only when the
// user explicitly opted in via [cache].fim_fastpath = true.
func fimFastPathEnabled(cfg interface{ FIMFastPathEnabled() bool }) bool {
	return cfg.FIMFastPathEnabled()
}

// eligibleForFIM applies conservative eligibility rules. The fast path
// must NOT fire on large edits, multi-hunk edits, or ambiguous cases.
// Returns true only for small, single-hunk edits.
func eligibleForFIM(req editRequest) bool {
	const maxLinesForFIM = 20
	if req.LinesChanged > maxLinesForFIM {
		return false
	}
	if !req.SingleHunk {
		return false
	}
	return true
}
