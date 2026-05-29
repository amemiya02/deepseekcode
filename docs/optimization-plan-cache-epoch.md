# Cache Epoch Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `deepseekcode` provably stronger than DeepSeek-Reasonix on
cache and context cost reliability while preserving agentic engineering
behavior.

**Architecture:** Introduce an epoch-governed static prefix, a black-box
DeepSeek agent benchmark, and cache-aligned semantic compaction. The first
phase deliberately avoids broad feature expansion: the system must become
measurable before it becomes larger.

**Tech Stack:** Go, DeepSeek V4 request/usage fields, SQLite sessions, existing
agent event bus, external CLI benchmark harness, fixture git repositories.

---

## Status

This document captures the accepted design decisions from the cache/context
optimization review on 2026-05-27.

The north star is:

- Cache and context cost reliability first.
- Agentic engineering remains intact.
- DeepSeek cache hit rate must be extremely high.
- Improvements must be measured against both current `deepseekcode` and
  DeepSeek-Reasonix.

## Implementation Status (updated 2026-05-27, post cache-epoch review)

The review flagged that the Phase-1 code skeleton was in place but the gate
was not yet provable. The instrumentation gap is now closed; the remaining
gap is the billed comparison run itself.

**Done and provable**

- **Real cache gate (was P0-1).** `dsc -p --trace-jsonl <path>` (or
  `DEEPSEEKCODE_TRACE_JSONL`) emits a real JSONL trace of
  `prefix.snapshot` / `epoch.frozen` / `pending_change` / `drift.blocked` /
  `usage` / `compaction` records. `benchrunner` reads that trace and computes
  the Cache Reliability gate from it (within-epoch prefix stability, drift
  count, compaction stability, post-warm hit rate). Verified on a live smoke
  run (real `cache_hit_tokens`/`cache_miss_tokens` from the API).
- **Gate fails closed on malformed/incomplete trace.** `parseAgentTrace`
  counts parse errors, `usage` with no `epoch_id`, `prefix.snapshot` missing a
  hash, `usage` with no backing snapshot, and `compaction` missing a
  before/after hash; any non-zero counter fails an enforced gate (tests:
  `TestCacheGate_MalformedJSONLFailsClosed`,
  `TestCacheGate_UsageWithoutSnapshotFailsClosed`,
  `TestCacheGate_EmptyEpochUsageFailsClosed`).
- **Skill directory in the model-visible prefix (was P0).** A single
  canonical `skills.Store` (`skills.LoadScan`) feeds the prompt's stable skill
  directory (`PromptIndex` → `IndexText`), the epoch skill-dir hash
  (`VersionHash`), and the `skill_read` body source — one loader, so
  model-visible skills and the cache-epoch hash cannot diverge. The directory
  `name | short_description | run_mode | version_hash | allowed_tools` is
  rendered into `SystemPromptBuilder` **before the dynamic boundary**, so a
  body-only edit moves the actual request bytes, not just an internal hash
  (test: `prompt.TestBuildPrefixReflectsSkillBodyEdit`). No local absolute
  paths or bodies enter the prefix; bodies load on demand via `skill_read`.
  The old `prompt.LoadSkills` dual loader was deleted, and its full parsing
  semantics were migrated into `skills.parseSkillFile` so nothing the prompt
  loader accepted is dropped: heading-only `SKILL.md` files (no frontmatter,
  first `# Heading` is the name), quoted frontmatter values, a colon inside a
  description, and **case-insensitive** frontmatter keys (`Name:` /
  `Description:` / `Allowed-Tools:`) all still parse, and heading-only skills
  stay readable via `skill_read` (tests: `skills.TestParseSkillFile_Semantics`,
  `skills.TestParseSkillFile_CapitalizedAllowedTools`,
  `skills.TestHeadingOnlySkillReadable`). Skill discovery is **session-start
  only** by design — the Store is loaded once and not re-read per turn, so a
  mid-session `SKILL.md` edit does not move the frozen prefix (it takes effect
  on the next session); this matches how the system prompt and tool set are
  frozen, and is documented on `skills.LoadScan`.
