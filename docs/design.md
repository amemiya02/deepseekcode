# deepseekcode — v0.1 Design Document

A terminal-native coding agent purpose-built for DeepSeek models. Single Go
binary. Distinctive UX around reasoning, cache economics, and a two-model
duet that's surgical, not noisy.

Target milestone: submission to
[deepseek-ai/awesome-deepseek-agent](https://github.com/deepseek-ai/awesome-deepseek-agent)
as a reference implementation.

---

## Table of contents

1. [Vision and posture](#1-vision-and-posture)
2. [Primary user](#2-primary-user)
3. [Reference repos studied](#3-reference-repos-studied)
4. [Stack](#4-stack)
5. [Model strategy](#5-model-strategy)
6. [Agent loop architecture](#6-agent-loop-architecture)
7. [Tool surface](#7-tool-surface)
8. [Permissions and safety](#8-permissions-and-safety)
9. [Session and state](#9-session-and-state)
10. [TUI](#10-tui)
11. [Two-Model Duet (Pro Validator)](#11-two-model-duet-pro-validator)
12. [Slash commands](#12-slash-commands)
13. [Differentiators — the launch story](#13-differentiators--the-launch-story)
14. [Repo identity and distribution](#14-repo-identity-and-distribution)
15. [Directory layout](#15-directory-layout)
16. [Configuration](#16-configuration)
17. [v0.1 scope and cut line](#17-v01-scope-and-cut-line)
18. [Timeline](#18-timeline)
19. [v0.2 backlog](#19-v02-backlog)
20. [Open questions](#20-open-questions)

---

## 1. Vision and posture

`deepseekcode` is a **reference implementation**, not a framework or a
research toolkit. One opinionated CLI, one blessed workflow, optimized for
the 30-second demo that wins over a reviewer skimming
`awesome-deepseek-agent`.

Three guiding principles:

- **It just works.** Single static binary, sane defaults, install in one
  line. No "first-run wizard" — the first prompt is the wizard.
- **Distinctive on screenshot.** Every feature must survive being reduced
  to a single still image. If the demo gif needs a voiceover to be
  impressive, it's the wrong feature.
- **DeepSeek-native.** Features should be ones that *only* make sense on
  DeepSeek: structured `reasoning_content`, the steep cache-hit discount,
  the flash↔pro family. A wrapper that swaps in OpenAI would be missing
  the point.

---

## 2. Primary user

The **terminal-native individual developer**:

- Lives in tmux/zellij with neovim/helix
- Runs `gh` CLI rather than opening GitHub in a browser
- Has strong opinions about latency, keybindings, and binary size
- Reads HN / lobste.rs, posts screenshots on X/Mastodon
- Overlaps directly with the people who curate awesome-lists

Mainstream IDE users and enterprise teams are *not* the v0.1 audience.
They get served later, by features built on top of what ships here.

---

## 3. Reference repos studied

| Repo | Lang | Loop file(s) | Key takeaway |
|------|------|---|---|
| [charmbracelet/crush](https://github.com/charmbracelet/crush) | Go | `internal/agent/agent.go` | **Closest model to copy.** Callback-driven ReAct, OpenAI-style tool calls, reasoning_content first-class, `csync.Map` for per-session cancel, `loop_detection.go` as a `StopCondition`. |
| [sst/opencode](https://github.com/sst/opencode) | TS | `packages/opencode/src/session/prompt.ts` | Steal: finish-reason override at line ~1266 (treat `tool_calls.len > 0` as authoritative even when `finish_reason=stop`). DeepSeek occasionally does this. |
| [cline/cline](https://github.com/cline/cline) | TS | `src/core/task/index.ts`, `StreamChunkCoordinator.ts`, `TaskPresentationScheduler.ts` | Steal: split stream-arrival from UI-presentation. Without it, the Reasoning Tape stutters on bursty SSE. |
| [plandex-ai/plandex](https://github.com/plandex-ai/plandex) | Go | `app/server/model/plan/tell_stream_main.go` | Steal: two-tier timeout — `firstTokenTimeout` distinct from per-chunk stall. Reasoner cold starts can be 30s+. |
| [Aider-AI/aider](https://github.com/Aider-AI/aider) | Py | `aider/coders/base_coder.py` | Steal: `reasoning_tags.py` text-based `<think>` parsing as fallback for providers that don't expose `reasoning_content` natively. |
| [Hmbown/DeepSeek-TUI](https://github.com/Hmbown/DeepSeek-TUI) | Rust | `crates/tui/src/cycle_manager.rs` | Same lineage as claw-code (shared scaffold). Demonstrates the `cycle_manager` + `seam_manager` split for safe resume points. |
| [ultraworkers/claw-code](https://github.com/ultraworkers/claw-code) | Rust | `crates/src/cycle_manager.rs` | Steal: `prefix_cache.rs` content-addressed cache of stable prompt prefixes for provider cache reuse. |
| [anomalyco/opencode](https://github.com/anomalyco/opencode) | TS | (fork of sst/opencode) | Stale fork; no independent ideas. |

What **not** to copy: Crush's dep on the private `charm.land/fantasy` SDK
(we write our own slim `LLM` interface); Cline's 3000-line god-class
(decompose by file like Cline itself eventually did); plandex's custom
XML tool format (stick with native function-calling); aider's no-parallel,
no-tool-calling architecture.

---

## 4. Stack

| Concern | Choice |
|---------|--------|
| Language | **Go** (single static binary; 5ms cold start; trivial cross-compile; Charm ecosystem is best-in-class for TUIs) |
| TUI runtime | [Bubble Tea](https://github.com/charmbracelet/bubbletea) |
| Styling | [Lip Gloss](https://github.com/charmbracelet/lipgloss) |
| Markdown | [Glamour](https://github.com/charmbracelet/glamour) |
| HTTP | `net/http` stdlib + small SSE helper |
| SQLite | [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) (pure Go, **no CGO** — keeps cross-compile trivial) |
| Config | [BurntSushi/toml](https://github.com/BurntSushi/toml) |
| Logging | `log/slog` stdlib |
| Tests | stdlib `testing` + [stretchr/testify](https://github.com/stretchr/testify) for asserts |

No LLM SDK. No agent framework. The `internal/llm` package is ~400 LOC of
HTTP+SSE+typed events. The cost of writing it is one weekend; the cost of
inheriting a dep is forever.

Target platforms (cross-compiled from one machine, no CGO):
darwin-arm64, darwin-amd64, linux-amd64, linux-arm64, windows-amd64.

---

## 5. Model strategy

### 5.1 Models

Per the [DeepSeek API docs](https://api-docs.deepseek.com/zh-cn/quick_start/pricing):

| ID | Context | Max out | Input (cache hit / miss) | Output | Rate limit |
|----|---------|---------|--------------------------|--------|------------|
| `deepseek-v4-flash` | 1M | 384K | ¥0.02 / ¥1 per 1M tok | ¥2 / 1M | 2500 concurrent |
| `deepseek-v4-pro`   | 1M | 384K | ¥0.025 / ¥3 (discounted) per 1M tok | ¥6 (discounted) / 1M | 500 concurrent |

Both support thinking mode (separate `reasoning_content` channel) and
OpenAI-style function calling.

Deprecated aliases sunset 2026-07-24:
- `deepseek-chat` → flash non-thinking
- `deepseek-reasoner` → flash thinking

`deepseekcode` silently maps these for one release with a one-time
banner, then drops them.

### 5.2 Endpoint

OpenAI-format: `https://api.deepseek.com`.

Anthropic-format endpoint (`/anthropic`) exists but is **deferred to v0.2** —
shipping two protocol paths in v0.1 doubles the surface for no demo gain.

### 5.3 Defaults

- **Default main-loop model**: `deepseek-v4-flash`, thinking mode **ON**.
  - 1M context + 50× cache discount = "load whole repo cheaply" demo.
  - Thinking mode lights up the Reasoning Tape on first run.
  - ¥2/M output (~$0.28 USD) makes "free-tier-feeling" usage real.
- **Pro is opted into**, not defaulted to: via `/models deepseek-v4-pro`
  for the whole session, or auto-invoked by the Duet Validator for
  specific moments (§11).
- **Configurable per session and across sessions**: the active main-loop
  model is persisted in the session record (§9) so `--resume` preserves
  whatever the user last chose.

### 5.4 Cache strategy

The 50–120× cache-hit discount is *the* economic story. Every part of
the prompt construction is built around maximizing cache reuse:

1. **System prompt is cache-stable.** No timestamps, no PID, no
   per-turn salt. One canonical string per binary version.
2. **Repo skeleton block is content-hash-anchored.** When we include
   "here are the relevant files," we order them by content hash so
   re-visits hit cache.
3. **Tool descriptions are pinned.** Tool schemas serialized
   deterministically (sorted keys) so identical tool sets produce
   identical JSON.
4. **User-message turns are append-only** in the wire format. The
   prefix never mutates; only the tail grows.
5. **Cache statistics are surfaced** in the status line (§10) so users
   *see* the discount working.

---

## 6. Agent loop architecture

### 6.1 Shape

Pure ReAct, **callback-driven**, modeled on Crush's
`internal/agent/agent.go`. One loop per session:

```
streamModel(ctx, req) → events:
    OnReasoningStart / OnReasoningDelta / OnReasoningEnd
    OnTextDelta
    OnToolCall(call)   → dispatch (parallel if multiple)
    OnToolResult(result)
    OnStepFinish(stopReason, usage)
StopWhen: []StopCondition  →  loop-detection | token-budget | model-said-done
```

The callback table **is** the data source for the Reasoning Tape (§10) —
each callback maps to one tape entry. No separate "render" pipeline.

### 6.2 Tool-call format

OpenAI function-calling JSON. Native to DeepSeek's OpenAI endpoint. No
custom XML, no Anthropic `tool_use` translation layer.

### 6.3 Parallel tool calls

When the model emits multiple `tool_calls` in one response, they run in
parallel via a `sync.WaitGroup` + bounded `errgroup`. Results are
appended in original order so the conversation transcript is
deterministic.

### 6.4 Three load-bearing patches

1. **Stream/present split** (from Cline's `StreamChunkCoordinator` +
   `TaskPresentationScheduler`). SSE chunks arrive on one schedule
   (network); UI deltas render on a throttled schedule (16ms tick).
   Without this, the Reasoning Tape stutters when DeepSeek bursts 200
   tokens at once.

2. **Finish-reason override** (from
   `sst/opencode/packages/opencode/src/session/prompt.ts:1266`).
   DeepSeek (like the providers opencode is guarding against)
   occasionally returns `finish_reason=stop` while *also* emitting
   `tool_calls`. We treat `len(tool_calls) > 0` as authoritative and
   continue the loop. Without this guard, the agent stops mid-task.

3. **Two-tier timeout** (from
   `plandex/app/server/model/plan/tell_stream_main.go`).
   - `firstTokenTimeout = 45s` — time from request send to first chunk
   - `chunkStallTimeout = 20s` — max gap between chunks once streaming
   - Reasoner cold starts can be 30s+; conflating these timeouts
     punishes the wrong condition.

### 6.5 Interruption

- Per-session `context.CancelFunc` stored in a `sync.Map[sessionID,
  CancelFunc]`.
- Ctrl-C calls `cancel()` and aborts the in-flight stream cleanly.
- For any in-flight tool calls without a result, we synthesize
  `{tool_call_id, role: "tool", content: "user cancelled"}` entries
  before persisting the message so the next turn has a valid history.
- `defer flushUI()` in the loop guarantees partial deltas are persisted
  even on panic.

### 6.6 Stop conditions

- **Loop detection**: same tool call (name + arg hash) appearing 3+
  times in the last 5 steps → emit a `StopReason=LoopDetected` and
  return a synthetic tool result asking the model to summarize what it
  was trying to do.
- **Token-budget compaction**: when remaining context drops below 20k
  tokens (large model) or 20% of window (small model), an automatic
  summarize-and-trim runs. This is `sessionAgent.Run`'s exact
  threshold from Crush.
- **Model-said-done**: no `tool_calls` and not a tool result → loop
  exits; control returns to user prompt.

### 6.7 Single-agent only in v0.1

No subagents in v0.1. A `Spawner` interface is reserved in
`internal/agent/spawner.go` with no implementation so v0.2 can add
subagents additively.

---

## 7. Tool surface

12 built-in tools. All emit structured JSON, all describe themselves
with deterministic schemas (sorted keys) for cache stability.

| Tool | Purpose | Notes |
|------|---------|-------|
| `read_file` | Read file content | Args: `path`, optional `start_line`, `end_line`. Returns line-numbered output (cat -n format) so models can reason about positions. |
| `write_file` | Write or overwrite | Args: `path`, `content`. Snapshot recorded (§8). |
| `edit_file` | String-replace edit | Args: `path`, `old_string`, `new_string`, optional `replace_all`. Fails on ambiguous match. Snapshot recorded. |
| `bash` | Run shell command | Args: `command`, optional `timeout_ms` (default 120s). Runs in `cwd`. Permission gate per §8. |
| `glob` | Filename glob | Args: `pattern`, optional `cwd`. Returns sorted paths. |
| `grep` | Content search | Args: `pattern`, optional `path`, `glob_filter`, `case_insensitive`. ripgrep wrapper. |
| `ls` | List directory | Args: `path`. Lightweight; no deep recursion. |
| `todo_write` | Plan management | Args: `todos: [{subject, status, ...}]`. Renders as inline list block in scrollback. |
| `git_diff` | Structured diff | Args: optional `ref_a`, `ref_b`, `path`. Returns parsed hunks, not raw text. |
| `git_show` | Read historical content | Args: `ref`, `path`. Read a file at a past commit. |
| `git_blame` | Per-line authorship | Args: `path`, optional `range`. Returns lines with commit+author. |
| `git_log` | Commit history | Args: optional `path`, `n` (default 20), `since`. Returns parsed commits, not pager output. |

### 7.1 MCP

[Model Context Protocol](https://modelcontextprotocol.io/) supported via
`[mcp_servers]` config block. Servers are spawned **lazily** on first
tool-call referencing them (stdio transport only in v0.1). **None
enabled by default** — the binary's tool surface is what ships.

### 7.2 Deferred to v0.2

- `web_fetch` — security surface (SSRF, data exfil); not worth the
  v0.1 attention.
- Anything that requires a sandbox to be safe.

---

## 8. Permissions and safety

### 8.1 Tiered defaults

| Tool class | Default |
|------------|---------|
| Read-only (`read_file`, `glob`, `grep`, `ls`, `git_*`) | Always allow, never prompt |
| Edit/write **inside** cwd | Always allow, never prompt |
| Edit/write to `.git/`, `.env*`, or paths matching secret patterns | Always prompt |
| Edit/write **outside** cwd | Always prompt |
| `bash` | Prompt per unique command pattern |
| `web_fetch` (v0.2) | Off entirely unless config-enabled; per-host first-time prompt |

### 8.2 `bash` approval

Command patterns are matched by **leading verb + first-arg shape**:
- `git status` and `git status -sb` collapse to `git status *`
- `npm install foo` and `npm install bar baz` collapse to `npm install *`
- `rm -rf node_modules` collapses to `rm -rf *`

Approval choices on first encounter:
- **`o` once** — allow this call, prompt again next time
- **`s` session** — allow for this session, prompt again next session
- **`a` always** — append pattern to `~/.deepseek/config.toml` allowlist
- **`d` deny** — refuse; the model gets a tool error and can adapt

### 8.3 Mode flags

| Flag | Behavior |
|------|----------|
| `--yolo` | Auto-approve everything. For automation (`tmux send-keys`, CI). |
| `--read-only` | Block all write/edit/bash tools. For "let me explore this repo safely." |
| `--ask-all` | Prompt every tool, ignoring allowlist. For first-time-using-the-agent paranoia. |

### 8.4 Snapshot rollback

Before any tool that mutates files (`write_file`, `edit_file`, `bash` —
the latter snapshots the whole working tree), affected files are copied
to `.deepseek/snapshots/<session_id>/<step_index>/`.

- `/undo` reverts the last step's snapshots
- `/undo 3` reverts the last 3 steps
- Snapshots auto-prune to the last 30 sessions

### 8.5 No process sandbox in v0.1

bubblewrap (Linux) / sandbox-exec (macOS) deferred to v0.2.
Snapshot rollback + `--read-only` mode cover most of the safety story
without breaking "my `$PATH` works" expectations.

---

## 9. Session and state

### 9.1 Storage

- **Global SQLite database** at `~/.deepseek/sessions.db` (one file,
  all sessions across all projects)
- **Per-project pointer** at `./.deepseek/last_session` (a single line
  containing the session UUID; auto-gitignored via a `.deepseek/.gitignore`
  the binary writes on first use)

Why global: cross-project resume from a fzf picker (§9.3) is a feature.
Per-repo DBs would scatter state and clutter every project with a binary
file.

### 9.2 Schema

```sql
CREATE TABLE sessions (
    id              TEXT PRIMARY KEY,        -- UUID v7 (sortable)
    parent_id       TEXT,                    -- branching: NULL for root
    branch_point    INTEGER,                 -- message idx in parent
    project_path    TEXT NOT NULL,           -- absolute cwd at creation
    model           TEXT NOT NULL,           -- last active main-loop model
    duet_enabled    INTEGER NOT NULL,
    created_at      INTEGER NOT NULL,
    last_used_at    INTEGER NOT NULL,
    summary         TEXT,                    -- auto-generated short title
    FOREIGN KEY (parent_id) REFERENCES sessions(id)
);

CREATE TABLE messages (
    session_id          TEXT NOT NULL,
    idx                 INTEGER NOT NULL,
    role                TEXT NOT NULL,       -- user | assistant | tool
    content             TEXT,
    reasoning_content   TEXT,
    tool_calls          TEXT,                -- JSON array
    tool_results        TEXT,                -- JSON
    model               TEXT,                -- which model produced this msg (for /tape per-step glyph)
    cache_hit_tokens    INTEGER,
    miss_tokens         INTEGER,
    output_tokens       INTEGER,
    cost_yuan           REAL,
    ts                  INTEGER NOT NULL,
    PRIMARY KEY (session_id, idx),
    FOREIGN KEY (session_id) REFERENCES sessions(id)
);

CREATE INDEX idx_sessions_project ON sessions(project_path, last_used_at DESC);
CREATE INDEX idx_sessions_parent  ON sessions(parent_id);
```

### 9.3 Resume rules

| Invocation | Behavior |
|------------|----------|
| `dsc` (no args, project has session <24h old) | Auto-resume most recent |
| `dsc` (no args, no recent session) | New session |
| `dsc -c` / `--continue` | Resume last session in cwd (any age) |
| `dsc -r` / `--resume` | fzf-style picker across **all** projects |
| `dsc --new` | Force new session even if recent exists |
| `dsc --resume <id>` | Resume by explicit ID |

### 9.4 Branching

From the `/tape` fullscreen view, pressing `b` at any message creates a
**child session** with `parent_id` + `branch_point=idx`. No message copy —
on load, the runtime walks `parent_id` chains, replays messages up to
`branch_point`, then continues with the child's own messages.

`/sessions` opens a tree view showing the parent/child relationship,
visually like:

```
session a3f1 — "refactor auth middleware"
├── a3f1.b1 — "branch: try cookie approach"
└── a3f1.b2 — "branch: try JWT approach"  ← active
```

### 9.5 TTL and privacy

- Sessions older than 90 days are pruned on startup (configurable).
- Snapshots pruned to last 30 sessions.
- Strictly **local-only**. No telemetry. No "anonymous usage stats."
  If telemetry is ever added it will be opt-in and will never include
  message content.

---

## 10. TUI

### 10.1 Layout — scrollback-first

Standard chat scrollback. Model output, tool calls, tool results
streaming linearly. No persistent side panels. tmux/zellij users
already split their own panes; we don't steal columns.

### 10.2 Reasoning blocks

Every `reasoning_content` span renders as a folded line:

```
▸ thinking (12s · 1.3k tok)
```

Keybindings:
- `r` — toggle the most recent reasoning block
- `R` — toggle all reasoning blocks
- `?` — show keymap overlay

When expanded, the block renders as dimmed/italic markdown.

### 10.3 Status line

One line by default, two when the Duet Validator has fired at least
once this session:

```
flash · 7 steps · cache 91% · ¥0.04 · /tape
```

```
flash · 7 steps · cache 91% · ¥0.04
pro   · 2 calls · cache 73% · ¥0.02
```

The model token (`flash` / `pro`) updates live when `/models` switches
the main-loop model.

### 10.4 `/tape` fullscreen view

The headline feature. Pressing `/tape` (or hitting a configurable hot
key — default `Alt+t`) pushes a fullscreen view of the *entire*
session's reasoning timeline:

```
┌─ tape ──────────────────────────────── 23 steps · 18.2k reasoning tok ─┐
│                                                                        │
│  ◇ step 4  flash  reasoning  3.1s  450 tok                             │
│    > looking at auth middleware to understand the                      │
│    > current JWT validation logic                                      │
│                                                                        │
│  ◇ step 4  flash  tool_call  read_file pkg/auth/jwt.go                 │
│                                                                        │
│  ◆ step 5  pro    validation  1.2s  approved                           │
│    > the proposed delete of pkg/auth/cookie.go is consistent           │
│    > with the user's stated goal of JWT-only auth                      │
│                                                                        │
│  ◇ step 5  flash  tool_call  rm pkg/auth/cookie.go                     │
│                                                                        │
│  ▸ step 6  flash  reasoning  collapsed                                 │
│                                                                        │
├────────────────────────────────────────────────────────────────────────┤
│ j/k step · J/K turn · b branch from here · o open file · q exit       │
└────────────────────────────────────────────────────────────────────────┘
```

Keybindings:
- `j` / `k` — step forward/back one entry
- `J` / `K` — jump to next/previous user turn
- `b` — branch a new session from this point (§9.4)
- `o` — open the file referenced at this step in `$EDITOR`
- `e` — expand/collapse the current entry
- `q` — return to scrollback

### 10.5 Color and theme

- **Flash voice**: cyan/teal accent
- **Pro voice**: magenta accent
- **Reasoning text**: dimmed (60% fg)
- **Tool calls**: bright fg, monospace-emphasized
- **Errors**: red
- Two themes ship: `dark` (default) and `light`. Adaptive lipgloss
  styles so colors look right on both.

### 10.6 Vim keys throughout

Scrollback navigation uses `hjkl`, `gg`/`G`, `Ctrl-d`/`Ctrl-u`. Prompt
editor supports vim mode (toggleable in config).

---

## 11. Two-Model Duet (Pro Validator)

### 11.1 Concept

Flash runs the main loop. Pro adjudicates the dangerous moments. **Pro
never runs on every turn** — that would halve cache hits and double cost
for no quality gain. Pro is invoked surgically, at moments where its
premium is justified.

### 11.2 Triggers (2, not 3)

1. **Destructive-tool gate** (automatic):
   - `write_file` outside cwd
   - `write_file` / `edit_file` matching `.git/`, `.env*`, secret-like patterns
   - `bash` matching destructive patterns:
     - `rm`, `rm -rf`
     - `git push`, `git push --force`
     - `git reset --hard`, `git checkout .`, `git clean -f`
     - `curl -X POST|PUT|DELETE|PATCH`
     - `kubectl delete`, `kubectl apply -f` (when not in `--dry-run`)
     - `terraform apply`, `terraform destroy`
     - SQL `DROP`, `DELETE`, `TRUNCATE` via `psql`/`mysql`/etc
     - `npm/pnpm/yarn publish`
     - `docker push`
   - Pattern list lives in config so users can extend
2. **`failure_retry_with_pro`** (automatic, internal):
   - If the main-loop model attempts the same fix twice and both fail
     (same tool call name + arg hash + non-zero/error result), the
     **next single attempt** routes to Pro.
   - This is a per-attempt routing, not a session switch. Does not
     persist; not surfaced as a session-mode change.
   - Disabled by `[duet] retry_on_failure = false`.

### 11.3 What Pro receives

```jsonc
{
  "system": "<pro validator system prompt: 'You are a senior reviewer...'>",
  "messages": [
    /* recent N turns from the session as context */
    /* last assistant turn with proposed tool_call */
    /* a user-role message: "About to execute this tool call. Should we?" */
  ],
  "tools": []  // pro is in adjudicator mode; it doesn't call tools, only judges
}
```

Pro responds with structured output (forced JSON-mode):

```json
{
  "decision": "approve" | "block",
  "reasoning": "<one paragraph>"
}
```

### 11.4 TUI rendering

Pro validation renders inline in scrollback, **before** the tool
result, as a folded reasoning block with the pro glyph:

```
◆ pro check (1.2s · 800 tok): approved
◆ pro check (3.4s · 1.2k tok): blocked — would delete uncommitted changes in pkg/auth/
```

Distinct color from flash voice (magenta vs cyan). `r`/`R` toggles
apply to pro blocks too. `/tape` view shows the per-step model
attribution glyph (`◇` flash, `◆` pro).

### 11.5 Block handling

When pro returns `block`:

```
◆ pro check (3.4s): blocked — <reason>

  [o] override and run anyway   [e] edit/rephrase   [c] cancel
```

- `o` — proceed with the tool call. Override is **logged** in the
  session metadata so users can see their "ignored pro N times this
  session" pattern.
- `e` — opens the prompt editor pre-filled with the user's last message,
  letting them rephrase the goal so the next model turn doesn't try
  this approach.
- `c` — cancel the tool call; model gets a tool-result of "user cancelled
  after pro validation block."

### 11.6 Failure mode

| Pro failure | Behavior |
|-------------|----------|
| 10s timeout | Render `◆ pro validation skipped: timeout` — fall through to standard permission prompt. **Never auto-approves destructive ops because pro failed.** |
| Network error | Same as timeout. |
| Pro returns malformed JSON | Treat as `block` with reason "validator response was unparseable"; user gets override choice. |
| Rate limited | Skip with `◆ pro validation skipped: rate limited` + fall through. |

### 11.7 Edge cases

- **User has switched main-loop model to Pro via `/models`**: the
  destructive-tool gate becomes a **silent no-op**. Pro can't validate
  itself meaningfully. Standard permission prompt still gates
  destructive paths. No `◆ pro check` annotations.
- **`--yolo` mode**: pro validator still runs (yolo means "auto-approve
  the permission prompt," not "skip safety"). Block → still surfaces
  the o/e/c choice; user can immediately press `o`.
- **`--read-only` mode**: no destructive tools can run at all, so the
  validator is structurally unreachable. Config flag is allowed but
  unused.

### 11.8 Disabling

- CLI: `--no-duet`
- Config: `[duet] enabled = false`

When disabled, destructive operations go through the standard
permission prompt without pro intervention.

---

## 12. Slash commands

| Command | Purpose |
|---------|---------|
| `/models` | List/switch main-loop model. No-arg opens fzf picker (id · pricing · `*` active marker). `/models <id>` direct switch. Persists across resume. Overridden on next `/models`. |
| `/tape` | Open fullscreen reasoning timeline (§10.4). |
| `/sessions` | Open session tree picker (§9.4). |
| `/undo` / `/undo N` | Revert last N snapshot(s) (§8.4). |
| `/help` | Keymap overlay. |
| `/clear` | Compact context manually (otherwise auto at token-budget threshold). |
| `/quit` | Persist state, exit cleanly. |

Vocabulary note: there is **no `/escalate`**. The concept of "use a
better model for a moment" is split:
- User-driven: `/models deepseek-v4-pro` switches for the rest of the session
- Automatic: the Duet's destructive-tool gate and `failure_retry_with_pro`

---

## 13. Differentiators — the launch story

Five differentiators ship in v0.1, laddered for the README/demo:

| # | Feature | Why it's distinctive |
|---|---------|----------------------|
| 1 | **Reasoning Tape** (headline 1) | Only possible because DeepSeek emits structured `reasoning_content`. Inline collapsible + `/tape` fullscreen + `b` branch = the screenshot that goes viral. |
| 2 | **Two-Model Duet — Pro Validator** (headline 2) | Only possible because flash+pro share the family and pro is cheap enough to invoke surgically. The `◆ pro check` annotation is unmistakable in a demo. |
| 3 | **Cost HUD** | DeepSeek's 50–120× cache-hit discount turned into a live dial users *watch*. Reinforces the "frugal by design" story. |
| 4 | **Session branching** | `b` from `/tape` forks via SQLite parent_id+branch_point. Replay-from-anywhere as a first-class behavior, not a buried feature. |
| 5 | **Structured git tools** | `git_diff/show/blame/log` as typed tools, not bash wrappers. Lets the model reason about history without re-parsing pager output. |

The README hero gif is **30 seconds, one shot**: a user opens
`deepseekcode`, asks "refactor pkg/auth from cookies to JWT," watches
flash think (collapsed reasoning), watches a tool call run, sees a
`◆ pro check (1.2s): approved` flash by before the destructive `rm`,
then opens `/tape`, scrubs back to a decision point, presses `b`,
forks a session to try a different approach. Final frame: status line
shows `flash · 14 steps · cache 87% · ¥0.04`.

---

## 14. Repo identity and distribution

### 14.1 Names

- **Repo**: `github.com/<user>/deepseekcode`
  - Mirrors working dir + `claude-code`/`opencode` naming pattern
  - Personal account first; transfer to org if it takes off (GitHub
    preserves links)
- **Binary**: `dsc` (3 chars)
- **Long alias** symlink: `deepseekcode` (for autocomplete visibility
  and grep across PATH)
- **License**: **MIT**
- **Trademark fallback**: if DeepSeek pushes back on the name at
  awesome-list submission time, repo move is cheap (one-time, GitHub
  preserves redirects).

### 14.2 Install matrix at launch

| Channel | How |
|---------|-----|
| Homebrew (own tap) | `brew install <user>/deepseekcode/deepseekcode` |
| `curl | sh` | `curl -fsSL https://deepseekcode.dev/install.sh | sh` |
| Go install | `go install github.com/<user>/deepseekcode/cmd/dsc@latest` |
| GitHub Releases | Prebuilt: darwin-arm64, darwin-amd64, linux-amd64, linux-arm64, windows-amd64 |

Community-contributed later: AUR, nixpkgs, scoop, MacPorts.
Homebrew-core takes months; the own-tap is the day-one path.

### 14.3 Docs

For v0.1: `README.md` + `docs/` folder with:
- `install.md`
- `config.md`
- `tools.md`
- `tape.md`
- `duet.md`
- `mcp.md`
- `design.md` (this file)

**No mkdocs/Astro site for v0.1.** That's a week we don't have.

---

## 15. Directory layout

```
deepseekcode/
├── cmd/
│   └── dsc/
│       └── main.go              # CLI entrypoint, flag parsing, dispatch
├── internal/
│   ├── llm/                     # hand-rolled DeepSeek client
│   │   ├── client.go            # HTTP + SSE
│   │   ├── events.go            # typed event channel
│   │   ├── request.go           # request builder, cache-stable serialization
│   │   ├── cache_metrics.go     # parse usage block, surface hit rate
│   │   └── client_test.go
│   ├── agent/                   # ReAct loop
│   │   ├── agent.go             # Run, callback dispatch
│   │   ├── stream_coordinator.go  # Cline-style chunk arrival
│   │   ├── presentation_scheduler.go  # Cline-style render throttle
│   │   ├── stop_conditions.go   # loop detection, token budget
│   │   ├── duet.go              # pro validator dispatch
│   │   ├── spawner.go           # v0.2 subagent interface stub
│   │   └── agent_test.go
│   ├── tools/                   # built-in tool implementations
│   │   ├── registry.go          # tool registration + schema serialization
│   │   ├── read_file.go
│   │   ├── write_file.go
│   │   ├── edit_file.go
│   │   ├── bash.go
│   │   ├── glob.go
│   │   ├── grep.go              # ripgrep wrapper
│   │   ├── ls.go
│   │   ├── todo_write.go
│   │   ├── git_diff.go
│   │   ├── git_show.go
│   │   ├── git_blame.go
│   │   ├── git_log.go
│   │   └── ..._test.go
│   ├── mcp/                     # MCP client (stdio, lazy)
│   │   └── client.go
│   ├── permissions/             # tiered approval logic
│   │   ├── policy.go
│   │   ├── bash_patterns.go     # leading-verb pattern matcher
│   │   ├── allowlist.go         # persistent allowlist (config)
│   │   └── policy_test.go
│   ├── snapshots/               # pre-edit file snapshots, /undo
│   │   └── manager.go
│   ├── session/                 # SQLite-backed sessions
│   │   ├── store.go             # CRUD
│   │   ├── schema.go            # CREATE statements + migrations
│   │   ├── branch.go            # parent-walk + replay
│   │   └── store_test.go
│   ├── tui/                     # Bubble Tea
│   │   ├── app.go               # root model
│   │   ├── scrollback.go        # main chat view
│   │   ├── reasoning_block.go   # collapsible reasoning component
│   │   ├── status_line.go       # model · steps · cache · cost
│   │   ├── tape.go              # /tape fullscreen view
│   │   ├── sessions_picker.go   # /sessions tree
│   │   ├── models_picker.go     # /models fzf
│   │   ├── theme.go             # dark/light styles
│   │   └── keymap.go
│   └── config/                  # ~/.deepseek/config.toml loader
│       ├── config.go
│       └── defaults.go
├── docs/
│   ├── install.md
│   ├── config.md
│   ├── tools.md
│   ├── tape.md
│   ├── duet.md
│   ├── mcp.md
│   └── design.md                # this file
├── .github/
│   └── workflows/
│       ├── ci.yml               # test + build matrix
│       └── release.yml          # goreleaser → GH Releases + brew tap
├── install.sh                   # served from deepseekcode.dev
├── .goreleaser.yaml
├── go.mod
├── go.sum
├── LICENSE                      # MIT
├── README.md
└── Makefile
```

---

## 16. Configuration

`~/.deepseek/config.toml`:

```toml
# DeepSeek API
[api]
key = "${DEEPSEEK_API_KEY}"          # env var interpolation
base_url = "https://api.deepseek.com"
first_token_timeout_ms = 45000
chunk_stall_timeout_ms = 20000

# Defaults
[defaults]
model = "deepseek-v4-flash"
thinking = true
theme = "dark"                       # dark | light
vim_keybindings = true

# Duet
[duet]
enabled = true
retry_on_failure = true
validator_timeout_ms = 10000
# Additional destructive bash patterns (regex). Built-ins always active.
extra_destructive_patterns = [
    # "^my-deploy-script",
]

# Permissions
[permissions]
# bash patterns auto-allowed (added by user pressing 'a' during a prompt)
allow_bash = [
    "git status *",
    "git log *",
    "ls *",
]
# extra paths considered "secret-like" beyond .env*
secret_path_patterns = [
    "*.pem",
    "*.key",
    "id_rsa*",
]

# Sessions
[sessions]
ttl_days = 90
snapshot_keep = 30

# MCP servers (none enabled by default)
# [mcp_servers.example]
# command = "node"
# args = ["/path/to/server.js"]
# env = { FOO = "bar" }
```

Precedence: CLI flags > project `./.deepseek/config.toml` > user
`~/.deepseek/config.toml` > built-in defaults.

---

## 17. v0.1 scope and cut line

### IN v0.1

1. Go binary, 5-platform cross-compile, MIT
2. Hand-rolled `internal/llm` (HTTP+SSE+typed events)
3. DeepSeek v4-flash default with thinking ON
4. Callback-driven ReAct loop + 3 patches
5. Parallel tool calls, OpenAI function-calling
6. 12 built-in tools (read/write/edit/bash/glob/grep/ls/todo + 4 git)
7. Permissions tier + bash allowlist + `--yolo`/`--read-only`/`--ask-all`
8. Snapshot rollback + `/undo`
9. SQLite global session store + branch-by-reference + resume rules
10. Bubble Tea TUI + scrollback + inline collapsible reasoning
11. **Reasoning Tape** `/tape` fullscreen + `b` branch ← headline 1
12. **Cost HUD** in status line (live cache % + ¥/$)
13. **Structured git tools** (4)
14. **Two-Model Duet — Pro Validator** ← headline 2
15. `/models` command (picker + direct)
16. MCP support via `[mcp_servers]` config (no defaults)
17. Install matrix: brew tap + install.sh + go install + GH Releases
18. README + 7 docs/ files + demo GIF

### DEFERRED to v0.2

- Subagents (Spawner interface lights up)
- Process sandbox (bubblewrap/sandbox-exec)
- `web_fetch` tool
- Anthropic-format endpoint (`/anthropic` base URL)
- `/sessions` tree picker (basic resume is enough for v0.1)
- AUR/nixpkgs/scoop packaging
- Dedicated docs site (mkdocs/Astro)

### OUT (probably never)

- IDE integrations (VS Code, JetBrains)
- Cloud sync of sessions
- Multi-user / team features
- GUI variant

### Cut order if velocity slips

Top = cut first:

1. `web_fetch` (already v0.2 anyway — verify nothing v0.1 needs it)
2. `/sessions` tree picker UI (basic resume still works)
3. `failure_retry_with_pro` auto-trigger (duet destructive gate stays)
4. MCP support (lose ecosystem story, but core agent intact)
5. Snapshot rollback (lose `/undo`, keep tiered permissions)

**Non-negotiable** — cutting any of these breaks the launch:
- Reasoning Tape (inline + `/tape`)
- Duet destructive-tool gate
- `/models` switch
- Cost HUD
- 12-tool core
- Install matrix

---

## 18. Timeline

Solo dev, focused, ~8–10 weeks.

| Week | Deliverable |
|------|-------------|
| 1 | Project scaffold; `internal/llm` (HTTP+SSE+typed events); config loader; basic single-tool ReAct loop with stdout output (no TUI yet); 4 simplest tools (read/write/edit/bash). |
| 2 | TUI scaffold (Bubble Tea); scrollback view; reasoning block rendering; parallel tool calls; permissions tier + bash allowlist + mode flags. |
| 3 | Remaining tools (glob/grep/ls/todo + 4 git); snapshot rollback + `/undo`; SQLite session store with basic resume (`-c`, `-r`, `--new`); no branching yet. |
| 4 | `/tape` fullscreen view + branching (`b` key); `/models` picker + direct switch; status line + Cost HUD; 3 loop patches (stream split, finish-reason override, two-tier timeout). |
| 5 | **Duet Validator**: destructive-tool gate + pattern config; pro prompt template; structured-output JSON parsing; inline `◆` annotations; block-handling UI (o/e/c); two-line cost HUD. |
| 6 | `failure_retry_with_pro` automatic trigger; MCP client (stdio, lazy); install matrix (Homebrew tap, install.sh, goreleaser config); CI release pipeline; cross-platform smoke tests. |
| 7 | docs/ files (7); README hero + demo GIF; polish pass on TUI edge cases; pro validator edge cases (override logging, pro-as-main no-op). |
| 8–10 | Buffer for slippage and pre-submission hardening; eat feedback from a handful of friends-and-family beta users; submit PR to `deepseek-ai/awesome-deepseek-agent`. |

---

## 19. v0.2 backlog

What gets the next launch tweet:

- **Subagents** — `Spawner` interface implemented; investigate-style
  delegated reads; Reasoning Tape gets multi-lane rendering.
- **Process sandbox** — bubblewrap on Linux, sandbox-exec on macOS.
- **`web_fetch`** — with per-host allowlist + content-size cap.
- **Anthropic-format endpoint** support — `[api] format = "anthropic"`
  toggle.
- **`/sessions` tree picker** UI.
- **Two-Model Duet — Mode B** (Pro-as-Pre-Planner) as an opt-in
  alternative to Validator mode.
- **Reasoning Tape: shareable export** — `dsc tape export <session> > tape.html`.

---

## 20. Open questions

Things deliberately *not* resolved in this doc, to be answered during
implementation:

1. **Token-counting strategy.** Do we ship a bundled BPE tokenizer for
   accurate token counts in the Cost HUD, or trust the API's usage
   response after each call? Bundled is more accurate but adds binary
   size; API-only is simpler but means Cost HUD lags by one turn.
   *Lean: API-only for v0.1; revisit if users complain.*

2. **`grep` implementation.** Shell out to `rg` (assume installed) or
   bundle `BurntSushi/ripgrep`-as-library (substantial Rust dep via
   FFI, breaks CGO-free promise)? *Lean: shell out, fall back to Go's
   `regexp` + `filepath.WalkDir` if `rg` not on PATH.*

3. **First-run config wizard.** Do we run a 4-question wizard on first
   launch (API key, model, theme, duet on/off) or just print "set
   `DEEPSEEK_API_KEY` and re-run"? *Lean: print the env-var nudge for
   v0.1; wizard in v0.2 once we know what people actually misconfigure.*

4. **Auto-prune cadence.** Run snapshot/session pruning synchronously
   on startup (small latency hit) or in a background goroutine after
   the TUI is up? *Lean: background, after 5s of idle, so cold-start
   latency stays sub-100ms.*

5. **Glamour markdown rendering of model output.** Render in real-time
   as tokens arrive (re-render every N chars) or buffer until newline
   boundaries? Real-time looks magical but is CPU-expensive on long
   responses. *Lean: line-buffered with a 250ms flush timer.*

---

*End of document. Pushback welcome on any section before scaffolding begins.*
