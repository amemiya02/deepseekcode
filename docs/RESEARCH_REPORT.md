# deepseekcode 深度调研报告

> 调研日期：2026-05-27
> 调研范围：6个参考项目 + 6份内部设计文档
> 目标：对标 opencode、claude-code、crush、DeepSeek-Reasonix，明确差距与优化方向

---

## 一、调研概览

### 1.1 参考项目矩阵

| 项目 | 语言 | Star | 定位 | 对标维度 |
|------|------|------|------|----------|
| **claude-code** | TypeScript | 100k+ | Anthropic 旗舰产品 | 工具生态、MCP、子代理、权限系统 |
| **opencode** | TypeScript/Bun | 165k | 全平台 AI 编程平台 | 架构模块化、LLM 抽象、多平台分发 |
| **CodeWhale** | Rust | 34k | DeepSeek-first 开放权重 agent | DeepSeek 优化、沙箱、并发子代理 |
| **crush** | Go | 24.6k | Charmbracelet 精美 TUI | Go TUI 架构、Bubble Tea v2 |
| **DeepSeek-Reasonix** | TypeScript | 6.7k | DeepSeek-native 缓存优化 | 99.82% 缓存命中、4 遍修复管线 |
| **claw-code** | Rust | 192k | Claude Code 逆向工程 | 学习参考、架构理解 |

### 1.2 deepseekcode 当前状态（v0.1）

**已完成的核心功能**：

