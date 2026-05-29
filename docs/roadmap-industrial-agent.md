# Roadmap — Industrial-Grade DeepSeek Coding Agent

Phase-1 deliverable. Audience: maintainers and contributors. This document is
grounded in the per-subsystem capability/gap inventory, in `docs/design.md`
(source of truth), `docs/PARITY.md`, `CONTEXT.md`, `CLAUDE.md`, and `AGENTS.md`,
and in transferable-design analyses of six reference agents (opencode, crush,
Claude Code, DeepSeek-Reasonix, CodeWhale, claw-code). Every gap cited below
points at a concrete file and line range from the inventory.

---

## Progress log

Living status of implementation (branch `feat/industrial-agent`). Each entry
is a landed, independently-verified commit (`go test ./...` green, golden
cache bytes unmoved, `go vet` clean, `-race` where relevant).

| Stage | Status | Notes |
|---|---|---|
| T6.3 | ✅ done | `internal/llmtest` offline mock-DeepSeek SSE harness + end-to-end loop tests (finish-reason override, tool-pairing, thinking-struct, both timeout tiers). |
| T1.1 | ✅ done | Partial assistant turn persisted on mid-stream error instead of discarded. |
| T1.2 | ✅ done | `Replay` repairs dangling `tool_calls` from an interrupted session; the other two pieces were already handled (see the T1.2 entry below). |
| T1.3 | ◧ partial | `StopStepTimeout` (non-success) + `StopUserRequested` wired + `IsSuccess()`. Deferred: budget/denied EventInfo → typed events; remove the unreachable tool-error abort branch. |
| T1.4 | ✅ done | Mid-stream stall (first-token / chunk-stall) re-issues the identical request **once** before salvaging the partial (T1.1); a parse error or cancel/step-deadline never re-issues. A 400 context-overflow routes to a single deterministic compaction + re-attempt (`llm.IsContextOverflow`, typed `ErrFirstTokenTimeout`/`ErrChunkStall` sentinels). No duplicate persisted turn. |
| T2.1 | ✅ done | First loop detection per Run injects a synthetic-result-per-dangling-call + one user nudge and continues once; a per-Run `loopFloor` forgives the pre-nudge repeats so the recovery turn isn't instantly re-tripped; a second detection hard-stops. Nudge is message-tail only (fingerprint untouched). |
| T2.2 | ✅ done | Fuzzy multi-strategy edit replacer (exact → line-trimmed → whitespace-normalized → indentation-flexible → block-anchor), first **unique** match wins, ambiguous rejected; `apply_patch` `locateChunk` now rejects ambiguous context too. 22-case corpus: exact-only baseline 0.091 → cascade 1.000 (asserted). Byte-fidelity preserved (original substrings, full-file `want`); `Description`/`Parameters` byte-identical so the fingerprint is unmoved. |
| T2.3 | ✅ done | Model-driven escalation: `<<<NEEDS_PRO>>>` marker (whole-first-line) or ≥3 unrecoverable repair errors re-issues the turn once on `deepseek-v4-pro` via the shared `streamWithReissue`. Only `req.Model` changes → fingerprint unmoved; flash turn discarded (no duplicate persist); `respModel` drives persistence/cost/trace; opt-in contract (`EnableEscalation`) keeps the default golden untouched. **Hardened after a 3-lens adversarial workflow** (storm-history pollution → `StormBreaker` Snapshot/Restore; flash spend charged + pro re-gated; trace attribution; marker tightened). |
| T2.4 | ✅ done | MCP stdio liveness watcher: process exit → `StateDegraded` (tools drop via the existing `State!=StateConnected` filter), then exactly one bounded reconnect with negative-result backoff. `r.mu` sole authority, no IO under lock, watcher bound to its transport's `Done()` by value; `-race` clean. ADR-0001 invariant (degrade surfaces as `PendingMCPToolRemoved`, never mid-epoch fingerprint movement) covered compositionally by the new mcp-layer `DriftRemoved` test + existing `TestCapabilityDiff_MCPAddRemove`/`TestEpochMutationsAfterFreezeBecomePending`. |
| T3.1 | ✅ done | Permission gate resolves symlinks to agree with the tool layer. |
| T3.3 | ✅ done | Snapshot durable writes (temp+fsync+rename), mutex, tested `Prune`. Deferred: wiring `Prune` to a startup cadence. |
| T5.1 | ✅ done | TUI key-flow regression harness pinning the `intercepted` contract. |

**T2 is complete** (T2.1–T2.4). T2.2/T2.4 landed via a worktree-isolated parallel
workflow integrated by cherry-pick; T2.3 was implemented inline then hardened by a
3-lens adversarial-verification workflow that caught three real defects before
merge. T1 is closed except the T1.3 leftovers. Next up: **T4** — T4.1 (reconcile
the char/4 token estimate against provider usage) → T4.2 (cache-aware budget
projection + unknown-model guard) → T4.3 (cache-preserving compaction: summary as
tail, wire `mergeCompactSummaries`) → T4.4 (unify compaction triggers, delete dead
`context_fold.go`). Then the remaining T3/T5/T6/T7 stages.

---

## 1. Executive summary

`deepseekcode` is already a mature, DeepSeek-native coding agent, not a
prototype. The hard parts are done and done well: a callback-driven ReAct loop
with the correct finish-reason override and a two-tier stream timeout
(`internal/agent/agent.go`, `internal/llm/client.go`); a cache-stable wire
format whose serializer and Prefix Fingerprint share one canonicalization so
the fingerprint *is* the DeepSeek cache key by construction
(`internal/llm/static_prefix.go`, `request.go`); the visible/latent split of
ADR-0001 implemented exactly as `CONTEXT.md` specifies (`prefix_epoch.go`,
`capability_set.go`); a fail-closed cache-reliability eval gate over JSONL
traces (`bench/cmd/benchrunner/main.go`); pure-Go SQLite sessions with
branch-by-reference replay (`internal/session/`); and a Bubble Tea TUI with a
disciplined append-only scrollback and content-hash render cache
(`internal/tui/`). The thesis of this roadmap is therefore **deepening and
hardening, not greenfield**: the inventory surfaces a coherent class of
"built-but-not-wired" facades (truly dead `context_fold.go` / `struct_search` /
`AnalyzeSchema` with zero non-test refs, the no-production-caller-but-test-covered
`mergeCompactSummaries`, the name-mismatched and externally-unimported
`eventschema`, unwired snapshot pruning, parse-but-ignore agent frontmatter) and a
coherent class of "industrial robustness" gaps (no
mid-stream partial persistence, no resumable streams, char/4 token accounting
on a 1M window, sandbox off by default, snapshots without fsync). Closing those
two classes — while never moving a cache-stable byte, never regressing the
thinking-struct contract, and never breaking the TUI key-flow invariant — is
what turns a strong reference implementation into an agent that beats
DeepSeek-Reasonix on V4 adaptation and stands with opencode / Claude Code /
crush on reliability.

---

## 2. Current capabilities — the baseline we will not break

Grouped by subsystem. Depth and test-coverage honesty per the inventory.

