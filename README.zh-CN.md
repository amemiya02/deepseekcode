# deepseekcode

[![Release](https://img.shields.io/github/v/release/amemiya02/deepseekcode?style=flat-square&label=release)](https://github.com/amemiya02/deepseekcode/releases)
[![Go Reference](https://img.shields.io/badge/go-reference-007d9c?style=flat-square&logo=go&logoColor=white)](https://pkg.go.dev/github.com/amemiya02/deepseekcode)
[![Go Report Card](https://goreportcard.com/badge/github.com/amemiya02/deepseekcode?style=flat-square)](https://goreportcard.com/report/github.com/amemiya02/deepseekcode)
[![Go Version](https://img.shields.io/badge/go-1.26.3-00add8?style=flat-square&logo=go&logoColor=white)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue?style=flat-square)](LICENSE)

[English](README.md) · [简体中文](README.zh-CN.md)

`deepseekcode` 是面向 DeepSeek 模型和 OpenAI-compatible chat-completions
端点的终端编码 Agent。它以单个 Go 二进制 `dsc` 分发，包含交互式 TUI、
一次性 CLI 模式、仓库工具、SQLite 会话，以及保守的权限模型。

## 为什么选择 dsc（DeepSeek 深度特化）

- **可证明的前缀缓存稳定性** -- 单一 canonical 序列化器同时喂给 wire
  字节和缓存指纹，二者构造级一致不可能发散；`dsc trace inspect` 的
  `prefixes==1` 可证明整次会话缓存稳定
  （[docs/prefix-cache.md](docs/prefix-cache.md)）。
- **真实的信号驱动 Flash→Pro 路由** -- 通过 `--auto-route` 选择启用
  （在歧义或反复修复时自动升级），而非仅是 prompt 指令；破坏性工具调用
  始终触发 Duet pro 校验器（[docs/duet.md](docs/duet.md)）。
- **真实的 OS 沙箱** -- sandbox-exec (macOS) / Landlock (Linux) 真隔离 +
  真 PTY，不只是路径围栏
  （[docs/SANDBOX.md](docs/SANDBOX.md)）。
- **自动推理强度** -- 逐回合按多语言关键词自动开关 thinking，加
  `low/medium/high/max` 推理档位，简单任务自动降档。

## DeepSeek V4 Agent 故事

DeepSeekCode 专为 DeepSeek V4 模型构建，相比通用编码 Agent 具有可验证的优势：

> **实测，而非声称。** 在一次可复现的 12 回合 `deepseek-v4-flash` 会话中，
> deepseekcode 维持 **94.7% 前缀缓存命中率**，而缓存不稳定的 Agent 为 **0%**
> —— **便宜 4.5×**，每个数字都取自 DeepSeek 自己的 `prompt_cache_hit_tokens`。
> 自行验证：`make demo-cache`（实测）或 `make demo-cache-offline`（无需 API
> key）。方法学：[bench/cache-demo](bench/cache-demo/)。

- **可证明的前缀缓存稳定性** — 单一 canonical 序列化器保证 wire 字节
  与缓存指纹构造级一致；`dsc trace inspect` 可验证
  （[docs/prefix-cache.md](docs/prefix-cache.md)）。
- **选择性 Pro 校验** — 破坏性工具调用获得第二次
  `deepseek-v4-pro` 判断，而非每回合都调用；低成本加安全保障
  （[docs/duet.md](docs/duet.md)）。
- **1M 上下文工作流** — 大仓库阅读和长历史会话无需激进压缩；1M 上下文
  是使能器，不是工具和检索的替代。
- **工具调用修复管道** — 自动修复截断 JSON、从推理文本中回收调用、
  风暴守卫抑制重复失败调用。

完整故事及本地验证命令：
[docs/deepseek-v4-agent-story.md](docs/deepseek-v4-agent-story.md)

基准证据：
[bench/README.md](bench/README.md)

工具调用基准（tau-bench-lite，8 个任务）：dsc 在每次解题成本（~$0.00038）上与强 DeepSeek 原生 flash 基线持平——我们如实报告持平，不在简单工具调用任务上声称能力领先（路由优势需要更难的任务集才能体现）。见 [bench/taubench](bench/taubench/)。

## 功能

- 交互式 Bubble Tea TUI，以及可脚本化的 `dsc -p "prompt"` 模式。
- DeepSeek-first runtime，支持 thinking、`reasoning_effort` 控制、长上下文、
  前缀缓存指标，以及 `deepseek-v4-flash` / `deepseek-v4-pro`。
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
- [DeepSeek V4 Agent 故事](docs/deepseek-v4-agent-story.md)

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

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=amemiya02/deepseekcode&type=Date)](https://star-history.com/#amemiya02/deepseekcode&Date)

## 许可证

MIT
