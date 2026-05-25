package llm

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
)

type PrefixFingerprint struct {
	SystemSHA256   string
	ToolsSHA256    string
	CombinedSHA256 string
}

// ComputeFingerprint hashes system prompt + sorted tool names.
func ComputeFingerprint(sys string, tools []Tool) PrefixFingerprint {
	sysH := sha256hex(sys)
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Function.Name
	}
	sort.Strings(names)
	tlsH := sha256hex(strings.Join(names, ","))
	return PrefixFingerprint{sysH, tlsH, sha256hex(sysH + ":" + tlsH)}
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
	fp := ComputeFingerprint(staticSystem, tools)
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