### Agent loop (`internal/agent/`)
- **ReAct core with correct finish-reason override.** `hasTools := len(step.ToolCalls)>0` is authoritative; stop conditions run *before* the natural exit (`agent.go:357-373`). Depth: deep. Coverage: strong.
- **Parallel tool execution + one atomic pre-tool snapshot** of the union of `AffectedPathsFor()` so `/undo` reverts a whole step (`agent.go:897-958`). Depth: deep. Coverage: partial.
- **Event bus** with backpressure: lossy drops for ordinary events, blocking delivery for reply-carrying `EventPermissionAsk`/`EventQuestionAsk` (`bus.go:89-152`). Coverage: strong.
- **Subagent spawning** (sync + async), depth limit 2, task-tool stripping, worktree isolation, child trace tee-in (`spawn_dispatch.go:36-231`). Coverage: strong.
- **Stop conditions**: MaxSteps(50) + LoopDetection(window 5, repeats 3) keyed on tool+sha8(args) (`stop_conditions.go:49-94`). Coverage: strong.
- **Connect-phase retry** with backoff+jitter, correct transient classification (`client.go:61-106`). Coverage: strong.

### Context & cache (`internal/agent/`, `internal/llm/`)
- **Cache-stable serialization, single source of truth.** `MarshalCacheStable` and `StaticPrefix.Fingerprint` both call `canonicalizeTools` (`static_prefix.go:23`); wire bytes and cache key cannot diverge. `TestCacheStableGolden` + `TestFingerprintTracksWireStaticHead` pin it. Depth: deep. Coverage: strong.
- **Prefix Fingerprint = model-visible bytes only; latent identity in Capability Set** (`capability_set.go:32`, `prefix_epoch.go:130`), exactly ADR-0001.
- **Frozen-epoch override** keeps the prefix cache-stable across mid-session drift (`prefix_epoch.go:155`, `agent.go:694-699`).
- **Boundary-safe deterministic compaction** — two-pass `adjustBoundary` never splits a tool_use from its tool_result (`compact.go:129`). Coverage: strong.
- **Pre-HTTP CNY budget gate** prices all input as cache-miss, gates before `Client.Stream` (`budget_projection.go`, `budget.go`).
- **DeepSeek V4 thinking-mode replay sanitization** prepends a stable placeholder inside `MarshalCacheStable` so it never destabilizes the cache (`sanitize.go:16`).

### Tools (`internal/tools/`)
- **Cache-stable Registry** — `AsLLMTools()` name-sorted, `MustParams` deterministic schemas (`registry.go:147`). Depth: deep.
- **Path safety** — `ResolveAndCheck` resolves symlinks on both cwd and target, rejects escapes, with a strong adversarial test surface (`path_safety.go:14`).
- **File tools + `apply_patch`** — atomic tempfile+rename, single-occurrence enforcement, two-phase path validation (`patch.go:206`). Coverage: partial.
- **bash classifier** — per-segment, quote/paren-aware, subcommand-aware destructive classification (`bash_validate.go:31`).
- **web_fetch SSRF guard**, structured git tools, lazy `skill_read` with fixed schema that preserves the fingerprint.

### Permissions, sandbox, undo (`internal/permissions/`, `internal/sandbox/`, `internal/snapshots/`)
- **Tiered `Decide()`** with documented 4-stage precedence and child-mode clamping that prevents subagent escalation (`policy.go:92-263`).
- **Two-Model Duet** as a `PreToolUse` builtin hook, surgical not per-turn, with `duetSelfValidates` no-op when main is pro (`builtin_duet.go:23`).
- **OS sandbox abstraction** (seatbelt / landlock / noop), build-tagged, idempotent `Wrap` (`sandbox_darwin.go`, `sandbox_linux.go`).
- **Snapshot rollback** with `.absent` tombstones for created files (`snapshots/manager.go:48`).

### Sessions & repair (`internal/session/`, `internal/repair/`)
- **Pure-Go SQLite** (WAL, `MaxOpenConns(1)`), idempotent versioned migrations, branch-by-reference + `Replay` with O(unique) storage (`store.go`, `branch.go`).
- **Atomic compaction** in one tx with idx renumbering (`store.go:453`).
- **Three-stage repair** — Scavenge (DSML + OpenAI shapes) / RepairJSONArgs / StormBreaker (`repair/scavenge.go:18`). Coverage: strong.
- **Transcript receipts** audit log (`session/transcript.go:32`).

### TUI (`internal/tui/`)
- **Bubble Tea root + event bridge** with the EventDone deferred-emit ordering guarantee. Depth: deep. Coverage: none (the gap below).
- **Append-only scrollback** with seq-based cache invalidation (`scrollback.go`, `visual.go`).
- **Coalesced ~12fps redraw** decoupling SSE arrival from viewport rebuild (`chrome.go`).
- **Content-hash LRU render cache** for tool call/result items (`render_cache.go`).
- **Status line + HUD** with cache%, cost, epoch, prefix hash, pending changes.

### MCP / skills / LSP / config (`internal/mcp/`, `internal/skills/`, `internal/lsp/`, `internal/config/`)
- **Skills store** — single canonical loader, `BodyHash`→`version_hash` drift, lazy body load keeps bodies out of the prefix (`skills/store.go:148`).
- **MCP canonical drift** — `CompareToolLists` + `schemasEqual` recursive key-sort, the load-bearing ADR-0001 fix (`mcp/drift.go:29`).
- **Layered TOML config** with env-expand, secret resolution, `ValidateStrict` (`config/config.go:140`).

### Observability & CLI (`internal/agent/trace.go`, `bench/`, `cmd/dsc/`)
- **Trace emission** — per-epoch/per-turn/per-compaction with measured before/after prefix hashes, parent/child attribution, fail-closed `child_trace_incomplete`.
- **Benchrunner gate** — 1683 LOC, fail-closed cache-reliability gate over the 4 committed golden trace fixtures in `bench/golden-traces/` (1 pass + 3 fail), exercised by `TestGoldenTraceGates`.
- **CLI subcommand pattern** — `runXxxCommand(args)(string,error)` returns text for testability (`cmd/dsc/main.go`).

These are the invariants the roadmap protects. No stage below may regress them.

---

## 3. Gap matrix

Status legend: ✅ solid · ⚠️ partial / present-but-shallow · ❌ absent · n/a not applicable.
Reference columns reflect the transferable-design analyses, not exhaustive feature audits.
The table holds 29 decision-relevant rows, each cross-referenced to an inventory
file:line and a track.

