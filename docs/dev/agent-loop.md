# 深入：agent turn 循环

本文是 [architecture.md](architecture.md) §2 的深入篇之一，面向准备修改 `internal/agent` 的贡献者。读完后你应该能回答：循环在哪、为什么 `finish_reason=stop` 不一定停、压缩什么时候触发、坏 tool call 怎么被救回来、thinking 为什么有时不开，以及——动了这些之后哪些测试会拦你。

一个命名陷阱先排掉：**主循环不在 `loop.go`**。`internal/agent/loop.go` 只有 32 行，是 `dsc --compact` 的 CLI 助手（`RunCompact`）；主循环是 `internal/agent/agent.go` 的 `Agent.Run`，循环体挂在函数内的 `agentLoop:` 标签上。本文锚点一律给「文件 + 函数名」，行号会漂移，函数名更稳。

## 0. 解决什么问题

DeepSeek 的流式 API 每次只回「一步」：一段 reasoning、一段文本、零或多个 tool call、一个 finish reason。要把它变成能连续干活的 agent，循环层必须解决四件事：

1. **续轮判定**——什么时候继续发请求，什么时候把控制权还给用户（且 finish reason 不可尽信，见 §2.3）；
2. **上下文管理**——1M 窗口终会写满，要在写满前压缩、写满后自救（§2.6）；
3. **输出修复**——模型会把 tool call 写进 reasoning 文本、会输出截断的 JSON 参数，循环不能因此崩溃或傻停（§2.4）;
4. **每轮决策**——thinking 开不开、effort 给多少、用哪个模型，每轮都要重新决定，且决策不能破坏前缀缓存（§2.5）。

## 1. 核心数据结构

都在 `internal/agent` 下：

- **`Agent`**（`agent.go`）——循环的全部状态：`Messages`（完整对话）、`steps`（本轮 step 历史）、`StopWhen`（停止条件链）、`CompactionCfg` / `SemanticCfg`（压缩配置）、`Thinking` / `ThinkingMode` / `AutoReasoning`（thinking 决策输入）、`stormBreaker`（重复调用熔断）。构造函数 `New` 给默认值：`Thinking: true`（force-on）、`MaxSteps(50)`、`loopDetection(5, 3)`、`NewStormBreaker(6, 3)`。
- **`StepRecord`**（`stop_conditions.go`）——一步的结果摘要：`FinishReason`、`Usage`、`ToolCalls`、`Model`（升级轮记录实际产出模型）、`MessageCount`（步前 transcript 边界，供 `/undo` 对账）。停止条件只看 `[]StepRecord`，不碰 `Agent` 内部。
- **`StopCondition`**（`stop_conditions.go`）——`func(steps []StepRecord) (stop bool, reason StopReason)`。`Run` 在每步完成后顺序执行 `a.StopWhen`，首个命中者生效。
- **`StopReason`**（`stop_conditions.go`）——`Run` 的终止原因枚举。只有两个值算「干净完成」（`IsSuccess()`），其余一律不得渲染成 done：

  | 值 | `String()` | 含义 | 成功？ |
  | --- | --- | --- | --- |
  | `StopModelDone` | `model_done` | 模型自然收尾（无 tool call） | ✓ |
  | `StopVerifiedDone` | `verified_done` | 模型收尾且 Verify hook 通过 | ✓ |
  | `StopMaxSteps` | `max_steps` | 步数上限（默认 50） | ✗ |
  | `StopLoopDetected` | `loop_detected` | 同一 tool call 重复过多 | ✗ |
  | `StopUserRequested` | `user_requested` | 用户显式中止 | ✗ |
  | `StopContextCancel` | `context_cancel` | 环境取消（`ctx.Err()`） | ✗ |
  | `StopStepTimeout` | `step_timeout` | per-step 超时 | ✗ |
  | `StopUnknown` | `unknown` | 错误等异常出口 | ✗ |
- **`streamResult`**（`agent.go`）——一次流式往返的汇总：`text` / `reasoning` / `toolCalls` / `finish` / `usage` / `blocks`，由 `consumeStream` 从 SSE 事件聚合而来。
- **`CompactionConfig`**（`compact.go`）——确定性压缩配置；`SemanticCompactionConfig`（`semantic_compact.go`）——语义压缩的比例阈值。数值见 §2.6。

## 2. 控制流走查

### 2.1 一轮的骨架：`Agent.Run`

