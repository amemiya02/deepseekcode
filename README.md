# deepseekcode

[![Release](https://img.shields.io/github/v/release/amemiya02/deepseekcode?style=flat-square&label=release)](https://github.com/amemiya02/deepseekcode/releases)
[![Go Reference](https://img.shields.io/badge/go-reference-007d9c?style=flat-square&logo=go&logoColor=white)](https://pkg.go.dev/github.com/amemiya02/deepseekcode)
[![Go Report Card](https://goreportcard.com/badge/github.com/amemiya02/deepseekcode?style=flat-square)](https://goreportcard.com/report/github.com/amemiya02/deepseekcode)
[![Go Version](https://img.shields.io/badge/go-1.26.3-00add8?style=flat-square&logo=go&logoColor=white)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue?style=flat-square)](LICENSE)

[English](README.md) · [简体中文](README.zh-CN.md)

`deepseekcode` is a terminal coding agent for DeepSeek models and
OpenAI-compatible chat-completions endpoints. It ships as a single Go binary
named `dsc`, with an interactive TUI, one-shot CLI mode, repository tools,
SQLite-backed sessions, and a conservative permission model.

## Why dsc (DeepSeek specialization)

> **Measured, not claimed.** In a reproducible 12-turn session on
> `deepseek-v4-flash`, dsc holds a **94.7% prefix cache-hit rate** vs **0%** for
> a cache-naive agent — **4.5× cheaper**, every figure read from DeepSeek's own
> `prompt_cache_hit_tokens`. Reproduce: `make demo-cache` (live) or
> `make demo-cache-offline` (no API key). Evidence: [bench/](bench/README.md).

- **Provable prefix-cache stability** — a single canonical serializer feeds both
  the wire bytes and the cache fingerprint, so they cannot diverge by
  construction; `dsc trace inspect` shows `prefixes==1` to prove a stable run
  ([docs/prefix-cache.md](docs/prefix-cache.md)).
- **Real, signal-driven Flash→Pro routing** — opt-in via `--auto-route`
  (escalates on ambiguity / repeated repair), not just a prompt instruction; an
  always-on Duet pro-validator fires on destructive tool calls
  ([docs/duet.md](docs/duet.md)).
- **Real OS sandbox** — sandbox-exec (macOS) / Landlock (Linux) with a real PTY,
  not just path containment ([docs/SANDBOX.md](docs/SANDBOX.md)).
- **Auto reasoning-effort** — per-turn thinking on/off via multi-language
  keyword detection plus `low/medium/high/max` effort, dialed down automatically
  on simple tasks.

On tool-use (tau-bench-lite, 8 tasks) dsc is at **parity** on cost-per-solved
(~$0.00038) with a strong DeepSeek-native flash baseline — reported honestly,
with no capability claim on easy tasks ([bench/taubench](bench/taubench/)).

## Features

- Interactive Bubble Tea TUI and scriptable `dsc -p "prompt"` mode.
- DeepSeek-first runtime with thinking, `reasoning_effort` control, long context,
  prefix-cache metrics, and `deepseek-v4-flash` / `deepseek-v4-pro` support.
- OpenAI-compatible provider configuration for alternate chat-completions
  endpoints.
- Repository tools for file reads/edits, patches, shell commands, git, grep,
  LSP queries, web fetch/search, and user questions.
- Persistent project sessions with resume, branching, scrollback export, and
  `/undo` for recent edit steps.
- Extensibility through custom slash commands, `SKILL.md` discovery, MCP tools,
  subagents, and isolated git worktrees.
- Safety controls for read-only mode, ask-before-tool mode, auto-approve mode,
  secret path checks, bash allowlists, optional sandboxing, and Pro validation
  for destructive operations.

## Installation

macOS / Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/amemiya02/deepseekcode/main/install.sh | sh
```

Homebrew:

```sh
brew install amemiya02/deepseekcode/deepseekcode
```

Scoop:

```sh
scoop bucket add deepseekcode https://github.com/amemiya02/deepseekcode-scoop
scoop install deepseekcode
```

Go:

```sh
go install github.com/amemiya02/deepseekcode/cmd/dsc@latest
```

From source:

```sh
git clone https://github.com/amemiya02/deepseekcode
cd deepseekcode
make build
./bin/dsc -version
```

Requirements:

- Go version matching `go.mod` or newer when building from source.
- `DEEPSEEK_API_KEY`, or another provider key configured in `.deepseek/config.toml`.
- Git and language servers are optional, but enable richer repository context.

## Quick Start

```sh
export DEEPSEEK_API_KEY=sk-...

dsc                              # open the TUI
dsc -p "summarize this repo"     # run one prompt and exit
dsc --read-only                  # inspect without write/edit/bash tools
dsc init                         # create starter project config
dsc doctor                       # check local setup
```

Session shortcuts:

```sh
dsc -c                 # continue the latest session in this project
dsc -r <session-id>    # resume a specific session
dsc -new               # force a new session
```

## Configuration

Configuration is layered from built-in defaults, user config, project config,
and CLI flags. Project config lives at `./.deepseek/config.toml`; user config
lives at `~/.deepseek/config.toml`.

Minimal DeepSeek config:

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

OpenAI-compatible endpoint:

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
[docs/PROVIDERS.md](docs/PROVIDERS.md) for the full reference.

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `DEEPSEEK_API_KEY` | *(required)* | DeepSeek API key |
| `DEEPSEEKCODE_BASE_URL` | `https://api.deepseek.com` | API base URL; set to a mirror for China-mainland access |
| `DEEPSEEKCODE_PROXY` | *(none)* | Explicit HTTP/HTTPS proxy URL; overrides `HTTPS_PROXY`/`HTTP_PROXY` |
| `DEEPSEEKCODE_LANG` | auto-detected | UI locale override (`zh-CN`, `en`); falls back to `LANG` |

## Documentation

- [Configuration](docs/config.md)
- [Providers](docs/PROVIDERS.md)
- [Tools](docs/tools.md)
- [Permissions](docs/permissions.md)
- [Sandboxing](docs/SANDBOX.md)
- [Custom commands](docs/commands.md)
- [Skills](docs/skills.md)
- [MCP](docs/mcp.md)
- [LSP](docs/lsp.md)
- [Pricing](docs/pricing.md)
- [Model compatibility](docs/MODEL_COMPATIBILITY.md)

## Development

```sh
make build       # build ./bin/dsc
make test        # go test ./...
make test-race   # go test -race ./...
make lint        # go vet ./...
make fmt         # gofmt -s -w .
make run         # build and launch the TUI
```

Before opening a pull request:

```sh
make fmt
make lint
make test
```

## Contributing

Issues and pull requests are welcome. Keep README changes synchronized between
`README.md` and `README.zh-CN.md` with matching `##` structure. Document only
features that are implemented and testable in this repository.

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=amemiya02/deepseekcode&type=Date)](https://star-history.com/#amemiya02/deepseekcode&Date)

## License

MIT