- **Compaction stability is measured, not asserted.** The agent computes the
  static-prefix fingerprint of the frozen baseline and of the request
  compaction actually fed the model, and emits both as
  `before_static_prefix_hash`/`after_static_prefix_hash`. The harness compares
  them — the hardcoded `static_prefix_hash_changed=false` is gone (tests:
  `TestCacheGate_CompactionMovedPrefixFails`,
  `TestCacheGate_CompactionMissingHashFails`). **Both** compaction paths emit a
  measured record now: the deterministic fallback also publishes
  `EventSemanticCompaction` with before/after hashes (`UsedSemantic=false`,
  before == after since it never rebuilds the prefix), so a compaction-required
  task always sees a record when compaction actually ran (test:
  `TestDeterministicCompactionEmitsMeasuredRecord`). A task can set
  `require_compaction_record: true` (enabled on `ctx-compaction`) to **fail
  closed** when no compaction record exists at all — the "Compaction" dimension
  can no longer pass on the absence of evidence (tests:
  `TestCacheGate_RequireCompactionRecordMissingFails`,
  `TestCacheGate_RequireCompactionRecordPresentPasses`).
- **Post-warm minimum.** A cache-gated task must produce at least
  `min_post_warm_turns` warm turns (default 1); one that never measured a warm
  turn fails instead of passing on N/A (test:
  `TestCacheGate_InsufficientPostWarmFails`).
- **MCP + AgentProfile runtime (was P1-4).** Both the TUI and one-shot paths
  set `a.MCPRegistry` (one-shot now also connects MCP), so startup MCP schema
  feeds `mcp_schema_hash`. The epoch's `agent_profile_hash` comes from the
  active profile name, and `Agent.SwitchProfile` creates/switches an epoch and
  records one expected cache miss (test:
  `TestSwitchProfileCreatesEpochAndExpectsCacheMiss`).
- **Parent/subagent isolation evidence (was P1).** Subagent runs (in-process
  via `LoopSpawner`) now tee their own epoch/usage/compaction events into the
  parent's trace through `Agent.AttachChildTraceSink`, stamped
  `agent_role="subagent"` and `parent_epoch_id=<parent epoch>`, over a shared
  synchronized writer. The child's own `EpochManager` mints a distinct
  `epoch_id`, so the harness can prove a child did not reuse the parent epoch
  rather than hardcoding it. The harness validates the parent link itself: a
  child epoch that carries **no** `parent_epoch_id` (`ChildMissingParent`) or
  one pointing at an epoch the root never emitted (`ChildUnknownParent`) fails
  the dimension — a `subagent` record cannot pass just by existing. Under
  `require_subagent_isolation` the child trace must also be **complete**, and
  completeness is judged **per child epoch**, never in aggregate: a single
  `epoch_id` only counts when its own records show a valid parent link, at least
  one `usage` turn, AND an `agent.done` terminator (every `EventDone` now emits
  an `agent.done` record stamped with its `epoch_id`). The gate requires **≥1
  complete child epoch and 0 incomplete child epochs** — so a two-child trace
  where `c1` has usage-but-no-done and `c2` has done-but-no-usage fails, even
  though the aggregate "some child had usage" and "some child finished" both
  look satisfied. A child that only emitted a `prefix.snapshot` is partial
  evidence and fails. The dimension is evaluated
  **only when a child epoch is present** (otherwise `N/A`, never a hardcoded
  ✅), and `require_subagent_isolation: true` (`subagent-parallel`) **fails
  closed** with no child trace. Async subagents are not lost at exit: the parent
  tracks child trace handles and the one-shot CLI calls `Agent.WaitChildTraces`
  to flush in-flight child traces before closing the root trace; if a child
  handle times out (never reached `EventDone`) a `child_trace_incomplete` marker
  is written so the gate fails closed rather than trusting a cut-off child
  (tests: `TestE2ESubagentChildTrace`, `TestWaitChildTracesFlushesAsyncChild`,
  `TestWaitChildTracesTimeoutEmitsIncomplete`,
  `TestCacheGate_SubagentIsolationRequiresChildTrace`,
  `TestCacheGate_ChildEpochEvaluated`, `TestCacheGate_PartialChildTraceFails`,
  `TestCacheGate_PerChildCompletenessFails`,
  `TestCacheGate_ChildTraceIncompleteFails`,
  `TestCacheGate_ChildMissingParentFails`,
  `TestCacheGate_ChildUnknownParentFails`).

- Agent frontmatter now parses hidden, max_steps, permission_ruleset,
  temperature, top_p, and default_agent.