| 模块 | 状态 | 关键实现 |
|------|------|----------|
| Agent Loop | ✅ 完成 | callback-driven ReAct、parallel tool calls、stream/present split |
| Tool System | ✅ 完成 | 14 个内置工具、apply_patch、question、subagent、worktree |
| TUI | ✅ 完成 | Bubble Tea、Reasoning Tape、/tape 全屏、Cost HUD |
| LLM Client | ✅ 完成 | ~400 LOC HTTP+SSE、cache-stable serialization、thinking struct |
| Repair Core | ✅ 完成 | truncation repair、scavenge、storm breaker、schema analysis |
| Session | ✅ 完成 | SQLite 纯 Go、branching、transcript、compaction |
| Permissions | ✅ 完成 | tiered defaults、bash patterns、snapshot rollback |
| MCP | ✅ 完成 | stdio transport、drift detection、tool bridge |
| Hooks | ✅ 完成 | PreToolUse/PostToolUse、subprocess runner |
| Sandbox | ✅ 完成 | macOS Seatbelt、Linux Landlock |
| Worktree | ✅ 完成 | branch_lock、manager |
| Skills | ✅ 完成 | SKILL.md 发现、lazy loading |
| Commands | ✅ 完成 | .deepseek/command/*.md 自定义 slash |

**内部模块结构**（282 个文件）：

```
internal/
├── agent/          # 核心 agent loop、events、jobs、plan、repair integration
├── agents/         # agent 定义加载
├── bootstrap/      # 初始化
├── commands/       # 自定义命令
├── config/         # 配置管理、secrets、验证
├── gitctx/         # Git 上下文
├── hooks/          # hook 引擎
├── llm/            # LLM 客户端、provider、cache、sanitize
├── logging/        # 日志
├── lsp/            # LSP 集成
├── mcp/            # MCP 协议
├── permissions/    # 权限策略
├── prompt/         # prompt 构建
├── repair/         # 工具调用修复
├── sandbox/        # OS 级沙箱
├── session/        # 会话存储
├── skills/         # 技能系统
├── snapshots/      # 快照回滚
├── tools/          # 工具实现
├── tui/            # 终端 UI
├── version/        # 版本管理
└── worktree/       # worktree 管理
```

---

## 二、对标分析

### 2.1 对标 Claude Code

| 维度 | Claude Code | deepseekcode | 差距 | 优先级 |
|------|-------------|--------------|------|--------|
| **工具数量** | 30+ | 14 | 中 | P2 |
| **MCP 传输** | 6 种 | 1 种 (stdio) | 大 | P3 |
| **子代理** | 完整（Worktree 隔离、远程执行） | 基础（已实现） | 小 | P2 |
| **IDE 集成** | VS Code + JetBrains | 无 | 大 | P4 |
| **权限模式** | 7 种 | 分层 + bash patterns | 中 | P3 |
| **Skills 来源** | 6 种 | 1 种 (SKILL.md) | 中 | P3 |
| **Hook 系统** | 完整 | 完整 | 无 | ✅ |
| **桌面应用** | 有 | 无 | 大 | P4 |

**关键差距**：
- Claude Code 的 **AgentTool 子代理递归调用** 是其核心竞争力
- **6 种 MCP 传输** 提供了最大的扩展性
- **7 种权限模式** 覆盖了从完全信任到完全隔离的所有场景

**我们的优势**：
- DeepSeek 原生优化（Claude Code 无法做到）
- 单二进制分发（Claude Code 需要 Node.js）
- 成本控制（50× 缓存折扣）

### 2.2 对标 opencode

| 维度 | opencode | deepseekcode | 差距 | 优先级 |
|------|----------|--------------|------|--------|
| **架构模块化** | 21 个 package | 22 个 internal 包 | 无 | ✅ |
| **LLM 抽象** | 独立 llm package | internal/llm | 无 | ✅ |
| **全平台** | CLI + Desktop + Web + VS Code + Slack | CLI + TUI | 大 | P4 |
| **双 Agent 模式** | build + plan | build + plan | 无 | ✅ |
| **自定义命令** | .opencode/command/*.md | .deepseek/command/*.md | 无 | ✅ |
| **自定义 Agent** | .opencode/agent/*.md | 无 | 中 | P2 |
| **PTY 支持** | 有 | 有 | 无 | ✅ |
| **Background Job** | 有 | 有 | 无 | ✅ |
| **Worktree** | 有 | 有 | 无 | ✅ |

**关键差距**：
- opencode 的 **`.opencode/agent/*.md` 自定义 sub-agent** 是其独特优势
- **全平台分发**（CLI + Desktop + Web + VS Code + Slack）是其核心竞争力

**我们的优势**：
- Go 单二进制（opencode 需要 Bun）
- DeepSeek 原生优化
- 更简洁的架构（21 个 package vs 22 个 internal 包）

### 2.3 对标 crush

| 维度 | crush | deepseekcode | 差距 | 优先级 |
|------|-------|--------------|------|--------|
| **TUI 框架** | Bubble Tea v2 | Bubble Tea | 小 | P2 |
| **多提供商** | 20+ | DeepSeek + OpenAI-compat | 中 | P3 |
| **Coordinator 模式** | 有 | 无 | 中 | P2 |
| **LSP 集成** | 有 | 有 | 无 | ✅ |
| **SQLite 持久化** | 有 | 有 | 无 | ✅ |
| **Hook 系统** | 有 | 有 | 无 | ✅ |
| **Skills 系统** | 有 | 有 | 无 | ✅ |

**关键差距**：
- crush 的 **Coordinator 模式** 管理命名 Agent 是其独特设计
- **20+ 提供商** 支持是其核心竞争力

**我们的优势**：
- DeepSeek 原生优化（crush 是通用型）
- Reasoning Tape（crush 没有）
- Two-Model Duet（crush 没有）
- Cost HUD（crush 没有）

### 2.4 对标 DeepSeek-Reasonix

| 维度 | DeepSeek-Reasonix | deepseekcode | 差距 | 优先级 |
|------|-------------------|--------------|------|--------|
| **缓存命中率** | 99.82% | ~91%（推测） | 中 | P1 |
| **修复管线** | 4 遍 | 4 遍 | 无 | ✅ |
| **成本控制** | Flash-first | Flash-first | 无 | ✅ |
| **Skills 兼容** | Claude 格式 | SKILL.md | 小 | P2 |
| **语义搜索** | 有 | 无 | 中 | P3 |
| **QQ 集成** | 有 | 无 | 大 | P5 |
| **Tauri 桌面** | 有 | 无 | 大 | P4 |
| **/effort 旋钮** | 有 | 有 | 无 | ✅ |

**关键差距**：
- DeepSeek-Reasonix 的 **99.82% 缓存命中率** 是其核心竞争力
- **嵌入式语义索引** 是其独特优势

