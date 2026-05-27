package agent

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

func decodeTrace(t *testing.T, b []byte) []map[string]any {
	t.Helper()
	var recs []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("bad JSONL line %q: %v", line, err)
		}
		recs = append(recs, m)
	}
	return recs
}

func TestTraceSinkEmitsRealEpochAndUsage(t *testing.T) {
	var buf bytes.Buffer
	s := NewTraceSink(&buf, "deepseek-v4-flash")

	s.Handle(EventEpochCreated{EpochID: "epoch_1", StaticPrefixHash: "abc123", ToolsHash: "tools9", Reason: "session_start"})
	s.Handle(EventEpochFrozen{EpochID: "epoch_1"})
	// Two turns: first is cold-start (miss), second is warm (mostly hit).
	s.Handle(EventStepFinish{Reason: StopUnknown, Usage: llm.Usage{PromptCacheHitTokens: 0, PromptCacheMissTokens: 1000, CompletionTokens: 50}})
	s.Handle(EventStepFinish{Reason: StopModelDone, Usage: llm.Usage{PromptCacheHitTokens: 980, PromptCacheMissTokens: 20, CompletionTokens: 40}})

	recs := decodeTrace(t, buf.Bytes())

	var snapshots, usages int
	for _, r := range recs {
		switch r["type"] {
		case "prefix.snapshot":
			snapshots++
			if r["static_prefix_hash"] != "abc123" {
				t.Errorf("snapshot prefix hash = %v, want abc123", r["static_prefix_hash"])
			}
			if r["tools_hash"] != "tools9" {
				t.Errorf("snapshot tools hash = %v, want tools9", r["tools_hash"])
			}
		case "usage":
			usages++
			if r["epoch_id"] != "epoch_1" {
				t.Errorf("usage epoch_id = %v, want epoch_1", r["epoch_id"])
			}
		}
	}
	// One snapshot at epoch create + one per turn = 3.
	if snapshots != 3 {
		t.Errorf("prefix.snapshot count = %d, want 3", snapshots)
	}
	if usages != 2 {
		t.Errorf("usage count = %d, want 2", usages)
	}
}

func TestTraceSinkRecordsDriftAndCompaction(t *testing.T) {
	var buf bytes.Buffer
	s := NewTraceSink(&buf, "deepseek-v4-flash")

	s.Handle(EventEpochCreated{EpochID: "e1", StaticPrefixHash: "h1", ToolsHash: "t1"})
	s.Handle(EventDriftBlocked{EpochID: "e1", Which: "tools"})
	// Stable compaction: the agent measured equal before/after prefix hashes.
	s.Handle(EventSemanticCompaction{
		FromIdx: 0, ToIdx: 5, UsedSemantic: true, SummaryCost: 0.002,
		StaticPrefixHashBefore: "pfx", StaticPrefixHashAfter: "pfx",
	})

	recs := decodeTrace(t, buf.Bytes())

	var sawDrift, sawCompaction bool
	for _, r := range recs {
		switch r["type"] {
		case "drift.blocked":
			sawDrift = true
			if r["which"] != "tools" {
				t.Errorf("drift which = %v", r["which"])
			}
		case "compaction":
			sawCompaction = true
			// The hardcoded boolean is gone; the record carries the measured
			// before/after hashes for the harness to compare.
			if _, ok := r["static_prefix_hash_changed"]; ok {
				t.Error("compaction record must not carry a hardcoded static_prefix_hash_changed")
			}
			if r["before_static_prefix_hash"] != "pfx" || r["after_static_prefix_hash"] != "pfx" {
				t.Errorf("compaction before/after = %v/%v, want pfx/pfx",
					r["before_static_prefix_hash"], r["after_static_prefix_hash"])
			}
			if r["kind"] != "semantic" {
				t.Errorf("compaction kind = %v, want semantic", r["kind"])
			}
		}
	}
	if !sawDrift {
		t.Error("expected a drift.blocked record")
	}
	if !sawCompaction {
		t.Error("expected a compaction record")
	}
}

// TestTraceSinkStampsRunIDAndRole verifies every record carries the run_id
// and agent_role used to attribute it to the root (vs a subagent) epoch.
func TestTraceSinkStampsRunIDAndRole(t *testing.T) {
	var buf bytes.Buffer
	s := NewTraceSink(&buf, "deepseek-v4-flash")
	s.Handle(EventEpochCreated{EpochID: "e1", StaticPrefixHash: "h1", ToolsHash: "t1"})

	recs := decodeTrace(t, buf.Bytes())
	if len(recs) == 0 {
		t.Fatal("no records emitted")
	}
	for _, r := range recs {
		if r["agent_role"] != "root" {
			t.Errorf("agent_role = %v, want root", r["agent_role"])
		}
		if rid, _ := r["run_id"].(string); rid == "" {
			t.Error("run_id missing from record")
		}
	}
}
