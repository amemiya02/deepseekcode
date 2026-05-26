package llm

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
)

// PrefixInput holds the inputs for prefix fingerprint computation.
type PrefixInput struct {
	SystemPrompt string
	Tools        []Tool
}

type PrefixFingerprint struct {
	SystemSHA256   string
	ToolsSHA256    string
	CombinedSHA256 string
}

// ComputeFingerprint hashes system prompt + canonical serialized tool specs.
// Tools are sorted by name and their JSON-Schema parameters are canonicalized
// (key-sorted) so that key order differences do not affect the fingerprint.
func ComputeFingerprint(input PrefixInput) PrefixFingerprint {
	sysH := sha256hex(input.SystemPrompt)
	tlsH := hashToolsCanonical(input.Tools)
	return PrefixFingerprint{sysH, tlsH, sha256hex(sysH + ":" + tlsH)}
}

// hashToolsCanonical hashes the canonical serialized form of tools.
// Tools are sorted by name and parameters are canonicalized by key-sort.
// The output matches the tool JSON bytes produced by MarshalCacheStable.
func hashToolsCanonical(tools []Tool) string {
	if len(tools) == 0 {
		return sha256hex("")
	}

	// Sort tools by name (same as MarshalCacheStable)
	sorted := make([]Tool, len(tools))
	copy(sorted, tools)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Function.Name < sorted[j].Function.Name
	})

	// Canonicalize each tool's parameters (same as MarshalCacheStable)
	for i, t := range sorted {
		if len(t.Function.Parameters) > 0 {
			canon, err := canonicalJSON(t.Function.Parameters)
			if err == nil {
				sorted[i].Function.Parameters = canon
			}
		}
	}

	// Serialize tools to JSON - use the exact same struct as MarshalCacheStable
	b, err := json.Marshal(sorted)
	if err != nil {
		return sha256hex("")
	}
	return sha256hex(string(b))
}

func sha256hex(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }

// PrefixMonitor detects cache-prefix drift across turns. Not
// goroutine-safe; callers must serialize Check calls.
type PrefixMonitor struct {
	pinned      *PrefixFingerprint
	checkCount  uint64
	changeCount uint64
}

func NewPrefixMonitor() *PrefixMonitor { return &PrefixMonitor{} }

// Check pins on first call; on drift re-pins and returns which changed.
func (m *PrefixMonitor) Check(staticSystem string, tools []Tool) (changed bool, which string) {
	m.checkCount++
	fp := ComputeFingerprint(PrefixInput{SystemPrompt: staticSystem, Tools: tools})
	if m.pinned == nil {
		m.pinned = &fp
		return false, ""
	}
	if fp.CombinedSHA256 == m.pinned.CombinedSHA256 {
		return false, ""
	}
	m.changeCount++
	sysC, toolsC := fp.SystemSHA256 != m.pinned.SystemSHA256, fp.ToolsSHA256 != m.pinned.ToolsSHA256
	switch {
	case sysC && toolsC:
		which = "sys+tools"
	case sysC:
		which = "sys"
	default:
		which = "tools"
	}
	m.pinned = &fp
	return true, which
}

func (m *PrefixMonitor) StabilityRatio() float64 {
	if m.checkCount == 0 {
		return 1
	}
	return float64(m.checkCount-m.changeCount) / float64(m.checkCount)
}