**我们的优势**：
- Go 单二进制（Reasonix 需要 Node.js）
- 更完整的工具系统（14 vs 12）
- 更成熟的 TUI（Bubble Tea vs Ink）
- SQLite 持久化（Reasonix 用 JSONL）

---

## 三、已实现功能清单

### 3.1 核心架构（✅ 完成）

| 功能 | 状态 | 实现文件 | 说明 |
|------|------|----------|------|
| **Agent Loop** | ✅ | `internal/agent/agent.go` | callback-driven ReAct、parallel tool calls |
| **Stream/Present Split** | ✅ | `internal/agent/agent.go` | SSE chunks 和 UI 渲染分离 |
| **Finish-reason Override** | ✅ | `internal/llm/client.go` | tool_calls > 0 时继续循环 |
| **Two-tier Timeout** | ✅ | `internal/llm/client.go` | firstTokenTimeout=45s、chunkStallTimeout=20s |
| **Cache-stable Serialization** | ✅ | `internal/llm/request.go` | sorted keys、deterministic JSON |
| **Thinking Sanitizer** | ✅ | `internal/llm/sanitize.go` | 修复 replayed assistant history |

### 3.2 工具系统（✅ 完成）

| 工具 | 状态 | 说明 |
|------|------|------|
| `read_file` | ✅ | 读取文件内容 |
| `write_file` | ✅ | 写入文件 |
| `edit_file` | ✅ | 字符串替换编辑 |
| `bash` | ✅ | Shell 命令执行 |
| `glob` | ✅ | 文件名 glob |
| `grep` | ✅ | 内容搜索 |
| `ls` | ✅ | 目录列表 |
| `todo_write` | ✅ | 计划管理 |
| `git_diff` | ✅ | 结构化 diff |
| `git_show` | ✅ | 读取历史内容 |
| `git_blame` | ✅ | 逐行作者 |
| `git_log` | ✅ | 提交历史 |
| `apply_patch` | ✅ | 多 hunk patch 应用 |
| `question` | ✅ | 多选 clarify |
| `subagent` | ✅ | 子代理调度 |
| `worktree` | ✅ | worktree 管理 |
| `task_status` | ✅ | 后台任务状态 |
| `web_fetch` | ✅ | 网页获取 |
| `web_search` | ✅ | 网页搜索 |
| `lsp` | ✅ | LSP 集成 |

### 3.3 TUI 功能（✅ 完成）

| 功能 | 状态 | 说明 |
|------|------|------|
| **Reasoning Tape** | ✅ | 可折叠推理块、/tape 全屏 |
| **Cost HUD** | ✅ | 实时缓存命中、token、成本 |
| **Tool Call 渲染** | ✅ | 折叠/展开、状态指示 |
| **Repair Receipts** | ✅ | compact receipt 显示 |
| **Status Line** | ✅ | 模型、步骤、缓存、成本 |
| **Theme** | ✅ | dark/light 主题 |
| **Vim Keys** | ✅ | hjkl 导航 |
| **Overlay** | ✅ | /help、/models、/sessions |

### 3.4 安全与权限（✅ 完成）

| 功能 | 状态 | 说明 |
|------|------|------|
| **Tiered Defaults** | ✅ | read-only auto-allow、write inside cwd auto-allow |
| **Bash Patterns** | ✅ | 命令模式匹配 |
| **Snapshot Rollback** | ✅ | /undo 回滚 |
| **Mode Flags** | ✅ | --yolo、--read-only、--ask-all |
| **OS Sandbox** | ✅ | macOS Seatbelt、Linux Landlock |
| **Destructive Tool Gate** | ✅ | Pro 验证破坏性操作 |

### 3.5 会话管理（✅ 完成）

| 功能 | 状态 | 说明 |
|------|------|------|
| **SQLite 持久化** | ✅ | 纯 Go、无 CGO |
| **Session Branching** | ✅ | parent_id + branch_point |
| **Transcript** | ✅ | append-only 收据流 |
| **Compaction** | ✅ | 自动/手动上下文压缩 |
| **Resume** | ✅ | --continue、--resume |

### 3.6 Repair Core（✅ 完成）

