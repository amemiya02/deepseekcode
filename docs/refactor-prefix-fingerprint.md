# Refactor: Consolidate the Prefix Fingerprint & Drift core

> **For agentic workers:** REQUIRED SUB-SKILL: use
> `superpowers:subagent-driven-development` or `superpowers:executing-plans`.
> Steps use checkbox (`- [ ]`) syntax for tracking. Domain terms (**Static
> Prefix**, **Prefix Fingerprint**, **Capability Set**, **Drift**, **Prefix
> Epoch**, **Pending Change**) are defined in [`/CONTEXT.md`](../CONTEXT.md).

**Goal.** Make "what are the model-visible prefix bytes, what is their hash, and
did they change?" answerable in exactly **one deep module** in `internal/llm`,
consumed everywhere else. Adopt philosophy **P1**: the **Prefix Fingerprint** is
the canonical hash of *model-visible bytes only*; the latent **Capability Set**
(agent profile, connected MCP, skill catalog) is tracked separately by
`EpochManager` policy and never folded into the fingerprint.

**Why (three confirmed defects this fixes).**
1. `skill_dir` double-counts with `system` — the skill directory is rendered
   into the system prompt (`internal/prompt/builder.go:106`), yet
   `computeComponentHashes` also hashes `Skills.VersionHash()` separately
   (`internal/agent/prefix_epoch.go:312`). One skill edit reports both
   `system_changed` and `skill_body_changed`.
2. `mcp_schema` is broken three ways: it double-counts with `tools` when the MCP
   tier is active; it is the **only key-order-sensitive** component because
   `mcp.Registry.SchemaHash` writes `t.InputSchema` raw with no canonicalization
   (`internal/mcp/registry.go:209`) while everything else key-sorts — so a
   reordered-keys reconnect causes **phantom drift**; and it hashes bytes the
   model never sees when the MCP tier is inactive (default `ActiveTiers =
   {TierCore}`, `mcp__*` tools default to `TierProfile`).
3. Canonicalization is duplicated between `MarshalCacheStable`
   (`internal/llm/request.go:103`) and `hashToolsCanonical`
   (`internal/llm/prefix_drift.go:34`), kept in sync only by the comment
   *"use the exact same struct as MarshalCacheStable"*.

**North-star constraint.** `MarshalCacheStable`'s output **is the DeepSeek cache
key**. Its bytes MUST stay byte-identical across this refactor — changing them
invalidates every user's prompt cache. The Phase-0 golden test is the anchor
that guarantees this; the *fingerprint value* intentionally changes, the *wire
bytes* must not.

---

## Target shape

```go
// internal/llm — single source of truth for the model-visible prefix.
type StaticPrefix struct {
    System   string    // already contains the rendered "## Skills" directory
    Tools    []Tool    // the actually-sent set (membership decided by caller)
    FewShots []Message // leading few-shot turns; currently always empty
}

func (StaticPrefix) Canonical() []byte        // THE canonicalization
func (StaticPrefix) Fingerprint() Fingerprint // per-component sub-hashes + Combined
func Diff(old, new StaticPrefix) PrefixDiff    // typed; tool add/remove/schema breakdown

type Fingerprint struct{ System, Tools, FewShots, Combined string }

type PrefixMonitor struct{ pinned *Fingerprint } // thin wrapper, replaces today's
func (m *PrefixMonitor) Check(p StaticPrefix) (d PrefixDiff, drifted bool)
```

`MarshalCacheStable` builds its static head (model + system message + sorted
canonical tools + leading few-shots) via the *same* canonicalization
`StaticPrefix.Canonical()` uses, then appends the dynamic conversation tail.
Fingerprint = `sha256(StaticPrefix.Canonical())`. Desync with the wire is
impossible by construction.

**Placement (forced by import layering `llm < {mcp,skills,tools} < agent`):**
- `internal/llm`: `StaticPrefix`, `Fingerprint`, `Diff`, `PrefixMonitor` — the
  deep core, operating only on `llm` types.
