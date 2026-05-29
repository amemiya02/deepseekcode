package llm

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
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
// It is a thin shim over StaticPrefix.Fingerprint() (static_prefix.go); callers
// are being migrated to StaticPrefix directly (M2 of
// docs/refactor-prefix-fingerprint.md).
func ComputeFingerprint(input PrefixInput) PrefixFingerprint {
	return StaticPrefix{System: input.SystemPrompt, Tools: input.Tools}.Fingerprint()
}

// hashToolsCanonical hashes the canonical serialized form of tools. It builds on
// canonicalizeTools (static_prefix.go) — the same canonicalization
// MarshalCacheStable uses — so the bytes hashed here are exactly the tool bytes
// that appear on the wire.
func hashToolsCanonical(tools []Tool) string {
	if len(tools) == 0 {
		return sha256hex("")
	}
	canon, _ := canonicalizeTools(tools) // best-effort: leaves malformed schemas raw
	b, err := json.Marshal(canon)
	if err != nil {
		return sha256hex("")
	}
	return sha256hex(string(b))
}

func sha256hex(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }

// PrefixMonitor detects cache-prefix drift across turns by comparing each
// StaticPrefix against the previously pinned one. Not goroutine-safe; callers
// must serialize Check calls.
type PrefixMonitor struct {
	prev        *StaticPrefix
	prevFP      PrefixFingerprint
	checkCount  uint64
	changeCount uint64
}

func NewPrefixMonitor() *PrefixMonitor { return &PrefixMonitor{} }

// Check pins on the first call (zero PrefixDiff, drifted=false). On later calls
// it compares against the pin: an identical fingerprint is no drift; a changed
// fingerprint re-pins and returns the typed PrefixDiff with drifted=true. The
// stable path only hashes — the diff breakdown is computed only when the
// fingerprint actually moved.
func (m *PrefixMonitor) Check(p StaticPrefix) (PrefixDiff, bool) {
	m.checkCount++
	fp := p.Fingerprint()
	if m.prev == nil {
		m.pin(p, fp)
		return PrefixDiff{}, false
	}
	if fp.CombinedSHA256 == m.prevFP.CombinedSHA256 {
		return PrefixDiff{}, false
	}
	m.changeCount++
	d := Diff(*m.prev, p)
	m.pin(p, fp)
	return d, true
}

// pin stores a defensive copy of the prefix so later caller-side slice mutation
// cannot retroactively change what the monitor compares against.
func (m *PrefixMonitor) pin(p StaticPrefix, fp PrefixFingerprint) {
	cp := StaticPrefix{System: p.System}
	if len(p.Tools) > 0 {
		cp.Tools = append([]Tool(nil), p.Tools...)
	}
	m.prev = &cp
	m.prevFP = fp
}

func (m *PrefixMonitor) StabilityRatio() float64 {
	if m.checkCount == 0 {
		return 1
	}
	return float64(m.checkCount-m.changeCount) / float64(m.checkCount)
}

// EpochComponentHashes holds individual SHA-256 hashes for the
// components that constitute a static prefix epoch.
type EpochComponentHashes struct {
	SystemSHA256       string
	ToolsSHA256        string
	SkillDirSHA256     string
	MCPSchemaSHA256    string
	AgentProfileSHA256 string
	FewShotsSHA256     string
}

// ComputeEpochHash returns sha256hex(sys:tools:skill:mcp:profile:fewshots).
func ComputeEpochHash(components EpochComponentHashes) string {
	combined := components.SystemSHA256 + ":" +
		components.ToolsSHA256 + ":" +
		components.SkillDirSHA256 + ":" +
		components.MCPSchemaSHA256 + ":" +
		components.AgentProfileSHA256 + ":" +
		components.FewShotsSHA256
	return sha256hex(combined)
}
