# Handoff: deepseekcode → production-grade

**Repo:** `git@github.com-amemiya02:amemiya02/deepseekcode.git` (private)
**Local:** `/Users/zichuanxu/vscode/DeepSeekCode`
**Working tree at handoff:** `main` branch, 1 commit (`e2f0e42 Initial v0.1 implementation`) on remote. `CLAUDE.md` is uncommitted locally — first thing to commit.
**Goal:** ship a coding agent good enough to be featured on https://github.com/deepseek-ai/awesome-deepseek-agent, on par with Claude Code / claw-code, decisively better than Hmbown/DeepSeek-TUI.

---

## 0 · Read these first (do not skim)

Everything important about *what exists* is already written down. Do **not** re-derive from source — read these:

| File | Why |
|---|---|
| `docs/design.md` (1014 lines, 20 numbered sections) | Source of truth for v0.1 scope, decisions, trade-offs, cut order. Every architectural "why" lives here. |
| `CLAUDE.md` | Distilled architecture + non-obvious gotchas (cache-stable serialization, finish-reason override, two-tier timeout, thinking-as-struct, TUI key fall-through). |
| `README.md` / `README.zh-CN.md` | Feature list + install matrix. Bilingual; **edit together with matching `##` structure**. Memory rule at `~/.claude/projects/-Users-zichuanxu-vscode-DeepSeekCode/memory/readme_bilingual.md`. |
| `Makefile` | All dev commands. `make build`, `make test`, `make test-race`, `make cover`, `make lint`, `make fmt`, `make tidy`. |

Section refs below use `design.md §N` shorthand.

---

## 1 · What is built (v0.1, on `main`)

73 files, 9356 LOC Go. Compiles, `go test ./...` passes, `go vet ./...` clean.

| Layer | Package | What's there |
|---|---|---|
| Entry | `cmd/dsc/main.go` | CLI flags, `runTUI()`, `runOneShot()` (`-p` mode), stdout callbacks. |
| LLM | `internal/llm/` | Hand-rolled HTTP+SSE client (~400 LOC, no SDK). Typed events: `EventTextDelta` / `EventReasoningDelta` / `EventToolCallDelta` / `EventFinish` / `EventError`. Two-tier stream timeout. **Cache-stable** `MarshalCacheStable()` (sorts tools, canonical JSON). Cost / cache-hit-rate metrics. |
| Agent | `internal/agent/` | Callback-driven ReAct loop. Parallel tool execution. Stop conditions (max-steps, loop-detection). Pro Validator (Duet) implementation. `Spawner` interface stub for subagents (not wired). |
| Permissions | `internal/permissions/` | Tiered policy (default / yolo / read-only / ask-all). Bash pattern allowlist with command reduction. Destructive-op detection. |
| Tools (12) | `internal/tools/` | `read_file` / `write_file` / `edit_file` / `bash` / `glob` / `grep` / `ls` / `todo_write` + structured `git_diff` / `git_show` / `git_blame` / `git_log`. |
| Sessions | `internal/session/` | SQLite (`modernc.org/sqlite`, no CGO). Branch-by-reference (`parent_id + branch_point`, `Replay` walks chain). |
| Snapshots | `internal/snapshots/` | Tar-free per-step file snapshots for `/undo`. Tombstone files for "absent" state. |
| TUI | `internal/tui/` | Bubble Tea app. Reasoning Tape, Cost HUD, status bar, slash commands, models picker, sessions picker, permission asks via reply channel. |
| Distribution | `.goreleaser.yaml`, `install.sh`, `.github/workflows/` | 5-platform cross-compile config, curl|sh installer, CI matrix + tag-triggered release. **Homebrew tap config is present but commented out** — needs a tap repo before uncommenting. |

### Two recent fixes (in `main` but not as separate commits — they shipped inside the initial commit)

