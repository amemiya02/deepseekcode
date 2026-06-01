# DeepSeek V4 Agent Story

DeepSeekCode is a terminal coding agent purpose-built for DeepSeek V4 models.
This page explains what makes it different from generic coding agents and how
you can verify each claim locally.

## What problem does this solve?

Coding agents that call LLM APIs face three recurring reliability problems:

1. **Cost unpredictability** — prompt-cache instability means the same
   conversation can cost 50x more than expected.
2. **Destructive mistakes** — an agent that deletes files or runs dangerous
   commands without a second check can cause real damage.
3. **Context limitations** — large repositories exceed typical context windows,
   forcing agents to choose between missing context and truncated output.

DeepSeekCode addresses each of these with mechanisms you can verify, not just
claims you have to trust.

## Why is DeepSeekCode different?

### 1. Provable Prefix-Cache Stability

DeepSeek V4 offers a 50x prompt-cache discount on cache hits. But the discount
only applies when the **exact same prefix bytes** are sent on subsequent turns.
Most agents serialize requests differently each time (map iteration order,
schema drift, dynamic system prompts), which silently invalidates the cache.

DeepSeekCode solves this with a **single canonical serializer** that feeds both
the wire bytes sent to the API and the cache fingerprint used for verification.
They cannot diverge by construction.

**How to verify:**

```sh
# Build the binary
make build

# Run a one-shot prompt with trace output
./bin/dsc -p "list the files in this project" --trace-jsonl /tmp/trace.jsonl

# Inspect the trace for prefix stability
./bin/dsc trace inspect /tmp/trace.jsonl
```

A stable run shows `prefixes==1` — every turn within an epoch used the same
static prefix hash. See [prefix-cache.md](prefix-cache.md) for details.

### 2. Selective Pro Validation

DeepSeekCode uses a two-model "Duet" architecture: the main loop runs on the
fast, cheap `deepseek-v4-flash`, while destructive operations (writes outside
cwd, secret paths, dangerous bash commands) are validated by
`deepseek-v4-pro` before execution.

This is NOT running every turn on pro. The pro model is invoked **only** when
the agent attempts a potentially destructive action. This keeps costs low while
adding a safety net where it matters most.

**How to verify:**

The Pro validator is integrated into the permission system. When a destructive
tool call is detected, the Duet hook runs the pro model in JSON mode to get an
approve/block decision. See [duet.md](duet.md) for the full architecture.

You can observe Pro validation in the trace:
- Repair events (`repair` type in JSONL) show when tool calls were modified
- Permission receipts show when the Duet validator was consulted

### 3. 1M Context for Large Repos

DeepSeek V4 supports up to 1M token context windows. DeepSeekCode uses this
for:

- **Long-history workflows**: Multi-step coding sessions where the conversation
  grows beyond typical 128K limits.
- **Large-repo reading**: Reading many files in a single session without
  losing earlier context.

**Important:** 1M context does not eliminate the need for tools and retrieval.
It means the agent can hold more conversation history and file contents
simultaneously, reducing the need for aggressive compaction and improving
continuity across long sessions.

### 4. Tool-Call Repair Pipeline

DeepSeek sometimes emits malformed tool calls (truncated JSON, calls hidden in
reasoning text, repeated failed calls). DeepSeekCode includes a repair pipeline
that sits between model output and tool execution:

1. **Schema flattening**: Complex nested schemas (depth > 2) are flattened for
   DeepSeek compatibility and rehydrated before execution.
2. **Scavenging**: Tool calls accidentally emitted in reasoning or content text
   are recovered when the official `tool_calls` array is empty.
3. **Truncation repair**: Missing closing braces in JSON arguments are
   auto-closed when unambiguous; string-internal truncation drops the call
   and asks the model for continuation instead of executing an invented value.
4. **Storm guard**: Repeated identical failed tool calls are suppressed after a
   threshold, with a diagnostic message that helps the model choose a different
   approach.

Every repair action produces a report for trace/debugging. No repair silently
modifies tool calls — all changes are observable.

## How to verify locally

### Prerequisites

- `DEEPSEEK_API_KEY` set in environment
- `dsc` binary built (`make build`)

### Quick verification

```sh
# 1. Prefix cache stability
./bin/dsc -p "read README.md" --trace-jsonl /tmp/trace.jsonl
./bin/dsc trace inspect /tmp/trace.jsonl
# Look for: prefixes==1, cache hit rate > 0

# 2. Tool-call repair (run a task that might trigger repair)
./bin/dsc -p "edit main.go to add a comment"
# Check the trace for repair events

# 3. Benchmark case-study
make bench-case-study
# Check bench/results/<timestamp>/summary.md
```

### Full benchmark

```sh
# Run the benchmark harness with cache-gated tasks
go run ./bench/cmd/benchrunner/ --agent deepseekcode-current
```

See [bench/README.md](../bench/README.md) for the full benchmark documentation.

## Current limitations

- **Pro validation is not a replacement for user review.** The pro model can
  still approve a destructive action that the user would not want. Always use
  the permission system (`--ask-all` mode) for sensitive operations.
- **1M context does not guarantee accuracy.** Longer context means more
  information available, but the model can still make mistakes. Use retrieval
  tools and verify critical changes.
- **Prefix stability is per-epoch.** Changing the model, switching profiles, or
  adding skills mid-session creates a new epoch with one expected cache miss.
  This is by design — it keeps the cache honest.
- **Repair pipeline is conservative.** When truncation is inside a quoted
  string (unsafe to auto-repair because the content is unknown), the pipeline
  drops the call and asks the model for continuation. This means some truncated
  calls require an extra turn, but no call executes with an invented value.
- **Some complex schemas cannot be flattened.** Schemas containing arrays of
  objects, `oneOf`, or `anyOf` at any depth pass through unchanged because
  flattening would lose structural information. This applies to both built-in
  and MCP tools — the restriction is schema-pattern-based, not source-based.

## Further reading

- [Prefix cache documentation](prefix-cache.md)
- [Duet pro-validator architecture](duet.md)
- [Model compatibility](MODEL_COMPATIBILITY.md)
- [Benchmark harness](../bench/README.md)
- [Configuration](config.md)