`Run(ctx, userPrompt)`（`agent.go`）先做轮级初始化：清 stop 标记与 steer 队列、重置 loop-nudge 状态、跑 `SessionStart` hook、把用户消息追加进 `Messages` 并持久化。然后进入 `agentLoop`，每次迭代是「一步」：

```
agentLoop:
  ① ctx 取消检查（区分 StopUserRequested / StopContextCancel）
  ② drainSteer —— 消费 turn 进行中用户追加的指令
  ③ stepContext —— 套上 per-step 超时（StepTimeout，同时覆盖模型轮与工具执行）
  ④ runStep —— 一次 LLM 往返（见 §2.2）
  ⑤ 错误分流：上下文溢出 → 压缩一次重试（§2.6）；取消/超时 → 对应 StopReason
  ⑥ hasTools := len(step.ToolCalls) > 0 —— 续轮判定的唯一依据（§2.3）
  ⑦ StopWhen 链 —— MaxSteps / loopDetection；首个命中即停
     （StopLoopDetected 首次命中例外：injectLoopBreakNudge 给模型一次换路机会）
  ⑧ !hasTools → 自然出口：先 drainSteer（迟到的 steer 让 turn 续命），
     再跑 Verify hook（通过 → StopVerifiedDone；失败 → 注入反馈后返回），
     否则 StopModelDone
  ⑨ hasTools → runToolCalls（工具批执行）→ maybeFeedPostEditDiagnostics
     → maybeRunVerifyHook → maybeCompact（§2.6）→ 回到 ①
```

几个容易看漏的点：

- **⑦ 在 ⑨ 之前**：停止条件在工具执行*前*检查，这样 loop-detection 能在重复的 tool call 真正执行前拦下它。首次 `StopLoopDetected` 不硬停——`injectLoopBreakNudge` 给每个悬空 tool call 合成一个结果（保住 tool_call/tool_result 配对），再追加一条 user 消息提示换路，`loopFloor` 抬高让触发检测的重复在恢复轮里被原谅；第二次命中才真停（守卫：`loop_nudge_test.go` 的 `TestLoopDetectNudgeContinuesOnce` / `TestLoopDetectHardStopsOnSecondDetection`）。
- **steer 有两个消费点**（② 和 ⑧）：⑧ 处理的是「steer 落在最后一步执行期间」的边角——没有它，turn 结束时 steer 会滞留队列（守卫：`loop_mock_test.go` 的 `TestLoopMidTurnSteerInjectsBeforeNextStep`）。
- **工具错误不终止循环**：`runToolCalls` 把工具错误序列化成 tool-result 块喂回模型，循环里没有工具失败的 abort 分支。

### 2.2 一步之内：`runStep`

`runStep`（`agent.go`）是一次完整的 LLM 往返：

1. `refreshGitContext` 刷新动态尾部的 git 上下文；
2. `routeTurn(lastUserText, a.repairErrorsLastTurn)` 决定本轮 `(model, thinking, effort)`（§2.5；实参是上一提交回合的不可恢复修复计数，回合末由 `runStep` 记账——见 [routing.md](routing.md) 信号清单）；
3. 组装 `llm.Request`：`fullMessages()` + `Tools.AsLLMToolsFiltered(a.ActiveTiers...)` + `ThinkingEnabled(turnThinking)` + effort。若前缀 epoch 已冻结，则改用 `epoch.FrozenTools` / `fullMessagesWithFrozenSystem`（缓存稳定性，详见前缀缓存深入篇与 [../reference/prefix-cache.md](../reference/prefix-cache.md)）；
4. 预算闸门：发流前用投影成本对会话预算做检查；
5. `streamWithReissue` 发流：瞬态中流停摆（`ErrFirstTokenTimeout` / `ErrChunkStall`，见 `isReissuableStreamErr`）按原请求重发一次，放弃前抢救部分输出（`partialBlocks`——刻意**不含** tool call，半成品 ToolUseBlock 会在下一请求留下悬空 tool_call）；`consumeStream` 把 SSE 事件聚合成 `streamResult`，finish reason、usage、tool calls 都从终结事件 `llm.EventFinish` 来；
6. `repairToolCalls(ctx, sr.reasoning, sr.text, sr.toolCalls, &sr.blocks)` 跑修复管线（§2.4），得到最终的 `assembledCall` 和本轮不可恢复修复错误数 `repairErrors`；
7. 同轮升级（T2.3）：模型自报 `<<<NEEDS_PRO>>>` 或 `repairErrors >= escalationRepairThreshold`（常量 = 3，`agent.go`）时，用强模型把**同一轮**重发一次，flash 轮丢弃（只计费、不入 transcript），升级轮重跑修复管线；
8. 追加 assistant 消息、持久化、计费、校准 chars-per-token、发布缓存归因回执，返回 `StepRecord`。