- Canonical event names defined in `internal/eventschema`. Downstream consumers
  (CLI, dashboards) should import these constants instead of hard-coding strings.

**Blocked / not yet done**

- **M1 / M5 real reports + Reasonix comparison (P0-2).** The harness can now
  run `current + optimized + reasonix` and produce a real gate, but a genuine
  billed matrix run has not been executed here, and a token-level Reasonix
  cache comparison needs a Reasonix-`--transcript` parser. Until that run is
  recorded, **M1 and M5 stay open** and the Reasonix dimension of the Phase-1
  Definition of Done is **blocked** — it must not be reported as complete.

## Decisions

The following decisions are locked for phase 1:

1. Same-session model-visible prefix is frozen after the first model request.
2. Any system, tool, skill, MCP, few-shot, or agent profile change creates or
   waits for a new `PrefixEpoch`.
3. Skill bodies are lazy-loaded; only a stable short skill directory enters the
   prefix.
4. MCP schema changes do not hot-replace live tool definitions. They enter a
   pending pool until the next epoch.
5. Tools are split into core tools, profile tools, and lazy capability tools.
6. Agent profiles are first-class runtime units, not just prompt fragments.
7. Cache Reliability Score is a hard gate before Agentic Engineering Score is
   considered.
8. `deepseek-agent-bench` is a black-box CLI harness first, with internal unit
   tests as a supplement.
9. Benchmark tasks use fixed fixture repositories and frozen commits.
10. Phase 1 scope is only:
    - `deepseek-agent-bench`
    - `PrefixEpoch`
    - semantic compaction
11. Delivery order is:
    - bench baseline
    - `PrefixEpoch`
    - semantic compaction

## Non-Goals

Phase 1 does not add:

- VS Code or desktop integrations.
- Broad provider expansion.
- New MCP transports.
- A large agent marketplace.
- A general plugin system.
- A full semantic vector index.
- TUI redesign beyond minimal cache/context status needed for receipts.

Those can follow only after the cache gate is measurable and stable.

## Reasonix Lessons To Adopt

DeepSeek-Reasonix gets several DeepSeek-native invariants right:

- Explicit immutable prefix.
- Append-only conversation log with a single compaction escape hatch.
- Cache-aligned semantic fold that reuses the live system prompt and tools.
- Flash model plus thinking disabled for fold summaries.
- Pinned skill memos across compaction.
- Subagents as isolated child loops that return distilled results.
- Typed events carrying prefix hash, usage, tool lifecycle, and compaction.

`deepseekcode` should go further by making prefix stability an epoch-level
runtime contract, not only a fingerprint check.

## Target Model

The target runtime shape is:

```text
AgentProfile
  -> PrefixEpoch
  -> ToolTier snapshot
  -> SkillDirectory snapshot
  -> MCPSchema snapshot
  -> cache-stable DeepSeek request
  -> receipts and benchmark trace
```

The prefix is no longer "whatever system and tools happen to be at request
time." It is a named immutable epoch.

## Core Concept: PrefixEpoch

> **Superseded (2026-05-29, M3 / ADR-0001):** the 6-component combined hash
> described below was replaced. `PrefixEpoch.StaticPrefixHash` is now the
> **Prefix Fingerprint** — the canonical hash of the *model-visible bytes only*
> (system + tools), equal to the DeepSeek cache key by construction. Latent
> identity (agent profile, skills, MCP) is tracked separately as a
> `CapabilitySet` and is **not** hashed into the prefix; pending changes come
> from `CapabilityDiff`. See
> `docs/adr/0001-prefix-fingerprint-is-model-visible-bytes-only.md` and
> `/CONTEXT.md`. The component list below is kept for historical context.

`PrefixEpoch` is a frozen model-visible prefix snapshot.

It owns:

- `epoch_id`
- `agent_profile_id`
- `model`
- `reasoning_effort`
- `static_system`
- `few_shots`
- `tool_specs`
- `stable_skill_directory`
- `mcp_schema_snapshot`
- `created_at`
- `created_reason`
- component hashes
- combined static prefix hash

The combined static prefix hash is derived from canonical serialized bytes of:

```text
static_system_hash
few_shots_hash
tools_hash
skill_directory_hash
mcp_schema_hash
agent_profile_hash
```

The existing `internal/llm/prefix_drift.go` already computes system and tool
fingerprints. `PrefixEpoch` should build on that instead of replacing it.