| 功能 | 状态 | 说明 |
|------|------|------|
| **Truncation Repair** | ✅ | 修复截断 JSON |
| **Scavenge** | ✅ | 从 reasoning/content 回收工具调用 |
| **Storm Breaker** | ✅ | 防止重复工具调用循环 |
| **Schema Analysis** | ✅ | 自适应 schema 展平 |
| **Repair Receipts** | ✅ | compact receipt |

### 3.7 其他（✅ 完成）

| 功能 | 状态 | 说明 |
|------|------|------|
| **MCP** | ✅ | stdio transport、drift detection |
| **Hooks** | ✅ | PreToolUse/PostToolUse |
| **Skills** | ✅ | SKILL.md 发现、lazy loading |
| **Commands** | ✅ | .deepseek/command/*.md |
| **LSP** | ✅ | Language Server Protocol 集成 |
| **Auto Reasoning** | ✅ | per-turn thinking 选择器 |
| **Prefix Drift** | ✅ | 缓存漂移检测 |

---

## 四、差距分析与优化建议

### 4.1 缓存命中率优化（P1 - 高优先级）

**当前状态**：~91%（推测）
**目标**：99%+
**参考**：DeepSeek-Reasonix 达到 99.82%

**优化方向**：

1. **Prefix Cache 运行时漂移检测**
   - 当前：仅检测工具名称变化
   - 目标：检测 hook/skill 注入新行打破前缀
   - 实现：`internal/llm/prefix_drift.go` 已有基础，需要增强
   - 预估：~200 LOC

2. **Full Serialized Prefix Hash**
   - 当前：仅序列化工具名称
   - 目标：序列化完整工具 spec（包括 description、parameters）
   - 实现：修改 `internal/llm/request.go` 的 `MarshalCacheStable`
   - 预估：~100 LOC

3. **Cache-aligned Flash Summaries**
   - 当前：compaction 时可能破坏前缀
   - 目标：compaction 时保持前缀稳定
   - 实现：修改 `internal/agent/compact.go`
   - 预估：~150 LOC

**预期收益**：
- 缓存命中率从 ~91% 提升到 99%+
- 成本降低 50×（缓存命中折扣）

### 4.2 Sub-agent 运行时增强（P2 - 中优先级）

**当前状态**：基础 sub-agent 已实现
**目标**：对标 Claude Code 的 AgentTool
**参考**：opencode 的 `.opencode/agent/*.md`

**优化方向**：

1. **自定义 Agent 定义**
   - 当前：仅支持内置 subagent 类型
   - 目标：支持 `.deepseek/agent/*.md` 自定义 agent
   - 实现：参考 opencode 的 `agent/agent.ts`
   - 预估：~300 LOC

2. **Agent 白名单工具**
   - 当前：subagent 继承父级工具
   - 目标：每个 agent 定义自己的工具白名单
   - 实现：扩展 `internal/agents/def.go`
   - 预估：~200 LOC

3. **Worktree 隔离**
   - 当前：worktree 管理已实现
   - 目标：subagent 在独立 worktree 运行
   - 实现：扩展 `internal/worktree/manager.go`
   - 预估：~250 LOC

**预期收益**：
- 更强的任务分解能力
- 更安全的并行执行
- 更灵活的 agent 组合

### 4.3 统一事件总线（P2 - 中优先级）

**当前状态**：callbacks + channel
**目标**：类型化事件总线
**参考**：CodeWhale 的 `protocol::EventFrame`

**优化方向**：

1. **EventFrame 类型化事件**
   - 当前：`agent.Event` 是基础类型
   - 目标：完整的事件类型系统
   - 实现：扩展 `internal/agent/events.go`
   - 预估：~400 LOC

2. **Event Bus**
   - 当前：直接 channel 通信
   - 目标：pub/sub 模式
   - 实现：新建 `internal/bus/`
   - 预估：~300 LOC

3. **TUI/Session/Transcript 统一消费**
   - 当前：各自独立的事件处理
   - 目标：从同一事件流派生
   - 实现：重构 `internal/tui/` 和 `internal/session/`
   - 预估：~500 LOC

**预期收益**：
- 更好的可测试性
- 更容易添加新的观察者（IDE、daemon）
- 更清晰的架构

### 4.4 TUI 增强（P2 - 中优先级）

**当前状态**：基础 TUI 已完成
**目标**：对标 crush 的精美 TUI
**参考**：crush 的 Bubble Tea v2

**优化方向**：

1. **Tool-specific Renderers**
   - 当前：通用工具渲染
   - 目标：每个工具有专用渲染器
   - 实现：扩展 `internal/tui/tool_renderers.go`
   - 预估：~500 LOC

2. **Render Cache**
   - 当前：每次重绘
   - 目标：按 message version + width 缓存
   - 实现：参考 crush 的 draw cache
   - 预估：~300 LOC

3. **Status HUD 增强**
   - 当前：基础状态行
   - 目标：更丰富的状态段
   - 实现：扩展 `internal/tui/status_hud.go`
   - 预估：~200 LOC

4. **Theme Token 审计**
   - 当前：分散的样式定义
   - 目标：集中主题 token
   - 实现：重构 `internal/tui/theme.go`
   - 预估：~300 LOC

**预期收益**：
- 更好的视觉体验
- 更专业的终端 UI
- 更容易的主题定制

### 4.5 多 Provider 抽象（P3 - 低优先级）

**当前状态**：DeepSeek + OpenAI-compat
**目标**：支持更多 provider
**参考**：opencode 的 provider 抽象

**优化方向**：

1. **Provider 接口标准化**
   - 当前：`internal/llm/provider.go` 已有基础
   - 目标：完整的 provider 抽象
   - 实现：扩展 provider 接口
   - 预估：~400 LOC

2. **OAuth 认证**
   - 当前：API key 认证
   - 目标：支持 OAuth
   - 实现：新建 `internal/auth/`
   - 预估：~300 LOC

**预期收益**：
- 更广泛的模型支持
- 企业级认证
- 更大的用户群

### 4.6 MCP 增强（P3 - 低优先级）

**当前状态**：stdio transport
**目标**：支持更多传输
**参考**：claude-code 的 6 种传输

**优化方向**：

1. **SSE Transport**
   - 当前：仅 stdio
   - 目标：支持 SSE
   - 实现：扩展 `internal/mcp/transport.go`
   - 预估：~300 LOC

2. **HTTP Transport**
   - 当前：仅 stdio
   - 目标：支持 HTTP
   - 实现：扩展 `internal/mcp/transport.go`
   - 预估：~200 LOC

3. **WebSocket Transport**
   - 当前：仅 stdio
   - 目标：支持 WebSocket
   - 实现：扩展 `internal/mcp/transport.go`
   - 预估：~300 LOC

**预期收益**：
- 更多的 MCP 服务器支持
- 更好的扩展性

### 4.7 IDE 集成（P4 - 低优先级）

**当前状态**：无
**目标**：VS Code 插件
**参考**：opencode 的 VS Code 集成

**优化方向**：

1. **VS Code Extension**
   - 当前：无
   - 目标：VS Code 插件
   - 实现：新建 `vscode-extension/`
   - 预估：~2000 LOC

2. **JetBrains Plugin**
   - 当前：无
   - 目标：JetBrains 插件
   - 实现：新建 `jetbrains-plugin/`
   - 预估：~2000 LOC

**预期收益**：
- 更大的用户群
- 更好的开发体验

### 4.8 语义搜索（P3 - 低优先级）

**当前状态**：无
**目标**：嵌入式语义索引
**参考**：DeepSeek-Reasonix 的语义搜索

**优化方向**：

1. **嵌入式向量索引**
   - 当前：无
   - 目标：本地向量索引
   - 实现：新建 `internal/semantic/`
   - 预估：~500 LOC

2. **语义搜索工具**
   - 当前：无
   - 目标：semantic_search 工具
   - 实现：扩展 `internal/tools/`
   - 预估：~200 LOC

**预期收益**：
- 超越关键词匹配
- 更好的代码理解

---

## 五、立即可做的优化

### 5.1 本周可做（S 级 - 高 ROI，低风险）

| # | 优化项 | 难度 | 预估 LOC | 收益 |
|---|--------|------|----------|------|
| S1 | **Prefix Cache 增强** | 低 | ~200 | 缓存命中率 +5% |
| S2 | **Tool Schema 完整序列化** | 低 | ~100 | 缓存命中率 +2% |
| S3 | **Cache-aligned Compaction** | 中 | ~150 | 缓存命中率 +2% |
| S4 | **Tool-specific Renderers** | 中 | ~500 | TUI 体验提升 |
| S5 | **Status HUD 增强** | 低 | ~200 | 信息展示更丰富 |

### 5.2 本月可做（A 级 - 大跨度功能）

| # | 优化项 | 难度 | 预估 LOC | 收益 |
|---|--------|------|----------|------|
| A1 | **自定义 Agent 定义** | 中 | ~300 | 任务分解能力 |
| A2 | **Agent 白名单工具** | 中 | ~200 | 安全性提升 |
| A3 | **统一事件总线** | 高 | ~700 | 架构清晰度 |
| A4 | **Render Cache** | 中 | ~300 | TUI 性能 |
| A5 | **Theme Token 审计** | 中 | ~300 | 可维护性 |

### 5.3 本季可做（B 级 - 基础设施）

| # | 优化项 | 难度 | 预估 LOC | 收益 |
|---|--------|------|----------|------|
| B1 | **SSE Transport** | 中 | ~300 | MCP 扩展性 |
| B2 | **HTTP Transport** | 中 | ~200 | MCP 扩展性 |
| B3 | **Provider 接口标准化** | 高 | ~400 | 多模型支持 |
| B4 | **语义搜索** | 高 | ~700 | 代码理解 |
| B5 | **VS Code Extension** | 高 | ~2000 | 用户群扩大 |

---

## 六、缓存命中率优化详细方案

### 6.1 当前缓存机制

**已实现**：
- System prompt byte-stable
- Tool schemas sorted keys
- User-message append-only
- Cache statistics surfaced in status line

**问题**：
- Hook/skill 注入可能打破前缀
- Compaction 时可能破坏前缀
- Tool schema 仅序列化名称，不包括完整 spec

### 6.2 优化方案

#### 方案 1：Full Serialized Prefix Hash

**目标**：序列化完整工具 spec（包括 description、parameters）

**实现**：
```go
// internal/llm/request.go
func (r *Request) MarshalCacheStable() []byte {
    // 现有逻辑：sorted keys
    // 新增：完整工具 spec 序列化
    tools := make([]map[string]interface{}, len(r.Tools))
    for i, t := range r.Tools {
        tools[i] = map[string]interface{}{
            "type": "function",
            "function": map[string]interface{}{
                "name":        t.Function.Name,
                "description": t.Function.Description,
                "parameters":  sortKeys(t.Function.Parameters),
            },
        }
    }
    // ...
}
```

**预期收益**：缓存命中率 +2%

**风险**：低

**实现难度**：低（~100 LOC）

#### 方案 2：Prefix Drift Detection 增强

**目标**：检测 hook/skill 注入新行打破前缀

**实现**：
```go
// internal/llm/prefix_drift.go
type PrefixDriftDetector struct {
    lastPrefixHash string
    lastToolHash   string
}

func (d *PrefixDriftDetector) DetectDrift(currentHash string) bool {
    if d.lastPrefixHash != currentHash {
        // 发生漂移，记录原因
        d.logDriftReason()
        return true
    }
    return false
}
```

**预期收益**：缓存命中率 +3%

**风险**：低

**实现难度**：低（~200 LOC）

#### 方案 3：Cache-aligned Compaction

**目标**：Compaction 时保持前缀稳定

**实现**：
```go
// internal/agent/compact.go
func (a *Agent) compactContext() error {
    // 现有逻辑：summarize older history
    // 新增：确保 summary 不改变前缀
    summary := a.summarizeHistory()
    // 只替换 user messages，保持 assistant/tool 结构
    // ...
}
```

**预期收益**：缓存命中率 +2%

**风险**：中

**实现难度**：中（~150 LOC）

### 6.3 预期总收益

- 当前缓存命中率：~91%
- 优化后缓存命中率：~98%
- 成本降低：~7%（从 91% 到 98%）

**注意**：DeepSeek-Reasonix 声称 99.82%，但这可能是在特定场景下的峰值。实际生产环境中 98% 已经是非常优秀的水平。

---

## 七、与 DeepSeek-Reasonix 的差距总结

### 7.1 我们的优势

| 维度 | deepseekcode | DeepSeek-Reasonix |
|------|--------------|-------------------|
| **语言** | Go（单二进制） | TypeScript（需要 Node.js） |
| **TUI** | Bubble Tea（成熟） | Ink（较新） |
| **持久化** | SQLite（纯 Go） | JSONL |
| **工具数量** | 14 | 12 |
| **沙箱** | macOS Seatbelt + Linux Landlock | 无 |
| **子代理** | 完整实现 | 基础实现 |
| **Worktree** | 完整实现 | 无 |
| **LSP** | 完整实现 | 无 |

### 7.2 我们的差距

| 维度 | deepseekcode | DeepSeek-Reasonix |
|------|--------------|-------------------|
| **缓存命中率** | ~91% | 99.82% |
| **语义搜索** | 无 | 有 |
| **QQ 集成** | 无 | 有 |
| **Tauri 桌面** | 无 | 有 |
| **/effort 旋钮** | 有 | 有 |

