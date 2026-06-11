# 工具系统：实现深入篇

> 深入篇之四（agent-loop / prefix-cache / routing / **tools** / tui / three-surfaces）。架构地图见 [architecture.md](architecture.md)；用户视角的工具清单、权限语义、沙箱配置见 [../reference/tools.md](../reference/tools.md)、[../reference/permissions.md](../reference/permissions.md)、[../reference/sandbox.md](../reference/sandbox.md)。本文面向贡献者：接口契约、执行管道五道关、快照回滚，以及从零新增一个 tool 的完整实操。

## 1. 解决什么问题

工具面要同时伺候三个互相牵制的主人：

- **模型**要一份字节级稳定的函数调用 schema——`Name`/`Description`/`Parameters` 进 prompt 前缀，是 DeepSeek 前缀缓存命中的前提（见 [prefix-cache.md](prefix-cache.md)）；
- **用户**要安全网——权限闸、Duet 验证器、沙箱、以及改错了能 `/undo` 回来的快照；
- **贡献者**要低摩擦扩展——接口必须小到一个 72 行的文件就能实现完整。

`internal/tools` 约 1.3 万行，承载 14 个内置工具（`builtins.go`）+ 会话级工具（question / subagent / web_fetch…，`cmd/dsc/main.go` 装配）+ MCP 桥接工具（`internal/mcp` 桥进同一接口）。本文讲这套机器怎么转，以及你加一个工具时每一站会发生什么。

## 2. 核心数据结构

### 2.1 `Tool` 接口：四个方法，没有第五个

`internal/tools/registry.go`：

```go
// Tool is the abstract built-in or MCP tool. Implementations are
// expected to be safe to call from arbitrary goroutines.
type Tool interface {
	// Name is the function-call identifier the model uses.
	Name() string

	// Description is a one- to four-sentence English summary. Goes into
	// the prompt; keep it specific.
	Description() string

	// Parameters returns the JSON-Schema describing the tool's args.
	// The bytes do not need to be canonical — canonicalization happens in
	// MarshalCacheStable (internal/llm/request.go), not in the registry.
	Parameters() json.RawMessage

	// Execute runs the tool. args is the model's JSON arguments. The
	// returned Result is shown to the model; any non-nil error indicates
	// infrastructure failure the agent should surface (and may retry).
	Execute(ctx context.Context, args json.RawMessage) (Result, error)
}
```

注意接口注释里的并发契约：`runToolCalls` 会把同一 step 的多个调用**并行**执行（`internal/agent/agent.go`），所以实现必须可从任意 goroutine 调用。

两个**可选接口**（同文件）改变工具在安全管道里的待遇：

```go
type ReadOnlyHint interface {
	IsReadOnly() bool
}

type PathAware interface {
	AffectedPaths(args json.RawMessage) []string
}
```

- `ReadOnlyHint`：声明"永不改状态"。权限闸据此直接放行（`permissions.isReadOnly`，`policy.go`）；不实现默认按"非只读"处理——安全的默认值。
- `PathAware`：静态声明本次调用会碰哪些路径。权限闸用它做 cwd/秘密路径检查，快照管理器用它决定拍谁。返回 `nil` 表示"静态不可知"——`bash`/`bash_pty` 就是这一类（`bash.go:58`、`bash_pty.go:48` 都直接 `return nil`）。

### 2.2 `Result`：两种失败，别用错

`internal/tools/result.go`：`Result{Content string, IsError bool}`。失败语义分两层——

- **工具跑了但失败**（文件不存在、参数不合法）：返回 `Errf("...")`，`IsError=true`，模型读到错误文本后自行调整。**Go error 返回 nil**。
- **基础设施失败**（agent 应该浮给用户、可能重试）：返回非 nil 的 `error`。`executeOne` 会把它序列化成 `"execution error: …"` 的 tool-result，不会中断循环。

`Result.Truncate` 按 rune 截断，`executeOne` 出口统一调用 `res.Truncate(tools.DefaultMaxResultBytes)`（50000 字节，`registry.go`），单次调用刷不爆上下文。