### Lifecycle

1. Session starts and builds `PrefixEpoch#1`.
2. First model request using that epoch freezes it.
3. Later capability changes are recorded as pending changes.
4. The live epoch remains unchanged.
5. User or runtime policy explicitly creates `PrefixEpoch#2`.
6. The first turn of a new epoch is expected to miss cache.
7. Subsequent turns in the same epoch must not drift.

### Pending Change Kinds

Supported pending changes:

- `system_changed`
- `tool_added`
- `tool_removed`
- `tool_schema_changed`
- `skill_added`
- `skill_removed`
- `skill_body_changed`
- `mcp_tool_added`
- `mcp_tool_removed`
- `mcp_tool_schema_changed`
- `agent_profile_changed`
- `few_shots_changed`

Pending changes are visible in receipts and status, but not model-visible until
epoch switch.

### Epoch Events

Add typed events after the core shape is stable:

- `prefix.epoch.created`
- `prefix.epoch.frozen`
- `prefix.pending_change`
- `prefix.epoch.switched`
- `prefix.drift.blocked`
- `prefix.drift.detected`

The first implementation can publish them through the existing event bus with a
new event type. Avoid hiding them as generic info strings once the structure is
known.

## Skill Design

Skills must support agentic behavior without bloating the stable prefix.

### Stable Skill Directory

The prefix contains only a deterministic directory:

```text
name
short_description
run_mode
version_hash
allowed_tool_names
```

Rules:

- Sort by skill name.
- Keep descriptions short.
- Do not include full skill body.
- Do not include timestamps or local absolute paths.
- Body changes update `version_hash` and create a pending change.

### Lazy Skill Body

Full bodies are loaded only when needed:

- inline through a stable `skill_read`-style tool, or
- by creating a subagent prefix from the skill body.

Once a skill affects the task, compaction must preserve its active constraints
as pinned conversation facts.

## MCP Design

MCP is cache-hostile because tool schemas can change while the session is live.
Phase 1 makes MCP schema changes epoch-governed.

Rules:

- Startup MCP discovery contributes to `mcp_schema_hash`.
- Same-name same-schema reconnect does not change the live epoch.
- Added, removed, or structurally changed MCP tools enter the pending pool.
- Live tools are not replaced mid-epoch.
- Subagents may have their own MCP epoch, but cannot mutate the parent epoch.

## Tool Tiers

Tool exposure is explicit and profile-driven.

### Core Tools

Core tools are small, stable, and present in the default coding profile.

Examples:

- file read
- file search
- literal content search
- edit/apply patch
- bash/run command
- todo
- plan/ask user when available

### Profile Tools

Profile tools enter only through an `AgentProfile` epoch.

Examples:

- semantic search
- git inspection tools
- web fetch
- MCP tools
- subagent spawning
- verification helpers

### Lazy Capability Tools

Lazy tools are stable dispatchers. They must not become a hidden universal
escape hatch.

Examples:

- `skill_read`
- `capability_request`

The dispatcher schema must be small and stable, and every dynamic result must
be recorded as conversation state rather than silently changing prefix.

## AgentProfile

An `AgentProfile` is a first-class runtime unit.

Fields:

```yaml
name: coding-default
model: deepseek-v4-flash
reasoning_effort: high
system_overlay: ""
tool_tier: core
allowed_tools: []
permission_policy: default
context_policy: cache-reliable
compaction_policy: semantic-safe
subagent_policy: isolated
cache_policy: strict-epoch
```

Initial profiles:

| Profile | Purpose | Tool Tier |
|---------|---------|-----------|
| `coding-default` | Normal implementation | core |
| `explore` | Broad read-only investigation | core + search + semantic |
| `implement` | Edits, tests, verification | core + git + verification |
| `review` | Read-only bug/risk review | read + git |
| `autonomous` | Longer agentic workflows | core + subagent + todo |

Profile switching creates or reuses a matching `PrefixEpoch`.

## Semantic Compaction

Semantic compaction is allowed, but it is controlled and observable.

### Thresholds

| Context Ratio | Behavior |
|---------------|----------|
| 75% | warn and prepare pinned facts |
| 80% | automatic semantic compaction |
| 90% | protection mode; preserve task continuity over full history |

### Summary Request

The semantic summary request must:

