# deepseekcode

[![Release](https://img.shields.io/github/v/release/amemiya02/deepseekcode?style=flat-square&label=release)](https://github.com/amemiya02/deepseekcode/releases)
[![Go Reference](https://img.shields.io/badge/go-reference-007d9c?style=flat-square&logo=go&logoColor=white)](https://pkg.go.dev/github.com/amemiya02/deepseekcode)
[![Go Report Card](https://goreportcard.com/badge/github.com/amemiya02/deepseekcode?style=flat-square)](https://goreportcard.com/report/github.com/amemiya02/deepseekcode)
[![Go Version](https://img.shields.io/badge/go-1.26.3-00add8?style=flat-square&logo=go&logoColor=white)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue?style=flat-square)](LICENSE)

[English](README.md) · [简体中文](README.zh-CN.md)

Terminal-native coding agent for DeepSeek models. Single Go binary, three
interfaces (TUI / Web SPA / Desktop), provable prefix-cache stability, real OS
sandbox, and signal-driven model routing.

## Why dsc

> **Measured, not claimed.** 94.7% prefix cache-hit rate on `deepseek-v4-flash`
> vs 0% for a cache-naive agent — **4.5× cheaper**. Every figure from
> DeepSeek's own `prompt_cache_hit_tokens`. Reproduce: `make demo-cache`.
> Evidence: [bench/](bench/README.md).

- **Provable prefix-cache stability** — single canonical serializer for wire
  bytes and cache fingerprint; `dsc trace inspect` shows `prefixes==1`
  ([docs/prefix-cache.md](docs/prefix-cache.md)).
- **Signal-driven Flash→Pro routing** — escalates on ambiguity / repeated
  repair; Duet pro-validator on destructive calls
  ([docs/duet.md](docs/duet.md)).
- **Real OS sandbox** — sandbox-exec (macOS) / Landlock (Linux) with real PTY
  ([docs/SANDBOX.md](docs/SANDBOX.md)).
- **Auto reasoning-effort** — per-turn thinking on/off via keyword detection;
  `low/medium/high/max` effort, auto-dialed on simple tasks.

## Interfaces

| Interface | Command | Description |
|---|---|---|
| **TUI** | `dsc` | Interactive Bubble Tea terminal UI (default) |
| **Web SPA** | `dsc serve --http :7432` | Svelte SPA with chat, file tree, diff viewer, settings |
| **Desktop** | `make desktop` | Native macOS `.app` via Wails v3 |

Build web-embedded binary: `make build-web`.

## Features

- **14 built-in tools** — file read/write/edit/patch, bash, glob, grep, git
  (diff/show/blame/log), questions, todo tracking.
- **Subagents** — define in `.deepseek/agent/<name>.md`; built-in profiles for
  explore, implement, review, autonomous.
- **Skills** — `SKILL.md` discovery from `.deepseek/skills/`, `.claude/skills/`,
  etc. Cache-stable progressive disclosure. Auto-promoted to slash commands.
- **Hooks** — `PreToolUse`, `PostToolUse`, `SessionStart`, `SessionEnd`.
  Fail-open. In-process or subprocess.
- **Memory** — persistent BM25-indexed long-term memory, JSONL storage, SHA
  dedup, near-duplicate reconciliation.
- **CodeGraph** — tree-sitter knowledge graph for structural queries
  (callers/callees/definitions). Available as MCP server.
- **MCP** — Model Context Protocol servers via config. None enabled by default.
- **Snapshots & /undo** — pre-mutation file snapshots. `/undo` restores last
  step. Atomic rollback for multi-file patches.
- **Permissions** — read-only mode (`--read-only`), ask-all (`--ask-all`),
  auto-approve (`--yolo`). Per-pattern bash gates.

## Installation

```sh
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/amemiya02/deepseekcode/main/install.sh | sh

# Homebrew
brew install amemiya02/deepseekcode/deepseekcode

# Scoop
scoop bucket add deepseekcode https://github.com/amemiya02/deepseekcode-scoop
scoop install deepseekcode

# Go
go install github.com/amemiya02/deepseekcode/cmd/dsc@latest

# From source
git clone https://github.com/amemiya02/deepseekcode && cd deepseekcode
make build && ./bin/dsc -version
```

Requires: `DEEPSEEK_API_KEY` (or configured provider key). Git and LSP optional.

## Quick Start

```sh
export DEEPSEEK_API_KEY=sk-...

dsc                              # open the TUI
dsc -p "summarize this repo"     # one-shot CLI
dsc --read-only                  # inspect only
dsc -c                           # continue last session
dsc -r <session-id>              # resume specific session
dsc init                         # project config scaffold
dsc doctor                       # check local setup
```

## Configuration

Layered: built-in defaults → `~/.deepseek/config.toml` → `./.deepseek/config.toml` → CLI flags.

```toml
# Minimal DeepSeek
[active]
provider = "deepseek"

[providers.deepseek]
type = "deepseek"
base_url = "https://api.deepseek.com"
env_var = "DEEPSEEK_API_KEY"

[defaults]
model = "deepseek-v4-flash"
thinking = true

# OpenAI-compatible
# [active]
# provider = "openai"
#
# [providers.openai]
# type = "openai-compat"
# base_url = "https://api.openai.com"
# env_var = "OPENAI_API_KEY"
# default_model = "gpt-4o"
```

Full reference: [docs/config.md](docs/config.md) · [docs/PROVIDERS.md](docs/PROVIDERS.md)

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `DEEPSEEK_API_KEY` | *(required)* | DeepSeek API key |
| `DEEPSEEKCODE_BASE_URL` | `https://api.deepseek.com` | API base URL (set mirror for China-mainland) |
| `DEEPSEEKCODE_PROXY` | *(none)* | HTTP/HTTPS proxy; overrides `HTTPS_PROXY` |
| `DEEPSEEKCODE_LANG` | auto | UI locale (`zh-CN`, `en`) |

## Documentation

[Config](docs/config.md) · [Providers](docs/PROVIDERS.md) · [Tools](docs/tools.md) · [Permissions](docs/permissions.md) · [Sandbox](docs/SANDBOX.md) · [Skills](docs/skills.md) · [Hooks](docs/hooks.md) · [MCP](docs/mcp.md) · [LSP](docs/lsp.md) · [Prefix Cache](docs/prefix-cache.md) · [Duet](docs/duet.md) · [Pricing](docs/pricing.md) · [Web SPA](docs/WEB.md)

## Development

```sh
make build       # build ./bin/dsc
make build-web   # build with embedded web SPA
make desktop     # build macOS .app
make test        # go test ./...
make test-race   # go test -race ./...
make lint        # go vet ./...
make fmt         # gofmt -s -w .
make ci          # SPA tests + Go tests
```

PR checklist: `make fmt && make lint && make test`

## Contributing

Issues and pull requests welcome. Keep `README.md` and `README.zh-CN.md`
synchronized with matching `##` structure. Document only implemented, testable
features.

## Acknowledgments

Cache design inspired by [reasonix](https://github.com/esengine/deepseek-reasonix).

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=amemiya02/deepseekcode&type=Date)](https://star-history.com/#amemiya02/deepseekcode&Date)

## License

MIT
