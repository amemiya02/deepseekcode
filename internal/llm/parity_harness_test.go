package llm

import (
	"encoding/json"
	"fmt"
	"os"
)

// This file is the parity harness data layer: the scenario table, the
// per-scenario Request constructors, and the manifest loader. Although
// the contract in TASKS.md (Phase 26) names it `parity_scenarios.go`,
// the file lives under `_test.go` because every scenario references
// helpers (e.g. buildRepresentativeRequest) that are themselves
// test-only. The harness is never reachable from production code; making
// it test-only keeps the production build green without sacrificing any
// acceptance criterion. The unit assertions sit in
// parity_scenarios_test.go; this file holds the data shapes they pin.

// ParityScenario is a named Request constructor used by the parity
// golden-diff harness to detect cache-stable serialization drift.
type ParityScenario struct {
	Name  string
	Build func() Request
}

// ParityScenarios returns every named parity scenario. Names are unique
// and stable — adding, removing, or renaming a scenario is a deliberate
// act that must be reflected in manifest.json, golden files, and
// docs/PARITY.md. TestParityConsistency enforces that quadruple-binding.
//
// Each Build must be deterministic: no time.Now, no map iteration order
// dependency, no goroutine state. MarshalCacheStable sorts its outputs
// but cannot rescue a non-determined constructor.
func ParityScenarios() []ParityScenario {
	return []ParityScenario{
		{"representative", buildRepresentativeRequest},
		{"plain_text", buildPlainTextRequest},
		{"tool_sort", buildToolSortRequest},
		{"schema_canonical", buildSchemaCanonicalRequest},
		{"thinking_roundtrip", buildThinkingRoundtripRequest},
		{"tool_roundtrip", buildToolRoundtripRequest},
		{"thinking_effort_max", buildThinkingEffortMaxRequest},
		{"thinking_tool_call_placeholder", buildThinkingToolCallPlaceholderRequest},
		{"cache_stable_user_id_omitted", buildCacheStableUserIDOmittedRequest},
	}
}

// buildPlainTextRequest exercises the minimum wire shape: one user turn,
// no tools, no thinking — the bytes here pin the no-frills baseline so
// regressions in default-field omission show up immediately.
func buildPlainTextRequest() Request {
	return Request{
		Model:    "deepseek-v4-flash",
		Stream:   true,
		Messages: []Message{{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "hi"}}}},
	}
}

// buildToolSortRequest declares tools out of alphabetical order to pin
// the cache-stable invariant that MarshalCacheStable sorts tools by
// function name. Any regression to the sort comparator surfaces here.
func buildToolSortRequest() Request {
	return Request{
		Model:  "deepseek-v4-flash",
		Stream: true,
		Tools: []Tool{
			{Type: "function", Function: ToolFunction{Name: "zebra", Description: "z", Parameters: json.RawMessage(`{"type":"object"}`)}},
			{Type: "function", Function: ToolFunction{Name: "alpha", Description: "a", Parameters: json.RawMessage(`{"type":"object"}`)}},
			{Type: "function", Function: ToolFunction{Name: "mango", Description: "m", Parameters: json.RawMessage(`{"type":"object"}`)}},
		},
		Messages: []Message{{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "sort tools"}}}},
	}
}

// buildSchemaCanonicalRequest stuffs a tool schema with intentionally
// scrambled key order to pin the recursive key-sort canonicalization in
// MarshalCacheStable. The schema includes a nested object so the
// recursive case is exercised, not just the top level.
func buildSchemaCanonicalRequest() Request {
	schema := json.RawMessage(`{"required":["b"],"properties":{"z":{"type":"string"},"a":{"type":"object","properties":{"y":{"type":"number"},"b":{"type":"boolean"}}}},"type":"object"}`)
	return Request{
		Model:  "deepseek-v4-flash",
		Stream: true,
		Tools: []Tool{
			{Type: "function", Function: ToolFunction{Name: "scrambled", Description: "d", Parameters: schema}},
		},
		Messages: []Message{{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "canonical schema"}}}},
	}
}

