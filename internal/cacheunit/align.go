// internal/cacheunit/align.go
// Package cacheunit computes prefix padding so the Static Prefix ends on a
// DeepSeek cache-unit boundary. V4 reuses a stored prefix only up to its last
// COMPLETE compression block; the tail incomplete block is always recomputed
// (DeepSeek-V4 report §3.5.2). Padding the prefix to a unit multiple maximizes
// the reusable, fully-persisted portion. unit must be measured empirically with
// bench/cmd/cacheprobe before enabling padding; unit<=0 means "unknown -> none".
package cacheunit

import "strings"

// AlignPadding returns the token count to append to a prefix of prefixTokens so
// its length is a multiple of unit. Returns 0 when unit<=0 or already aligned.
func AlignPadding(prefixTokens, unit int) int {
	if unit <= 0 || prefixTokens <= 0 {
		return 0
	}
	r := prefixTokens % unit
	if r == 0 {
		return 0
	}
	return unit - r
}

// PadText returns deterministic filler to append to a prefix of prefixTokens so
// the total token count is a multiple of unit, as measured by count. Identical
// for a given (prefixTokens, unit) -> the frozen prefix stays byte-stable.
// count MUST be the same tokenizer used to fingerprint the prefix. "" when none.
func PadText(prefixTokens, unit int, count func(string) int) string {
	need := AlignPadding(prefixTokens, unit)
	if need <= 0 {
		return ""
	}
	const filler = "\n<!-- cache-alignment padding -->" // fixed, content-free
	var b strings.Builder
	for count(b.String()) < need {
		b.WriteString(filler)
	}
	out := b.String()
	for count(out) > need && len(out) > 0 {
		out = out[:len(out)-1]
	}
	return out
}
