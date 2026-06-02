# Cache-Alignment Notes — §3.4 / §3.6 / §3.7 status

Status record for the three cache-alignment work items from the competitive-proof
plan. This documents *what is built and what its scope is* — it deliberately states
no competitive numbers, because none have been measured yet (the production-scale
`cacheprobe` and drift A/B are gated on explicit user go-ahead and live API spend).

## §3.4 — Cache-unit prefix padding

**Built, DISABLED by default.** The padding helper (`internal/cacheunit.PadText`)
and its injection site (`internal/agent/agent.go`, the `EpochComponents` build site)
are implemented. Padding is appended to the static system prompt *before* the epoch
is created and hashed, so it lives inside the frozen, fingerprinted prefix and stays
byte-stable across turns (verified by the agent epoch/fingerprint tests).

The behavior is gated on the `cacheUnit` field (`internal/agent/agent.go:153`):

- `cacheUnit == 0` is the default → padding is disabled entirely. Zero behavior
  change, no extra wire bytes.
- `cacheUnit > 0` → the static prefix is padded so it ends on a cache-unit boundary.

The unit is intentionally left at `0` pending a live `cacheprobe` measurement at
production prefix scale (`bench/cmd/cacheprobe`, swept around the real ~8K-token
prefix). We do not hardcode a unit we have not measured.

**Expected effect: small.** The padding only affects the *tail block* of an ~8K-token
prefix — at most the final partial cache unit — so the upper bound on its benefit is
well under 1% of the prefix. For that reason §3.4 will be **documented, not featured**:
once measured, the real value will be recorded in
`bench/cache-demo/production-prefix.md`. It is a correctness/tidiness refinement of an
already byte-stable prefix, not a headline result.

## §3.6 — Boundary-aligned compaction

**Locked.** Two parts:

1. **Never-evict-the-frozen-prefix guarantee.** This was already true by architecture
   — compaction only ever rewrites an assistant *body* message and never touches the
   frozen static prefix. It is now pinned by a guard test,
   `TestCompactionPreservesFrozenPrefix`
   (`internal/agent/compact_frozen_prefix_test.go`), which asserts the Prefix
   Fingerprint is byte-identical before and after a forced compaction. The guard test
   converts an architectural property into a regression-protected invariant.

2. **Marginal boundary alignment.** `alignTailToCacheUnit`
   (`internal/agent/compact.go`) re-aligns the rebuilt post-compaction tail to a
   cache-unit boundary via the same `cacheunit.PadText` helper, gated on `unit > 0`
   (so it is a no-op under the default `cacheUnit == 0`). It appends a filler
   `TextBlock` to the summary message only — it never alters the frozen prefix bytes,
   which is exactly what the guard test protects.

Like §3.4, the alignment portion is a marginal refinement and is gated off by default;
the load-bearing deliverable here is the locked guarantee.

## §3.7 — Cross-session cache warmth

**Done.** (Correcting any earlier "partial" note.) The cross-session warmth path is
wired into the live `dsc` startup at `cmd/dsc/main.go:610`: on startup `dsc` loads the
prior session's warmth sidecar, computes whether the current static prefix is likely
still warm against the saved fingerprint, appends a warmth notice when it is, and then
saves the current fingerprint so the next session can check against it. This is the
full intended behavior, not a stub — the feature is complete and shipped on this branch.