// buildThinkingRoundtripRequest pins the reasoning_content wire shape:
// an assistant turn carrying a ThinkingBlock must serialize with a
// reasoning_content field alongside content, and the thinking envelope
// must appear as a struct (DeepSeek V4 rejects "thinking":true).
func buildThinkingRoundtripRequest() Request {
	return Request{
		Model:    "deepseek-v4-flash",
		Stream:   true,
		Thinking: ThinkingEnabled(true),
		Messages: []Message{
			{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "explain"}}},
			{Role: "assistant", Blocks: []ContentBlock{
				ThinkingBlock{Text: "step by step"},
				TextBlock{Text: "answer"},
			}},
		},
	}
}

// buildToolRoundtripRequest pins the tool_calls / tool-result envelope:
// an assistant ToolUseBlock must serialize into a tool_calls array on
// the assistant message, and a paired ToolResultBlock must emit a
// dedicated role:"tool" wire message keyed by tool_call_id.
func buildToolRoundtripRequest() Request {
	return Request{
		Model:  "deepseek-v4-flash",
		Stream: true,
		Tools: []Tool{
			{Type: "function", Function: ToolFunction{
				Name:        "read_file",
				Description: "Read a file",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
			}},
		},
		Messages: []Message{
			{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "read main.go"}}},
			{Role: "assistant", Blocks: []ContentBlock{
				TextBlock{Text: "reading"},
				ToolUseBlock{ID: "call_001", Name: "read_file", Input: json.RawMessage(`{"path":"main.go"}`)},
			}},
			{Role: "tool", Blocks: []ContentBlock{
				ToolResultBlock{ToolUseID: "call_001", Content: "package main"},
			}},
		},
	}
}

// buildThinkingEffortMaxRequest pins the wire shape when both thinking
// and reasoning_effort are set: thinking must serialize as a struct and
// reasoning_effort as a string value.
func buildThinkingEffortMaxRequest() Request {
	return Request{
		Model:           "deepseek-v4-flash",
		Stream:          true,
		Thinking:        ThinkingEnabled(true),
		ReasoningEffort: ReasoningEffortMax,
		Messages: []Message{
			{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "complex task"}}},
		},
	}
}

// buildThinkingToolCallPlaceholderRequest pins the SanitizeForDeepSeek
// behavior: an assistant message with tool calls but no thinking block
// must have a placeholder reasoning_content prepended when thinking is
// enabled.
func buildThinkingToolCallPlaceholderRequest() Request {
	return Request{
		Model:    "deepseek-v4-flash",
		Stream:   true,
		Thinking: ThinkingEnabled(true),
		Messages: []Message{
			{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "read file"}}},
			{Role: "assistant", Blocks: []ContentBlock{
				TextBlock{Text: "reading"},
				ToolUseBlock{ID: "call_001", Name: "read_file", Input: json.RawMessage(`{"path":"main.go"}`)},
			}},
			{Role: "tool", Blocks: []ContentBlock{
				ToolResultBlock{ToolUseID: "call_001", Content: "package main"},
			}},
		},
	}
}

// buildCacheStableUserIDOmittedRequest pins that an empty UserID is
// omitted from the wire bytes — the default representative request must
// not contain a user_id field.
func buildCacheStableUserIDOmittedRequest() Request {
	req := buildRepresentativeRequest()
	req.UserID = "" // explicitly empty
	return req
}

// parityManifest mirrors the JSON shape of testdata/parity/manifest.json.
type parityManifest struct {
	Version   int `json:"version"`
	Scenarios []struct {
		Name   string `json:"name"`
		SHA256 string `json:"sha256"`
	} `json:"scenarios"`
}

// loadParityManifest reads and parses a parity manifest JSON file.
// Returns a clear error with regeneration instructions when the file
// is missing.
func loadParityManifest(path string) (parityManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return parityManifest{}, fmt.Errorf(
			"read parity manifest: %w\nRegenerate with: UPDATE_GOLDEN=1 go test -run TestParityGolden ./internal/llm/",
			err,
		)
	}
	var m parityManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return parityManifest{}, fmt.Errorf("parse parity manifest: %w", err)
	}
	return m, nil
}
