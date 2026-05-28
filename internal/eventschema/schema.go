package eventschema

const (
	ModelTurnStarted    = "model.turn.started"
	ModelTurnFinished   = "model.turn.finished"
	PrefixEpochCreated  = "prefix.epoch.created"
	PrefixEpochFrozen   = "prefix.epoch.frozen"
	PrefixPendingChange = "prefix.pending_change"
	PrefixDriftBlocked  = "prefix.drift.blocked"
	SessionCompacted    = "session.compacted"
	ToolStarted         = "tool.started"
	ToolFinished        = "tool.finished"
	SubagentStarted     = "subagent.started"
	SubagentFinished    = "subagent.finished"
)

var known = map[string]bool{
	ModelTurnStarted:    true,
	ModelTurnFinished:   true,
	PrefixEpochCreated:  true,
	PrefixEpochFrozen:   true,
	PrefixPendingChange: true,
	PrefixDriftBlocked:  true,
	SessionCompacted:    true,
	ToolStarted:         true,
	ToolFinished:        true,
	SubagentStarted:     true,
	SubagentFinished:    true,
}

func Known(name string) bool {
	return known[name]
}