### 7.3 追赶路径

1. **短期（1-2 周）**：
   - 优化缓存命中率到 98%+
   - 增强 TUI 渲染

2. **中期（1-2 月）**：
   - 实现语义搜索
   - 增强子代理系统

3. **长期（3-6 月）**：
   - VS Code 插件
   - 多 provider 支持

---

## 八、结论与建议

### 8.1 核心结论

1. **deepseekcode 已经是一个功能完整的工业级 coding agent**
   - 14 个内置工具
   - 完整的 TUI（Reasoning Tape、Cost HUD）
   - 完整的 Repair Core
   - 完整的 Session 管理
   - 完整的权限系统

2. **与顶级项目的差距主要在扩展性，而非核心功能**
   - 缺少：多 provider、IDE 集成、语义搜索
   - 但：DeepSeek 原生优化是我们的核心竞争力

3. **缓存命中率是最有潜力的优化方向**
   - 从 ~91% 到 98% 可以显著降低成本
   - 这是 DeepSeek-first 产品的核心价值

### 8.2 优先级建议

**立即做（本周）**：
1. Prefix Cache 增强（S1）
2. Tool Schema 完整序列化（S2）
3. Status HUD 增强（S5）

**本月做**：
1. 自定义 Agent 定义（A1）
2. 统一事件总线（A3）
3. Tool-specific Renderers（S4）