### 2.3 finish-reason 覆写

这是本循环对 DeepSeek 最重要的防御。`Run` 的续轮判定（`agent.go`，紧跟 `a.steps = append` 之后）：

```go
// finish-reason override: even if the model said finish_reason=stop,
// if it emitted tool calls we keep looping. See docs/design.md §6.4.
hasTools := len(step.ToolCalls) > 0
```

注意它**根本不读 `step.FinishReason`**——续轮只看修复管线产出的 tool call 列表。这一刀切掉两类故障：模型一边发 tool call 一边说 `stop`（直接覆写）；模型把 tool call 写在 reasoning 文本里、结构化字段为空（scavenge 捞回后 `hasTools` 仍为真，照样续轮）。`FinishReason` 仅作为记录保留在 `StepRecord` 里供停止条件与 trace 使用。

守卫：`loop_mock_test.go` 的 `TestParityScenario_finish_stop_with_tool_calls` 与 `TestLoopFinishReasonOverrideContinuesAndPairsResult`（后者还断言被执行的 tool call 有配对的 tool result）；`repair_integration_test.go` 的 `TestFinishReasonOverride`。

### 2.4 repair 管线：scavenge、截断修复、storm-breaker

入口是 `repairToolCalls`（`agent.go`），每步流式结束后必经，分六步；纯解析逻辑在 `internal/repair` 包：

1. **Scavenge**——`repair.ScavengeToolCalls(reasoning, content, allowed, opts)`（`internal/repair/scavenge.go`）从 reasoning 与 content 文本里捞显式 tool call：先扫 JSON 形态（`extractToolCalls`），再扫 DSML XML 信封（`parseDSMLToolCalls`，`dsml.go`）；只接受注册表里存在的工具名，按「名字 + 规范化参数」去重，默认每源最多扫 100 KiB、最多捞 4 个 call（`ScavengeOptions` 默认值）。
2. **合并**——声明的（结构化 `tool_calls`）与捞回的合并成一个列表。
3. **参数修复**——对每个 call 跑 `repair.RepairJSONArgs`（`internal/repair/truncation.go`，内部走 `RepairTruncatedJSON`）：剥控制字符、去尾逗号、补未闭合的括号/引号。三种结局：`NeedMore`（截断发生在字符串*内部*，补全等于编造内容——丢弃该 call、计一次 `repairErrors`，让模型重发）；`Changed`（修好了，发 `KindArgsCompleted` 事件）；`!Valid`（修完仍非法，保留 call 但计 `repairErrors`，工具层会回可见错误）。
4. **Rehydrate**——被 schema 扁平化过的参数还原嵌套（`adapter.Rehydrate`，与本循环主线关系不大）。
5. **Storm-breaker**——`a.stormBreaker.Filter`（`internal/repair/storm.go`）：滑动窗口 6 内同一「工具 + 规范化参数」重复达 3 次即抑制（`New` 里 `NewStormBreaker(6, 3)`）；**只抑制只读工具**，mutating 工具永不抑制（守卫：`repair_integration_test.go` 的 `TestRepairIntegration_MutatingNeverSuppressed`）；每次抑制计一次 `repairErrors`。
6. **重建 blocks**——assistant 消息里的 ToolUseBlock 重写为「最终保留的 call」，保证 transcript 与实际执行一致（守卫：`TestRepairIntegration_BlocksUpdatedAfterRepair`）。

`repairErrors` 的去向：同轮内驱动 T2.3 升级（§2.2 第 7 步）。成功的修复（`KindArgsCompleted`）刻意**不**计入——只有失败才升级。

### 2.5 thinking 选择

决策函数是 `selectTurnThinking(userText, repairErrorsLastTurn)`（`agent.go`），由 `routeTurn` 每轮调用：

