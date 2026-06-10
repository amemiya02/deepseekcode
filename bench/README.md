# Black-Box Benchmark Harness

A structured benchmark harness for comparing coding agents on standardized tasks.

## Overview

This harness runs coding agents against a set of tasks and produces:
- **JSONL traces**: One JSON object per line for each run, suitable for aggregation
- **Markdown reports**: Human-readable summary with per-agent, per-task results

## Directory Layout

```
bench/
  cmd/benchrunner/    # Go benchmark runner
  agents/             # Agent adapter YAML configs
  tasks/              # Task YAML specs
  fixtures/           # Fixture repos (git repos at frozen commits)
  prompts/            # Prompt files for tasks
  traces/             # Output: JSONL traces (gitignored)
  reports/            # Output: Markdown reports (gitignored)
```

## Quick Start

### Prerequisites

- Go 1.22+
- Agent binary built (`make build` for deepseekcode)
- API keys set in environment

### Run All Benchmarks

```bash
# Build the runner
go build ./bench/cmd/benchrunner/

# Run all agents against all tasks
./benchrunner

# Or run directly
go run ./bench/cmd/benchrunner/
```

### Filter by Agent or Task

```bash
# Run only deepseekcode-current
go run ./bench/cmd/benchrunner/ --agent deepseekcode-current

# Run only ctx-long-readonly task
go run ./bench/cmd/benchrunner/ --task ctx-long-readonly

# Run specific combination
go run ./bench/cmd/benchrunner/ --agent deepseekcode-current --task ctx-long-readonly
```

### Dry Run

See what would run without executing:

```bash
go run ./bench/cmd/benchrunner/ --dry-run
```

## Agent Adapter YAML Format

Each agent is defined in `bench/agents/*.yaml`:

```yaml
id: deepseekcode-current
command: ./bin/dsc
args:
  - -p
input_mode: prompt_arg    # "prompt_arg" or "stdin"
env:
  DEEPSEEK_API_KEY: required  # "required" means check env, or literal value
trace_path: bench/traces/deepseekcode-current/
usage_parser: deepseekcode
```

### Fields

- **id**: Unique agent identifier
- **command**: Path to agent binary
- **args**: Command-line arguments (prompt appended for prompt_arg mode)
- **input_mode**: How to pass the prompt
  - `prompt_arg`: Append prompt as final argument
  - `stdin`: Write prompt to stdin
- **env**: Environment variables (key: value, or "required" to inherit from env)
- **trace_path**: Directory for JSONL traces
- **usage_parser**: Agent name for usage parsing

## Task YAML Format

Each task is defined in `bench/tasks/*.yaml`:

```yaml
id: ctx-long-readonly
fixture_repo: fixtures/ctx-long-readonly
commit: HEAD
prompt_file: prompts/ctx-long-readonly.md
timeout_seconds: 300
read_only: true
success:
  tests: []  # no tests for read-only
  diff_invariants:
    - no_changes: true
metrics:
  require_cache_gate: true
```

### Fields

- **id**: Unique task identifier
- **fixture_repo**: Path to fixture (relative to bench dir). Can be a git repo or plain directory.
- **commit**: Git commit to reset to (HEAD, specific hash, etc.). Ignored for plain directories.
- **prompt_file**: Path to prompt markdown file (relative to bench dir)
- **timeout_seconds**: Maximum execution time (default: 300)
- **read_only**: If true, pass `--read-only` to the agent (default: false)
- **success.tests**: Test commands to run after the agent exits
- **success.diff_invariants**: Constraints on file changes
  - `no_changes: true` — no file modifications allowed (read-only tasks)
  - `no_changes_outside: [path]` — only changes under the listed paths are allowed
- **metrics.require_cache_gate**: Whether the Cache Reliability gate is enforced for this task
- **metrics.min_post_warm_turns**: Minimum warm (non-cold-start) turns required before the post-warm hit rate is trusted (default 1 for cache-gated tasks)
- **metrics.require_subagent_isolation**: Fail closed unless the trace contains a child (subagent) epoch to judge parent/subagent isolation
- **metrics.require_compaction_record**: Fail closed unless the trace contains at least one compaction record (with before/after prefix hashes)

## Output Format

### JSONL Traces

Each run produces a `.jsonl` file with one JSON object per line:

