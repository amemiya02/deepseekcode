// Package traceschema is the single source of truth for an agent trace
// record — one JSONL line emitted by internal/agent, read by
// internal/traceinspect, and read by the benchmark harness
// (bench/cmd/benchrunner). All three alias traceschema.Record, so renaming or
// retyping a field is a compile error in every consumer instead of a silent
// reader breakage.
//
// Note: the benchrunner's own run-summary writer (its TraceRecord type, with
// success/error/duration fields) is a deliberately distinct schema and is NOT
// this type — only the agent-trace reader/emitter triple is unified here.
package traceschema

// Version is stamped onto SchemaVersion of every emitted record. Bump it when
// the record shape changes in a way a reader must branch on.
const Version = 1

// Record is one agent-trace JSONL line. Tags use omitempty to keep the emitted
// bytes byte-stable with the pre-T6.1 emitter (readers ignore omitempty).
type Record struct {
	Type string `json:"type"`
	Turn *int   `json:"turn,omitempty"`

	// run_id/agent_role/parent_epoch_id identify which agent emitted the
	// record (root vs subagent), so the benchmark can attribute a child epoch
	// to its parent instead of hardcoding a pass.
	RunID         string `json:"run_id,omitempty"`
	AgentRole     string `json:"agent_role,omitempty"`
	ParentEpochID string `json:"parent_epoch_id,omitempty"`

	EpochID          string `json:"epoch_id,omitempty"`
	OldEpochID       string `json:"old_epoch_id,omitempty"`
	Model            string `json:"model,omitempty"`
	StaticPrefixHash string `json:"static_prefix_hash,omitempty"`
	ToolsHash        string `json:"tools_hash,omitempty"`
	Reason           string `json:"reason,omitempty"`

	// usage
	CacheHitTokens  *int     `json:"cache_hit_tokens,omitempty"`
	CacheMissTokens *int     `json:"cache_miss_tokens,omitempty"`
	OutputTokens    *int     `json:"output_tokens,omitempty"`
	CostCNY         *float64 `json:"cost_cny,omitempty"`

	// pending_change / drift.blocked
	Kind        string `json:"kind,omitempty"`
	Description string `json:"description,omitempty"`
	Which       string `json:"which,omitempty"`

	// compaction — the measured static-prefix fingerprints before and after.
	BeforeStaticPrefixHash string   `json:"before_static_prefix_hash,omitempty"`
	AfterStaticPrefixHash  string   `json:"after_static_prefix_hash,omitempty"`
	SummaryCostCNY         *float64 `json:"summary_cost_cny,omitempty"`

	// SchemaVersion is stamped by the emitter (= Version) on every record.
	// Absent on legacy traces predating T6.1 → decodes as 0. Placed last so
	// the emitted key order of all prior fields is unchanged.
	SchemaVersion int `json:"schema_version,omitempty"`
}