- `internal/agent`: `EpochManager` keeps the **Prefix Epoch** lifecycle and owns
  the new **Capability Set** + `CapabilityDiff` (composing `mcp.CompareToolLists`
  and `skills.Store.Diff` as adapters). It *consumes* the `llm` fingerprint; it
  never re-implements canonicalization.

"One drift verdict" (decision B) resolves under P1 into two: **Drift**
(cache-correctness, `llm`) and **Capability change** (policy → Pending Change,
`agent`).

---

## M0 — Characterization harness (safety net, zero behavior change) — DONE

**Reality check (2026-05-29):** most of the originally-planned M0 was *already
covered* by the existing suite — do **not** re-create it:

- Golden `MarshalCacheStable` bytes: `internal/llm/e2e_cache_stable_test.go`
  (`TestCacheStableGolden` + `testdata/cache_stable.golden.json`), plus
  determinism, tool-sort, schema-canonical, block-flattening.
- Fingerprint reorder/key-order invariance + full `PrefixMonitor` behavior:
  `internal/llm/prefix_drift_test.go`.
- Freeze→pending, profile-switch→epoch+expect-miss, skill-body→pending-not-live,
  MCP reconnect→no-drift: `internal/agent/prefix_epoch_test.go` (28 tests),
  `internal/agent/profile_epoch_test.go`.

Two real blind spots remained — the existing MCP tests feed `MCPSchemaSnapshot`
as an *opaque string* (never the live `SchemaHash` path with reordered keys),
and nothing pinned the fingerprint↔wire-bytes linkage. Both are now closed:

- [x] `internal/mcp/schema_hash_characterization_test.go` —
      `TestSchemaHash_KeyOrderSensitivity_Characterization` pins the phantom
      drift: on a key-reordered schema, `CompareToolLists`/`PendingSchemaChanges`
      see no change while `SchemaHash` flips. (The `SchemaHash` assertion is
      deleted in M3; the canonical-path assertions stay.)
- [x] `internal/llm/fingerprint_wire_linkage_test.go` —
      `TestFingerprintTracksWireStaticHead` pins that the fingerprint moves iff
      the `MarshalCacheStable` static head moves, on identical inputs.
- [x] Both pass; `internal/llm`, `internal/mcp`, `internal/agent` stay green.

The agent-side `epoch_characterization_test.go` originally planned here is
redundant with the existing 28+3 epoch tests and was **not** created.

## M1 — Single canonicalization + `StaticPrefix` in `internal/llm` — DONE

**Files:** created `internal/llm/static_prefix.go`; modified
`internal/llm/request.go`, `internal/llm/prefix_drift.go`.

- [x] Extracted the tool-sort + `canonicalJSON` loop into one helper,
      `canonicalizeTools` (static_prefix.go). `MarshalCacheStable` (wire) and
      `hashToolsCanonical` (fingerprint) now both call it — the duplicated
      canonicalization and its "same as MarshalCacheStable" comment-contract are
      gone.
- [x] `ComputeFingerprint` is now a thin shim over `StaticPrefix.Fingerprint()`.
- [x] Introduced `StaticPrefix{System,Tools,FewShots}` + `Fingerprint()` as the
      new seam, **value-preserving**: it reproduces the existing
      `PrefixFingerprint` exactly, so no fingerprint or epoch-hash values moved.
- [x] **Wire-byte golden (`TestCacheStableGolden`) still passes** — cache key
      byte-identical. Full `go test ./...` green; `go vet` clean.

**Deliberate deferrals to M3** (to keep M1 a pure, zero-value-change refactor):
- `StaticPrefix.Canonical() []byte` was **not** added yet. Making its bytes
  equal the wire static head requires factoring the head out of
  `MarshalCacheStable`'s single-struct marshal; doing it now would ship a method
  whose bytes aren't yet the real head. It lands in M3 with the redefinition.
