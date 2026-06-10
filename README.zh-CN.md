# deepseekcode

[![Release](https://img.shields.io/github/v/release/amemiya02/deepseekcode?style=flat-square&label=release)](https://github.com/amemiya02/deepseekcode/releases)
[![Go Reference](https://img.shields.io/badge/go-reference-007d9c?style=flat-square&logo=go&logoColor=white)](https://pkg.go.dev/github.com/amemiya02/deepseekcode)
[![Go Report Card](https://goreportcard.com/badge/github.com/amemiya02/deepseekcode?style=flat-square)](https://goreportcard.com/report/github.com/amemiya02/deepseekcode)
[![Go Version](https://img.shields.io/badge/go-1.26.3-00add8?style=flat-square&logo=go&logoColor=white)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue?style=flat-square)](LICENSE)

[English](README.md) · [简体中文](README.zh-CN.md)

面向 DeepSeek 模型的终端编码 Agent。单个 Go 二进制，三种界面（TUI / Web SPA
/ 桌面应用），可证明的前缀缓存稳定性、真实 OS 沙箱和信号驱动的模型路由。

## 为什么选择 dsc

> **实测，而非声称。** `deepseek-v4-flash` 上 94.7% 前缀缓存命中率，缓存不
> 稳定的 Agent 为 0% —— **便宜 4.5×**。每个数字取自 DeepSeek 自己的
> `prompt_cache_hit_tokens`。自行复现：`make demo-cache`。
> 证据：[bench/](bench/README.md)。
>
> **正面对决 (2026-06-10)** vs Reasonix，5 个真实开源 issue（gRPC、Cobra、
> chi）：dsc 解决 **7/10**（70%）vs Reasonix **6/10**（60%）；Reasonix 缓存
> 命中 93.9% vs dsc 89.6%，但因超回合上限 DNF 3 次。
> [完整结果](bench/README.md#h2h-cache-benchmark-2026-06-10)。

- **可证明的前缀缓存稳定性** — 单一 canonical 序列化器同时喂给 wire 字节和
  缓存指纹，构造级一致不可能发散；`dsc trace inspect` 的 `prefixes==1`
  可证明缓存稳定（[docs/prefix-cache.md](docs/prefix-cache.md)）。
- **信号驱动 Flash→Pro 路由** — 在歧义或反复修复时自动升级；破坏性调用触发
  Duet pro 校验器（[docs/duet.md](docs/duet.md)）。
- **真实 OS 沙箱** — sandbox-exec (macOS) / Landlock (Linux) 真隔离 + 真 PTY
  （[docs/SANDBOX.md](docs/SANDBOX.md)）。
- **自动推理强度** — 逐回合按关键词自动开关 thinking，`low/medium/high/max`
  档位，简单任务自动降档。

## 界面

| 界面 | 命令 | 说明 |
|---|---|---|
| **TUI** | `dsc` | 交互式 Bubble Tea 终端 UI（默认） |
| **Web SPA** | `dsc serve --http :7432` | Svelte 单页应用，含聊天、文件树、diff 查看器、设置 |
| **桌面应用** | `make desktop` | Wails v3 原生 macOS `.app` |

构建内嵌 SPA 的二进制：`make build-web`。

## 功能

- **14 个内置工具** — 文件读写/编辑/patch、bash、glob、grep、git
  （diff/show/blame/log）、用户提问、todo 跟踪。
- **子 Agent** — 在 `.deepseek/agent/<name>.md` 定义；内置 explore、implement、
  review、autonomous profile。
- **Skills** — 从 `.deepseek/skills/`、`.claude/skills/` 等发现 `SKILL.md`。
  缓存安全的渐进式披露。自动提升为斜杠命令。
- **Hooks** — `PreToolUse`、`PostToolUse`、`SessionStart`、`SessionEnd`。
  失败即放行。支持进程内或子进程。
- **Memory** — 持久化 BM25 索引长期记忆，JSONL 存储，SHA 去重，近似重复对齐。
- **CodeGraph** — Tree-sitter 知识图谱，结构化查询（调用者/被调用者/定义）。
  可作为 MCP 服务器。
- **MCP** — 通过配置支持 Model Context Protocol 服务器。默认不启用。
- **快照与 /undo** — 修改前自动快照文件。`/undo` 恢复上一步。多文件 patch
  原子回滚。
- **权限** — 只读模式（`--read-only`）、全部确认（`--ask-all`）、
  自动批准（`--yolo`）。bash 按模式门控。

## 安装

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

# 从源码构建
git clone https://github.com/amemiya02/deepseekcode && cd deepseekcode
make build && ./bin/dsc -version
```

要求：`DEEPSEEK_API_KEY`（或已配置的 provider key）。Git 和 LSP 可选。

## 快速开始

```sh
export DEEPSEEK_API_KEY=sk-...

dsc                              # 打开 TUI
dsc -p "summarize this repo"     # 单次 CLI
dsc --read-only                  # 只读检查
dsc -c                           # 继续上次会话
dsc -r <session-id>              # 恢复指定会话
dsc init                         # 项目配置脚手架
dsc doctor                       # 检查本地环境
```

## 配置

层级叠加：内置默认值 → `~/.deepseek/config.toml` → `./.deepseek/config.toml`
→ CLI flags。

```toml
# 最小 DeepSeek 配置
[active]
provider = "deepseek"

[providers.deepseek]
type = "deepseek"
base_url = "https://api.deepseek.com"
env_var = "DEEPSEEK_API_KEY"

[defaults]
model = "deepseek-v4-flash"
thinking = true

# OpenAI-compatible 端点
# [active]
# provider = "openai"
#
# [providers.openai]
# type = "openai-compat"
# base_url = "https://api.openai.com"
# env_var = "OPENAI_API_KEY"
# default_model = "gpt-4o"
```

完整参考：[docs/config.md](docs/config.md) · [docs/PROVIDERS.md](docs/PROVIDERS.md)

## 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `DEEPSEEK_API_KEY` | *(必填)* | DeepSeek API 密钥 |
| `DEEPSEEKCODE_BASE_URL` | `https://api.deepseek.com` | API 基础 URL（大陆可设镜像） |
| `DEEPSEEKCODE_PROXY` | *(无)* | HTTP/HTTPS 代理；优先于 `HTTPS_PROXY` |
| `DEEPSEEKCODE_LANG` | 自动检测 | UI 语言（`zh-CN`、`en`） |

## 文档

[配置](docs/config.md) · [Providers](docs/PROVIDERS.md) · [工具](docs/tools.md) · [权限](docs/permissions.md) · [沙箱](docs/SANDBOX.md) · [Skills](docs/skills.md) · [Hooks](docs/hooks.md) · [MCP](docs/mcp.md) · [LSP](docs/lsp.md) · [前缀缓存](docs/prefix-cache.md) · [Duet](docs/duet.md) · [价格](docs/pricing.md) · [Web SPA](docs/WEB.md)

## 开发

```sh
make build       # 构建 ./bin/dsc
make build-web   # 构建内嵌 web SPA 的版本
make desktop     # 构建 macOS .app
make test        # go test ./...
make test-race   # go test -race ./...
make lint        # go vet ./...
make fmt         # gofmt -s -w .
make ci          # SPA 测试 + Go 测试
```

PR 检查清单：`make fmt && make lint && make test`

## 参与贡献

欢迎提交 issue 和 pull request。修改 README 时，请同步更新 `README.md` 和
`README.zh-CN.md`，并保持匹配的 `##` 结构。只记录已实现且可测试的功能。

## 致谢

缓存设计参考了 [reasonix](https://github.com/esengine/deepseek-reasonix)。

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=amemiya02/deepseekcode&type=Date)](https://star-history.com/#amemiya02/deepseekcode&Date)

## 许可证

MIT