| Capability area | deepseekcode | opencode | crush | Claude Code | DeepSeek-Reasonix | Verdict |
|---|---|---|---|---|---|---|
| Cache-stable wire + fingerprint = cache key | ✅ `static_prefix.go` | ⚠️ | ⚠️ names-only | ✅ Anthropic-native | ⚠️ none | **Lead. Protect.** |
| Thinking-as-struct correctness + replay hygiene | ✅ `request.go`; ⚠️ replay (no load-time stamp test) | n/a | n/a (bool) | n/a | ✅ `healing.ts` | **Close replay gap (T1)** |
| Mid-stream partial persistence on error | ❌ `agent.go:774-786` | ⚠️ cleanup() | ⚠️ | ✅ | ⚠️ | **Critical (T1)** |
| Resumable / salvageable stream | ❌ connect-only retry `client.go:77` | ⚠️ | ⚠️ | ✅ | ⚠️ | **Critical (T1)** |
| Cancel synthesizes tool results (no dangling tool_calls) | ❌ `agent.go:334-337` | ✅ cleanup() | ✅ | ✅ | ✅ fixToolCallPairing | **High (T1)** |
| Tool-call repair (leaked/truncated/loop) | ✅ `repair/scavenge.go` | ⚠️ | ⚠️ | ⚠️ | ✅ 4-pass | **Lead. Deepen (T2)** |
| Doom-loop detection + corrective nudge | ⚠️ halts, no nudge `stop_conditions.go:82` | ✅ | ✅ | ⚠️ | ✅ storm | **High (T2)** |
| Token accounting vs provider usage | ❌ char/4 `compact.go:55` | ✅ usage at step-finish | ✅ estimated flag | ✅ | ✅ tokenizer | **High (T4)** |
| Realized cache-hit-rate surface | ⚠️ trace only `trace.go:184` | ⚠️ | ⚠️ | ✅ break detector | ✅ cacheHitRatio | **High (T4/T6)** |
| Compaction merges prior summary | ❌ `mergeCompactSummaries` defined + unit-tested, no production caller (`compact_summary.go:196`) | ✅ anchored update | ⚠️ | ✅ micro-compact | ✅ fold | **High (T4)** |
| Compaction preserves cached prefix | ⚠️ mid-stream system msg busts body cache `compact.go:96` | ✅ | ⚠️ | ✅ | ✅ summarizer reuses prefix | **High (T4)** |
| Fuzzy multi-strategy edit replacer | ❌ single exact match | ✅ 9-strategy `edit.ts` | ⚠️ | ⚠️ | ✅ byte-exact + escalate | **High (T2)** |
| Read-before-write freshness guard | ❌ | ⚠️ | ✅ filetracker | ✅ readFileState | ✅ ReadTracker | **High (T3)** |
| OS sandbox on by default | ❌ off `defaults.go:62` | ✅ | n/a | ✅ | ⚠️ | **Critical (T3)** |
| Sandbox covers file tools + reads | ❌ bash-only, read-open `sandbox_darwin.go:68` | ✅ | n/a | ✅ | ⚠️ | **High (T3)** |
| Permission gate resolves symlinks | ❌ lexical `policy.go:140` | ✅ | ✅ | ✅ | ✅ | **Critical (T3)** |
| Destructive bash snapshotted / revertible | ❌ nil affected paths | ⚠️ | ⚠️ | ⚠️ | ⚠️ | **Medium (T3, side-git)** |
| Snapshot pruning wired | ❌ `Prune` no caller | ✅ | ✅ | ✅ | ✅ | **High (T3)** |
| Snapshot durability (fsync/atomic) | ❌ `manager.go:79` | ✅ | ✅ | ✅ | ✅ | **High (T3)** |
| /undo reconciles transcript | ❌ files only `app.go:1238` | ✅ revert msg/part | ✅ | ✅ | ⚠️ | **High (T3)** |
| Agent-def frontmatter applied at spawn | ❌ parse-but-ignore `spawn_dispatch.go:117` | ⚠️ | n/a | ✅ | ⚠️ | **High (T7)** |
| MCP liveness / reconnect | ❌ `StateDegraded` unused | ⚠️ | ✅ backoff | ✅ | ⚠️ | **High (T2)** |
| LSP write-path (didChange / diagnostics feedback) | ❌ didOpen-once `lsp/client.go:112` | n/a | ✅ | n/a | ✅ post-edit | **Medium (T2)** |
| Deterministic offline mock-DeepSeek harness | ❌ | ✅ mock-service | ⚠️ | ⚠️ | ⚠️ | **High (T6)** |
| Trace replay (offline re-grade) | ❌ | ⚠️ | ⚠️ | ⚠️ | ⚠️ | **Medium (T6)** |
| Single trace schema source of truth | ❌ 3 copies | n/a | n/a | n/a | n/a | **High (T6)** |
| TUI key-flow regression tests | ❌ none `app.go` | n/a | ✅ golden | ⚠️ | ⚠️ | **High (T5)** |
| Streaming progress (per-tool ready, ctx-fill) | ⚠️ caption only `chrome.go:127` | ⚠️ | ⚠️ | ⚠️ | ✅ ready count | **Medium (T5)** |
| Admin policy floor above mode flags | ❌ flags override globally | ⚠️ | ⚠️ | ⚠️ | n/a | **Low (T7)** |

---

## 4. DeepSeek-specific advantages — how we beat DeepSeek-Reasonix

deepseekcode's edge is not feature count; it is that the cache-stability
machinery is *rigorous and provable* where Reasonix's is prompt-discipline only.
We extend that edge along seven concrete lines.

