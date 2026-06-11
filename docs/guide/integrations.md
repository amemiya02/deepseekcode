# 外部集成：MCP、LSP、CodeGraph

> **目标：** 接入一个 MCP server，理解 LSP 与 CodeGraph 能带来什么。
>
> **前提：** 已完成[进阶三件套教程](skills-agents-hooks.md)，了解 `.deepseek/config.toml` 的基本结构。

---

## 1. 接入第一个 MCP server

[Model Context Protocol（MCP）](https://modelcontextprotocol.io/) 是一套标准接口，允许 `dsc` 在运行时接入外部工具服务器。接入后，MCP server 提供的工具与内置工具一样直接供模型调用。

### config.toml 配置

在 `~/.deepseek/config.toml`（或项目级 `.deepseek/config.toml`）中添加：

```toml
# stdio transport（默认）：将 server 作为子进程启动
[mcp_servers.git]
command = "uvx"
args = ["mcp-server-git", "--repository", "."]
```

键名规则（源自 `internal/config/config.go`，toml tag `mcp_servers`）：
- 顶层键固定为 `mcp_servers`；
- 方括号内是自定义 server 名称（本例为 `git`）；
- 工具在会话内以 `mcp__git__<tool_name>` 形式出现。

### SSE 远程 server

```toml
[mcp_servers.remote-tools]
transport = "sse"
url = "https://example.com/mcp/sse"
```

`transport` 省略时默认 `"stdio"`。

### 按需过滤工具

```toml
# 只暴露指定工具（二选一，不能同时用）
[mcp_servers.fs]
command = "mcp-fs"
disabled_tools = ["delete_file"]   # 隐藏危险工具

[mcp_servers.github]
command = "mcp-github"
enabled_tools = ["search_issues", "read_file"]   # 仅暴露白名单
```

完整配置语法见 [MCP 参考](../reference/mcp.md)。

### 验证：工具列表是否出现新工具

`dsc` 启动时自动连接所有已配置的 MCP server，完成 `initialize` 握手和 `tools/list` 拉取。验证方法：

1. 在 TUI 中输入 `/mcp`——全屏弹层显示所有 MCP server 的连接状态（connected / degraded / failed）、工具数量与最近错误；
2. 向模型提问"你现在有哪些工具？"——已接入的 MCP 工具会以 `mcp__<server>__<tool>` 形式列出；
3. 查看 TUI 状态栏——显示已连接 MCP 工具总数。

若 server 启动失败，先确认 `command` 在 PATH 中可执行；若连接成功但工具未出现，检查 server 的 stderr（`mcp-server-git` 等工具通常会在 stderr 输出错误信息）。

---

## 2. 启用 LSP

`dsc` 集成了 Language Server Protocol 客户端，提供类 IDE 的代码智能：定义跳转、引用查找、悬停类型信息、诊断报错。

### 自动发现

`dsc` 启动时扫描项目目录，根据检测文件自动启动对应语言服务器：

| 语言 | LSP server | 检测文件 |
|------|-----------|---------|
| Go | `gopls` | `go.mod` |
| Rust | `rust-analyzer` | `Cargo.toml` |
| TypeScript/JavaScript | `typescript-language-server` | `tsconfig.json` |
| Python | `pylsp` | `pyproject.toml` 或 `requirements.txt` |

无需额外配置——只要检测文件存在且 server 二进制在 PATH 中，LSP 自动生效。

### 安装 server 二进制

```bash
# Go
go install golang.org/x/tools/gopls@latest

# Rust
rustup component add rust-analyzer

# TypeScript/JavaScript
npm install -g typescript-language-server typescript

# Python
pip install python-lsp-server
```

### LSP 带来什么

发现后，`lsp` 工具注册到会话，模型可调用以下四类操作：

| action | 说明 |
|--------|------|
| `hover` | 光标处的类型信息与文档注释 |
| `definition` | 跳转到符号定义位置 |
| `references` | 查找全部引用 |
| `diagnostics` | 当前文件的编译器/linter 错误和警告 |

### 验证

```bash
dsc doctor
```

输出示例：

```
deepseekcode doctor
──────────────────────────────────────────────────
  ✓ lsp                 available: gopls
```

若无语言服务器被检测到，doctor 显示 `no language servers detected for this project`。

完整参考见 [LSP 参考](../reference/lsp.md)。

---

## 3. CodeGraph

### 是什么

CodeGraph 是 `dsc` 内置的代码符号知识图谱，基于 **tree-sitter** 将代码库解析为符号节点（文件、函数、类型、接口）和类型化边（调用、导入、定义、实现），并用 PageRank 计算中心度排序。实现位于 `internal/codegraph`。

与 embedding 语义搜索不同，CodeGraph 是**纯语法解析**：本地运行、无 API 成本、后台增量索引（文件变更后 ~500ms 完成更新）。

### 能带来什么

| 查询意图 | 对应能力 |
|---------|---------|
| 某函数被哪些地方调用？ | 调用者（callers）查询 |
| 某函数调用了哪些？ | 被调用者（callees）查询 |
| 改动某符号会影响哪些？ | blast-radius / impact 分析 |
| 最重要的符号是哪些？ | PageRank 中心度排序 |
| 符号定义在哪？ | 符号搜索与节点查询 |

### 作为 MCP server 暴露

CodeGraph 以 MCP server 形式对外暴露，工具集前缀为 `codegraph_*`（如 `codegraph_search`、`codegraph_callers`、`codegraph_impact` 等）。配置方式与任意 MCP server 相同，在 `~/.deepseek/config.toml` 中声明即可接入。

接入后，模型可直接通过 `codegraph_*` 工具查询代码结构，而无需读取整个文件——这不仅提升了理解精度，也减少了请求体体积，对前缀缓存友好。

---

## 下一步

- [省钱之道：前缀缓存与智能路由](cost-and-cache.md)