### 2.3 `Registry`：注册、tier、与 load-bearing 的排序

`registry.go` 的 `Registry` 是 name→Tool 的并发安全 map，外加 tier 表和一个会话级 `FileTracker`（§5）。关键事实：

- **`Register(t)`**：同名后注册覆盖先注册——有意为之，MCP 工具可以遮蔽内置工具。tier 按 `defaultTier(name)` 自动判定（`read_file`/`bash`/`glob` 等十个是 `TierCore`，其余默认 `TierProfile`）；`RegisterWithTier` 显式指定。
- **`All()` 名字排序是 load-bearing 的**——注释原话："stable order maximizes prompt-cache hits across turns"。请求层按这个顺序序列化工具，乱序 = 缓存键漂移。
- **`AsLLMToolsFiltered(tiers...)`**：agent 每次发请求都走它（`a.Tools.AsLLMToolsFiltered(a.ActiveTiers...)`，agent.go 多处），按 tier 过滤后转成 `llm.Tool`。schema 的规范化**不在这里做**——`internal/llm` 的 `canonicalizeTools` 是单一事实源（[prefix-cache.md](prefix-cache.md) §3.2）。
- **派生注册表**：`Subset` / `TierTools` / `CloneForCWD` 都会共享父注册表的 `fileTracker`——否则子代理里 read-before-write 守卫会静默失效（代码注释里的 "T3.2 / adversarial HIGH-1"）。

### 2.4 注册点全景：你的工具该写在哪

| 注册点 | 谁 | 什么工具 |
|---|---|---|
| `internal/tools/builtins.go` `RegisterBuiltinsWithSandbox` | 三条装配路径共用（`cmd/dsc/main.go` 的 TUI 与 one-shot 两处 + `internal/acp/factory.go`） | 14 个无运行时依赖的内置工具：4 文件 + 2 bash + glob/grep/ls + todo_write + 4 git |
| `cmd/dsc/main.go`（TUI 与 one-shot 各一份） | 需要 agent/registry/provider 等运行时依赖的会话级工具 | `NewQuestionTool(a)`、`NewBackgroundBashToolWithSandbox`、`NewTaskStatusTool`、`NewWebFetchTool`、`NewWebSearchTool`、`NewLSPTool`、subagent/spawn_batch/worktree、`NewSkillReadTool`（显式 `TierCore`） |
| `internal/mcp` 桥接 | `mcp.BridgeAll(mcpReg)` → `reg.Register(t)` | 外部 MCP 服务器的工具，进同一接口 |
| `tools.RegisterMemoryTools` | 管理自己 memory store 的调用方 | remember / recall / forget |

## 3. 控制流走查：一次 tool call 的五道关

总入口是 `internal/agent/agent.go` 的 `runToolCalls`（step 级）与 `executeOne`（单调用级）。顺序与 [routing.md](routing.md) 的记载一致：**权限闸先行，PreToolUse hook（含 Duet）随后，最后才 `tool.Execute`**。

```text
runToolCalls（一个 step 的全部调用）
  ├─ ⓪ 快照（串行，先于并行执行）── Persister.TakeSnapshot(stepIdx, 所有调用 AffectedPaths 的并集)
  └─ 并行 executeOne（每个调用一个 goroutine）
        ├─ ① 调用限额 + 未知工具检查 + validateToolArgs（轻量 schema 校验）
        ├─ ② 权限闸 Permissions.Decide → Allow / Ask / Deny
        ├─ ③ PreToolUse hook（含 Duet builtin）── deny → "blocked by hook"
        ├─ ④ tool.Execute（bash 在这里进沙箱）
        └─ ⑤ PostToolUse / PostToolUseFailure hook → Result.Truncate(50000)
  └─ 结果按原始顺序回写 transcript（保持确定性）
```