- `ThinkingMode == "off"`（环境变量 `DEEPSEEKCODE_THINKING_MODE`，供成本 A/B）→ 永不 thinking；
- `ThinkingMode == "adaptive"` → 修复轮（`repairErrorsLastTurn > 0`）必 thinking；否则仅首轮（`turnsSeen == 0`）且消息非琐碎时 thinking；
- 默认（`""` / `"on"`）→ `AutoReasoning` 开则走 `llm.SelectThinking`（关键词启发式）；否则 `a.Thinking && !llm.IsTrivialMessage(userText)`——而 `New` 里 `Thinking: true`，所以**出厂默认是 force-on，唯一的豁免就是琐碎消息门控**。

`llm.IsTrivialMessage`（`internal/llm/auto_reasoning.go`）的实际条件，以代码为准：

```go
func IsTrivialMessage(userText string) bool {
	trimmed := strings.TrimSpace(userText)
	lower := strings.ToLower(trimmed)
	for _, kw := range highEffortKeywords {
		if strings.Contains(lower, kw) {
			return false
		}
	}
	return utf8.RuneCountInString(trimmed) <= trivialMaxRunes
}
```

即：先 `TrimSpace`，**命中任一高 effort 关键词（`debug`、`error`、`调试`、`错误`、`报错`、`出错`、`崩溃`，及繁体/日文变体——见同文件 `highEffortKeywords`）则无论多短都非琐碎**；否则 rune 数 `<= trivialMaxRunes`（常量 = 8）才算琐碎。所以「你好」不 thinking，「报错」（2 rune）照样 thinking。这个门控的来由：非 reasoning 轮的 `reasoning_content` ≈ 答案本身，force-on 会让「你好」渲染出一个装着答案副本的「Thought for <1s」块。

配套的 `effectiveReasoningEffort(thinking)`（`agent.go`）：不 thinking → 返回空（wire 字段省略）；thinking 且配置了合法 effort → 用配置值；否则默认 `llm.ReasoningEffortMax`。

不变量：thinking 与 effort 只改 wire 请求的 `reasoning_effort` 字段，**不进静态前缀**，所以每轮翻转不会引起缓存失效（`selectTurnThinking` 与 `routeTurn` 的文档注释都钉了这条）。守卫：`internal/llm/auto_reasoning_test.go` 的 `TestIsTrivialMessage` 与 `TestSelectThinking*` 系列；`loop_mock_test.go` 的 `TestLoopThinkingSerializesAsStruct` / `TestLoopReasoningEffortAppearsWhenThinkingEnabled` / `TestLoopReasoningEffortOmittedWhenThinkingDisabled` / `TestLoopReasoningEffortDefaultsMaxWhenThinkingEnabled`。

### 2.6 compaction：两级触发 + 溢出自救

数值先钉死（`internal/agent/compact.go`）：

- `MaxContextTokens = 1_000_000`（包级常量，DeepSeek V4 的 1M 窗口；实例可经 `Agent.MaxContextTokens` 字段覆盖）；
- `DefaultCompactionConfig()`：`AutoCompactInputTokens: 800_000`（自动压缩线，环境变量 `DEEPSEEKCODE_AUTO_COMPACT_INPUT_TOKENS` 可覆盖，非法值回落默认而非崩溃）、`PreserveRecentMessages: 4`、`TailTokensFloor: 16_384`、`MinFoldTokens: 400`。

**常规触发点**只有一个：`Run` 循环每个执行了工具的迭代末尾调 `maybeCompact(ctx)`（`agent.go`）。它的注释钉了一条设计原则：压缩失败只上报、永不中断循环——压缩是优化，不是正确性要求。内部两级：

1. **语义路径**（默认开，`DisableSemanticCompaction` 关）：`ContextPressure` 算占用比例，`ShouldSemanticCompact`（`semantic_compact.go`）按阈值给动作——`>= 0.50` 警告（`WarnThreshold` 默认值）、`>= 0.90` 压缩（`CompactThreshold` 默认值）。压缩动作走 `SemanticCompact`：用 LLM 把窗口前段折叠成摘要。
2. **确定性兜底**：语义路径没产出时落到 `CompactSession`，按 `ShouldCompact` 的估算 token 阈值触发。阈值先经 `reconcileCompactThreshold` 与比例阈值取小——默认参数下 `min(800_000, 0.90 × 1_000_000) = 800_000`，绝对线更严、先触发（该函数注释推演了这笔账）。