1. **`thinking` field shape**: DeepSeek V4 rejects boolean `"thinking":true` with `expected struct ThinkingOptions`. Fixed via `llm.ThinkingEnabled(bool) *ThinkingOptions`. Regression test at `internal/llm/thinking_shape_test.go`. **Do not revert** to bool.
2. **TUI input was dead**: `Update()` returned early on every `KeyMsg`, never reaching `a.input.Update(msg)`. Fixed via `handleKey() (cmd, intercepted bool)` and fall-through. Reasoning toggles moved off plain `r`/`R` (collided with typing) to `ctrl+r`/`ctrl+t`. Layout cleaned up + rounded border around input.

---

## 2 · Gap to production-grade (the actual handoff)

Below is the punch list, **bucketed by gap and broken down to the finest sub-tasks I can produce without first running the binary against real DeepSeek traffic**. The next agent should **walk the TUI manually first** (one full coding task end-to-end) before committing to which bucket to attack first — bug discovery comes free that way.

Each bucket lists: (a) reference repo to study, (b) concrete sub-tasks ordered roughly by dependency.

### Bucket A · Run-it-once verification (DO THIS FIRST, before any new feature)

We **never** drove the binary end-to-end against the real DeepSeek API in this session. Reality check before scope expansion:

- A.1 — Export `DEEPSEEK_API_KEY`, run `./bin/dsc -p "list files in cmd/"`. Confirm the `-p` path no longer 400s on the `thinking` field. If it does: the fix in `internal/llm/request.go` only covers the v4 endpoint shape; the deployed DeepSeek API might use a different envelope. Check actual docs at https://api-docs.deepseek.com/.
- A.2 — Launch `./bin/dsc` (no args), type a prompt, hit Enter. Verify: keystrokes register, Enter submits, streaming text appears, reasoning fold renders, status bar updates with cost.
- A.3 — Trigger a tool call (e.g. "read README.md and summarize"). Verify: permission prompt appears for the first `read_file`, response routes correctly via the reply-channel pattern.
- A.4 — Trigger a destructive op (e.g. "delete pkg/foo/old.txt"). Verify: Duet fires, ◆ pro check line renders, approve/block decision is honored.
- A.5 — Test `/undo` after an edit. Test `/models` switch from flash → pro and back. Test `/sessions` list, `/tape` view.
- A.6 — Resize the terminal. Ctrl+C mid-stream. Ctrl+D quit. Confirm no panics, no zombie goroutines (`runtime/pprof` if needed).
- A.7 — Test `-c` (continue) and `-r <id>` (resume). Sessions are persisted but resume-path is probably under-tested.
- A.8 — Run `go test -race ./...`. The TUI/agent goroutine boundary is fertile ground for races.
- A.9 — **Write down every UX paper-cut** found in A.1–A.8 as numbered issues. This list, not this document, drives the next sprint.

**Skill to use:** `superpowers:systematic-debugging` for anything that misbehaves.

### Bucket B · TUI polish to Claude-Code-tier

Target: a screenshot that survives being posted on X without explanation. Reference: Claude Code's TUI, plus claw-code's `crates/tui/src/cycle_manager.rs` for streaming-render discipline.