- **⓪ 快照**在并行开跑前**串行**完成——这是 `/undo` 能一次回滚整个 step 的根基，§4 详述。
- **① `validateToolArgs`**（agent.go）只查三件事：是合法 JSON、是对象、`required` 字段齐全——"catches the most common model mistakes without a full JSON Schema validator dependency"。更深的畸形修复在 `internal/repair`。
- **② 权限闸**：`a.Permissions.Decide(permissions.Check{Tool, Args})`（`internal/permissions/policy.go`）。`Decide` 的优先级从上到下：全局 mode 旗标（yolo/read-only/ask-all/plan）→ 规则引擎（`Rules.Evaluate`）→ `MinModeFor` 最低权限模式 → 分层默认：只读工具直接放行；`PathAware` 工具逐路径做 symlink 解析（`resolveAffected`）+ 秘密模式 + cwd 围栏检查；`write_file`/`edit_file` 全路径安全则自动放行（除非 `ConfirmEdits`）；bash 走 `ClassifyBash` 意图分级 + allowlist（分级表见 [../reference/bash-validation.md](../reference/bash-validation.md)）。`Ask` 时 `executeOne` 发 `EventPermissionAsk`（自带 reply channel）并阻塞等 UI 回答；用户选 "always" 会经 `AllowBashPattern` 持久化模式。三种判决都会落 `ReceiptPermission` 回执。
- **③ PreToolUse hook**：`a.HookRunner.Run(ctx, hooks.EventPreToolUse, …)`。Duet 验证器就是注册在这个事件上的 builtin hook（`hooks.NewDuetHook`，`internal/hooks/builtin_duet.go`；`cmd/dsc/main.go` 的 `buildHookRunner` 注册）——只拦 destructive 调用，pro 模型裁决，网络失败 fail-open。细节与失败语义见 [routing.md](routing.md) 的 Duet 章节，此处不重复。
- **④ Execute + 沙箱**：沙箱不是管道的一站，而是 `bash`/`bash_pty` 工具**自带**的执行环境——`Bash` 结构体持有 `sandbox.Sandbox` + `sandbox.Profile`（`bash.go`），`Execute` 内部经 `wrapWithSandbox` 包裹命令。`sandbox.Sandbox` 接口（`internal/sandbox/sandbox.go`）四个方法：`Name`/`Available`/`Wrap`/`WasDenied`；macOS 实现是 seatbelt（`sandbox_darwin.go`），Linux 是 Landlock（`sandbox_linux.go`），其余 noop。其他工具不进沙箱——它们的围栏是 ② 的路径检查与 `tools.ResolveAndCheck`（`path_safety.go`）。
- **⑤ 收尾**：成功与失败各发一种 post hook（"decision is informational only — cannot undo completed tool execution"），结果经 `Truncate` 截断后返回。

## 4. 快照与 /undo

### 4.1 谁被快照：精确的说法

不是"写工具被快照"，而是：**step 内所有调用的 `AffectedPaths` 并集**（`runToolCalls` → `AffectedPathsFor`，`internal/agent/persistence.go`）。后果有二：

- `read_file` 也实现了 `PathAware`（为了权限检查），所以它读的文件也会被拍进快照——无害，多一份拷贝而已；
- `bash` 的 `AffectedPaths` 返回 `nil`，所以 **bash 不触发快照**。destructive bash 的安全网是 `ClassifyBash` 的 always-ask + Duet 验证器，不是快照——`internal/snapshots/manager.go` 的包注释明说了这个分工。

实现了 `AffectedPaths` 的内置工具共六个：`read_file`、`write_file`、`edit_file`、`apply_patch`（返回 patch 里全部目标路径）、`bash`、`bash_pty`（后两个返回 nil）。

### 4.2 原子多文件回滚的故事

`snapshots.Manager`（`internal/snapshots/manager.go`）：

