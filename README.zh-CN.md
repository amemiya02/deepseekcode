# deepseekcode

[![Go Reference](https://pkg.go.dev/badge/github.com/amemiya02/deepseekcode.svg)](https://pkg.go.dev/github.com/amemiya02/deepseekcode)

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README.md) · [简体中文](README.zh-CN.md)

`deepseekcode` 是面向 DeepSeek 模型和 OpenAI-compatible chat-completions
端点的终端编码 Agent。它以单个 Go 二进制 `dsc` 分发，包含交互式 TUI、
一次性 CLI 模式、仓库工具、SQLite 会话，以及保守的权限模型。

## 功能

- 交互式 Bubble Tea TUI，以及可脚本化的 `dsc -p "prompt"` 模式。
- DeepSeek-first runtime，支持 thinking、长上下文、前缀缓存指标，以及
  `deepseek-v4-flash` / `deepseek-v4-pro`。
- 可配置 OpenAI-compatible provider，用于接入其他 chat-completions 端点。
- 仓库工具覆盖文件读写、patch、shell 命令、git、grep、LSP 查询、网页抓取/搜索和用户提问。
- 持久化项目会话，支持恢复、分支、导出 scrollback，以及用 `/undo` 撤销最近编辑 step。
- 可通过自定义 slash 命令、`SKILL.md` 发现、MCP 工具、子 Agent 和隔离 git worktree 扩展。
- 安全控制包括只读模式、工具前确认模式、自动批准模式、敏感路径检查、bash allowlist、可选沙箱，以及破坏性操作的 Pro 校验。

## 安装

macOS / Linux：

```sh
curl -fsSL https://raw.githubusercontent.com/amemiya02/deepseekcode/main/install.sh | sh
```

Homebrew：

```sh
brew install amemiya02/deepseekcode/deepseekcode
```

Scoop：

```sh
scoop bucket add deepseekcode https://github.com/amemiya02/deepseekcode-scoop
scoop install deepseekcode
```

Go：

```sh
go install github.com/amemiya02/deepseekcode/cmd/dsc@latest
```

从源码构建：

```sh
git clone https://github.com/amemiya02/deepseekcode
cd deepseekcode
make build
./bin/dsc -version
```

要求：

- 从源码构建时，需要与 `go.mod` 匹配或更新的 Go 版本。
- `DEEPSEEK_API_KEY`，或在 `.deepseek/config.toml` 中配置其他 provider key。
- Git 和 language server 是可选依赖，但能提供更完整的仓库上下文。

## 快速开始

```sh
export DEEPSEEK_API_KEY=sk-...

dsc                              # 打开 TUI
dsc -p "summarize this repo"     # 运行一次 prompt 后退出
dsc --read-only                  # 只检查，不允许 write/edit/bash 工具
dsc init                         # 创建项目初始配置
dsc doctor                       # 检查本地环境
```

会话快捷参数：

```sh
dsc -c                 # 继续当前项目最近一次会话
dsc -r <session-id>    # 恢复指定会话
dsc -new               # 强制创建新会话
```

## 配置

配置按内置默认值、用户配置、项目配置、CLI flags 的顺序叠加。项目配置位于
`./.deepseek/config.toml`；用户配置位于 `~/.deepseek/config.toml`。

最小 DeepSeek 配置：

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

OpenAI-compatible 端点：

```toml
[active]
provider = "openai"

[providers.openai]
type = "openai-compat"
base_url = "https://api.openai.com"
env_var = "OPENAI_API_KEY"
default_model = "gpt-4o"
```

完整说明见 [docs/config.md](docs/config.md) 和
[docs/PROVIDERS.md](docs/PROVIDERS.md)。

## 文档

- [配置](docs/config.md)
- [Providers](docs/PROVIDERS.md)
- [工具](docs/tools.md)
- [权限](docs/permissions.md)
- [沙箱](docs/SANDBOX.md)
- [自定义命令](docs/commands.md)
- [Skills](docs/skills.md)
- [MCP](docs/mcp.md)
- [LSP](docs/lsp.md)
- [Reasoning tape](docs/tape.md)
- [模型兼容性](docs/MODEL_COMPATIBILITY.md)

## 开发

```sh
make build       # 构建 ./bin/dsc
make test        # go test ./...
make test-race   # go test -race ./...
make lint        # go vet ./...
make fmt         # gofmt -s -w .
make run         # 构建并启动 TUI
```

提交 pull request 前：

```sh
make fmt
make lint
make test
```

## 参与贡献

欢迎提交 issue 和 pull request。修改 README 时，请同步更新 `README.md` 和
`README.zh-CN.md`，并保持匹配的 `##` 结构。只记录本仓库中已经实现且可测试的功能。

## 许可证

MIT
