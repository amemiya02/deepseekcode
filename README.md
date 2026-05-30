# deepseekcode

[![CI](https://github.com/amemiya02/deepseekcode/actions/workflows/ci.yml/badge.svg)](https://github.com/amemiya02/deepseekcode/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/amemiya02/deepseekcode.svg)](https://pkg.go.dev/github.com/amemiya02/deepseekcode)
[![Go Report Card](https://goreportcard.com/badge/github.com/amemiya02/deepseekcode)](https://goreportcard.com/report/github.com/amemiya02/deepseekcode)
[![Baseline](https://img.shields.io/badge/baseline-v0.3.1-blue)](#versioning)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README.md) · [简体中文](README.zh-CN.md)

`deepseekcode` is a terminal-native coding agent for DeepSeek models and
OpenAI-compatible chat-completions endpoints. It ships as a single Go
binary named `dsc`, with a Bubble Tea TUI, one-shot CLI mode, structured
tools, SQLite-backed sessions, cache-aware request serialization, and a
permission model designed for day-to-day repository work.

Current documented baseline: **v0.3.1**.

## Why deepseekcode

DeepSeek exposes reasoning, prefix-cache metrics, long context, and
cost characteristics that are worth treating as first-class UX. This
project builds around those primitives instead of hiding them behind a
generic chat wrapper.

- It keeps reasoning visible with collapsible thinking blocks and a
  fullscreen `/tape` timeline.
- It keeps costs visible with cache-hit and usage reporting in the TUI
  and CLI step output.
- It keeps edits reviewable with structured file tools, pre-edit
  snapshots, `/undo`, and typed git helpers.
- It keeps dangerous actions explicit with tiered permissions, secret
  path checks, bash allowlists, and the Pro validator hook.

## Features

- **TUI and one-shot mode**: run `dsc` for the interactive terminal UI,
  or `dsc -p "prompt"` for scriptable stdout output.
- **DeepSeek-first runtime**: defaults to `deepseek-v4-flash`, supports
  `deepseek-v4-pro`, DeepSeek thinking options, prefix-cache metrics,
  and JSON-mode validation.
- **OpenAI-compatible providers**: configure alternate chat-completions
  endpoints in `.deepseek/config.toml`.
- **Reasoning Tape**: view model reasoning, tool calls, and repair
  events in the chat stream or the `/tape` fullscreen view.
- **Two-Model Duet**: uses the Pro-capable provider path as a validator
  for destructive tool calls instead of invoking it on every turn.
- **Tool-rich agent loop**: file edits, patch application, bash, git,
  web fetch/search, LSP queries, subagents, worktrees, tasks, and user
  questions are exposed through function calling.
- **Sessions and resume**: stores sessions in pure-Go SQLite and
  supports `-c`, `-r`, and the `/sessions` picker.
- **Custom commands and skills**: loads `.deepseek/command/*.md` slash
  commands and discovers `SKILL.md` files from project and home
  directories.
- **MCP integration**: configured MCP servers are connected at startup
  and their tools are bridged into the agent registry.
- **Optional sandboxing**: when enabled in config, bash tools use the
  platform sandbox implementation available on the host.

## Install

### macOS / Linux (one-liner)

```sh
curl -fsSL https://raw.githubusercontent.com/amemiya02/deepseekcode/main/install.sh | sh
```

Installs `dsc` to `~/.local/bin`. Override with `PREFIX=/usr/local sh` or pin a version with `DSC_VERSION=v0.3.1 sh`.

### macOS (Homebrew)

```sh
brew install amemiya02/deepseekcode/deepseekcode
```

### Windows (Scoop)

```sh
scoop bucket add deepseekcode https://github.com/amemiya02/deepseekcode-scoop
scoop install deepseekcode
```

### Go

```sh
go install github.com/amemiya02/deepseekcode/cmd/dsc@latest
```

### Build from source

```sh
git clone https://github.com/amemiya02/deepseekcode
cd deepseekcode
make build
./bin/dsc -version
```

Prerequisites:

- Go version matching `go.mod` or newer.
- A DeepSeek API key, or another configured provider key.
- Git is optional, but enables git-aware prompt context and git tools.
- Language servers are optional; `dsc` connects to detected servers when
  they are present.

## Quick Start

```sh
export DEEPSEEK_API_KEY=sk-...

dsc
dsc -p "summarize this repository"
dsc --read-only -p "explain the architecture"
dsc init
dsc doctor
```

Useful session flags:

```sh
dsc -c                 # continue the most recent session in this project
dsc -r <session-id>    # resume a specific session
dsc -new               # force a new session
```

## Configuration

Configuration is layered from built-in defaults, user config,
project-local config, and CLI flags. Project-local configuration lives
at `./.deepseek/config.toml`; user-wide configuration lives at
`~/.deepseek/config.toml`.

Minimal DeepSeek configuration:

```toml
[active]
provider = "deepseek"

[providers.deepseek]
type = "deepseek"
base_url = "https://api.deepseek.com"
env_var = "DEEPSEEK_API_KEY"
first_token_timeout_ms = 45000
chunk_stall_timeout_ms = 20000

[defaults]
model = "deepseek-v4-flash"
thinking = true
```

OpenAI-compatible endpoints use the same provider mechanism:

```toml
[active]
provider = "openai"

[providers.openai]
type = "openai-compat"
base_url = "https://api.openai.com"
env_var = "OPENAI_API_KEY"
default_model = "gpt-4o"
```

See [docs/config.md](docs/config.md) and
[docs/PROVIDERS.md](docs/PROVIDERS.md) for the full field reference.

## CLI Reference

Top-level commands:

```sh
dsc                  # launch the TUI
dsc init             # create DEEPSEEK.md and .deepseek/config.toml
dsc doctor           # check config, provider, terminal, SQLite, git, MCP, LSP, and updates
dsc upgrade          # check the latest GitHub release and print an upgrade command
dsc agent list       # list project agents
dsc agent show NAME  # print an agent definition
dsc agent new NAME   # scaffold .deepseek/agent/NAME.md
dsc agent validate   # validate project agent definitions
dsc trace inspect TRACE.jsonl
```

Main flags:

```sh
-version          print build version
-p "prompt"      run one model turn and exit
-model ID        override the main-loop model
-read-only       block write, edit, and bash tools
-ask-all         ask before every tool call
-yolo            auto-approve all tool calls
-no-duet         disable the Pro validator hook
-debug           write structured logs under .deepseek/log/
-trace-jsonl P   write benchmark/diagnostic trace events to a JSONL file
```

## TUI Commands

Keys:

```text
Enter       send prompt
Shift+Enter insert newline
Ctrl+C      cancel current run, or quit when idle
Ctrl+D      quit
Ctrl+R      toggle the most recent thinking block
Ctrl+T      toggle all thinking blocks
PgUp/PgDn   scroll
```

Slash commands:

```text
/help       show keymap and commands
/clear      clear scrollback
/quit       exit
/models     list or switch the main-loop model
/tape       open the reasoning tape
/sessions   list this project's sessions
/export     open full scrollback in $PAGER
/undo       restore the previous edit step
/compact    force message compaction
```

Custom slash commands are loaded from `.deepseek/command/*.md` in the
project and home directories. Discovered skills are also promoted to
slash commands unless a user command with the same name exists.

## Built-in Tooling

`dsc` exposes tools through model function calling. The active set may
also include MCP tools from configured servers.

Core repository tools:

- `read_file`, `write_file`, `edit_file`, `apply_patch`
- `glob`, `grep`, `ls`
- `bash`, `bash_pty`, `background_bash`
- `git_diff`, `git_show`, `git_blame`, `git_log`
- `todo_write`, `task_status`, `question`

Context and extension tools:

- `lsp` for language-server-backed symbol queries when a server is
  detected.
- `skill_read` for loading discovered `SKILL.md` bodies on demand.
- `web_fetch` and `web_search` when web tooling is enabled in config.
- `task` for subagent dispatch and `worktree` for isolated git
  worktree management.

See [docs/tools.md](docs/tools.md) for tool parameters and safety notes.

## Safety Model

The permission policy is intentionally conservative:

- Read-only tools are allowed by default.
- File writes are allowed inside the current working directory unless
  the target path looks secret-like or unsafe.
- Bash is controlled by allowlist patterns and permission prompts.
- `--read-only`, `--ask-all`, and `--yolo` override the default policy.
- Destructive operations can be checked by the Duet validator hook.
- Mutating file tools snapshot affected paths before execution; `/undo`
  restores the last edit step.
- Optional sandboxing can be enabled for bash execution where the host
  platform supports it.

See [docs/permissions.md](docs/permissions.md) and
[docs/SANDBOX.md](docs/SANDBOX.md).

## Project Files

- [docs/config.md](docs/config.md) — configuration reference
- [docs/PROVIDERS.md](docs/PROVIDERS.md) — provider setup
- [docs/tools.md](docs/tools.md) — built-in tool surface
- [docs/commands.md](docs/commands.md) — custom slash commands
- [docs/skills.md](docs/skills.md) — skill discovery
- [docs/duet.md](docs/duet.md) — Pro validator behavior
- [docs/tape.md](docs/tape.md) — reasoning tape behavior
- [docs/upgrade.md](docs/upgrade.md) — upgrade command behavior
- [docs/MODEL_COMPATIBILITY.md](docs/MODEL_COMPATIBILITY.md) — DeepSeek wire-format facts & supported models

## Development

Common commands:

```sh
make build
make test
make test-race
make lint
make fmt
make tidy
make run
```

Run a focused test:

```sh
go test ./internal/llm/ -run TestThinkingSerializesAsStruct -v
```

Before opening a pull request, run:

```sh
make fmt
make lint
make test
```

## Versioning

Release builds are stamped by `make build` through `git describe`, so a
tagged build such as `v0.3.1` will appear in `dsc -version`. Use
`dsc upgrade` to check for and apply newer releases.

## Contributing

Issues and pull requests are welcome. Keep changes grounded in the
current CLI, TUI, config, and tool behavior; avoid documenting a feature
until it is implemented and testable in this repository.

For README changes, update both `README.md` and `README.zh-CN.md` with
matching `##` structure.

## License

MIT
