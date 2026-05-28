# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
make build         # → ./bin/dsc (ldflags-stamped with version/commit/date)
make install       # go install into $GOBIN
make test          # go test ./...
make test-race     # go test -race ./...
make cover         # writes coverage.html
make lint          # go vet ./...
make fmt           # gofmt -s -w .
make tidy          # go mod tidy
make run           # build + ./bin/dsc (TUI)

# Run a single test
go test ./internal/llm/ -run TestThinkingSerializesAsStruct -v

# One-shot prompt (no TUI). Requires DEEPSEEK_API_KEY in env.
./bin/dsc -p "explain pkg/foo"
```

## High-level architecture

`deepseekcode` is a single-binary terminal coding agent for DeepSeek models. The full design is in `docs/design.md` (1000+ lines, source of truth). The big-picture pieces that span multiple files:

### Agent loop (`internal/agent/agent.go`)

Callback-driven ReAct loop modeled on `charmbracelet/crush`. `Agent.Run()` calls `runStep()` which streams one model turn via `llm.Client.Stream()`, then `runToolCalls()` executes tool calls in parallel and appends results to `Messages`. Three subtle behaviors that aren't obvious from the code alone:

- **Finish-reason override** (per `sst/opencode`): even when DeepSeek returns `finish_reason=stop`, if `tool_calls` is non-empty the loop continues. `hasTools := len(step.ToolCalls) > 0` is authoritative.
- **Pre-tool snapshot**: before parallel execution, `runToolCalls` takes one snapshot of the union of affected paths (via `AffectedPathsFor`), so `/undo` reverts a whole step atomically.
- **Two-tier stream timeout** (per `plandex-ai/plandex`): `Client.FirstTokenTimeout` (45s — reasoner cold start) is distinct from `ChunkStallTimeout` (20s — gap between chunks once streaming).

### Context limits and compaction (`internal/agent/compact.go`)

DeepSeek V4 models have 1M token context windows. The defaults reflect this:
`MaxContextTokens = 1_000_000`, `AutoCompactInputTokens = 800_000`. Override
the auto-compact threshold via `DEEPSEEKCODE_AUTO_COMPACT_INPUT_TOKENS` env var.
`EstimateTokens` uses char/4 (no tokenizer dependency) — it's intentionally rough.

### Prefix epoch system (`internal/agent/prefix_epoch.go`)

Epoch IDs are generated from a **process-wide atomic counter** + millisecond
timestamp (`epoch_{ms}_{seq}`), not `time.Now().UnixNano()` alone. This
prevents ID collisions on Windows where clock resolution is ~15ms. The
`EpochManager` mutex protects all state; the counter lives outside the struct
so separate manager instances (parent/child) never collide.

### Budget projection (`internal/agent/budget_projection.go`)

`ProjectedTurnCostCNY` runs **before** the HTTP request. It prices all input
as cache miss (conservative) and falls back to 2048 output tokens when
`MaxTokens <= 0`. This gates expensive turns against the session budget
before any API call is made.

### Wire format — DeepSeek V4 specifics (`internal/llm/request.go`)

- **`thinking` is a struct, not a bool**: DeepSeek V4 rejects `"thinking":true` with `expected struct ThinkingOptions`. Always use `llm.ThinkingEnabled(bool)` which returns `*ThinkingOptions{Type:"enabled"}` or nil. Regression test: `thinking_shape_test.go`.
- **Cache-stable serialization**: `Request.MarshalCacheStable()` sorts tools by function name and canonicalizes every JSON-Schema (recursive key-sort). Every byte difference invalidates DeepSeek's prompt cache, and the 50× cache-hit discount is the cost story. Don't introduce non-deterministic field ordering anywhere upstream of this.
- The system prompt (`DefaultSystemPrompt`) must stay byte-stable across turns for the same reason.

### Two-Model Duet (`internal/agent/duet_pro.go`)

`pro` (`deepseek-v4-pro`) is invoked *only* as a JSON-mode validator on destructive tool calls (writes outside cwd, secret paths, `bash` matching `permissions.DestructivePatterns`). It is NOT called every turn. If the user switches the main loop to `pro` via `/models`, `duetSelfValidates()` makes the Duet a silent no-op.

### Permissions (`internal/permissions/policy.go`)

Tiered defaults: read-only auto-allow, write inside cwd auto-allow, secrets always ask, bash gated by `bashPattern` allowlist (reduces `"git status -sb"` → `"git status *"`). Mode flags (`--yolo`, `--read-only`, `--ask-all`) override the policy globally.

### Sessions (`internal/session/`)

SQLite via `modernc.org/sqlite` (pure Go, **no CGO** — preserves single-binary cross-compile). Branching is by reference: a child session stores `parent_id + branch_point`, and `Replay` walks the chain. No message copying. Schema is in `schema.go`.

### TUI (`internal/tui/app.go`)

Bubble Tea model. The **key flow** is the non-obvious bit: `Update()` only `return`s early from the `KeyMsg` case when `handleKey()` reports `intercepted=true`. Otherwise it falls through so `a.input.Update(msg)` at the bottom receives the keystroke. If you intercept a new key, remember to return `(cmd, true)` from `handleKey`, never raw `return a, cmd` in `Update`. Reasoning fold toggles are bound to `ctrl+r` / `ctrl+t` — never to a plain letter, because plain letters become first-character typing collisions.

The agent runs in a goroutine; `wireCallbacks()` translates `agent.Callbacks` events into typed `tea.Msg` values via `tea.Program.Send`. Permission asks use a reply channel so the agent goroutine blocks until the user answers.

### CLI subcommands (`cmd/dsc/`)

Routing lives in `cmd/dsc/main.go` — each subcommand is dispatched before
flag parsing via `os.Args[1] == "<name>"`. Subcommand implementations are in
separate files (`trace.go`, `agent_cmd.go`, `session_inspect.go`). The pattern
is: a `runXxxCommand(args) (string, error)` function returns text (never
prints directly) so tests can assert on the output.

- `dsc trace inspect TRACE.jsonl` — summarizes a JSONL trace via `internal/traceinspect`
- `dsc agent list|show|new|validate` — manages `.deepseek/agent/*.md` definitions
- Agent definitions support extended frontmatter: `hidden`, `max_steps`, `permission_ruleset`, `temperature`, `top_p`, `default_agent`

### Tools (`internal/tools/`)

Each tool implements `Tool` (Name/Description/Parameters/Execute). `Registry.AsLLMTools()` returns them sorted by name for cache-stable tool listings. Structured git tools (`git_diff/git_show/git_blame/git_log`) emit typed output rather than wrapping pager output.

### Event schema (`internal/eventschema/`)

Canonical event-name constants (`ModelTurnStarted`, `PrefixEpochCreated`, etc.)
so downstream consumers (CLI, dashboards, benchmarks) reference the same string
identifiers. `Known(name) bool` validates event names.

### TUI render cache (`internal/tui/render_cache.go`)

LRU cache keyed by content-based hashes (toolCallID + tool + args + theme +
width + expanded + IsError + duration + content). `Scrollback.Clear()` resets
the cache. Cache is only used for `itemToolCall` and `itemToolResult` — other
item kinds bypass it.

## Conventions worth knowing

- **Bilingual README**: `README.md` (EN) and `README.zh-CN.md` (zh) must be edited together with matching `##` structure. Applies to README only, not `docs/*.md`.
- **`internal/` packages are not importable from outside** — write tests inside the package, not in `/tmp` scratch programs.
- **No external LLM SDK**. The `internal/llm` package is ~400 LOC of hand-rolled HTTP+SSE+typed events. Don't introduce one.
- **Module path is `github.com/amemiya02/deepseekcode`**. Binary name is `dsc`.
- **Branch model**: PRs go to `main`. SSH remote uses the `github.com-amemiya02` host alias.
- **Agent definitions**: `.deepseek/agent/*.md` with YAML frontmatter. Parsed by `internal/agents/def.go`. `agents.Load(cwd, home)` silently skips malformed files — use `dsc agent validate` (which walks and checks each file) for strict validation.
- **PXR workflow**: `.pxr/` contains plan-execute-review task cards and implementation reports. Not production code.