- `FewShots` is carried on `StaticPrefix` but **not** folded into the
  fingerprint (it's always empty today; folding it changes the combined-hash
  value, a deliberate change for M3's visible/latent split).

## M2 — `Diff` + thin `PrefixMonitor`; rewire the per-turn + compaction checks — DONE

**Files:** modified `internal/llm/static_prefix.go`,
`internal/llm/prefix_drift.go`, `internal/agent/agent.go`, +
`internal/llm/prefix_drift_test.go`, `internal/llm/fingerprint_wire_linkage_test.go`.

- [x] Added `llm.Diff(old,new StaticPrefix) PrefixDiff` with `ToolDiff`
      (Added/Removed/SchemaChanged) — in `static_prefix.go`. Tool comparison uses
      `llm`'s own `canonicalizeTools` + per-tool canonical-byte compare, **not**
      `mcp/drift.go:canonicalEqual` (that's unexported in `mcp`, which `llm`
      cannot import). `PrefixDiff` covers system + tools; few-shots is deferred to
      M3, consistent with M1.
- [x] Rewrote `PrefixMonitor` over `StaticPrefix`/`Diff`: `Check(StaticPrefix)
      (PrefixDiff, bool)`. The stable path only hashes (fingerprint compare); the
      breakdown is computed via `Diff` only when the fingerprint moved. The legacy
      `which` label is preserved via `PrefixDiff.Which()`.
- [x] `agent.go` per-turn check, `staticPrefixHash`, and the receipt fingerprint
      now build `llm.StaticPrefix` directly. `EventDriftBlocked.Which` /
      `prefix_cache_which` receipt field preserved via `PrefixDiff.Which()`.
- [x] Behavior preserved: cache-key golden byte-identical; the 6 `PrefixMonitor`
      tests updated to the new signature and green; full `go test ./...` + `go
      vet ./...` clean.

## M3 — Split epoch identity: Prefix Fingerprint (visible) vs Capability Set (latent) — DONE

**Files:** created `internal/agent/capability_set.go` (+ `_test.go`); modified
`internal/agent/prefix_epoch.go`, `internal/agent/agent.go`,
`internal/llm/prefix_drift.go`, `internal/mcp/registry.go`; rewrote
`internal/agent/prefix_epoch_test.go`, repurposed
`internal/mcp/schema_hash_characterization_test.go`, trimmed
`internal/mcp/registry_epoch_test.go`, fixed `internal/agent/profile_epoch_test.go`.

- [x] `PrefixEpoch.StaticPrefixHash` ← `StaticPrefix.Fingerprint().CombinedSHA256`
      (model-visible only). `ComponentHashes` keeps just `{system, tools}` for the
      epoch events. Removed `EpochComponentHashes`, `ComputeEpochHash`
      (`llm`), `computeComponentHashes`, `hashFewShots` (`agent`).
- [x] New `CapabilitySet{ProfileID, Skills *skills.Store, MCPTools
      []mcp.McpToolMeta}` + `CapabilityDiff` using `skills.Store.Diff` +
      `mcp.CompareToolLists` + profile-name compare.
- [x] `DetectDrift` is now **purely capability-based** (`CapabilityDiff`).
      Model-visible byte drift stays the job of the per-turn `llm.PrefixMonitor`
      (`EventDriftBlocked`) — keeping it out of `DetectDrift` is what makes the
      double-report impossible by construction (a skill edit can only surface as
      `skill_body_changed`, never also `system_changed`). *(This is a cleaner
      split than the plan's original "visible drift via `llm.Diff` inside
      DetectDrift" wording.)*
- [x] Epoch-mint trigger unchanged structurally; a profile switch with no
      visible-byte change no longer moves the fingerprint (the explicit
      `SwitchEpoch` still mints an epoch + one expected miss). Verified by
      `TestEpochProfileChangeDoesNotChangeFingerprint`.
- [x] `buildEpochComponents` builds `StaticPrefix` inputs + a `CapabilitySet`;
      no `SchemaHash`/`VersionHash` string components.
- [x] Removed `mcp.Registry.SchemaHash` (+ dead `sha256hex`, `sort`/`crypto/sha256`
      imports). No raw `sb.Write(t.InputSchema)` hash remains.
- [x] **Regression:** reordered MCP JSON-Schema keys → zero drift/pending —
      `mcp.TestMCPSchemaKeyReorder_NoPhantomDrift` and
      `agent.TestCapabilityDiff_MCPKeyReorderNoDrift`.
- [x] **Regression:** skill-body edit moves the fingerprint via `system` yet
      reports exactly one `skill_body_changed` — `agent.TestSkillEditReportedOnceNotAlsoSystem`.
- [x] `go build`, `go vet`, full `go test ./...`, and `go test -race` (agent/llm/mcp)
      all green; cache-key golden still byte-identical.

**Deferred:** folding `FewShots` into the fingerprint (still always empty;
`StaticPrefix.Fingerprint()` covers system+tools). `ComputeFingerprint`/
`PrefixInput` kept as a thin tested shim (removing them was churn for no gain).
**M4** (trace/benchmark/docs + the optimization-plan breadcrumb) remains.

## M4 — Trace, benchmark, and docs — DONE

**Files:** modified `CLAUDE.md`, `docs/optimization-plan-cache-epoch.md`
(verified `internal/agent/trace.go` + `bench/cmd/benchrunner` need no change).

- [x] The trace's `static_prefix_hash` flows from the epoch's (now-visible)
      fingerprint, and `bench/cmd/benchrunner` reads it opaquely (no legacy
      6-component references anywhere). The cache gate ("static prefix stable
      within epoch") now *tautologically* tracks the cache key. Full suite —
      including `bench/cmd/benchrunner` — is green; no change required.
- [x] No test asserted the legacy 6-component composition (the agent-side ones
      were already rewritten in M3); nothing else to update.
- [x] CLAUDE.md "Prefix epoch system" + "Wire format" sections describe the
      shared `canonicalizeTools` and the visible/latent split.
- [x] `docs/optimization-plan-cache-epoch.md` "Core Concept: PrefixEpoch" carries
      a superseded breadcrumb pointing at ADR-0001 + this plan + CONTEXT.md.

---

## Outcome

All milestones M0–M4 complete. The refactor is **behavior-preserving on the
cache key** (the `MarshalCacheStable` golden is byte-identical throughout) while
fixing three confirmed defects — phantom MCP drift, the skill/system
double-report, and wasteful profile-rename cache misses — each guarded by a
regression test. "What the model sees" now lives in one deep `internal/llm`
module (`StaticPrefix`/`Fingerprint`/`Diff`), and latent capability identity is
a separate `agent.CapabilitySet`. `go build`, `go vet`, `go test ./...`, and
`go test -race` (agent/llm/mcp) are green.

---

## Definition of done

- `MarshalCacheStable` bytes byte-identical to pre-refactor (M0 golden).
- One canonicalization; no duplicated tool-hash logic; no raw non-canonical
  schema hash anywhere.
- Prefix Fingerprint = `sha256(canonical static request head)`; test proves
  "fingerprint stable ⟺ static wire head stable".
- Phantom-MCP-drift and skill/system double-count regression tests pass.
- All existing tests green (`make test`, `make test-race`); benchmark cache gate
  green.

## Risks & mitigations

- **Silent cache-key change.** Mitigated by the M0 wire-byte golden gate held
  across every later milestone.
- **Receipt behavior regression** (pending changes the dashboards/gate expect).
  Mitigated by M0 characterization of current `EpochManager` receipts.
- **Scope creep into `EpochManager` policy.** Out of scope — this refactor only
  changes *what is hashed and where canonicalization lives*, not *when epochs
  are minted* (beyond the deliberate "don't mint on invisible capability change"
  fix, which is called out and tested).