两条路径压缩成功后做同一组收尾（`maybeCompact` 两个分支各一份）：`Messages` 替换为「摘要消息 + 保留尾部」、被折叠消息归档到 `~/.deepseek/archive`、`cacheEpoch.afterCompaction()` 跳缓存代际、`Tools.FileTracker().Clear()`（折叠掉的 `read_file` 结果已不在 transcript，强制改前重读）、发布 `EventCompaction` / `EventSemanticCompaction`。

**溢出自救**是另一条独立路径：`runStep` 返回 400「context too long」（`llm.IsContextOverflow`，`internal/llm/errors.go`）时，`Run` 在 `overflowRetried` 闸门内压缩一次再重试该步。走的是 `compactForOverflow`（`agent.go`）——临时钉死确定性路径（语义摘要自己也要发一大段对话，可能再次溢出），借 `ForceCompact` 把阈值压到 1 强制折叠。每个「新鲜溢出」只许自救一次，但任一步成功完成后闸门复位，长会话里后续溢出有自己的机会。

守卫：`loop_mock_test.go` 的 `TestLoopContextOverflowRoutesToCompaction`、`loop_compact_test.go`（`--compact` 路径）、`compact_test.go` / `semantic_compact_test.go` / `semantic_compact_threshold_test.go`（阈值与边界）、`e2e_compact_test.go` / `e2e_compact_full_test.go`（mock 服务器全链路）、`compact_frozen_prefix_test.go`（压缩不得动冻结前缀）。

### 2.7 循环的观测点

调试循环行为时不必加 print——循环在每个关键节点都向 `Bus`（`bus.go`）发布结构化事件，TUI 直接订阅、gateway 转成 SSE（接线见 [architecture.md](architecture.md) §1）。与本文相关的：

- **步与轮的边界**：每步结束发 `EventStepFinish`（续轮的步 `Reason: StopUnknown`，终止步带真实 `StopReason`）；`Run` 退出时 defer 发 `EventDone{Reason, Err}`——无论哪条出口路径都保证发出。
- **流式过程**：`consumeStream` 把 `EventTextDelta` / `EventReasoningStart` / `EventReasoningDelta` / `EventReasoningEnd` 实时转发到总线（UI 用这对 Start/End 界定 thinking 块的渲染边界，订阅方在 `internal/tui/app.go`）。
- **修复**：管线每个动作发一条 `EventRepair{Kind, Tool, CallID, ...}`，Kind 对应 `internal/repair` 的 `KindArgsCompleted` / `KindArgsNeedMore` / `KindArgsInvalid` / `KindSuppressed` 等；同时经 `publishRepairEvent` 落持久化回执。
- **压缩**：`EventCompactionWarning`（过警告线）→ `EventCompaction` + `EventSemanticCompaction`（成功后，带前后前缀哈希）。
- **升级与自救**：`EventEscalated{Trigger, FromModel, ToModel}`；溢出自救、流重发、预算跳过升级等都以 `EventInfo` 文本事件留痕。
- **缓存归因**：每步一条 `EventCacheReceipt`（命中/未命中 token 与主因），是 TUI 缓存 HUD 的数据源。

写循环测试时有两个断言面：多数 `loop_mock_test.go` 测试断言 `Run` 的返回值（`StopReason` / `err`）、工具执行计数与 `a.Messages` 形态；需要观察过程行为时则 `a.Bus().Subscribe(...)` 收事件（如 `TestLoopContextOverflowRoutesToCompaction` 靠订阅确认压缩确实发生）。配 `internal/llmtest.NewServer` 的脚本化 turn，全程离线。

## 3. 不变量与测试守卫

改循环前先知道哪些行为被钉死了：

1. **agent 级 parity 三场景**（登记在 [parity.md](parity.md)「Agent-level parity scenarios」一节，mock 回路验证、无 golden 文件）：
   - `finish_stop_with_tool_calls` → `internal/agent/loop_mock_test.go`（`TestParityScenario_finish_stop_with_tool_calls`）；
   - `tool_call_in_reasoning_scavenge` → `internal/repair/scavenge_test.go`；
   - `truncated_tool_args_repair` → `internal/agent/loop_mock_test.go`（`TestParityScenario_truncated_tool_args_repair`）。