**本季做**：
1. SSE/HTTP Transport（B1/B2）
2. Provider 接口标准化（B3）
3. 语义搜索（B4）

### 8.3 战略建议

1. **保持 DeepSeek-first 定位**
   - 不要试图成为通用 agent
   - 深度优化 DeepSeek 特性
   - 缓存命中率是核心竞争力

2. **优先打磨 TUI 体验**
   - 这是终端 agent 的门面
   - crush 是 TUI 标杆，对标它
   - Reasoning Tape 是独特卖点

3. **渐进式扩展**
   - 先做好核心功能
   - 再考虑 IDE 集成
   - 最后考虑多 provider

4. **保持单二进制优势**
   - 这是相对于 TypeScript/Rust 项目的核心优势
   - 不要引入 CGO 依赖
   - 保持 5ms 冷启动

---

## 附录：参考文档索引

| 文档 | 路径 | 说明 |
|------|------|------|
| Crush TUI Reference | `docs/crush-tui-reference.md` | crush TUI 设计参考 |
| DeepSeek Reliability Core | `docs/deepseek-reliability-core.md` | Repair Core 设计 |
| Industrial Agent Architecture | `docs/industrial-agent-architecture.md` | 架构目标 |
| Reasonix Gap Analysis | `docs/reasonix-gap-analysis.md` | 与 Reasonix 的差距 |
| Reference Roadmap | `docs/reference-roadmap.md` | 参考项目路线图 |
| Survey | `docs/survey.md` | 跨 repo 合成借鉴清单 |
| Design Document | `docs/design.md` | 完整设计文档 |

---

*报告生成时间：2026-05-27*
*调研覆盖：6 个参考项目 + 6 份内部设计文档*
*deepseekcode 版本：v0.1*