```json
{"type":"run.started","agent":"deepseekcode-current","task":"ctx-long-readonly","timestamp":"2026-05-27T10:00:00Z"}
{"type":"turn.started","turn":1}
{"type":"usage","turn":1,"cache_hit_tokens":0,"cache_miss_tokens":12000,"output_tokens":800,"cost_cny":0.0123}
{"type":"run.finished","success":true,"duration_ms":12345}
```

### Record Types

- **run.started**: Agent and task identifiers, timestamp
- **usage**: Token counts and cost for the turn
- **run.finished**: Success/failure, duration, exit code, line counts, errors

The file above (`<task>.jsonl`) is the harness's run-level summary. The
**authoritative cache evidence** is a separate per-run file the agent itself
writes — see below.

### Agent trace (`<task>.agent.jsonl`) — source of truth for the gate

deepseekcode agents emit a real epoch/usage/compaction/drift trace via
`dsc -p --trace-jsonl <path>` (the harness sets `DEEPSEEKCODE_TRACE_JSONL`
per run). The Cache Reliability gate is computed entirely from this trace —
there are no fabricated placeholders. Record types:

Every record carries `run_id` and `agent_role` (`"root"` for the top-level
agent; a subagent stamps `"subagent"` plus the `parent_epoch_id` it ran
under) so the harness can attribute each epoch to the root or a child agent.

```json
{"type":"prefix.snapshot","run_id":"run_…","agent_role":"root","epoch_id":"epoch_…","static_prefix_hash":"…","tools_hash":"…","reason":"session_start"}
{"type":"epoch.frozen","run_id":"run_…","agent_role":"root","epoch_id":"epoch_…"}
{"type":"prefix.snapshot","run_id":"run_…","agent_role":"root","turn":1,"epoch_id":"epoch_…","static_prefix_hash":"…","tools_hash":"…"}
{"type":"usage","run_id":"run_…","agent_role":"root","turn":1,"epoch_id":"epoch_…","cache_hit_tokens":1152,"cache_miss_tokens":2951,"output_tokens":15,"cost_cny":0.0030}
{"type":"pending_change","run_id":"run_…","agent_role":"root","epoch_id":"epoch_…","kind":"skill_body_changed","description":"…"}
{"type":"drift.blocked","run_id":"run_…","agent_role":"root","epoch_id":"epoch_…","which":"tools"}
{"type":"compaction","run_id":"run_…","agent_role":"root","epoch_id":"epoch_…","kind":"semantic","before_static_prefix_hash":"…","after_static_prefix_hash":"…"}
{"type":"prefix.snapshot","run_id":"run_…","agent_role":"subagent","parent_epoch_id":"epoch_…","epoch_id":"epoch_child_…","static_prefix_hash":"…","tools_hash":"…"}
{"type":"usage","run_id":"run_…","agent_role":"subagent","parent_epoch_id":"epoch_…","epoch_id":"epoch_child_…","turn":1,"cache_hit_tokens":0,"cache_miss_tokens":512,"output_tokens":8,"cost_cny":0.0006}
{"type":"agent.done","run_id":"run_…","agent_role":"subagent","parent_epoch_id":"epoch_…","epoch_id":"epoch_child_…","reason":"model_done"}
{"type":"agent.done","run_id":"run_…","agent_role":"root","epoch_id":"epoch_…","reason":"model_done"}
```

`agent.done` is the per-agent terminator (one per root/subagent run). A
`child_trace_incomplete` record is written by the harness only when a child
trace handle timed out before its `agent.done` (the subagent was cut off).

Compaction stability is **measured, not asserted**: the agent computes the
static-prefix fingerprint of the frozen baseline (`before_static_prefix_hash`)
and of the request compaction actually fed the model (`after_…`). The harness
compares the two — there is no hardcoded `static_prefix_hash_changed` boolean.
Equal hashes prove compaction reused the frozen prefix.

### Fail-closed instrumentation

There is no "N/A pass". If a task sets `require_cache_gate: true` and the
agent produces **no** trace, the gate **fails** for that task. The gate also
fails closed on a **malformed or incomplete** trace — any of:

- a malformed (non-JSON) line,
- a `usage` record with no `epoch_id`,
- a `prefix.snapshot` missing its `epoch_id`/`static_prefix_hash`,
- a `usage` record whose epoch produced no `prefix.snapshot`, or
- a `compaction` record missing a `before`/`after` hash.

For agents with `enforce_cache_gate: true` a failed gate also fails the CI
exit code. External agents (e.g. Reasonix) that don't emit this trace show a
missing-trace gate row — reported, never enforced.

