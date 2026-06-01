// Package repair provides tool-call repair utilities for DeepSeek reliability.
package repair

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// Kind enumerates the categories of repair reports.
type Kind string

const (
	// KindNone indicates no repair was needed.
	KindNone Kind = ""
	// KindArgsCompleted indicates truncated JSON arguments were successfully repaired.
	KindArgsCompleted Kind = "args_completed"
	// KindArgsInvalid indicates arguments could not be repaired.
	KindArgsInvalid Kind = "args_invalid"
	// KindRecovered indicates a tool call was recovered from hidden content.
	KindRecovered Kind = "recovered"
	// KindSuppressed indicates a tool call was suppressed by storm breaker.
	KindSuppressed Kind = "suppressed"
	// KindSchemaComplex indicates a tool schema exceeds complexity thresholds.
	KindSchemaComplex Kind = "schema_complex"
	// KindContinue indicates the repair pipeline should continue to the next stage.
	KindContinue Kind = "continue"
	// KindArgsNeedMore indicates arguments were truncated inside a string
	// and the model should be asked to re-emit complete arguments.
	KindArgsNeedMore Kind = "args_need_more"
	// KindSchemaFlattened indicates a tool schema was flattened for model compatibility.
	KindSchemaFlattened Kind = "schema_flattened"
)

// Action represents the outcome of a repair strategy.
type Action string

const (
	// ActionNone indicates no repair was needed.
	ActionNone Action = "none"
	// ActionRepaired indicates the tool call was successfully repaired.
	ActionRepaired Action = "repaired"
	// ActionRejected indicates the tool call was rejected and should not be executed.
	ActionRejected Action = "rejected"
	// ActionContinue indicates the repair pipeline should continue to the next stage.
	ActionContinue Action = "continue"
)

// KindFromAction maps an Action to a Kind for reporting purposes.
func KindFromAction(action Action) Kind {
	switch action {
	case ActionNone:
		return KindNone
	case ActionRepaired:
		return KindArgsCompleted
	case ActionRejected:
		return KindArgsInvalid
	case ActionContinue:
		return KindContinue
	default:
		return KindNone
	}
}

// Pipeline orchestrates repair strategies for tool calls.
type Pipeline struct {
	// strategies will be added by later tasks
}

// NewPipeline creates a new repair pipeline with default strategies.
func NewPipeline() *Pipeline {
	return &Pipeline{}
}

// Repair applies repair strategies to a tool call and returns the potentially
// modified call along with a report describing what was done.
func (p *Pipeline) Repair(call llm.ToolCall) (llm.ToolCall, Report) {
	// No-op: pass through unchanged
	return call, Report{
		Kind:    KindFromAction(ActionNone),
		Tool:    call.Function.Name,
		CallID:  call.ID,
		Message: "no repair needed",
	}
}

// Report describes a single repair action.
type Report struct {
	Kind       Kind
	Tool       string
	CallID     string
	Message    string
	BeforeHash string
	AfterHash  string
}

// Result is the aggregate output of a repair pipeline step.
type Result struct {
	Calls   []llm.ToolCall
	Reports []Report
}

// HashArgs returns the SHA-256 hex hash of the raw argument string.
// This provides a stable fingerprint for deduplication and change detection.
func HashArgs(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// CanonicalArgs returns a canonical JSON representation with object keys
// sorted recursively. If the input is not valid JSON, it returns the original
// string and false.
func CanonicalArgs(raw string) (string, bool) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return raw, false
	}
	var buf bytes.Buffer
	if err := writeCanonicalJSON(&buf, v); err != nil {
		return raw, false
	}
	return buf.String(), true
}

// writeCanonicalJSON writes a canonical JSON representation with sorted keys.
func writeCanonicalJSON(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return err
			}
			buf.Write(kb)
			buf.WriteByte(':')
			if err := writeCanonicalJSON(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonicalJSON(buf, e); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		buf.Write(b)
	}
	return nil
}