- B.1 — **Inline diff renderer for `edit_file` / `write_file` results.** Currently raw text. Use a unified-diff parser + lipgloss color (green + / red -). Render inside the existing `itemToolResult` chatItem.
- B.2 — **Syntax highlighting for `read_file` / `bash` output.** Wire `github.com/alecthomas/chroma/v2` (pure Go). Lazy-load lexer by file extension. Skip for outputs > N KB.
- B.3 — **Markdown rendering for assistant text.** Use Glamour (already a stack choice per design §4). Currently we render raw text.
- B.4 — **Permission prompt modal redesign.** Today it's a 4-line inline strip. Promote to a centered modal box with: tool name, args (pretty-printed JSON, truncated), affected paths highlighted, 4 buttons (`once / session / always / deny`) with current selection styled. Reference: Claude Code's permission prompts.
- B.5 — **Sessions tree picker.** Today `/sessions` is a flat list. Render the parent → branch tree (data is already there via `parent_id`). Use indented tree characters (`├── ` etc.).
- B.6 — **Tape branch-from-step.** Today `/tape` Enter just exits; design called for "select a step → fork a new session here". Wire via `session.Store.NewBranch(parent_id, branch_point)`.
- B.7 — **Stream-arrival vs UI-presentation split** (steal from `cline/cline`'s `TaskPresentationScheduler.ts`). Bursty SSE causes the reasoning tape to stutter. Buffer deltas at the agent boundary; flush at fixed 60 Hz via a `time.Ticker`.
- B.8 — **Cursor / focus visibility.** Verify the textarea cursor blinks correctly in all states. Today `textarea.Blink` runs but visual confirmation is owed.
- B.9 — **Wrap long tool outputs** in `items.go::render()`. Today some kinds use `oneline` (truncate), some emit full text — audit each `itemKind` and pick a deliberate policy.
- B.10 — **Theme switching at runtime.** Add `/theme <dark|light>` slash command. Today theme is config-only.
- B.11 — **Status bar density.** Add: current step elapsed time, tokens/sec (windowed average), warning indicator when context is approaching 1M.
- B.12 — **Help overlay** as a centered modal, not inline scrollback noise. Bind to `?`.
- B.13 — **Color contrast pass** in `theme.go` against WCAG AA on both dark and light backgrounds.

**Skill to use:** `frontend-design:frontend-design` — terminal but the design principles transfer.

### Bucket C · Agent loop hardening

Reference: `charmbracelet/crush` (`internal/agent/agent.go`), `sst/opencode` (`packages/opencode/src/session/prompt.ts`).

- C.1 — **Request retry on transient failures.** Today a 429 / 5xx kills the run. Add exponential backoff with jitter inside `llm.Client.Stream()`, bounded retries, surfaced as `OnInfo` callback.
- C.2 — **Context-window guard.** Track token usage cumulatively; when approaching 1M, auto-compact older turns (summarize → replace) or warn the user. See `claw-code`'s `prefix_cache.rs` for content-addressed prefix caching.
- C.3 — **Tool-call argument validation** before execution. Today `Execute` gets raw JSON; if the model emits a malformed `path` arg, the tool errors out and the loop wastes a turn. Validate against the JSON schema and round-trip a structured error.
- C.4 — **Per-step timeout.** Today `MaxSteps(50)` caps step count; nothing caps step *duration*. Add a configurable per-step `time.Duration`.
- C.5 — **Loop-detection tuning.** Current `LoopDetection(5, 3)` parameters were guessed. Instrument real runs (with `OnStepFinish` logging), tune.
- C.6 — **Cancellation propagation audit.** When user Ctrl+Cs, `runCancel()` fires — but does it cancel in-flight tool calls (especially `bash`)? `executeOne` does check `ctx.Err()` but `tool.Execute(ctx, ...)` implementations must honor the context. Verify each of the 12 tools.
- C.7 — **Reasoning-tag fallback** (steal from `aider`'s `reasoning_tags.py`). For providers that don't expose `reasoning_content` as a stream channel, parse `<think>...</think>` from text deltas. Useful when DeepSeek behavior changes or when running against compatible OpenAI-style endpoints.
- C.8 — **Subagent dispatch.** `internal/agent/spawner.go` is a stub. Implement a `Task` tool that spins up a child Agent with a filtered toolset + isolated message history. Limit fanout. Aggregate results. Reference: Claude Code's Task tool semantics; `opencode`'s nested session pattern.
- C.9 — **Plan-then-execute mode.** Optional flag `--plan`: first turn is reasoning-only, outputs a written plan; user approves; subsequent turns execute. Reference: Claude Code's plan mode.

### Bucket D · Tools — depth + safety

- D.1 — **`web_fetch` tool.** Pure-Go, follows redirects, strips JS, returns rendered text. Time-limited. Per-domain rate limits.
- D.2 — **`web_search` tool.** Pluggable backends (Brave, Exa, fallback to scraping DuckDuckGo). Surface as opt-in due to API keys.
- D.3 — **`apply_patch` tool.** Accept unified diff, apply atomically. Better than `edit_file` for multi-hunk changes. Reference: `aider`'s diff format choices.
- D.4 — **`bash` sandboxing.** Today bash runs with the user's full privileges. Add OS-conditional sandbox: `sandbox-exec` on macOS, `bubblewrap` on Linux. Read-only mount of cwd by default; opt-in writes. Reference: design §17 v0.2 backlog.
- D.5 — **Tool-result truncation policy.** Today large outputs flood the context. Add per-tool `MaxBytes`; truncate with `[... N more bytes ...]` marker; provide a `read_more` tool to fetch the rest by handle.
- D.6 — **`edit_file` fuzzy matching.** Current implementation requires exact `old_string`. Add levenshtein/normalized-whitespace fallback when exact match fails, surface the candidate diff for model self-correction. Reference: claw-code's edit strategy.
- D.7 — **MCP client.** Wire `github.com/modelcontextprotocol/sdk-go` (or equivalent). Spawn MCP server subprocesses on startup per user config; merge their tool list into `tools.Registry`. Reference: design §17 (MCP was explicitly deferred from v0.1).

### Bucket E · Sessions / persistence completeness

- E.1 — **`-c` (continue last session) end-to-end.** Today `.deepseek/last_session` is written but `runTUI` doesn't read it. Wire `cfg.continueSes` → `store.GetSession(lastID)` → preload messages → resume.
- E.2 — **`-r <id>` (resume by ID).** Same plumbing, different source.
- E.3 — **`-r` (no arg → picker).** Open the sessions overlay before the chat loop starts.
- E.4 — **Session deletion / pruning.** `store.PruneOlderThan` exists; wire a `/sessions delete <id>` action and a config-driven auto-prune on startup.
- E.5 — **Export / import.** `dsc session export <id> > out.jsonl` + `dsc session import < in.jsonl`. Format = JSONL of `llm.Message`. Useful for bug reports.
- E.6 — **Cost accounting per session.** Aggregate `Usage` rows; surface in `/sessions` table. Today the status bar shows the *current* session only.

### Bucket F · Distribution — make it installable for real

- F.1 — **Create `homebrew-deepseekcode` tap repo** under `amemiya02`. Uncomment the brew block in `.goreleaser.yaml`. Set `HOMEBREW_TAP_GITHUB_TOKEN` secret on the main repo. Verify a tag → release → brew formula round-trip on a personal Mac.
- F.2 — **`deepseekcode.dev` domain or change `install.sh` URL.** Today `install.sh` references `deepseekcode.dev/install.sh` which doesn't exist. Either: (a) buy the domain + GH Pages, or (b) host installer at `raw.githubusercontent.com/amemiya02/deepseekcode/main/install.sh` and update README to match.
- F.3 — **CHANGELOG.md.** Empty repo. Adopt Keep a Changelog. Backfill v0.1 from `docs/design.md §17`.
- F.4 — **CONTRIBUTING.md.** Pointer to `docs/design.md §17` scope, the cut order, and the "open an issue before expanding scope" rule from README.
- F.5 — **GitHub Issues templates** (`.github/ISSUE_TEMPLATE/`). Bug / Feature / Tool-request.
- F.6 — **Demo GIF.** Use `vhs` (Charm's own tool). One 30-second loop: launch → ask "explain pkg/foo" → reasoning collapse → tool call → Cost HUD ticks → /undo. **This is the single highest-leverage marketing artifact.** Without it, no curated list will accept the PR.
- F.7 — **Screenshots** for README (5–7 stills): Reasoning Tape, Duet ◆ line, Cost HUD, /tape fullscreen, /models picker, /sessions picker, permission prompt.
- F.8 — **Tag v0.1.0** only after A.1–A.9 pass and B/C buckets reach acceptable quality.

### Bucket G · Testing rigor

- G.1 — **Mock DeepSeek SSE server** for E2E tests. `httptest.Server` serving canned SSE chunks. Drives full `Agent.Run()` deterministically.
- G.2 — **Snapshot test for `/tape` rendering** at fixed terminal sizes (80x24, 120x40, 200x60). Bubble Tea has `teatest` for this.
- G.3 — **Property test for cache-stable serialization**: random tool order in → identical bytes out. `quick.Check` or `gopter`.
- G.4 — **Race test in CI**. `make test-race` should run on every PR.
- G.5 — **Permission policy tests** for edge cases: symlinks pointing outside cwd, paths with `..`, secrets matching multiple patterns.
- G.6 — **Coverage gate** at ~70% line on `internal/agent` and `internal/llm` (the two most failure-prone packages).

**Skill to use:** `superpowers:test-driven-development` for new feature work; `superpowers:systematic-debugging` for the failures the E2E harness exposes.

### Bucket H · Observability + debug ergonomics

- H.1 — **Structured logging via `log/slog`** behind `--debug` flag. Today errors go to stderr; nothing else. Want: every HTTP request/response, every tool dispatch, every callback fire, with redaction of API key + secret paths.
- H.2 — **Local log file** at `.deepseek/log/<date>.jsonl`, rotated. Off by default; `--debug` enables.
- H.3 — **`dsc doctor` subcommand.** Checks: API key reachable, SQLite store writable, snapshots dir writable, terminal supports 256-color + UTF-8, version of `git` available.
- H.4 — **Crash recovery.** Today a panic in a callback is swallowed (`agent.fire`). Convert swallowed panics into a logged `OnInfo("internal error: ...")` so silent failures stop being silent.

### Bucket I · Compatibility / portability

- I.1 — **Anthropic-format endpoint** (design §19 v0.2 backlog). Add `cmd/dsc-anthropic-proxy/main.go` that serves an Anthropic-compatible `/messages` endpoint and forwards to DeepSeek. Lets people use deepseekcode as a drop-in for tools that speak Anthropic API.
- I.2 — **OpenAI-format pass-through.** Already close — DeepSeek's API *is* OpenAI-shape. Document that any OpenAI-compatible base URL works via `--base-url`.
- I.3 — **Windows audit.** All path handling uses `filepath`, but the `bash` tool assumes a POSIX shell. On Windows, route through `pwsh -c` or document Windows as "WSL only".
- I.4 — **Locale handling.** UTF-8 width measurement for the Cost HUD / wrap logic. `go-runewidth` or `uniseg`.

### Bucket J · Security

- J.1 — **Prompt-injection guards on tool output.** Today file contents go back to the model verbatim. Strip known injection patterns ("Ignore previous instructions...", base64-encoded directives) before re-feeding, or at minimum sandwich tool output between unambiguous delimiters the model is trained to trust.
- J.2 — **Tool-call rate limit** per session. Hard cap at e.g. 200 calls / session. Surface as `OnInfo` warning at 150, block at 200.
- J.3 — **Audit log for `bash` allowlist additions.** When the user picks `[a]lways`, write to `.deepseek/audit.log` so the persistence is visible.
- J.4 — **Secrets-in-output detection.** Regex-scan tool output for things that look like API keys / private keys before sending to the model. Warn the user; offer to redact.
- J.5 — **`SECURITY.md`** with a reporting channel.

---

## 3 · Reference repos — what to actually steal from each

Already enumerated in `docs/design.md §3`. Recap with **specific** files to read:

| Repo | URL | Read first |
|---|---|---|
| `charmbracelet/crush` | https://github.com/charmbracelet/crush | `internal/agent/agent.go`, `internal/agent/loop_detection.go`, `internal/llm/csync.go`. Our agent is closest to this; if Crush adds something, copy it. |
| `sst/opencode` | https://github.com/sst/opencode | `packages/opencode/src/session/prompt.ts` (~line 1266 for the finish-reason override). Their tool-result truncation policy. Their MCP integration. |
| `cline/cline` | https://github.com/cline/cline | `src/core/task/StreamChunkCoordinator.ts`, `src/core/task/TaskPresentationScheduler.ts`. Stream-arrival vs UI-presentation split — required for Bucket B.7. |
| `plandex-ai/plandex` | https://github.com/plandex-ai/plandex | `app/server/model/plan/tell_stream_main.go`. We already copied the two-tier timeout; revisit when stream behavior changes. |
| `Aider-AI/aider` | https://github.com/Aider-AI/aider | `aider/coders/base_coder.py`, `aider/coders/reasoning_tags.py`. Reasoning-tag fallback (Bucket C.7). Edit format choices (Bucket D.6). |
| `Hmbown/DeepSeek-TUI` | https://github.com/Hmbown/DeepSeek-TUI | `crates/tui/src/cycle_manager.rs`. The competitor we are explicitly trying to beat — read it to know where they're weak. (My read on it: TUI-only, no agent loop, no tools.) |
| `ultraworkers/claw-code` | https://github.com/ultraworkers/claw-code | `crates/src/cycle_manager.rs`, `crates/src/prefix_cache.rs`. Their prefix-cache strategy is the model for our Bucket C.2. Their `seam_manager` pattern informs safe-resume points. |
| `anomalyco/opencode` | https://github.com/anomalyco/opencode | Stale fork of `sst/opencode` per design §3. Skip unless they've diverged. |

---

## 4 · Suggested workflow for the next session

1. **Open a session, do Bucket A end-to-end.** Don't write code. Just drive the binary, take notes, file issues (as GitHub issues — the repo is empty so far, this populates it).
2. **Pick one bucket from B/C/D and brainstorm scope** using `superpowers:brainstorming` *before* committing to an implementation. The user has shown strong "scope-creep is the enemy" preferences (see `docs/design.md §17`).
3. **Write a plan** for the chosen bucket using `superpowers:writing-plans`. Land it in the repo as `docs/v0.2-plan.md` (or whichever version).
4. **Implement with TDD** (`superpowers:test-driven-development`) when adding tools or agent-loop logic; the API surface is small enough that tests pay off fast.
5. **Self-review with `oh-my-claudecode:code-reviewer`** before opening a PR.
6. **PRs go to `main`.** SSH remote uses `github.com-amemiya02` host alias (already configured).

### Skills the next session should keep within reach

- `superpowers:using-superpowers` (start every session)
- `superpowers:systematic-debugging` (Bucket A reality check)
- `superpowers:brainstorming` (before any new feature)
- `superpowers:writing-plans` (for each bucket)
- `superpowers:test-driven-development` (Bucket G + everything else)
- `superpowers:verification-before-completion` (before claiming a bucket done)
- `frontend-design:frontend-design` (Bucket B)
- `oh-my-claudecode:code-reviewer` (PR gate)
- `golang-patterns` (idiom checks in long-form code)
- `grill-me` (when the user wants to stress-test the v0.2 plan)

---

## 5 · Things to be cautious about

- **Bilingual README sync** is a hard rule. Memory file enforces it. Don't touch one without the other.
- **`internal/llm/request.go` cache-stable serialization is load-bearing.** A "cleanup" PR that reorders fields, drops `sort.SliceStable`, or replaces `MarshalCacheStable` with `json.Marshal` will silently destroy the 50× cost story. The `thinking_shape_test.go` is one regression test; add more.
- **The `thinking` field must stay a `*ThinkingOptions` struct**, not a bool. DeepSeek V4 will 400 on bool.
- **`Agent.System` (DefaultSystemPrompt) must be byte-stable across turns.** Don't interpolate dates / session IDs / anything dynamic into it.
- **Don't import `internal/` from outside.** Tests go inside the package.
- **`docs/design.md §17`** has an explicit cut order if v0.2 velocity slips. Honor it; don't redesign features that are already on the chopping block.
- **The git remote is the SSH alias** `git@github.com-amemiya02:...`, not plain `github.com`. Pushes from a fresh worktree need the alias.

---

## 6 · Personal-memory notes the next agent inherits

Located at `/Users/zichuanxu/.claude/projects/-Users-zichuanxu-vscode-DeepSeekCode/memory/`:

- `readme_bilingual.md` — the EN+ZH sync rule
- `MEMORY.md` — index

User profile (from the global memory): email `dongyuyang@initial-s.com`, today's date `2026-05-23`. The user is the developer building this; they're terse, prefer specific over general, push back hard on scope creep, and explicitly chose a bigger v0.1 (with Duet) over my recommendation to defer it. They asked twice for "broken down to the finest sub-tasks" — keep handoffs / plans concrete.