### Markdown Reports

Reports are written to `bench/reports/bench-YYYYMMDD-HHMMSS.md`:

```markdown
# Benchmark Report

Generated: 2026-05-27T10:00:00Z

## Summary

| Agent | Task | Success | Duration | Cache Hit% | Output Tokens | Cost (¥) |
|-------|------|---------|----------|------------|---------------|----------|
| deepseekcode-current | ctx-long-readonly | true | 12345ms | 45.2% | 800 | 0.0123 |

## Per-Agent Summary

| Agent | Tasks | Successes | Failures | Avg Duration | Total Cost (¥) |
|-------|-------|-----------|----------|--------------|----------------|
| deepseekcode-current | 6 | 5 | 1 | 15000ms | 0.0500 |
```

## Adding New Tasks

1. Create a fixture in `bench/fixtures/` (git repo or plain directory)
2. Write a prompt file in `bench/prompts/`
3. Create a task YAML in `bench/tasks/`
4. For read-only tasks, use `read_only: true` and `no_changes: true`
5. For write tasks, use `no_changes_outside` with specific allowed paths
6. Run `go run ./bench/cmd/benchrunner/ --task <id>` to verify

## Adding New Agents

1. Create an agent YAML in `bench/agents/`
2. Ensure the agent binary is built and accessible
3. Set required environment variables
4. Run the benchmark

## Architecture

The runner is a single Go binary that:
1. Loads agent and task configurations from YAML
2. For each (agent, task) pair:
   - Copies fixture repo to temp directory
   - Resets to frozen commit
   - Runs agent with prompt
   - Captures stdout/stderr
   - Enforces timeout
   - Parses usage from output
   - Writes JSONL trace
3. Generates summary report

## Dependencies

- `gopkg.in/yaml.v3`: YAML parsing
- Standard library: Everything else

## Reusing Pricing Logic

The runner reimplements pricing from `internal/llm/cache_metrics.go` to avoid importing internal packages. If pricing changes, update both locations.

## M5: Optimized Benchmark

The optimized adapter (`deepseekcode-optimized`) exercises the new
PrefixEpoch, semantic compaction, and tool tier features. It uses the
same `./bin/dsc` binary as the baseline but signals to the runner that
cache-reliable behavior is expected.

### Running the Optimized Benchmark

```bash
# Build the binary (required)
make build

# Run optimized agent only
go run ./bench/cmd/benchrunner/ --agent deepseekcode-optimized

# Run optimized vs baseline side-by-side
go run ./bench/cmd/benchrunner/ --agent deepseekcode-optimized
go run ./bench/cmd/benchrunner/ --agent deepseekcode-current

# Run all three agents
go run ./bench/cmd/benchrunner/

# Filter to a specific task
go run ./bench/cmd/benchrunner/ --agent deepseekcode-optimized --task ctx-long-readonly
```

### Comparing Against Baseline

Run both agents against the same task set, then compare:

1. **Cost**: Total `cost_cny` across all tasks. The 50x cache-hit
   discount means the optimized adapter should be significantly cheaper.
2. **Success rate**: Tasks passed / tasks attempted.
3. **Cache hit rate**: The optimized adapter targets >= 95% post-warm
   cache hits on eligible tasks.
4. **Trace quality**: Average turns per task — fewer turns means cleaner
   autonomous behavior.

Traces are written to `bench/traces/<agent-id>/`. Reports go to
`bench/reports/`.

### Cache Reliability Gate

The Cache Reliability gate is a pass/fail check computed from the agent
trace (`<task>.agent.jsonl`):

| # | Criterion | Threshold | Trace source |
|---|-----------|-----------|--------------|
| 0 | Trace present **and well-formed** | required (fail-closed) | file exists + integrity counters all 0 |
| 1 | Within-epoch prefix stability | 1 hash per epoch | `prefix.snapshot.static_prefix_hash` |
| 2 | Post-warm cache hit rate | >= 95%, >= `min_post_warm_turns` warm turns | `usage` records (excl. first/epoch) |
| 3 | Unauthorized drift count | 0 | `drift.blocked` records |
| 4 | Compaction prefix hash stability | `before` == `after`; record required if `require_compaction_record` | `compaction.before/after_static_prefix_hash` |
| 5 | Parent/subagent cache pollution | 0 pollution + valid parent link + (when required) complete child trace, **N/A unless a child epoch exists** | `agent_role`/`parent_epoch_id`/child `usage`/`agent.done` |