- **Take**：每个受影响文件拷到 `.deepseek/snapshots/<sessionID>/<stepIdx>/<base64-of-abspath>`；**尚不存在的文件写 `.absent` tombstone**，这样 `/undo` 知道回滚时要删掉它（撤销"新建文件"）。所有写入走 `copyFileDurable`：临时文件 + fsync + 原子 rename + 目录 fsync——中断只会留下 `.tmp-*` 残片，永远不会留半截快照。
- **关键时序**：`runToolCalls` 在并行执行**开跑前**一次性拍完整个 step 的并集，并把当前 step 标记 `Snapshotted = true`（`/undo` 的对账按快照数而非 step 数计）。这就是"原子"的含义：一个 step 改了五个文件，`/undo` 一步全回。
- **Undo**：逆时序恢复最近 N 个 step（`/undo N`），恢复同样走 durable 写（"…/undo is the user's last line of defense, so it must not itself leave a truncated file"），然后**消费掉**该 step 目录——再 `/undo` 不会重复看到。
- **入口链**：TUI `/undo` 命令（`internal/tui/app.go` `applyUndo`，agent 运行中会拒绝）→ `session.Persister.Undo`（`internal/session/persister.go`）→ `Manager.Undo`。`Manager.Prune` 随会话清理旧目录。

## 5. 不变量与测试守卫

动 `internal/tools` 前先认这几条：

1. **schema 字节 = 缓存键**。`registry.go` 包注释第一段就在讲这个；守卫在 llm 侧——`TestCacheStableGolden`（`internal/llm/e2e_cache_stable_test.go`，golden 在 `testdata/cache_stable.golden.json`，故意改动后 `UPDATE_GOLDEN=1` 再生成）与 `TestMarshalCacheStableIsToolOrderIndependent`（`cache_stable_property_test.go`）。parity 侧 `tool_sort` 场景钉死按 function name 字典序排序（[parity.md](parity.md)）。
2. **名字排序**：`All()`/`AsLLMToolsFiltered` 必须返回 name-sorted 序列。`registry_subset_test.go` 的 `TestSubsetAllSorted` 守这条。
3. **tier 行为**：`registry_tier_test.go` 一族（`TestRegisterAutoTier`、`TestRegisterWithTier`、`TestRegisterOverwritePreservesExplicitTier`、`TestDefaultProfilesOnlyCore`…）。改 `defaultTier` 必须同步这里。
4. **read-before-write（T3.2）**：`FileTracker`（`file_tracker.go`）记录每个文件最后一次被读/写时的 `(mtime, size)`，写类工具拒绝编辑"从未读过"或"读后被外部改过"的文件。一个 tracker 被注册表及其全部派生品共享；**nil tracker 是 no-op**（单测可以不接）。你写新的改文件工具，必须接同一个 tracker（`RegisterBuiltinsWithSandbox` 里 `ft := NewFileTracker()` 的注入模式）。
5. **路径安全**：文件工具解析用户路径一律过 `ResolveAndCheck`（`path_safety.go`）——symlink 解析后再做 cwd 判定，权限闸的 `resolveAffected` 与它语义对齐（`policy.go` 注释明确两者必须一致）。
6. **单工具测试模式**：每个工具一个 `*_test.go`，直调 `Execute(ctx, json.RawMessage(...))` 断言 `Result`；有依赖就手写 fake（`task_status_test.go` 的 `fakeJobStatusController` 是标准样板）；碰文件系统用 `t.TempDir()`；注册断言照 `builtins_test.go` 的 `TestRegisterBuiltinsContainsApplyPatch`。沙箱有独立 e2e（`sandbox_e2e_darwin_test.go` / `sandbox_e2e_linux_test.go`）。

## 6. 完整实操：从零新增一个 tool

以一个玩具工具 `word_count`（统计文件的行/词/字节数）为例。蓝本是**最简的现存工具 `ls`**（`internal/tools/ls.go`，全文 72 行）——值类型 struct、无构造函数、无依赖，五个方法即完整。

### 第 1 步：实现 `internal/tools/word_count.go`

