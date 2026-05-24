# deepseekcode

[English](README.md) · [简体中文](README.zh-CN.md)

为 DeepSeek 模型量身打造的终端原生编码 Agent。单一 Go 二进制，无运行时
依赖。围绕"推理过程"、"缓存经济学"和"双模型协奏（精准而不喧嚣）"
做出了与众不同的 UX。

> 状态：v0.1 开发中，目标是提交到
> [awesome-deepseek-agent](https://github.com/deepseek-ai/awesome-deepseek-agent)。
> 完整设计与权衡见 [`docs/design.md`](docs/design.md)。

## 与众不同之处

- **Reasoning Tape（推理磁带）** — DeepSeek 将 `reasoning_content` 作为
  独立通道返回。我们把它渲染成可折叠的内联时间线，并提供一个全屏可滚动
  的 `/tape` 视图，可以浏览并基于任意历史步骤分叉新会话。
  ([docs/tape.md](docs/tape.md))
- **Two-Model Duet —— Pro Validator（双模型协奏 —— Pro 校验器）** ——
  `deepseek-v4-flash` 驱动主循环，`pro` 仅在破坏性操作时作为校验器被精准
  调用，而不是每一轮都跑。([docs/duet.md](docs/duet.md))
- **Cost HUD（成本仪表盘）** —— 状态栏实时显示缓存命中率与 ¥/$ 消耗。
  让 DeepSeek 50 倍的缓存命中折扣变成一个你能盯着看的旋钮。
- **Structured git（结构化 git 工具）** —— `git_diff`、`git_show`、
  `git_blame`、`git_log` 全部以类型化工具的形式提供，而不是 pager 输出
  的包装器。
- **Session branching（会话分叉）** —— 可从任意历史步骤分叉出子会话。
  基于 SQLite 父子引用，零消息拷贝，分叉成本极低。

## 安装

```sh
# Homebrew（v0.1.0 发布后）
brew install amemiya02/deepseekcode/deepseekcode

# curl | sh
curl -fsSL https://deepseekcode.dev/install.sh | sh

# Go install
go install github.com/amemiya02/deepseekcode/cmd/dsc@latest

# 源码构建
git clone https://github.com/amemiya02/deepseekcode && cd deepseekcode && make build
```

完整安装矩阵见 [docs/install.md](docs/install.md)。

## 快速开始

```sh
export DEEPSEEK_API_KEY=sk-...
dsc                                # 启动 TUI
dsc -p "解释一下 pkg/auth 的实现"   # 单次提示 → 输出到 stdout，然后退出
dsc --read-only                    # 安全探索模式（禁止任何写操作）
dsc --yolo -p "跑一下测试"         # 自动放行所有工具调用（用于 CI / 自动化）
```

在 TUI 内：

```
⏎          发送提示
^C         中断当前运行（空闲时为退出）
^D         退出
r / R      展开/折叠 最近一个 / 所有 思考块
/help      键位 + 命令一览
/models    列出 / 切换主循环模型
/tape      打开推理磁带
/sessions  列出本项目的所有会话
/undo      回滚最近一次编辑
/compact   强制压缩当前会话消息列表
```

## 环境变量

| 名称 | 默认值 | 作用 |
|------|--------|------|
| `DEEPSEEK_API_KEY` | （必填） | DeepSeek API 凭据。 |
| `DEEPSEEKCODE_AUTO_COMPACT_INPUT_TOKENS` | `100000` | 自动会话压缩触发阈值。当估算 token 数超过该值时，较早的消息被折叠为一条摘要消息。会话较长时调小，几乎不想触发时调大。 |

## 一段话讲清架构

ReAct 主循环（回调驱动，参考 Crush 的 `internal/agent/agent.go`），构建在
一个手写的 DeepSeek 客户端之上（~400 行 HTTP+SSE+类型化事件，零外部 SDK）。
Bubble Tea TUI，含可折叠推理块与实时 Cost HUD。会话持久化使用纯 Go 的
SQLite（`modernc.org/sqlite`，无 CGO），让 `--continue` / `--resume` /
分叉这些能力不破坏"单二进制分发"的故事。工具调用支持并行，受分层权限
策略与快照回滚（`/undo`）保护。Pro Validator（即 Duet）作为结构化输出
裁判，坐镇模型与破坏性操作之间。

塑造了本项目设计的参考仓库：`charmbracelet/crush`（Go、回调 ReAct）、
`sst/opencode`（finish-reason 覆盖）、`cline/cline`（流到达 / 渲染拆分）、
`plandex-ai/plandex`（两段式流超时）。详见 `docs/design.md` §3。

## 文档

- [设计文档](docs/design.md) — 完整的架构、决策与权衡
- [安装](docs/install.md)
- [配置](docs/config.md)
- [工具](docs/tools.md)
- [推理磁带 (`/tape`)](docs/tape.md) — 头号特性
- [Two-Model Duet](docs/duet.md) — 第二头号特性
- [MCP](docs/mcp.md)

## 状态与路线图

**v0.1（当前）**：12 个内建工具、分层权限 + 快照回滚、带按引用分叉的
SQLite 会话、Reasoning Tape + `/tape` 全屏、`/models` 选择器、Pro
Validator、Cost HUD、五平台交叉编译、Homebrew tap + curl|sh + go install。
MCP 推迟。子 Agent 推迟（已预留 Spawner 接口骨架）。

**v0.2**：子 Agent · 进程沙箱（bubblewrap / sandbox-exec）· `web_fetch` ·
Anthropic 格式端点 · `/sessions` 树形选择器 · 可分享的 Tape 导出
（`dsc tape export`）。

## 参与贡献

欢迎 PR。设计文档（`docs/design.md`）写明了 v0.1 的范围、舍弃项、以及
若进度吃紧时的"砍特性顺序"。请尽量在这个轮廓内贡献；如果你想扩展范围，
请先开 issue 聊一聊。

## 协议

MIT