**Post-warm cache hit rate**: After the first turn warms the prompt
cache, all subsequent turns must achieve >= 95% cache hit tokens /
total prompt tokens. A cache-gated task must produce at least
`min_post_warm_turns` warm turns (default 1) — a task that requires the
gate but never measured a warm turn **fails**, it does not pass on N/A.

**Unauthorized drift**: The static prefix hash (system prompt + tool
definitions) must not change between turns within a single epoch. Any
change indicates a bug in prefix stability.

**Parent/subagent cache pollution**: a child epoch must not reuse or
mutate its parent's epoch. Subagents run in-process (via `LoopSpawner`)
and now tee their own epoch/usage events into the parent's trace,
stamped `agent_role="subagent"` and `parent_epoch_id=<parent epoch>`,
with a distinct child `epoch_id` minted by the child's own epoch
manager. The dimension is **evaluable only when the trace contains a
child (subagent) epoch**; a run that spawned none reports **N/A** —
never a hardcoded ✅. A task that sets `require_subagent_isolation: true`
(e.g. `subagent-parallel`) **fails closed** when no child epoch is
present, since isolation cannot be proven. When a child epoch is
present the harness checks it did not reuse the parent epoch **and** that
its `parent_epoch_id` actually points at a real root epoch — a child with
no parent link (`missing_parent`) or an unknown parent (`unknown_parent`)
fails. Under `require_subagent_isolation` the child trace must also be
**complete**, judged **per child epoch**: a single `epoch_id` counts only
when its own records carry a valid parent link, ≥1 `usage` turn, and an
`agent.done` terminator. The gate needs ≥1 complete child epoch and 0
incomplete ones, so a `c1` with usage-but-no-done plus a `c2` with
done-but-no-usage fails even though usage and done each appear *somewhere*.
A child that only emitted a `prefix.snapshot` is partial evidence and fails
(`partial`). Async subagents are flushed before exit:
the one-shot run waits on tracked child trace handles
(`Agent.WaitChildTraces`), so a `task` with `async:true` does not lose its
child epoch when the process exits; if a child handle times out before
`EventDone` the harness writes a `child_trace_incomplete` marker and the
dimension fails (`incomplete`) rather than trusting a cut-off child.

**Compaction prefix hash stability**: when compaction fires, the agent
emits the measured static-prefix fingerprint before and after; the
harness fails the gate if they differ. Compaction rewrites conversation
history, not the system prompt, so the two must match. **Both** the
semantic and deterministic paths emit a measured record (the
deterministic fallback emits `before == after`, since it never rebuilds
the prefix). A task can set `require_compaction_record: true` (enabled on
`ctx-compaction`) to additionally **fail closed** when the trace contains
**no** compaction record at all — so this dimension cannot pass on the
absence of evidence when compaction was the whole point of the task.

### Agentic Engineering Score

The Agentic Engineering Score measures output quality relative to cost:

| # | Criterion | Metric |
|---|-----------|--------|
| 1 | Cost efficiency | Total cost (¥) for all tasks |
| 2 | Task success rate | Tasks passed / tasks attempted |
| 3 | Trace quality | Turns per task (fewer = cleaner) |
| 4 | Debug quality | Error count across all tasks |
| 5 | Cache utilization | Avg cache hit% across eligible tasks |

The optimized adapter **wins** if it passes the Cache Reliability gate
and beats the baseline on at least one of: cost, trace quality, or
debug quality.

### Reports are generated, not templated

`benchrunner` writes a real `bench/reports/bench-YYYYMMDD-HHMMSS.md` from the
parsed traces on every run, including the per-(agent, task) Cache Reliability
gate table. Do **not** hand-author an `optimized-YYYY-MM-DD.md` with
placeholder values and call M5 done — a report only counts if it was produced
by an actual run whose `<task>.agent.jsonl` traces exist alongside it.

### Running the full Phase-1 matrix (current + optimized + Reasonix)

```bash
make build                              # ./bin/dsc
npm --prefix <reasonix-repo> run build  # if running Reasonix from source
go run ./bench/cmd/benchrunner/         # all three agents × all tasks
```