2. **tool_call / tool_result 永远配对**——loop-nudge 给悬空 call 合成结果、`partialBlocks` 不输出半成品 ToolUseBlock、repair 管线重建 blocks，全都在维护这一条；破坏它的直接后果是下一次请求被 API 拒收。
3. **前缀稳定**——循环的所有每轮决策（model / thinking / effort / steer / nudge）只动消息尾部或非前缀的 wire 字段，永不碰静态前缀；压缩跳代际但不重建前缀（`compact_frozen_prefix_test.go`、`prefix_epoch_test.go`）。
4. **压缩永不中断循环**；**工具错误永不中断循环**——两个「失败即喂回/上报」的兜底分支没有 abort 路径。
5. **新增 `StopReason` 必须同时维护 `String()` 与 `IsSuccess()`**（`stop_conditions_test.go` 的 `TestStopReasonString`、`loop_mock_test.go` 的 `TestStopReasonIsSuccess`）。

离线跑全套（不碰网络，mock 服务器在 `internal/llmtest`）：

```sh
go test ./internal/agent/ ./internal/repair/ ./internal/llm/
```

## 4. 常见修改场景实操

**想给循环加一个新的续轮/停止条件。** 两条路：若是「满足条件就停」，实现一个 `StopCondition`（`stop_conditions.go`），在 `New`（`agent.go`）的 `a.StopWhen` 字面量里接线，或让调用方自行追加——注意它在 `Run` 的 ⑦ 位执行，先于工具执行；若是「本该停但要续命」（类似 steer/nudge），改 `Run` 的 `!hasTools` 分支。会拦你的测试：`stop_conditions_test.go`（`TestMaxSteps` / `TestLoopDetection` 系列钉了现有条件的触发边界）、`loop_mock_test.go`（`TestParityScenario_finish_stop_with_tool_calls` 钉续轮判定、`TestLoopMidTurnSteerInjectsBeforeNextStep` 钉 steer 出口、`TestLoopUserRequestedStop` / `TestLoopStepTimeoutIsNonSuccess` 钉终止语义）、`loop_nudge_test.go`（钉 nudge 只给一次机会）。新增 `StopReason` 别忘了 §3 第 5 条。

**想调压缩阈值或换触发策略。** 数值改 `DefaultCompactionConfig` / `defaultSemanticCompactionConfig`，或用环境变量做实验；策略改 `maybeCompact` / `ShouldCompact` / `ShouldSemanticCompact`。会拦你的测试：`semantic_compact_threshold_test.go`（阈值语义）、`compact_test.go`（窗口与边界调整，tool_use/tool_result 不许被切断在 `adjustBoundary`）、`loop_mock_test.go` 的 `TestLoopContextOverflowRoutesToCompaction`（溢出自救必须走确定性路径）、`e2e_compact_full_test.go`。

**想改 repair 管线（加一种新的捞取形态/修复策略）。** 解析逻辑放 `internal/repair`（scavenge/truncation/storm 各自带表驱动测试与 `corpus_test.go` 语料），接线在 `agent.go` 的 `repairToolCalls`。会拦你的测试：`internal/repair/scavenge_test.go` / `truncation_test.go` / `storm_test.go`，以及 `internal/agent/repair_integration_test.go`（管线端到端：捞回的 call 会执行、修好的参数会执行、mutating 永不抑制、blocks 与执行一致）。注意 `repairErrors` 的计数语义（只计失败）牵动升级阈值。

**想改 thinking 门控。** 关键词与琐碎判定在 `internal/llm/auto_reasoning.go`（`IsTrivialMessage` / `SelectThinking`），轮级决策在 `agent.go` 的 `selectTurnThinking`。会拦你的测试：`internal/llm/auto_reasoning_test.go`（关键词矩阵 + `TestIsTrivialMessage` 的正反例）、`internal/agent/auto_reasoning_test.go`、`loop_mock_test.go` 的 effort 序列化三连（§2.5）、`route_turn_test.go`（路由与 thinking 的组合）。改完务必确认没有把任何决策写进系统提示或工具 schema——那会破坏 §3 第 3 条。

迭代时只跑被点名的守卫比全量快得多，例如：

```sh
go test -run 'TestParityScenario|TestLoopFinishReason' ./internal/agent/
go test -run TestIsTrivialMessage ./internal/llm/
```

---

相关阅读：请求生命周期全景见 [architecture.md](architecture.md) §2；wire 层为什么长这样见 [model-compatibility.md](model-compatibility.md)；场景登记与 golden 机制见 [parity.md](parity.md)；前缀缓存的运行期防线见 [../reference/prefix-cache.md](../reference/prefix-cache.md)。
