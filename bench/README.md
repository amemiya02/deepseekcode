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
fixture_repo: bench/fixtures/ctx-long-readonly
commit: HEAD
prompt_file: bench/prompts/ctx-long-readonly.md
timeout_seconds: 300
success:
  tests: []  # no tests for read-only
  diff_invariants:
    - no_changes_outside:
        - bench/fixtures/ctx-long-readonly
metrics:
  require_cache_gate: true
```

### Fields

- **id**: Unique task identifier
- **fixture_repo**: Path to fixture git repo (relative to bench dir)
- **commit**: Git commit to reset to (HEAD, specific hash, etc.)
- **prompt_file**: Path to prompt markdown file
- **timeout_seconds**: Maximum execution time (default: 300)
- **success.tests**: Test commands to run (empty for read-only tasks)
- **success.diff_invariants**: Constraints on file changes
- **metrics.require_cache_gate**: Whether cache hit rate is required

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
- **turn.started**: Turn number (1-indexed)
- **usage**: Token counts and cost for the turn
- **run.finished**: Success/failure, duration, exit code, line counts, errors

### Missing Fields

When instrumentation data is unavailable, fields are set to `null` (not omitted):

```json
{"type":"usage","turn":1,"cache_hit_tokens":null,"cache_miss_tokens":null,"output_tokens":null,"cost_cny":null}
```

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

1. Create a fixture repo in `bench/fixtures/`
2. Write a prompt file in `bench/prompts/`
3. Create a task YAML in `bench/tasks/`
4. Run the benchmark

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

The Cache Reliability gate is a pass/fail check on four criteria:

| # | Criterion | Threshold |
|---|-----------|-----------|
| 1 | Post-warm cache hit rate | >= 95% on eligible tasks |
| 2 | Unauthorized drift count | 0 |
| 3 | Parent/subagent cache pollution | 0 |
| 4 | Compaction prefix hash stability | 0 changes |

**Post-warm cache hit rate**: After the first turn warms the prompt
cache, all subsequent turns must achieve >= 95% cache hit tokens /
total prompt tokens. This verifies that PrefixEpoch is working
correctly.

**Unauthorized drift**: The static prefix hash (system prompt + tool
definitions) must not change between turns within a single epoch. Any
change indicates a bug in prefix stability.

**Parent/subagent cache pollution**: When a parent spawns a subagent,
the subagent's system prompt must not corrupt the parent's cached
prefix. Verified by comparing `static_prefix_hash` before and after
subagent execution.

**Compaction prefix hash stability**: When compaction fires, the
`static_prefix_hash` must remain unchanged. Compaction rewrites
conversation history, not the system prompt.

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

### Report Template

Fill in `bench/reports/optimized-YYYY-MM-DD.md` after each run. The
template includes placeholders for all Cache Reliability and Agentic
Engineering Score criteria.

## Future Work

- [ ] Support for multi-turn conversations
- [ ] Parallel execution of independent tasks
- [ ] Diff invariant checking (currently just logs)
- [ ] Test execution for tasks with `tests` field
- [ ] Aggregated statistics across multiple runs
- [ ] Comparison mode (diff two runs)