`reasonix-current` runs the headless `reasonix run <task>` mode. Reasonix
does not emit deepseekcode's epoch trace, so its gate row reads
"trace missing" (report-only) — a token-level cache comparison would require
parsing Reasonix's own `--transcript` JSONL (future work). Until a real
matrix run is recorded with all three adapters, the Reasonix comparison
required by the Phase-1 Definition of Done is **blocked**, and M5 must not be
marked complete.

### Inspecting a trace locally

Use `dsc trace inspect` to summarize one JSONL trace without running the full
benchmark:

```sh
./bin/dsc trace inspect bench/traces/deepseekcode-optimized/ctx-compaction.jsonl
```

The output shows usage turns, cache hit rate, hit/miss tokens, cost, root
epochs, subagent epochs, and per-epoch completion state.

## h2h cache benchmark — 2026-06-10

Head-to-head comparison of **dsc** (`e154c0e`) vs **Reasonix** (`013d55c7…`)
on `deepseek-v4-flash` across 5 real open-source issues (gRPC-8915, gRPC-9111,
gRPC-8831, cobra-1781, chi-1045), each run twice (10 runs per arm).

### Aggregate

| arm | runs | resolved | DNF | hit rate | billable (miss+out) |
|---|---|---|---|---|---|
| dsc | 10 | 7 | 1 | 89.6% | 472 661 |
| reasonix | 10 | 6 | 3 | 93.9% | 420 425 |

**Win gate: false** — dsc resolved more tasks (7 vs 6) but Reasonix achieved
higher cache hit rate (93.9% vs 89.6%). Reasonix's 3 DNFs were all turn-cap
overruns (>30 turns); dsc had 1.

### Per-run detail

| arm | task | repeat | resolved | DNF | hit rate | billable | err |
|---|---|---|---|---|---|---|---|
| dsc | grpc-8915 | 1 | true | false | 88.3% | 28 992 |  |
| reasonix | grpc-8915 | 1 | true | false | 88.0% | 30 423 |  |
| dsc | grpc-8915 | 2 | true | false | 85.6% | 20 205 |  |
| reasonix | grpc-8915 | 2 | true | false | 94.3% | 16 993 |  |
| dsc | grpc-9111 | 1 | false | true | 89.7% | 97 085 | turn cap exceeded: 34 > 30 |
| reasonix | grpc-9111 | 1 | true | false | 84.1% | 78 593 |  |
| dsc | grpc-9111 | 2 | true | false | 88.3% | 102 609 |  |
| reasonix | grpc-9111 | 2 | false | true | 94.7% | 61 286 | turn cap exceeded: 41 > 30 |
| dsc | grpc-8831 | 1 | true | false | 86.8% | 48 290 |  |
| reasonix | grpc-8831 | 1 | true | false | 85.7% | 76 321 |  |
| dsc | grpc-8831 | 2 | true | false | 81.0% | 35 103 |  |
| reasonix | grpc-8831 | 2 | false | true | 91.7% | 78 226 | turn cap exceeded: 39 > 30 |
| dsc | cobra-1781 | 1 | false | false | 93.8% | 51 128 |  |
| reasonix | cobra-1781 | 1 | false | true | 98.6% | 64 543 | turn cap exceeded: 67 > 30 |
| dsc | cobra-1781 | 2 | false | false | 93.3% | 68 632 |  |
| reasonix | cobra-1781 | 2 | false | false | 88.8% | 5 638 |  |
| dsc | chi-1045 | 1 | true | false | 84.9% | 9 366 |  |
| reasonix | chi-1045 | 1 | true | false | 89.5% | 4 578 |  |
| dsc | chi-1045 | 2 | true | false | 86.5% | 11 251 |  |
| reasonix | chi-1045 | 2 | true | false | 86.8% | 3 824 |  |

### Takeaway

dsc's task-resolution edge (70% vs 60%) comes from fewer turn-cap failures —
its compaction and routing keep sessions within budget. Reasonix achieves
tighter cache packing on the runs that complete, but pushes harder against the
turn ceiling. Both tools are in the same cache-efficiency league (~89–94%);
the differentiator is reliability under a fixed turn budget.

### Reproduce

```sh
make bench-h2h ARGS="-reasonix /path/to/reasonix"
```

## Future Work

- [ ] Support for multi-turn conversations
- [ ] Parallel execution of independent tasks
- [ ] Diff invariant checking (currently just logs)
- [ ] Test execution for tasks with `tests` field
- [ ] Aggregated statistics across multiple runs
- [ ] Comparison mode (diff two runs)
