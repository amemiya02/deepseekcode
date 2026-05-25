# deepseekcode

[English](README.md) · [简体中文](README.zh-CN.md)

A terminal-native coding agent purpose-built for DeepSeek models.
Single Go binary. No runtime dependencies. Distinctive UX around
reasoning, cache economics, and a Two-Model Duet that's surgical, not
noisy.

> Status: v0.1 in active development, targeting submission to
> [awesome-deepseek-agent](https://github.com/deepseek-ai/awesome-deepseek-agent).
> See [`docs/design.md`](docs/design.md) for the full design and
> trade-offs.

## What makes it different

- **Reasoning Tape** — DeepSeek emits `reasoning_content` as a separate
  channel. We render it as a collapsible inline timeline plus a
  fullscreen scrubbable view (`/tape`) you can browse and branch
  sessions from. ([docs/tape.md](docs/tape.md))
- **Two-Model Duet — Pro Validator** — `deepseek-v4-flash` drives the
  loop; `pro` is invoked surgically as a validator on destructive
  operations. Not on every turn. ([docs/duet.md](docs/duet.md))
- **Cost HUD** — live cache-hit % and ¥/$ in the status line. The 50×
  cache-hit discount becomes a dial you watch.
- **Structured git** — `git_diff`, `git_show`, `git_blame`, `git_log`
  as typed tools, not pager wrappers.
- **Session branching** — fork any session from any past step. Cheap,
  via SQLite parent references; no message copying.

## Install

```sh
# Homebrew (after v0.1.0 cut)
brew install amemiya02/deepseekcode/deepseekcode

# curl | sh
curl -fsSL https://deepseekcode.dev/install.sh | sh

# Go install
go install github.com/amemiya02/deepseekcode/cmd/dsc@latest

# from source
git clone https://github.com/amemiya02/deepseekcode && cd deepseekcode && make build
```

Full install matrix: [docs/install.md](docs/install.md).

## Quick start

```sh
export DEEPSEEK_API_KEY=sk-...
dsc                                # launch the TUI
dsc -p "explain pkg/auth"          # one-shot prompt → stdout, exit
dsc --read-only                    # safe-exploration mode
dsc --yolo -p "run the tests"      # auto-approve all tools (CI / automation)
dsc init                           # create DEEPSEEK.md + .deepseek/config.toml
```

Inside the TUI:

```
⏎          send prompt
^C         cancel current run (or quit if idle)
^D         quit
r / R      toggle most recent / all thinking blocks
/help      keys + commands
/models    list / switch the main-loop model
/tape      open the Reasoning Tape
/sessions  list this project's sessions
/undo      revert the last edit step
/compact   force-compact the running message list
```

## Environment variables

| Name | Default | Effect |
|------|---------|--------|
| `DEEPSEEK_API_KEY` | (required) | DeepSeek API credential. |
| `DEEPSEEKCODE_AUTO_COMPACT_INPUT_TOKENS` | `100000` | Trigger threshold for automatic session compaction. Once the estimated token count of the running message list exceeds this value, older turns are collapsed into a single summary message. Set lower for chattier sessions, higher to disable in practice. |

## Architecture in one paragraph

ReAct loop (callback-driven, modeled on Crush's `internal/agent/agent.go`)
over a hand-rolled DeepSeek client (~400 LOC HTTP+SSE+typed events,
no external SDK). Bubble Tea TUI with collapsible reasoning blocks and
a live Cost HUD. Sessions persist to a pure-Go SQLite
(`modernc.org/sqlite`, no CGO) so `--continue` / `--resume` / branching
work without breaking the single-binary distribution story. Tool calls
run in parallel, gated by a tiered permission policy with snapshot
rollback (`/undo`). Pro Validator (the Duet) sits between the model and
destructive operations as a structured-output adjudicator.

Reference repos that shaped the design: `charmbracelet/crush` (Go,
callback ReAct), `sst/opencode` (finish-reason override), `cline/cline`
(stream/present split), `plandex-ai/plandex` (two-tier stream timeout).
See `docs/design.md` §3.

## Documentation

- [Design document](docs/design.md) — full architecture, decisions, trade-offs
- [Install](docs/install.md)
- [Configuration](docs/config.md)
- [Tools](docs/tools.md)
- [Reasoning Tape (`/tape`)](docs/tape.md) — headline feature
- [Two-Model Duet](docs/duet.md) — second headline feature
- [Hooks](docs/hooks.md)
- [MCP](docs/mcp.md)
- [Custom Slash Commands](docs/commands.md)
- [Skills](docs/skills.md) — cross-tool SKILL.md discovery

## Status & roadmap

**v0.1 (current)**: 12 built-in tools, tiered permissions + snapshot
rollback, SQLite sessions with branch-by-reference, Reasoning Tape +
`/tape` fullscreen, `/models` picker, Pro Validator, Cost HUD,
5-platform cross-compile, Homebrew tap + curl|sh + go install. MCP
deferred. Subagents deferred (Spawner interface stub reserved).

**v0.2**: Subagents · process sandbox (bubblewrap/sandbox-exec) ·
`web_fetch` · Anthropic-format endpoint · `/sessions` tree picker ·
shareable Tape export (`dsc tape export`).

## Contributing

PRs welcome. The design doc (`docs/design.md`) documents what's in,
what's out, and the cut order if v0.1 velocity slips. Stick to that
shape; if you want to expand scope, open an issue first so we can talk
it through.

## License

MIT
