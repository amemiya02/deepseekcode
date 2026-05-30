# deepseekcode

[![CI](https://github.com/amemiya02/deepseekcode/actions/workflows/ci.yml/badge.svg)](https://github.com/amemiya02/deepseekcode/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/amemiya02/deepseekcode.svg)](https://pkg.go.dev/github.com/amemiya02/deepseekcode)
[![Go Report Card](https://goreportcard.com/badge/github.com/amemiya02/deepseekcode)](https://goreportcard.com/report/github.com/amemiya02/deepseekcode)
[![Baseline](https://img.shields.io/badge/baseline-v0.3.1-blue)](#版本管理)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README.md) · [简体中文](README.zh-CN.md)

`deepseekcode` 是面向 DeepSeek 模型和 OpenAI-compatible
chat-completions 端点的终端原生编码 Agent。它以单个 Go 二进制 `dsc`
分发，包含 Bubble Tea TUI、一次性 CLI 模式、结构化工具、SQLite 会话、
缓存友好的请求序列化，以及适合日常仓库工作的权限模型。

当前文档基线版本：**v0.3.1**。

## 为什么选择 deepseekcode

DeepSeek 暴露了推理内容、前缀缓存指标、长上下文和成本特征，这些能力值得
被做成一等 UX，而不是藏在通用聊天壳里。

- 用可折叠 thinking block 和全屏 `/tape` 时间线保留推理过程。
- 在 TUI 和 CLI step 输出里展示缓存命中与用量，让成本可见。
- 用结构化文件工具、编辑前快照、`/undo` 和类型化 git 工具让改动可审查。
- 用分层权限、敏感路径检查、bash allowlist 和 Pro validator hook 约束危险操作。

## 功能

- **TUI 和一次性模式**：运行 `dsc` 进入交互式终端 UI，或用
  `dsc -p "prompt"` 获得可脚本化的 stdout 输出。
- **DeepSeek-first runtime**：默认使用 `deepseek-v4-flash`，支持
  `deepseek-v4-pro`、DeepSeek thinking 选项、前缀缓存指标和 JSON-mode
  校验。
- **OpenAI-compatible providers**：可在 `.deepseek/config.toml` 中配置
  其他 chat-completions 端点。
- **Reasoning Tape**：在聊天流或 `/tape` 全屏视图中查看模型推理、工具调用
  和修复事件。
- **Two-Model Duet**：只在破坏性工具调用前使用 Pro-capable provider path
  做校验，而不是每一轮都调用。
- **丰富的 Agent 工具**：文件编辑、patch 应用、bash、git、网页抓取/搜索、
  LSP 查询、子 Agent、worktree、任务状态和用户提问都通过 function calling
  暴露。
- **会话与恢复**：使用纯 Go SQLite 保存会话，并支持 `-c`、`-r` 和
  `/sessions` 选择器。
- **自定义命令与技能**：加载 `.deepseek/command/*.md` slash 命令，并从项目
  与 home 目录发现 `SKILL.md`。
- **MCP 集成**：配置过的 MCP server 会在启动时连接，其工具会桥接到 Agent
  registry。
- **可选沙箱**：在配置启用后，bash 工具会使用当前宿主平台可用的 sandbox
  实现。

## 安装

### macOS / Linux（一键安装）

```sh
curl -fsSL https://raw.githubusercontent.com/amemiya02/deepseekcode/main/install.sh | sh
```

默认安装到 `~/.local/bin`。可通过 `PREFIX=/usr/local sh` 覆盖安装路径，或通过 `DSC_VERSION=v0.3.1 sh` 指定版本。

### macOS（Homebrew）

```sh
brew install amemiya02/deepseekcode/deepseekcode
```

### Windows（Scoop）

```sh
scoop bucket add deepseekcode https://github.com/amemiya02/deepseekcode-scoop
scoop install deepseekcode
```

### Go

```sh
go install github.com/amemiya02/deepseekcode/cmd/dsc@latest
```

### 从源码构建

```sh
git clone https://github.com/amemiya02/deepseekcode
cd deepseekcode
make build
./bin/dsc -version
```

前置条件：

- 与 `go.mod` 匹配或更新的 Go 版本。
- DeepSeek API key，或其他已配置 provider 的 key。
- Git 是可选依赖；安装后可启用 git-aware prompt context 和 git 工具。
- Language server 是可选依赖；存在时 `dsc` 会自动连接检测到的 server。

## 快速开始

```sh
export DEEPSEEK_API_KEY=sk-...

dsc
dsc -p "summarize this repository"
dsc --read-only -p "explain the architecture"
dsc init
dsc doctor
```

常用会话参数：

```sh
dsc -c                 # 继续当前项目最近一次会话
dsc -r <session-id>    # 恢复指定会话
dsc -new               # 强制创建新会话
```

## 配置

配置按内置默认值、用户配置、项目配置、CLI flags 的顺序叠加。项目配置位于
`./.deepseek/config.toml`；用户级配置位于 `~/.deepseek/config.toml`。

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

OpenAI-compatible 端点使用同一套 provider 机制：

```toml
[active]
provider = "openai"

[providers.openai]
type = "openai-compat"
base_url = "https://api.openai.com"
env_var = "OPENAI_API_KEY"
default_model = "gpt-4o"
```

完整字段说明见 [docs/config.md](docs/config.md) 和
[docs/PROVIDERS.md](docs/PROVIDERS.md)。

## CLI 参考

顶层命令：

```sh
dsc                  # 启动 TUI
dsc init             # 创建 DEEPSEEK.md 和 .deepseek/config.toml
dsc doctor           # 检查配置、provider、终端、SQLite、git、MCP、LSP 和更新
dsc upgrade          # 检查最新 GitHub release 并打印升级命令
dsc agent list       # 列出项目 agents
dsc agent show NAME  # 打印 agent 定义
dsc agent new NAME   # 生成 .deepseek/agent/NAME.md
dsc agent validate   # 校验项目 agent 定义
dsc trace inspect TRACE.jsonl
```

主要 flags：

```sh
-version          打印构建版本
-p "prompt"      运行一轮模型调用后退出
-model ID        覆盖主循环模型
-read-only       禁止 write、edit 和 bash 工具
-ask-all         每个工具调用前都询问
-yolo            自动批准所有工具调用
-no-duet         禁用 Pro validator hook
-debug           将结构化日志写入 .deepseek/log/
-trace-jsonl P   将 benchmark/diagnostic trace 事件写入 JSONL 文件
```

## TUI 命令

快捷键：

```text
Enter       发送提示
Shift+Enter 插入换行
Ctrl+C      中断当前运行，空闲时退出
Ctrl+D      退出
Ctrl+R      展开/折叠最近一个 thinking block
Ctrl+T      展开/折叠全部 thinking blocks
PgUp/PgDn   滚动
```

Slash commands：

```text
/help       显示快捷键和命令
/clear      清空 scrollback
/quit       退出
/models     列出或切换主循环模型
/tape       打开 reasoning tape
/sessions   列出当前项目会话
/export     在 $PAGER 中打开完整 scrollback
/undo       恢复上一次编辑 step
/compact    强制压缩消息
```

自定义 slash 命令会从项目和 home 目录下的 `.deepseek/command/*.md` 加载。
发现到的 skills 也会被提升为 slash commands；同名时用户命令优先。

## 内置工具

`dsc` 通过模型 function calling 暴露工具。实际可用工具还可能包括已配置
MCP server 提供的工具。

核心仓库工具：

- `read_file`, `write_file`, `edit_file`, `apply_patch`
- `glob`, `grep`, `ls`
- `bash`, `bash_pty`, `background_bash`
- `git_diff`, `git_show`, `git_blame`, `git_log`
- `todo_write`, `task_status`, `question`

上下文与扩展工具：

- `lsp`：检测到 language server 时，用于基于 LSP 的符号查询。
- `skill_read`：按需读取已发现的 `SKILL.md` 正文。
- `web_fetch` 和 `web_search`：web tooling 在配置中启用时可用。
- `task`：分派子 Agent；`worktree`：管理隔离的 git worktree。

工具参数与安全说明见 [docs/tools.md](docs/tools.md)。

## 安全模型

权限策略刻意保守：

- 只读工具默认允许。
- 文件写入默认允许当前工作目录内的目标，但敏感或不安全路径会触发询问。
- Bash 由 allowlist patterns 和权限提示控制。
- `--read-only`、`--ask-all` 和 `--yolo` 会覆盖默认策略。
- 破坏性操作可由 Duet validator hook 检查。
- 会修改文件的工具在执行前会快照受影响路径；`/undo` 恢复最近一次编辑 step。
- 宿主平台支持时，可通过配置启用 bash 执行沙箱。

见 [docs/permissions.md](docs/permissions.md) 和
[docs/SANDBOX.md](docs/SANDBOX.md)。

## 项目文件

- [docs/config.md](docs/config.md) — 配置参考
- [docs/PROVIDERS.md](docs/PROVIDERS.md) — provider 设置
- [docs/tools.md](docs/tools.md) — 内置工具说明
- [docs/commands.md](docs/commands.md) — 自定义 slash 命令
- [docs/skills.md](docs/skills.md) — skill 发现
- [docs/duet.md](docs/duet.md) — Pro validator 行为
- [docs/tape.md](docs/tape.md) — reasoning tape 行为
- [docs/upgrade.md](docs/upgrade.md) — upgrade 命令行为
- [docs/MODEL_COMPATIBILITY.md](docs/MODEL_COMPATIBILITY.md) — DeepSeek wire 形态事实与受支持模型

## 开发

常用命令：

```sh
make build
make test
make test-race
make lint
make fmt
make tidy
make run
```

运行单个测试：

```sh
go test ./internal/llm/ -run TestThinkingSerializesAsStruct -v
```

提交 PR 前建议运行：

```sh
make fmt
make lint
make test
```

## 版本管理

Release build 会通过 `make build` 使用 `git describe` 注入版本；因此
`v0.3.1` 这样的 tag 会出现在 `dsc -version` 输出中。使用 `dsc upgrade`
检查并应用新版本。

## 参与贡献

欢迎提交 issue 和 pull request。改动应基于当前 CLI、TUI、配置和工具行为；
一个特性只有在本仓库中已经实现并可测试时，才应写入文档。

修改 README 时，请同步更新 `README.md` 和 `README.zh-CN.md`，并保持匹配的
`##` 结构。

## 许可证

MIT