### Prompt-cache discipline (we already lead; make it provable and visible)
- **Registry golden test for tool-listing bytes** (analogous to `TestCacheStableGolden`) so any schema/description drift that would bust the 50× discount fails CI. Reasonix keeps fragments byte-stable by *convention* (a code comment); we pin it by *test*. (Tools inventory, DeepSeek-specific #1.)
- **Subagent prefix inheritance** — pass the parent's frozen rendered system+tools bytes into the child instead of re-rendering at spawn, then assert `child.fingerprint == parent.fingerprint`. This is the Claude Code `forkSubagent` lesson; it protects the 50× discount across the subagent boundary, which Reasonix does not do. (Claude Code transferable; our `spawn_dispatch.go` is the seam.)
- **Compaction that preserves the cached prefix.** Today the summary is a `role:"system"` message at `message[1]`, the first byte after the cached prefix, so every compaction guarantees a body cache miss (`compact.go:96`, context-cache gap). Reasonix folds only the tail and reuses the agent's live cached prefix for the summarizer call. Adopting that (T4) makes the 50× discount survive compaction — a direct Reasonix parity-then-beat.

### Thinking-struct correctness (validated by reference, hardened by us)
- opencode independently concluded DeepSeek has *no* effort tiers (`transform.ts` excludes deepseek); this validates our single-struct `ThinkingEnabled`. Do not build an effort-budget UI.
- The gap is **round-trip hygiene on replay**: Reasonix's `stampMissingReasoningForThinkingMode` + `fixToolCallPairing` run on session load. Our by-reference branching replays chains and can synthesize messages that 400 on `reasoning_content` or unpaired `tool_calls`. CodeWhale/Reasonix both solved this; we must (T1).

### Reasoner cold-start handling (extend the two-tier timeout)
- We already split `FirstTokenTimeout`(45s) from `ChunkStallTimeout`(20s). The V4 reasoner emits long *silent* reasoning gaps; a **reasoning-aware stall timeout** (longer tolerance while `reasoning_content` flows, tighter once visible text starts) reduces false-positive aborts specific to the V4 reasoner (agent-loop DeepSeek-specific #3). Drive the TUI cold-start caption from the *actual* timer state, not the hardcoded `elapsed>5s` heuristic (`chrome.go:127`).

### JSON mode + the Duet (a strictly richer Reasonix escalation)
- Keep the Duet's pro-validator prompt/schema byte-stable and route it through the cache-stable serializer (permissions DeepSeek-specific #2) so validator turns also earn the discount.
- Adopt Reasonix's **model-driven escalation** as a complement, not a replacement: a `<<<NEEDS_PRO>>>` first-line marker lets flash self-declare it is out of depth, plus auto-escalation after 3+ repair/SEARCH-mismatch errors in a turn. Our Duet only validates destructive calls; adding self-escalation captures most of pro's quality at flash's price (T2). The model-name is the *only* allowed interpolant and must be part of the fingerprint input.

### Model routing / FIM
- FIM and the `/beta` strict-tool mode are real DeepSeek capabilities (CodeWhale do-not-copy note). Treat them as opt-in wire surface only, and never let a `strict` flag reorder tool-schema keys upstream of `MarshalCacheStable`. FIM is out of Phase-1 scope; documented here so it is not added carelessly.

### Pricing / budget (turn the 50× into a budget feature, not just a HUD number)
- The single biggest source of false hard-blocks is `ProjectedTurnCostCNY` pricing all input as cache-miss (`budget_projection.go:14`). On a healthy 90%+ hit session this fires ~50× too early. **Discount the projection by the rolling session cache-hit rate** (which DeepSeek returns in every usage frame) (T4). Reasonix and CodeWhale both surface realized hit/miss split + cumulative savings; we should surface a "saved ¥X via cache" chip and a "claude-equivalent" framing — the cache-savings narrative is the DeepSeek cost story and a hard-to-copy differentiator.
- Make the pricing table config-overridable and guard the **unknown-model bypass**: an unknown model id returns `Cost=0`, which silently disables the budget gate and reports turns as free (`cache_metrics.go`, `budget_projection.go:11-13`). A future V4 id must warn, not bypass (T4).

**Where we beat Reasonix specifically:** provable cache reliability (the
benchrunner gate scores cache-hit stability as a first-class dimension, which no
generic harness does); fingerprint-equals-wire-bytes by construction (Reasonix
has no cache-stability machinery); subagent cache isolation already attributed
in traces; pure-Go single-static-binary with no bundled tokenizer blob.

---

## 5. TUI direction

Posture per `docs/design.md` §10: scrollback-first, no persistent side panels,
high information density, restrained — tmux/zellij users split their own panes.
The reference is crush's typed-style discipline and golden-per-width tests, not
its dependency stack. Every change respects the key-flow invariant
(`Update()` returns early only when `handleKey` reports `intercepted=true`;
reasoning fold is `ctrl+r`/`ctrl+t`; never a plain letter).

### Information architecture
- **One scrollback, three overlays** (tape / models / sessions) stay as-is. The append-only + seq-drift invariant is load-bearing and well-isolated; do not touch it.
- **Status HUD is the DeepSeek surface.** Promote a persistent cache chip ("cache 91% · saved ¥X") and a **context-fill bar against the 1M window** — `ContextLimit` is already plumbed into `HUDData` but never populated by `status.go` (TUI DeepSeek-specific #1). Fix the overloaded `hint` slot so a drift chip and a transient notice can coexist (`app.go:411-418`).

### Visual hierarchy
- Resolve the **dead UI vocabulary** (`IconArrowRight`, `ScrollbarThumb`, `SectionRule`, `StatusBad`, etc.): either finish the scrollbar/section-rule design or delete the symbols. Today the symbol set implies a richer hierarchy than the renderer delivers (TUI visual-hierarchy gap).
- **Theme-derived code highlighting.** Replace the hardcoded `dracula` chroma style (`highlight.go:30`) with a theme-branched style so light-theme code blocks are legible and the Ocean identity is coherent across markdown, cards, and code.

### Streaming feedback
- **Per-tool "ready" progress** (Reasonix): as each streamed tool-call's args first parse as valid JSON, surface an "N ready" count so a long multi-tool turn shows progress instead of a stagnant spinner — still not dispatching until stream end.
- **Authoritative cold-start caption** driven by the real `FirstTokenTimeout`/`ChunkStallTimeout` state, replacing the `elapsed>5s` guess.
- **Abort-as-clean-exit**: Esc / model-switch yields a "done" row, not a red error row.

### Tool / result presentation
- **Fix the off-by-one line counts** — `renderReadFileSummary`/`renderBashSummary`/`renderGrepSummary` use `strings.Count("\n")` and undercount the final line; tests currently encode the wrong value (`tool_renderers.go:48,73,98`; `status_hud_test.go:178`). Fix code and tests together.
- **Consolidate JSON-arg extraction** onto the `json.Unmarshal`-based `compactArgs` and delete the fragile `extractJSONString` substring scanner (`items.go:262`, `tool_renderers.go:121`).

### Reasoning fold & diff rendering
- The reasoning-tape overlay is a DeepSeek-thinking differentiator; expand it into a navigable, foldable timeline with the pro/flash glyphs (`◇`/`◆`) wired, so users can audit the reasoner's chain — bound only to `ctrl`-modified or overlay-local keys.
- **Diff auto-detection** (crush `diffdetect`, ~36 LOC, no deps): route raw `git diff` from bash through the syntax-highlighted diff renderer instead of plain text. Pin output with **golden-per-width tests** — the discipline crush proves, which a render cache cannot catch.

---

## 6. Phased roadmap

Seven tracks in priority order. Each stage is PR-sized: independently
buildable, testable, and reviewable. No big-bang stages.

### T1 — Stability & DeepSeek wire-format correctness

**T1.1 Partial-turn persistence on mid-stream error.**
*Goal:* honor the `defer flushUI()` promise in `design.md:241` that does not yet
exist — on `EventError` after deltas, append the partial assistant message to
`a.Messages` and the Persister before returning. *Modules:* `internal/agent/agent.go`
(`runStep` flush path). *Why:* a stall 15 min into a 1M-context turn currently
discards all work and leaves the persisted transcript inconsistent with what the
user saw (`agent.go:774-786`). *Risk:* the partial assistant turn must satisfy
`SanitizeForDeepSeek` (thinking placeholder) or the next request 400s — covered
by T1.2.

**T1.2 Replay hygiene — repair dangling tool calls on load. — DONE.**
On scoping, two of the three pieces were already handled and only the third
was a real gap:
- *Within-Run cancel pairing — already safe.* `runToolCalls` synthesizes a
  result for every call even on a cancelled context (`executeOne` returns
  `"user cancelled"` on `context.Canceled`) and appends one tool-result
  message per call, so a cancel never leaves a dangling `tool_call` in the
  live history.
- *`stampMissingReasoning` — already covered.* `MarshalCacheStable` calls
  `SanitizeForDeepSeek` on every request, so the thinking placeholder is
  stamped on fresh, replayed, and branched messages alike.
- *The real gap — crash between persists.* A process interrupted between
  persisting an assistant `tool_call` turn and persisting its results
  reloads via `Replay` with a dangling `tool_call` that DeepSeek 400s.
  Fixed in `internal/session/tool_pairing.go`: `Replay` now calls
  `repairDanglingToolCalls`, which synthesizes an `IsError` placeholder
  result for any unmatched `tool_call` id, inserted after the assistant
  message that issued it. No-op (no allocation) when already paired, so the
  cache-stable wire bytes built from a clean history are untouched, and the
  synthesized messages ride the tail, never the Static Prefix.

**T1.3 Typed stop reasons + structured control events.**
*Goal:* emit `StopUserRequested` from TUI cancel (`tui/app.go:684`), add
`StopStepTimeout`, and promote budget-blocked / drift-blocked / denied-by-rule
from stringly-typed `EventInfo` to typed events. *Modules:* `stop_conditions.go`,
`agent.go:347-350,721-728`, `events.go`. *Why:* honest analytics and
programmatic gating; "user stopped" vs "context expired" are currently
indistinguishable. The step-timeout path today does worse than lose the type: it
returns `StopUnknown, nil` and swallows the deadline into an `EventInfo`
(`agent.go:347-350`), i.e. a **silent success-shaped exit** — the caller's error
channel sees no error and the run looks like a clean finish. `StopStepTimeout`
must surface as a non-success stop reason so analytics and the TUI never render a
timeout as a done row. *Risk:* low; additive enum + event variants. Delete the
unreachable tool-error abort branch (`agent.go:379-381`) in the same PR.

**T1.4 Mid-stream salvage + one bounded re-issue.**
*Goal:* on first-token/chunk-stall timeout, salvage streamed tokens (via T1.1)
and re-issue the turn once before failing, classifying the error first
(context-overflow → compact, never retry). *Modules:* `internal/llm/client.go`,
`agent.go`. *Why:* long V4 turns over 1M context are exactly where transient
mid-stream stalls happen (agent-loop gaps 1-2; opencode `retry.ts`). *Risk:*
medium — must not double-charge or duplicate persisted turns; gate the re-issue
on T1.1 having salvaged a clean boundary.

### T2 — Agent-loop observability, error recovery, tool-call lifecycle

**T2.1 Loop-detection corrective nudge.**
*Goal:* on `StopLoopDetected`, inject a synthetic tool result asking the model to
summarize what it was trying to do (one chance to break out) before hard-stopping.
*Modules:* `stop_conditions.go:82`, `agent.go:361-367`. *Why:* `design.md:246-249`
specifies this; today a recoverable thrash becomes a hard stop, hurting autonomy
on the multi-hundred-step runs the raised limits target. *Risk:* low; cap to one
nudge per detection to avoid a nudge-loop.

**T2.2 Fuzzy multi-strategy edit replacer.**
*Goal:* add an ordered strategy cascade (exact → line-trimmed → whitespace-normalized
→ indentation-flexible → block-anchor) to `edit_file`, first unique match wins,
ambiguous rejected. *Modules:* `internal/tools/edit_file.go` (+ table tests).
*Why:* highest single reliability lever for an LLM edit tool — DeepSeek frequently
emits `old_string` with slightly-off whitespace/indentation, and a single
exact-match strategy burns a retry turn (opencode `edit.ts`, the #1 likely-lacks).
*Risk:* low — pure string algorithms, zero deps; the uniqueness guard preserves
correctness. Also fix `apply_patch`'s first-match-wins `locateChunk`
(`patch.go:271`) to reject ambiguous context like `edit_file` already does.

**T2.3 Model-driven escalation + auto-escalate on repeated repair.**
*Goal:* parse a `<<<NEEDS_PRO[: reason]>>>` first-line marker to re-issue the turn
once on pro; auto-escalate after 3+ repair/SEARCH-mismatch errors in a turn; emit
a `policy.escalated` event. *Modules:* `internal/agent/` (loop + prompt fragment),
`internal/repair/`. *Why:* a strictly richer Duet — flash self-declares out-of-depth
(Reasonix). *Risk:* the escalation-contract text lives in the system region;
model-name is the only allowed interpolant and must feed the fingerprint.

**T2.4 MCP liveness recovery.**
*Goal:* assign `StateDegraded` when a stdio server exits, stop advertising its
tools, attempt one bounded reconnect with negative-result backoff (crush
`manager.go`, 30s). *Modules:* `internal/mcp/registry.go`, `stdio.go:200-211`,
`server.go`. *Why:* a flaky MCP server currently poisons part of the tool set for
the whole session with no recovery (MCP gap, high). *Risk:* tool-set changes are
Capability-Set drift — must route through `CompareToolLists`, never move the
fingerprint mid-epoch.

### T3 — Industrial permission / sandbox / undo / checkpoint

**T3.1 Permission gate resolves symlinks (close the layer divergence).**
*Goal:* make `Policy.Decide` resolve symlinks on `AffectedPaths` so it agrees with
the tool layer's `ResolveAndCheck`. *Modules:* `internal/permissions/policy.go:140-149`.
*Why:* today a symlink inside cwd pointing outside (or into `.git`/a secret)
passes the gate as a "safe write inside cwd" while the tool resolves it elsewhere
(permissions gap, high). *Risk:* low and high-leverage; add adversarial tests
mirroring `path_safety_test.go`.

**T3.2 Read-before-write freshness guard.**
*Goal:* record mtime+normalized-content on every `read_file`; `edit_file`/`write_file`/
`apply_patch` reject with a model-actionable error if the file was never read or
changed since (re-read to recover). *Modules:* `internal/tools/` + a session-scoped
tracker; invalidate on history fold. *Why:* prevents clobbering edits made by
formatter/linter/user/parallel-tool between read and write (Claude Code
`readFileState`, crush `filetracker`, Reasonix `ReadTracker`). *Risk:* low;
orthogonal to `/undo` (which reverts *agent* edits).

**T3.3 Snapshot durability + pruning + concurrency guard.**
*Goal:* write snapshots via temp+rename with fsync; add a mutex to `snapshots.Manager`;
wire `Prune(keepSessionIDs)` to the parsed `SnapshotKeep` config on a background
cadence. *Modules:* `internal/snapshots/manager.go:79-93,151`, startup wiring.
*Why:* crash mid-step currently restores a truncated file as if complete; the
`.deepseek/snapshots` tree grows unbounded (`design.md:1004` specs a prune that
was never implemented); no fs-layer concurrency guard while SQLite is serialized
(session-undo gaps, high×3). *Risk:* low; pure local-disk hardening.

**T3.4 Sandbox on by default + cover file tools + scope reads (macOS first).**
*Goal:* default `Sandbox.Enabled=true`; wrap `write_file`/`edit_file`/`apply_patch`,
not just bash; scope seatbelt reads (the Darwin profile's blanket `allow file-read*`
leaves `~/.ssh`/`~/.aws` readable). Keep the policy-text/param-table sync invariant
and tool carve-outs (npx MCP needs `~/.npm` write). *Modules:* `internal/config/defaults.go:62`,
`internal/sandbox/sandbox_darwin.go:68`, tool wrap sites. *Why:* the single biggest
"industrial vs demo" gap — out of the box, safety reduces to the prompt + an
opt-in fail-open Duet (permissions gaps, high×4; CodeWhale seatbelt). *Risk:*
**highest UX risk in the roadmap** — a too-tight profile breaks "my `$PATH`
works." Ship macOS-only and degrade to no-op honestly; gate behind behavioral
tests that a denied write surfaces through the agent bash path. Linux landlock
read-scoping follows as a separate stage.

**T3.5 /undo reconciles the transcript.**
*Goal:* after `/undo N`, fork a child session at the corresponding `branch_point`
(or truncate `a.Messages` + persisted messages) so the model's view matches the
files. *Modules:* `internal/tui/app.go:1238`, `internal/session/branch.go`. *Why:*
today files revert but SQLite/in-memory/model views still contain the reverted
turn — the model believes the edit succeeded (session-undo gap, high). Fork-on-undo
is also the cache-friendly "time-travel" play: the Static Prefix stays byte-identical
so the 50× discount survives the rewind. *Risk:* medium; the snapshot-step↔message-idx
mapping is currently opaque after compaction — pair the namespaces in the same PR.

### T4 — Context management, cache hit, prefix epoch, compaction

**T4.1 Reconcile token estimate against provider usage.**
*Goal:* feed back `usage.PromptTokens` (already parsed at `client.go:246`) to learn
a per-session chars-per-token ratio; use it for the auto-compact trigger and the
budget projection. *Modules:* `internal/agent/compact.go:55`, `budget_projection.go:21`.
*Why:* char/4 is off 2-3× for code/CJK/JSON on a 1M window, so compaction fires
early (cache-busting summary) or late (provider 400); the budget gate inherits the
skew (context-cache gaps, high; opencode `overflow.ts`; Reasonix CJK note). *Risk:*
low; keep char/4 as the cold-start prior, calibrate after the first usage frame.
Do not bundle a tokenizer (single-binary constraint).

**T4.2 Cache-aware budget projection + unknown-model guard.**
*Goal:* discount `ProjectedTurnCostCNY` by the rolling session cache-hit rate; warn
(not silently free-pass) on unknown models. *Modules:* `budget_projection.go:14`,
`cache_metrics.go`. *Why:* assuming 100% miss fires the hard-block ~50× too early
on healthy sessions; an unknown V4 id bypasses the gate entirely (context-cache
gaps). *Risk:* low; keep the conservative all-miss path available as a config floor.

**T4.3 Cache-preserving compaction (summary as message tail, merge prior).**
*Goal:* stop injecting the summary as a mid-conversation `role:"system"` message at
`message[1]`; keep the cached prefix intact and fold the summary into the tail;
wire `mergeCompactSummaries` so re-compactions update the prior summary instead of
dropping it. *Modules:* `internal/agent/compact.go:96`, `compact_summary.go:196`,
`agent.go:445,482`. *Why:* every compaction currently busts the body cache and can
silently lose early-session facts across rounds (context-cache gaps, high; opencode
anchored-update; Reasonix prefix-reuse + negative-constraint preservation).
`mergeCompactSummaries` is **already defined and unit-tested** (three cases in
`compact_test.go`) but has **no production caller** — this is a wiring task, not a
revival of dead code; its existing tests must keep passing and it must not be deleted
by a no-refs cleanup. *Risk:*
medium — add a provider-acceptance test for the message shape and a benchrunner
assertion that compaction does not move the prefix (the gate already checks
before==after).

**T4.4 Unify compaction triggers; delete dead `context_fold.go`.**
*Goal:* reconcile the absolute 800k threshold with the ratio-of-max threshold so
they cannot disagree; delete `context_fold.go` (zero production callers, duplicates
`adjustBoundary`). *Modules:* `compact.go:253`, `semantic_compact.go:99`,
`agent.go:466-472`, remove `context_fold.go`. *Why:* two unreconciled triggers can
fight; the dead parallel implementation is drift risk between two algorithms that
must agree (context-cache gaps). *Risk:* low; deletion test confirms nothing
depends on it.

### T5 — TUI information architecture, visual hierarchy, streaming, tool/result, reasoning fold

**T5.1 Key-flow regression harness.**
*Goal:* construct an `App`, drive `Update`/`handleKey`/`setMode`/`dispatchAgentEvent`,
and assert the `intercepted` contract — including no j/k leak after `/clear`
(`app.go:347`). *Modules:* `internal/tui/*_test.go`. *Why:* CLAUDE.md names this the
single most fragile TUI invariant and it has zero regression protection (TUI gap,
high). *Risk:* none; pure test addition, must land before any other T5 stage.

**T5.2 Presentation-correctness fixes.**
*Goal:* fix off-by-one line counts (use `lineCount()`), consolidate onto
`compactArgs` and delete `extractJSONString`, theme-branch the chroma style.
*Modules:* `tool_renderers.go:48,73,98,121`, `items.go:262`, `highlight.go:30`,
and the tests that encode the wrong values. *Why:* small correctness bugs the tests
currently cement (TUI gaps, medium). *Risk:* low; fix code and tests in one PR.

**T5.3 DeepSeek cache surface + streaming progress.**
*Goal:* populate `ContextLimit` for a 1M-window fill bar, add a persistent
"cache % · saved ¥X" chip, de-overload the `hint` slot, add per-tool "N ready"
progress, and drive the cold-start caption from real timer state. *Modules:*
`status.go`, `status_hud.go`, `chrome.go:127`, `app.go:411-418`. *Why:* makes the
cache-stability investment visible — no generic agent TUI does this (TUI
DeepSeek-specific #1-3). *Risk:* low; all additive, no key bindings.

**T5.4 Diff auto-detection + golden-per-width tests.**
*Goal:* add a `diffdetect`-style scanner to route raw diffs through the highlighted
renderer; pin tool-call/result rendering with golden fixtures across narrow/default/
wide widths. *Modules:* `internal/tui/`, `testdata/`. *Why:* upgrades the raw-`git diff`-
via-bash path and catches layout regressions a render cache cannot (crush `diffview`).
*Risk:* low; golden tests are append-only.

### T6 — Benchmark, trace replay, eval harness

**T6.1 Single trace schema source of truth + schema-version field.**
*Goal:* collapse the three duplicated trace structs (`trace.go:17`,
`traceinspect.go:12`, `benchrunner/main.go:107`) toward one shared contract and add
a `schema_version` field. *Modules:* `internal/agent/trace.go`, `internal/traceinspect/`,
`bench/`. *Why:* a field rename in the emitter silently breaks the reader/harness
with no compile error — the central fragility of the eval pipeline (observability
gaps, high). *Risk:* medium — `bench/` is outside `internal/`; expose a small public
schema package the harness can import, or generate the harness struct from a pinned
fixture.

**T6.2 Reconcile `eventschema` with emitted trace names (wire or retire).**
*Goal:* `eventschema` has **zero importers outside its own package** (only
`schema.go` + `schema_test.go`), yet its 11 constants match *no* wire string the
emitter actually writes (e.g. `PrefixEpochFrozen = "prefix.epoch.frozen"` vs the
emitted `"epoch.frozen"` at `trace.go:173`), so it is anti-documentation despite
CLAUDE.md framing it as a canonical contract for downstream consumers. Resolve the
mismatch by **one of two routes, decided in this PR**: (a) *retire* — delete the
package and, in the same PR, remove the "canonical event-name constants … so
downstream consumers reference the same string identifiers" paragraph from
CLAUDE.md so the docs stop describing a contract that no longer exists; or
(b) *canonicalize* — make `eventschema` the single source of the emitted strings
and have `trace.go` reference the constants. Note that route (b) changes the
emitted wire strings, which are **not** cache-stable-protected (they are not part
of `MarshalCacheStable`) but **are** string-matched by `traceinspect` and the
benchrunner, so it is a coordinated three-surface change (`trace.go` +
`internal/traceinspect` + `bench/cmd/benchrunner`) gated by a name-match test that
asserts every emitted `Type` is `eventschema.Known()`. *Modules:*
`internal/eventschema/`, `trace.go:146-242`, `CLAUDE.md` (route a) or
`internal/traceinspect/` + `bench/cmd/benchrunner/` (route b). *Why:* removes a
live source of confusion that misrepresents the schema. *Risk:* low for route (a)
(deletion + doc edit); medium for route (b) (sequence it **with or after T6.1** so
the rename lands once against a consolidated schema, not three diverging copies).

**T6.3 Deterministic offline mock-DeepSeek SSE harness.**
*Goal:* an `httptest.Server` in `internal/llm` (or `internal/agent`) replaying scripted
SSE keyed by scenario, driving the real loop end-to-end — finish-reason override,
two-tier timeout, thinking-struct, tool-pairing — with no live credentials. *Modules:*
`internal/llm/`, `internal/agent/`, table-driven scenarios. *Why:* the single biggest
eval/observability win; lets the full loop be tested deterministically in CI (claw-code
`mock-anthropic-service`). *Risk:* low; tests live inside the package per the constraint.
Reimplement claw-code's Python linkage runner as a Go test.

**T6.4 Trace replay (offline re-grade) + realized-vs-projected cost in inspect.**
*Goal:* add a record/replay path that re-runs a captured trace against the loop offline,
and have `traceinspect` surface compaction/drift/pending records (it currently shows a
clean summary the gate would fail) plus realized-vs-projected cost. *Modules:*
`internal/traceinspect/traceinspect.go:87-109`, `cmd/dsc/trace.go`. *Why:* lets a
regression be caught without burning live tokens, and closes the project→measure→grade
loop (observability gaps, medium). *Risk:* medium; depends on T6.1's stable schema.

### T7 — Docs, config, agent profiles, UX polish

**T7.1 Apply agent-definition frontmatter at spawn.**
*Goal:* consume `def.MaxSteps`, `PermissionRuleset`, `Temperature`, `TopP`,
`DefaultAgent` in `spawn_dispatch.go`. Today the **already-consumed** set is
Tools/Mode/Worktree/Model **and `def.Prompt`** (`child.System = def.Prompt` at
`spawn_dispatch.go:113`, falling back to the parent system when empty), while
`MaxToolCalls` is hardcoded to 50 (`spawn_dispatch.go:117`). The precise
**parse-but-ignore** set this stage must wire is therefore exactly
{`MaxSteps`, `PermissionRuleset`, `Temperature`, `TopP`, `DefaultAgent`} — do not
re-implement `Prompt` handling, which already works. *Modules:*
`internal/agent/spawn_dispatch.go:45-117`, `internal/agents/def.go`. *Why:* a
parse-but-ignore facade — users author these fields, `dsc agent validate` accepts
them, and they silently do nothing (MCP/profiles gap, high). *Risk:* low; add a
test asserting parsed fields reach the spawned agent and the llm request.

**T7.2 `dsc doctor` + non-blocking config diagnostics + manifest-to-doc linkage.**
*Goal:* extend `doctor` to classify config/agent-def/MCP/session-store health with
field-level fix hints (cross-check builtin hook names against the registry); add a Go
test that fails CI when a doc reference (e.g. a PARITY scenario or feature) points at a
non-existent section. *Modules:* `cmd/dsc/doctor.go`, `internal/config/validate.go:76-102`.
*Why:* malformed agent defs are silently skipped; a typo'd builtin hook passes validation
then fails fail-open at runtime; docs rot apart from code (claw-code `ensure_refs_exist`,
config-ergonomics gaps). *Risk:* low; `doctor` already exists, follows the
`runXxxCommand` pattern.

**T7.3 Skill hot-reload + cheaper read-only subagents + admin policy floor.**
*Goal:* add a `/reload-skills` command (mint a new epoch — one deliberate cache miss);
let read-only/explore agent defs request a cheaper model tier and omit project memory;
add an optional `requirements.toml` floor that refuses disallowed approval/sandbox modes
above the `--yolo`/`--read-only` flags. *Modules:* `internal/skills/store.go`,
`internal/agents/def.go`, `internal/permissions/`, `internal/config/`. *Why:* skill edits
are invisible until restart (ergonomics cliff); explore fan-out should run on flash;
mode flags currently override the policy globally with nothing above them (skills/profiles
gaps; CodeWhale `requirements.toml`). *Risk:* low; all opt-in, none touch the hot path.
Reload must mint a new epoch via the existing `SwitchEpoch`, never mutate a frozen one.

**T7.4 Documentation truthing: PARITY/README/MODEL_COMPATIBILITY.**
*Goal:* add a `MODEL_COMPATIBILITY.md` documenting DeepSeek-specific wire facts
(thinking struct, cache-stable requirement, reasoner cold-start timeouts); extend
`docs/PARITY.md` with an honest "intentionally shallow" section; update `README.md` +
`README.zh-CN.md` together with matching `##` structure when capabilities land. *Modules:*
`docs/`, `README*.md`. *Why:* honest capability mapping; the bilingual README rule applies
to README only, not `docs/*.md`. *Risk:* low; documentation only.

---

## 7. Acceptance criteria

Per stage, the verifiable bar. Baseline for every stage: `make test` green,
`make lint` clean, `make test-race` green where concurrency changes, and
`TestCacheStableGolden` + `TestFingerprintTracksWireStaticHead` **unchanged**.

| Stage | Done when |
|---|---|
| T1.1 | New test: inject `EventError` after N deltas; assert the partial assistant message is in `a.Messages` and the Persister, and the next request passes `SanitizeForDeepSeek`. |
| T1.2 | Cancel mid-step leaves zero dangling `tool_calls`; a `Replay` of a thinking session produces a request DeepSeek accepts (validated against the T6.3 mock). |
| T1.3 | TUI ctrl+c yields `StopUserRequested` in the trace; step-timeout yields `StopStepTimeout` as a **non-success** stop reason (asserted not to be the old `StopUnknown, nil` clean-finish shape, so it is not rendered as a done row); budget/drift/denied events are typed; dead abort branch removed; golden bytes unchanged. |
| T1.4 | Simulated mid-stream stall re-issues exactly once; context-overflow routes to compaction (never retried); no duplicate persisted turn. |
| T2.1 | Loop-detection injects exactly one synthetic summary nudge; a test shows the model can break out before hard-stop. |
| T2.2 | Table tests: each fuzzy strategy matches its near-miss; ambiguous match is rejected; `apply_patch` rejects non-unique context. On a committed near-miss scenario set (≥20 `old_string` cases with realistic whitespace/indentation drift, checked into the package testdata), first-attempt edit success rises from the exact-match-only baseline to ≥95% — both numbers recorded in the test so the before/after delta is asserted, not described, and run against the T6.3 mock loop. |
| T2.3 | `<<<NEEDS_PRO>>>` re-issues once on pro; 3 repair errors auto-escalate; `policy.escalated` event present; escalation contract bytes are part of the fingerprint input (golden unchanged otherwise). |
| T2.4 | Killing a stdio MCP subprocess marks it `StateDegraded`, drops its tools via `CompareToolLists` (no fingerprint move), and reconnects once; `make test-race` green. |
| T3.1 | Adversarial test: symlink-in-cwd→outside is denied by `Decide`, matching `ResolveAndCheck`. |
| T3.2 | Edit of an unread or stale file returns the model-actionable error; re-read clears it; guard invalidated on history fold. |
| T3.3 | Crash-mid-snapshot test leaves no truncated restore (temp+rename+fsync); `Prune` trims to `SnapshotKeep`; concurrent Take/Undo race test green under `-race`. |
| T3.4 | macOS: a denied write surfaces "blocked by sandbox" through the agent bash path; reads of `~/.ssh` are denied; npx MCP server still starts; sandbox default is on; CI build matrix green on all 5 platforms (no-op elsewhere). |
| T3.5 | `/undo N` forks a child session (or truncates messages) so `a.Messages` matches disk; Static Prefix fingerprint identical pre/post undo. |
| T4.1 | After the first usage frame, the chars-per-token ratio tracks `usage.PromptTokens` within a measured band; auto-compact fires within ±10% of the real 800k-token point on a CJK-heavy fixture. |
| T4.2 | On a 90%-hit fixture, projected cost is within ~2× of realized (not ~50×); unknown model id logs a warning instead of free-passing. |
| T4.3 | Benchrunner asserts compaction prefix before==after (cache preserved); re-compaction merges the prior summary (early facts retained in a multi-round test); provider-acceptance test for the summary message shape passes. |
| T4.4 | Single reconciled trigger; `context_fold.go` deleted with a passing deletion test; no behavior change in existing compaction tests. |
| T5.1 | Tests drive `Update`/`handleKey`; `intercepted` contract asserted; no j/k leak after `/clear`. |
| T5.2 | A 3-line read renders "3 lines"; `extractJSONString` deleted; light theme renders legible code. |
| T5.3 | HUD shows a populated 1M context-fill bar and a saved-¥ chip; drift chip and transient hint coexist; "N ready" appears on a multi-tool turn; cold-start caption matches the real timeout. |
| T5.4 | Raw `git diff` from bash renders highlighted; golden fixtures pinned at ≥3 widths; a width-layout regression fails CI. |
| T6.1 | One schema definition (or generated harness struct); `schema_version` present on every record; a field rename breaks the build, not silently the reader. |
| T6.2 | Route (a): `eventschema` deleted and the canonical-contract paragraph removed from CLAUDE.md in the same PR. Route (b): every emitted trace `Type` satisfies `eventschema.Known()` (name-match test green) and the emitted-string rename is reflected in `traceinspect` + benchrunner in one PR sequenced with/after T6.1. |
| T6.3 | The full agent loop runs against the mock with no network: finish-reason override, two-tier timeout, thinking-struct, and tool-pairing paths all exercised; reproduces all committed golden fixtures (the 4 in `bench/golden-traces/` — 1 pass + 3 fail — already gated by `TestGoldenTraceGates`, `bench/cmd/benchrunner/main_test.go:852`). |
| T6.4 | Trace replay reproduces every committed golden fixture offline (the 4 in `bench/golden-traces/`) plus any real captures; `dsc trace inspect` surfaces compaction/drift/pending and realized-vs-projected cost. |
| T7.1 | A spawned subagent honors `max_steps`/`temperature`/`top_p`/`permission_ruleset` from its def; test asserts the values reach the llm request. |
| T7.2 | `dsc doctor` reports field-level config/agent-def/MCP/session diagnostics; a doc-linkage test fails when a reference is dangling. |
| T7.3 | `/reload-skills` mints a new epoch (one expected cache miss, fingerprint moves once); explore agent runs on the cheaper tier; `requirements.toml` refuses a disallowed mode. |
| T7.4 | `MODEL_COMPATIBILITY.md` exists; PARITY "intentionally shallow" section present; README EN/zh structures match. |

---

## 8. Sequencing & risk

### Recommended order (highest leverage first)
1. **T6.3 mock-DeepSeek harness** — the prerequisite multiplier. Land it first so every subsequent stage in T1/T2/T4 has deterministic, offline, end-to-end loop coverage instead of low-level stubs.
2. **T1 stability** (T1.1 → T1.2 → T1.3 → T1.4) — partial persistence and cancel-synthesis are correctness foundations the whole loop rests on, and the 1M window makes them the highest-leverage V4-specific hardening.
3. **T5.1 key-flow harness** — cheap, blocks all later TUI work, protects the most fragile invariant.
4. **T3.1 + T3.2 + T3.3** — symlink-gate convergence, freshness guard, and snapshot durability are low-risk, high-value security/data-loss fixes that need no new UX.
5. **T4** — token reconciliation and cache-preserving compaction unlock the budget-as-feature and protect the 50× discount across compaction; depends on usage parsing already present.
6. **T2** — repair/escalation/MCP-liveness deepen autonomy once the loop is recoverable and observable.
7. **T3.4 sandbox-on-by-default** — deliberately late: highest UX risk, best done after the harness and behavioral tests exist.
8. **T6.1/T6.2, T7** — schema consolidation, dead-code removal, and config/docs polish are independent and can interleave throughout.

### Highest-risk invariants and their guardrails

| Risk | Guardrail |
|---|---|
| **Cache-stability / byte-determinism** (T2.3 escalation contract, T4.3 compaction shape, T6.1 schema). Any non-deterministic ordering upstream of `MarshalCacheStable` silently kills the 50× discount. | `TestCacheStableGolden` + `TestFingerprintTracksWireStaticHead` must be unchanged by every PR; benchrunner asserts compaction prefix before==after; the registry golden test (§4) pins tool-listing bytes; model-name is the only allowed interpolant in the prefix region and must feed the fingerprint. |
| **Thinking-struct contract** (T1.1/T1.2 partial + replay). A bare assistant turn or unpaired tool_call 400s on V4. | Every persisted/synthesized/replayed assistant message passes `SanitizeForDeepSeek`; T1.2 adds load-time stamping/pairing; the `thinking_shape_test.go` regression test stays green; validate replay against the T6.3 mock. |
| **TUI key-flow** (`intercepted` contract; never plain-letter; reasoning fold is ctrl+r/ctrl+t). | T5.1 lands before any other T5 stage; every new binding asserts `(cmd, true)` and uses ctrl-modified or overlay-local keys; golden-per-width tests catch layout regressions. |
| **Sandbox over-restriction** (T3.4 breaks "my `$PATH` works"). | macOS-first, degrade to no-op honestly elsewhere; gate on a behavioral test that a denied write surfaces through the agent path and that real tools (npx MCP) still start; keep the policy-text/param-table sync invariant pinned by tests. |
| **Single-binary / no-CGO / no-LLM-SDK.** | No tokenizer blob (T4.1 uses a CJK-weighted heuristic); no external LLM SDK (`internal/llm` stays hand-rolled); SQLite stays `modernc.org/sqlite`; diff/highlight use small or existing deps only; `internal/` tests stay inside their package. |
| **Epoch lifecycle correctness** (T7.3 reload, T2.4 MCP, T3.5 undo). | Capability-Set changes never move the fingerprint mid-epoch; reload/MCP/profile changes route through `SwitchEpoch`/`CompareToolLists`; `DetectDrift` reports pending changes, a frozen epoch is never mutated in place (per ADR-0001 / `CONTEXT.md`). |