```go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// WordCount reports line/word/byte counts for a single file.
type WordCount struct{}

func (WordCount) Name() string { return "word_count" }

// 红线：这段文案一旦发布就是全员缓存键的一部分，措辞一次到位（§7）。
func (WordCount) Description() string {
	return "Count lines, words, and bytes in a single file, like wc. " +
		"Use read_file when you need the content itself."
}

func (WordCount) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "File to count.",
			},
		},
		"required": []string{"path"},
	})
}

// 只读声明：权限闸据此免提示放行（permissions.isReadOnly）。
func (WordCount) IsReadOnly() bool { return true }

func (WordCount) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("invalid args: %v", err), nil // 模型可见的错误 → Errf，Go error 留给基础设施失败
	}
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return Errf("word_count %s: %v", p.Path, err), nil
	}
	s := string(data)
	lines := strings.Count(s, "\n")
	words := len(strings.Fields(s))
	return Result{Content: fmt.Sprintf("%d lines, %d words, %d bytes", lines, words, len(data))}, nil
}
```

逐项对照 §2.1 的接口：四个必选方法签名一字不差；`MustParams` 把 schema 写成 Go 字面量（marshal 失败即 panic，实践中不会）；错误走 `Errf` 而非 Go error。并发契约：无共享可变状态，天然 goroutine-safe。

### 第 2 步：注册

无运行时依赖 → 进 `builtins.go` 的 `RegisterBuiltinsWithSandbox`，一行：

```go
r.Register(WordCount{})
```

三条装配路径（TUI、one-shot、ACP）自动同时获得它。tier 走 `defaultTier` 的 default 分支 = `TierProfile`；想进核心层就改用 `r.RegisterWithTier(WordCount{}, TierCore)` 或在 `defaultTier` 的 `TierCore` case 加名字（并同步 `registry_tier_test.go`）。

若工具需要运行时依赖（agent 句柄、网络客户端…），注册点改为 `cmd/dsc/main.go`——注意 TUI 与 one-shot **两处都要加**（`NewQuestionTool(a)` 在两处各出现一次就是先例）。

### 第 3 步：安全语义自检

- 只读工具：实现 `IsReadOnly() bool { return true }`，到此为止。
- **改文件的工具**还要：实现 `PathAware.AffectedPaths`（快照与权限闸都靠它）；路径解析过 `ResolveAndCheck`；接受并尊重注册表共享的 `*FileTracker`（read-before-write）。照抄 `write_file.go` 的三件套。
- 执行外部命令的工具：别绕过 `internal/sandbox`，照 `bash.go` 持 `Sandbox`+`Profile` 经 `wrapWithSandbox` 走。

### 第 4 步：测试 `internal/tools/word_count_test.go`

照 §5 第 6 条的模式写四类用例：

```go
func TestWordCountName(t *testing.T)        // Name() == "word_count"（照 task_status_test.go 的 Name 用例）
func TestWordCountIsReadOnly(t *testing.T)  // IsReadOnly() == true
func TestWordCountExecute(t *testing.T)     // t.TempDir() 写文件 → Execute → 断言 Content
func TestWordCountExecuteErrors(t *testing.T) // 文件不存在 / 畸形 args → Result.IsError，且 err == nil
```

再到 `builtins_test.go` 补一个注册断言（照 `TestRegisterBuiltinsContainsApplyPatch`：`RegisterBuiltins` 后 `r.Get("word_count")` 必须命中）。

### 第 5 步：缓存验证收尾

`go test ./internal/tools/ ./internal/llm/` 应**全绿**——新工具不出现在 golden 的固定请求里，golden 变红说明你动到了序列化器本身。然后跑一次真实会话取证（`dsc -p … -trace-jsonl` + `dsc trace inspect`，判据 `prefixes 1`）——完整流程与判读见 [prefix-cache.md](prefix-cache.md) §8.1，那里是单一事实源，此处不复述。

最后更新 [../reference/tools.md](../reference/tools.md) 的用户清单（工具数与表格行）。

## 7. cache 红线