- use `deepseek-v4-flash`
- disable thinking
- use a 15 second timeout
- reuse the same static system prefix
- reuse the same tool list when tools are included
- preserve pinned skills and constraints
- preserve current objective
- preserve negative constraints
- preserve changed file paths
- preserve recent tool evidence
- record its own usage and cost

### Fallback

If the summary request fails, times out, or produces an empty summary:

1. Fall back to deterministic local compaction.
2. Emit a compaction receipt.
3. Continue the task.

The fallback path must not change `static_prefix_hash`.

### Summary Placement

The summary is conversation state, not static prefix.

It may be a synthetic system-style message if that is compatible with the
current DeepSeek message rules, but receipts must distinguish:

- `static_prefix_hash`
- `session_summary_hash`
- `tools_hash`

Compaction must not be reported as static prefix drift.

## Benchmark Harness

`deepseek-agent-bench` is the first deliverable.

It compares:

- `deepseekcode/current`
- `deepseekcode/optimized`
- `reasonix/current`

It is black-box first: each agent runs through its CLI adapter. Internal Go
tests still cover pure logic, but the benchmark is the final phase 1 gate.

### Directory Layout

```text
bench/
  agents/
    deepseekcode-current.yaml
    deepseekcode-optimized.yaml
    reasonix-current.yaml
  tasks/
    ctx-long-readonly.yaml
    ctx-compaction.yaml
    impl-small-safe.yaml
    impl-with-tests.yaml
    review-negative-claims.yaml
    skill-body-change.yaml
    mcp-schema-drift.yaml
    subagent-parallel.yaml
  fixtures/
  traces/
  reports/
```

The benchmark may live under `bench/` at the repository root. It should not be
placed under `internal/` because it must run non-Go agents.

### Agent Adapter

Each adapter defines:

```yaml
id: deepseekcode-current
command: ./bin/dsc
args:
  - -p
input_mode: prompt_arg
env:
  DEEPSEEK_API_KEY: required
trace_path: .bench/out/deepseekcode-current/trace.jsonl
usage_parser: deepseekcode
```

The adapter is responsible for:

- launching the agent
- feeding the prompt
- setting working directory
- collecting output
- collecting trace/usage receipts when supported
- enforcing timeout

### Task Spec

Each task defines:

```yaml
id: ctx-compaction
fixture_repo: fixtures/ctx-compaction
commit: "<frozen-commit-sha>"
prompt_file: prompts/ctx-compaction.md
timeout_seconds: 900
success:
  tests:
    - go test ./...
  diff_invariants:
    - no_changes_outside:
        - .
metrics:
  require_cache_gate: true
```

Every task starts from a clean worktree at the frozen commit.

### Trace Schema

The harness normalizes agent output into JSONL records.

Minimum record types:

```json
{"type":"run.started","agent":"deepseekcode-current","task":"ctx-compaction"}
{"type":"prefix.snapshot","run_id":"run-1","agent_role":"root","epoch_id":"epoch-1","static_prefix_hash":"...","tools_hash":"..."}
{"type":"usage","run_id":"run-1","agent_role":"root","turn":1,"epoch_id":"epoch-1","cache_hit_tokens":0,"cache_miss_tokens":12000,"output_tokens":800,"cost_cny":0.0123}
{"type":"compaction","run_id":"run-1","agent_role":"root","epoch_id":"epoch-1","kind":"semantic","before_static_prefix_hash":"...","after_static_prefix_hash":"..."}
{"type":"run.finished","success":true}
```

Compaction stability is measured by comparing `before_static_prefix_hash`
and `after_static_prefix_hash` (no hardcoded boolean). `run_id`/`agent_role`/
`parent_epoch_id` attribute each record to the root or a subagent epoch.
Agents that cannot emit a field get `null`, but for an **enforced** gate a
missing or malformed field fails the gate closed — it is not a silent
limitation.

## Scoring

The benchmark produces two scores.

### Cache Reliability Score

This is a gate. If it fails, the run fails regardless of agentic score.

| Check | Required |
|-------|----------|
| Static prefix hash stable within an epoch | 100% |
| Unauthorized drift | 0 |
| Live tool/schema drift | 0 |
| Compaction changes static prefix | 0 |
| Skill or MCP mutation pollutes live prefix | 0 |
| Parent/subagent cache pollution | 0 |
| Post-warm DeepSeek cache hit rate | >= 95% |

