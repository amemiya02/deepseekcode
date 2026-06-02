// internal/cacheunit/align.go
// Package cacheunit computes prefix padding so the Static Prefix ends on a
// DeepSeek cache-unit boundary. V4 reuses a stored prefix only up to its last
// COMPLETE compression block; the tail incomplete block is always recomputed
// (DeepSeek-V4 report §3.5.2). Padding the prefix to a unit multiple maximizes
// the reusable, fully-persisted portion. unit must be measured empirically with
// bench/cmd/cacheprobe before enabling padding; unit<=0 means "unknown -> none".
package cacheunit

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