`registry.go` 包注释的原话："every byte of tool JSON ends up in the prompt prefix and stable bytes are what makes the API-side prompt cache fire."

- **字节链路**：`Tool.Name()/Description()/Parameters()` → `AsLLMToolsFiltered` → `canonicalizeTools`（`internal/llm/static_prefix.go`，排序 + 规范化的单一事实源）→ `MarshalCacheStable` 的 wire 字节 = 缓存键（[prefix-cache.md](prefix-cache.md) §3）。
- **新增 tool** = 全体用户一次性的前缀变化：每个会话首 turn 付一次冷 miss，之后重钉基线、恢复稳态。这是可接受的、随版本发布的成本。
- **事后润色 Description** = 又一次全员 re-miss——所以 §6 第 1 步说"措辞一次到位"，别打算下个 PR 改一个词。攒批改、跟二进制版本边界走（与系统提示同规约，prefix-cache.md §8.2）。
- **会话中途注册**（MCP 晚连、lazy tier 激活）是预期内的一次 tools 漂移；epoch 冻结时它会变成 `PendingToolAdded` 的 pending change，对模型不可见直到 epoch 切换（prefix-cache.md §3.4）——策略层兜底，不需要工具作者操心。
- **守卫**：`TestCacheStableGolden` 钉字节、`TestMarshalCacheStableIsToolOrderIndependent` 钉排序无关性、parity `tool_sort` 场景钉排序本身（[parity.md](parity.md)）。

## 8. 改既有 tool 的注意事项

- **改 `Execute` 行为、不动 schema 字节**：缓存无感，正常改、正常测。
- **改 `Description`/`Parameters` 的任何字节**：全员 re-miss（§7），攒批、随版本发。改完跑 §6 第 5 步的取证流程。
- **改名**：等于删一个工具再加一个——双重前缀漂移，且模型对旧名的"肌肉记忆"作废。几乎永远不值得。
- **改 tier**（`defaultTier` 或注册处）：影响 `AsLLMToolsFiltered` 的输出集合，同样是前缀字节变化；同步 `registry_tier_test.go`。
- **改 bash 分级**（`bash_validate.go`）：`ClassifyBash` 同时服务权限闸与 Duet 的 destructive 判定，两道闸联动——流程见 [routing.md](routing.md) 的修改场景章节与 [../reference/bash-validation.md](../reference/bash-validation.md)。
- **改文件工具的路径处理**：`ResolveAndCheck`（工具层）与 `resolveAffected`（权限层）必须保持同一 symlink 语义，改一边就检查另一边（`policy.go` 注释点名了这条对齐）。

## 快速索引

| 想看什么 | 去哪里 |
|---|---|
| `Tool`/`ReadOnlyHint`/`PathAware` 接口、`Registry`、tier | `internal/tools/registry.go` |
| 14 个内置工具的注册 | `internal/tools/builtins.go` `RegisterBuiltinsWithSandbox` |
| 会话级工具装配 | `cmd/dsc/main.go`（TUI 与 one-shot 两处） |
| 执行管道（限额→权限→hook→Execute→截断） | `internal/agent/agent.go` `executeOne` |
| step 级快照时序 | `internal/agent/agent.go` `runToolCalls`、`internal/agent/persistence.go` `AffectedPathsFor` |
| 权限判决全逻辑 | `internal/permissions/policy.go` `Decide` |
| Duet builtin hook | `internal/hooks/builtin_duet.go`、深入见 [routing.md](routing.md) |
| 快照与回滚 | `internal/snapshots/manager.go`、`internal/session/persister.go`、`internal/tui/app.go` `applyUndo` |
| read-before-write 守卫 | `internal/tools/file_tracker.go` |
| 沙箱接口与实现 | `internal/sandbox/sandbox.go`、`sandbox_darwin.go`、`sandbox_linux.go` |
| 缓存字节守卫 | `internal/llm/e2e_cache_stable_test.go`、[prefix-cache.md](prefix-cache.md)、[parity.md](parity.md) |