Post-warm hit rate excludes the first model request of each epoch.

### Agentic Engineering Score

This score is considered only after the cache gate passes.

Inputs:

- task success rate
- tests pass rate
- diff correctness
- negative claim evidence
- tool efficiency
- subagent usefulness
- recovery from tool errors
- cost per successful task
- replay/debug quality

The phase 1 goal is not to maximize every agentic metric. It is to avoid
regressing them while cache reliability improves.

## Implementation Milestones

### M0: Black-Box Bench Baseline

**Files:**

- Create: `bench/README.md`
- Create: `bench/agents/deepseekcode-current.yaml`
- Create: `bench/agents/reasonix-current.yaml`
- Create: `bench/tasks/ctx-long-readonly.yaml`
- Create: `bench/tasks/ctx-compaction.yaml`
- Create: `bench/tasks/review-negative-claims.yaml`
- Create: `bench/tasks/skill-body-change.yaml`
- Create: `bench/tasks/mcp-schema-drift.yaml`
- Create: `bench/tasks/subagent-parallel.yaml`
- Create: benchmark runner under an implementation path chosen during M0.

**Acceptance:**

- [ ] The harness can run at least one read-only fixture against
      `deepseekcode/current`.
- [ ] The harness can run the same fixture against `reasonix/current`.
- [ ] Each run produces normalized JSONL.
- [ ] Each run produces a summary report.
- [ ] Missing cache fields are visible in the report instead of ignored.
- [ ] Fixture repos are reset to frozen commits before every run.

### M1: Baseline Report

**Files:**

- Create: `bench/reports/baseline-YYYY-MM-DD.md`
- Modify: `docs/optimization-plan-cache-epoch.md` only if the benchmark exposes
  an invalid assumption.

**Acceptance:**

- [ ] Current `deepseekcode` baseline is recorded.
- [ ] Current Reasonix baseline is recorded.
- [ ] Cache Reliability Score is computed for both.
- [ ] Agentic Engineering Score is computed for both.
- [ ] The report identifies which missing instrumentation must be added before
      optimized runs are judged final.

### M2: PrefixEpoch Core

**Files:**

- Create: `internal/agent/prefix_epoch.go`
- Create: `internal/agent/prefix_epoch_test.go`
- Modify: `internal/agent/agent.go`
- Modify: `internal/llm/prefix_drift.go`
- Modify: `internal/session` receipt persistence as needed.

**Acceptance:**

- [ ] First model request freezes the epoch.
- [ ] Same-epoch turns keep the same static prefix hash.
- [ ] Tool schema changes after freeze become pending changes.
- [ ] Skill directory changes after freeze become pending changes.
- [ ] MCP schema changes after freeze become pending changes.
- [ ] Explicit epoch switch records expected cache miss.
- [ ] Unauthorized drift emits a structured failure/receipt.
- [ ] Existing `MarshalCacheStable` tests still pass.
- [ ] New tests prove `static_prefix_hash` does not change during compaction.

### M3: Skill, MCP, And Tool Tier Integration

**Files:**

- Modify: `internal/skills`
- Modify: `internal/mcp`
- Modify: `internal/tools`
- Modify: `internal/agents`
- Create focused tests in the touched packages.

**Acceptance:**

- [ ] Stable skill directory is deterministic.
- [ ] Skill body edits do not alter the live epoch.
- [ ] MCP reconnect with same schema does not alter the live epoch.
- [ ] MCP same-name schema drift enters pending pool.
- [ ] Default profile exposes only core tools.
- [ ] Profile switch creates or reuses a matching epoch.

### M4: Semantic Compaction

**Files:**

- Modify: `internal/agent/compact.go`
- Modify: `internal/agent/compact_summary.go`
- Modify: `internal/agent/agent.go`
- Create: `internal/agent/semantic_compact.go`
- Create: `internal/agent/semantic_compact_test.go`

**Acceptance:**

- [ ] 75% context pressure emits warning/preparation receipt.
- [ ] 80% context pressure attempts semantic compaction.
- [ ] 90% context pressure enters protection mode.
- [ ] Semantic summary uses Flash.
- [ ] Semantic summary disables thinking.
- [ ] Semantic summary has a 15 second timeout.
- [ ] Semantic summary cost is recorded separately.
- [ ] Summary preserves pinned skill facts and negative constraints.
- [ ] Summary preserves current task and relevant file paths.
- [ ] Failure falls back to deterministic compaction.
- [ ] Static prefix hash is unchanged after semantic or deterministic
      compaction.

