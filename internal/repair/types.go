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
)

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