### M5: Optimized Benchmark Run

**Files:**

- Create: `bench/agents/deepseekcode-optimized.yaml`
- Create: `bench/reports/optimized-YYYY-MM-DD.md`

**Acceptance:**

- [ ] Optimized `deepseekcode` passes the Cache Reliability gate.
- [ ] Post-warm DeepSeek cache hit rate is at least 95% on eligible tasks.
- [ ] Unauthorized drift count is zero.
- [ ] Parent/subagent cache pollution count is zero.
- [ ] Compaction does not change static prefix hash.
- [ ] Optimized cost is lower than current baseline or task success is higher.
- [ ] Against Reasonix, optimized `deepseekcode` is not worse on the cache gate
      and wins at least one of cost, trace quality, or debug quality.

## Internal Test Matrix

Add unit tests for pure logic before relying on the black-box benchmark.

| Area | Test |
|------|------|
| Prefix hashing | Same canonical inputs produce same hash |
| Prefix freeze | Mutations after freeze become pending changes |
| Tool schema | Field order does not change hash |
| Tool schema | Description/schema change changes hash |
| Skill directory | Body change updates version hash but not live epoch |
| MCP snapshot | Same schema reconnect is stable |
| MCP snapshot | Same-name schema drift is pending |
| Compaction | Summary message changes conversation hash only |
| Compaction | Static prefix hash remains stable |
| Subagent | Child epoch does not mutate parent epoch |

## Benchmark Task Matrix

| Task | Purpose | Primary Gate |
|------|---------|--------------|
| `ctx-long-readonly` | Large read-only code understanding | cache hit and tool efficiency |
| `ctx-compaction` | Force 75/80/90 context thresholds | compaction safety |
| `impl-small-safe` | Small implementation with tests | agentic success |
| `impl-with-tests` | Edit plus verification | success per cost |
| `review-negative-claims` | Require evidence for absence claims | review quality |
| `skill-body-change` | Mutate skill body mid-session | epoch isolation |
| `mcp-schema-drift` | Change MCP tool schema mid-session | pending MCP pool |
| `subagent-parallel` | Parallel investigation | parent/child cache isolation |

## Risks

### Risk: Epoch Governance Feels Less Dynamic

Users may expect new tools or MCP servers to become immediately available.

Mitigation:

- Show pending capability changes clearly.
- Provide an explicit epoch switch command.
- Explain that the switch will cause one expected cache miss.

### Risk: Semantic Compaction Adds Cost

A summary model call can cost more than deterministic compaction.

Mitigation:

- Keep deterministic fallback.
- Use Flash.
- Disable thinking.
- Timeout at 15 seconds.
- Record summary cost separately.
- Let benchmark decide whether it is worth enabling by default.

### Risk: Bench Harness Becomes Too Heavy

A perfect benchmark can delay implementation.

Mitigation:

- M0 needs only a minimal black-box runner and three tasks.
- Add the full task matrix incrementally.
- Do not block PrefixEpoch unit tests on every fixture.

### Risk: Reasonix Comparison Is Noisy

Reasonix may expose different trace fields or CLI behavior.

Mitigation:

- Normalize only shared fields.
- Mark missing fields as missing instrumentation.
- Compare cache/cost only where DeepSeek usage fields are available.
- Keep raw logs for manual audit.

## Definition Of Done For Phase 1

Phase 1 is done only when:

- `deepseek-agent-bench` runs current, optimized, and Reasonix adapters.
- Optimized `deepseekcode` passes the Cache Reliability gate.
- Post-warm cache hit rate is at least 95% on eligible tasks.
- Unauthorized drift is zero.
- Semantic compaction does not change static prefix hash.
- Benchmark report shows optimized `deepseekcode` is not worse than Reasonix on
  cache reliability.
- At least one measurable area beats Reasonix: cost, trace/debug quality, or
  parent/subagent cache isolation.

## First Execution Step

Start with M0. Do not implement `PrefixEpoch` before the benchmark can produce
a current baseline. Without the baseline, the team cannot prove that the new
runtime actually improves over current `deepseekcode` or Reasonix.
