# 深入：Flash→Pro 路由与 Duet 验证器

本文是 [architecture.md](architecture.md) §2-④ 的深入篇之一，面向准备修改 `internal/routing`、agent 升级通道或 Duet 验证器的贡献者。用户视角的 Duet 故事（HUD 显示、失败模式表）见 [../reference/duet.md](../reference/duet.md)，本文不重复。读完后你应该能回答：哪些信号会把一个 turn 升到 pro、判定发生在循环的哪一点、Duet 在什么时机以什么标准否决一个工具调用，以及——改了这些之后哪些测试会拦你。

锚点一律给「文件 + 函数名」，行号会漂移，函数名更稳。

## 0. 解决什么问题（为什么路由）

`dsc` 默认整个主循环跑在 `deepseek-v4-flash` 上。`deepseek-v4-pro` 的缓存未命中输入 / 输出价格是 flash 的 **3 倍**（¥3/¥6 vs ¥1/¥2 每 1M tokens，完整定价表见 [../reference/pricing.md](../reference/pricing.md)，不在此复制）；wire 层约束与模型能力差异见 [model-compatibility.md](model-compatibility.md)。把主循环常驻 pro，等于为大量机械 turn（读文件、跑测试、改一行）支付 3 倍单价，而这些 turn 用 flash 做得一样好。

所以 pro 的钱只在三个「外科手术式」的时机花：

1. **turn 前路由**（§2.1–2.2）——发请求前用零成本启发式判定这个 turn 是否值得直接上 pro；
2. **turn 内升级**（§2.3）——flash 已经跑完一轮但自陈力不从心或修复连续失败时，同一 turn 在 pro 上重发一次；
3. **Duet 验证**（§3）——destructive 工具调用执行前，单独发一次一次性 pro 请求做裁决。

把三条通道放在一个 step 的时间轴上看：

```
用户输入
  │  （Run 入口，agentLoop 之前）
  ├─ 澄清前置闸 shouldClarify → 太模糊？问一句即返回，0 模型调用      §2.4
  ▼
runStep ─ routeTurn ──→ 通道① turn 前路由：本步用 flash 还是 pro      §2.1–2.2
  │
  ├─ 流式产出 + repairToolCalls
  │
  ├─ escalationTrigger → 通道② turn 内升级：丢弃 flash 轮，pro 重发    §2.3
  │
  └─ executeOne（逐个工具调用）
        ├─ 权限闸（permissions.Policy）
        ├─ 通道③ Duet：destructive 调用先过 pro 裁决                  §3
        └─ tool.Execute
```

三条通道共享一条红线：**任何模型 / thinking / effort 切换都只改 `llm.Request` 的对应字段，绝不动 Static Prefix**，所以路由不会移动 DeepSeek 的缓存键（机制见 [prefix-cache.md](prefix-cache.md)）。`routeTurn` 与 `routing` 包的文档注释都显式声明了这条不变量。

## 1. 核心数据结构

路由侧都在 `internal/routing/classifier.go`（整个包只有 `classifier.go` + `clarify.go` 两个实现文件）：

- **`routing.Config`** —— 分类器的静态参数：
  - `FlashModel` / `ProModel` —— 两档模型 id；
  - `StickyTurns` —— 升级后在 pro 上停留几步（防抖）。
- **`routing.Signals`** —— 逐 turn 输入：
  - `UserText` —— 用户原文；
  - `RepairErrorsLastTurn` —— 上一轮工具修复错误数。
- **`routing.Decision`** —— 裁决结果：
  - `Model` / `Thinking` / `Effort`（`""`、`low`…`max`）；
  - `StickyLeft` —— 剩余粘滞步数，跨步携带；
  - `Reason` —— `sticky` / `repair_errors` / `hard_reasoning` / `mechanical` 四值之一，trace 与测试都断言它。

agent 侧（`internal/agent/agent.go`）：

- **`Agent.AutoRoute`** —— 通道①总开关；
- **`Agent.EscalationModel`** —— pro 目标，同时是通道②的开关（非空即启用）；
- **`Agent.lastRoute routing.Decision`** —— 粘滞状态跨步携带（session 内存级，不持久化）；
- **`Agent.AutoClarify`** —— 澄清前置闸开关；
- **`escalationRepairThreshold`** —— 包级常量 `= 3`，通道②的修复错误阈值。

Duet 侧：

- **`config.DuetConfig`**（`internal/config/config.go`）—— 四个键全部被运行时消费（`cmd/dsc/main.go` 的 `buildHookRunner`）：`enabled`（默认 `true`）、`extra_destructive_patterns`，以及经 `hooks.DuetOptions` 传入的 `validator_timeout_ms`（单次 `ValidatePro` 的超时上限，默认 10000ms）与 `retry_on_failure`（瞬态失败重试一次再 fail-open，默认 `true`）；后两者由 `internal/hooks/builtin_duet_test.go` 钉住。
- **`hooks.HookOutput`**（`internal/hooks/hooks.go`）—— Duet 以 builtin hook 形式存在，裁决值为 `allow` / `deny` / `ask` / `continue`；`Runner.Run` 聚合时 `deny` 短路（`runner.go`）。
- **`tools.BashIntent`**（`internal/tools/bash_validate.go`）—— bash 四级分级 `read` / `safe` / `destructive` / `unknown`，Duet 的 bash 触发面就是 `BashDestructive` 这一级。

## 2. 升级信号 —— 控制流走查

### 2.1 判定点：`runStep` → `routeTurn`

每个 step（一次模型调用）开头，`Agent.runStep` 先调 `a.routeTurn(lastUserText, 0)`，返回值直接填进本次 `llm.Request` 的 `Model` / `Thinking` / `ReasoningEffort` 三个字段——生效范围就是这一步，下一步重新判定。`routeTurn` 把本次 `Decision` 存进 `a.lastRoute`，粘滞状态由此跨步传递。

开关与接线（`cmd/dsc/main.go` 的 `applyRoutingConfig`）：

- `AutoRoute && EscalationModel != ""` 才走 `routing.Classify`，否则回退旧路径——模型固定 `a.Model`，thinking 由 `selectTurnThinking` 决定（详见 [agent-loop.md](agent-loop.md) §2.5）；
- `[routing] auto_route`（或 `--auto-route`）开启路由；
- `escalation_model`（或 `--escalation-model`）留空时默认补成 `deepseek-v4-pro`——否则 `routeTurn` 会因 `EscalationModel` 为空而静默 no-op，`applyRoutingConfig` 的注释专门解释了这个坑。

### 2.2 信号全列表（`routing.Classify`，按优先级序）

`Classify(s, cfg, prev)` 是纯启发式、零额外 LLM 调用的函数，四个分支按序短路：

1. **`sticky`** —— 上一步已在 pro 且 `prev.StickyLeft > 0`：
   - 留在 pro + thinking + `max` effort，`StickyLeft` 减一；
   - 目的是防 flash↔pro 抖动（一个难问题后面往往跟着难的追问）；
   - `StickyTurns` 在 `routeTurn` 调用点硬编码为 `2`。
2. **`repair_errors`** —— `Signals.RepairErrorsLastTurn >= 3`：
   - 升 pro + `max` 并重新武装粘滞；
   - `runStep` 在每个提交回合末把不可恢复修复计数存入 `Agent.repairErrorsLastTurn`（升级重发时取 pro 回合的计数——flash 回合已被丢弃），下一回合开头传给 `routeTurn`；接线由 `TestRunStepFeedsCommittedRepairErrorsToNextRoute`（`loop_mock_test.go`）钉住。与 §2.3 通道②（同回合内重发，阈值同为 3）互补：通道②被预算闸跳过时，本信号在下一回合接棒。
3. **`hard_reasoning`** —— 用户文本超过 240 字节，**或**小写后包含 `hardWords` 之一：
   - 词表（`classifier.go` 包级变量）：`why`、`design`、`architect`、`refactor`、`debug`、`prove`、`root cause`、`race`、`deadlock`、`redesign`、`trade-off`、`tradeoff`；
   - 命中即 pro + thinking + `max` + 重新武装粘滞。
4. **`mechanical`** —— 兜底：flash、不开 thinking、effort 为 `""`（`routeTurn` 里经 `llm.ParseReasoningEffort("")` 判为无效 → wire 上整个字段省略）。

切换机制再强调一次：`Decision` 只影响请求体的三个字段。系统提示与工具 schema（即前缀字节）完全不动，所以 flash↔pro 切换不会引发缓存失效。

### 2.3 turn 内升级（通道②）：`escalationTrigger`

`routeTurn` 是「事前预测」，通道②是「事后补救」：flash 这一轮已经流完、`repairToolCalls` 修完之后、消息提交之前（`runStep` 中段），`escalationEnabled()` 成立时检查 `escalationTrigger(text, repairErrors)`。

前提 `escalationEnabled()`：`EscalationModel` 非空**且**不等于当前模型——pro 主循环永不向自己升级（注释明言这与 Duet 的自跳过互为镜像）。注意它不要求 `AutoRoute`：只配 `escalation_model` 不开 `auto_route`，通道②照常工作。

两个触发器（marker 优先，便于归因）：

- **`marker`** —— 模型自我声明：回复的首个非空行**整行**恰为 `<<<NEEDS_PRO: 原因>>>`（或无原因的 `<<<NEEDS_PRO>>>`），由 `needsProMarker` 解析；行内引用该 token（比如回显日志）不会误触发。
- **`repair_errors`** —— 本步不可恢复修复错误数 `>= escalationRepairThreshold`（3）。

触发后的完整序列（都在 `runStep` 内，每步至多一次）：

1. **预算重闸** —— pro 重发是 flash 已计费 turn 之上的真实增量开销，先用 `CheckBudget` + `ProjectedTurnCostCNY` 把「flash 已实现成本 + pro 全未命中投影」对会话上限再算一次；不够就放弃升级、提交 flash turn，绝不超支。
2. **storm 历史回滚** —— 升级前先 `stormBreaker.Snapshot()`，升级时 `Restore`：被丢弃的 flash turn 不得污染熔断抑制状态，否则提交的 pro turn 会被错误评判。
3. **事件广播** —— 发布 `EventEscalated{Trigger, FromModel, ToModel, Reason}`；trace 记录在 `trace.go`，ACP 转发在 `internal/acp/adapter.go`，三端因此都能显示升级提示。
4. **重发与归账** —— `streamWithReissue` 用同一请求体仅改 `Model` 重发（前缀字节不动，指纹不移）；`respModel` 改为 `EscalationModel`，持久化 / 成本 / trace 全部记到真正产出该 turn 的模型头上；flash 那轮的输出整体丢弃，transcript 里只有 pro 版本，不会出现重复 turn。

一个容易踩的现状：教模型使用 marker 的系统提示契约 `escalationContract` / `Agent.EnableEscalation`（注入点在 `prompt.DynamicContextBoundary` 之前，进前缀指纹、对固定目标模型字节稳定）**在当前 `cmd/dsc` 接线中没有调用方**——`applyRoutingConfig` 直接赋值 `EscalationModel`，不注入契约。也就是说 marker 通道依赖模型自发输出该 token，或由自定义组装显式调用 `EnableEscalation`。注意其 `PromptBuilder` 场景的 no-op 守卫：builder 每 turn 重建 `a.System`，会静默抹掉注入并造成指纹漂移，注释要求此类组装改走 builder 的静态段。

### 2.4 澄清前置闸：花 pro 的钱之前先确认值不值

路由的省钱兄弟：`Agent.Run` 在进入 `agentLoop` 之前，若 `shouldClarify(userPrompt)` 成立，直接发布一句 `Before I start: …` 并以 `StopModelDone` 返回——**零模型调用**。

判定是 `routing.NeedsClarification`（`internal/routing/clarify.go`），三类命中：

- 空 prompt；
- 整句命中 `vaguePhrases`：`fix it` / `make it better` / `improve this` / `do the thing` / `make it work` / `clean it up` / `optimize it`；
- ≤3 个词且不含 `/`、`.`（没有路径样 token）。

设计动机写在函数注释里：对模糊 prompt 猜错一发 pro/max turn，比先问一句贵得多。开关 `[clarify] auto_clarify` / `--auto-clarify`，默认关闭。

## 3. Duet 验证器

### 3.1 触发面：什么算 destructive

Duet 以 builtin hook `"duet"` 注册在 **PreToolUse** 事件上：`cmd/dsc/main.go` 的 `buildHookRunner` 在 `cfg.Duet.Enabled`（默认 `true`）时调用 `hooks.NewDuetHook(...)` 注册并补全 HookConfig；`--no-duet` 或 `[duet] enabled = false` 整体关闭。

是否触发由 `isDestructiveCall`（`internal/hooks/builtin_duet.go`）判定，按工具分两路：

**`bash`** → `permissions.IsDestructiveBash`（`internal/permissions/destructive.go`）：`tools.ClassifyBash(command) == BashDestructive`，或命中 `[duet] extra_destructive_patterns` 的用户自定义正则。`ClassifyBash`（`internal/tools/bash_validate.go`）把 `;` / `|` / `&&` / `||` 链拆段、取最严段定级，destructive 动词面包括：

- 文件系统：`rm` / `rmdir` / `mv`、`cp -f`、`sed -i`、`dd`、`mkfs` / `mount` / `umount`；
- 进程与系统：`chmod` / `chown` / `chgrp`、`kill` / `pkill` / `killall`、`shutdown` / `reboot` / `halt` / `poweroff`；
- git：`push --force` / `-f` / `--force-with-lease`、`reset --hard`、`checkout -- .`、`clean -f`；
- 发布与基础设施：`docker rm/rmi/push`、`kubectl delete`、`npm|pnpm|yarn publish`、`terraform apply|destroy`、`curl --upload-file`。

完整分级表的单一事实源是 [../reference/bash-validation.md](../reference/bash-validation.md)。注意两个易错点：

- **普通 `git push`（无 force）在 `ClassifyBash` 中是 `BashSafe`**，不触发 Duet（`classifyGit` 的 `push` 分支只在 force 旗标下返回 destructive）；
- `permissions.DestructivePatterns` 那张含 `git push` 的正则表按其注释只是历史存档与 extra-patterns 机制的参照，**活跃判定以 `ClassifyBash` 为准**。

**`write_file` / `edit_file`** → `permissions.IsDestructiveToolCall` → `IsDestructivePath`，三类命中：

- 目标在 cwd 之外（相对路径以 `..` 开头）；
- 路径任一段为 `.git`；
- 文件名命中 `[permissions] secret_path_patterns`。

args 解析失败按 destructive 处理（fail-closed）。**其余所有工具一律不触发 Duet**（`IsDestructiveToolCall` 的 default 分支返回 `false`）。

### 3.2 复核与否决 / 放行路径

时序锚点在 `Agent.executeOne`（`internal/agent/agent.go`）：**权限闸先行，PreToolUse hook（含 Duet）随后，最后才 `tool.Execute`**——代码注释原话是 "Pre-tool hook runs after permission gate, before Duet / execution"。

Duet hook 内部的分支顺序（`NewDuetHook` 返回的闭包）：

1. **自跳过** —— `modelFn()` 返回的当前主循环模型已是 `deepseek-v4-pro` 时直接 `continue`：pro 给自己当裁判没有意义。`modelFn` / `transcriptFn` 都是 `buildHookRunner` 传入的闭包，读的是 agent 实时状态，所以 `/models` 切到 pro 后立即生效。
2. **非 destructive** → `allow`，零额外开销。
3. **destructive** → `buildDuetPrompt` 组裁决提示：transcript 尾部至多 4000 字节 + 工具名 + 原始 args + approve/block 准则，要求只回 `{"approve": bool, "reasoning": "…"}`。
4. **`ValidatePro`**（`internal/llm/client.go`）—— 一次性、非流式的 `chat/completions` 调用：模型硬编码 `deepseek-v4-pro`，`Thinking: nil`（省钱），`response_format: json_object`，经 `MarshalCacheStable` 序列化。这是**独立请求**，不读不写主循环的 `Messages`，也有自己独立的前缀——所以验证调用不会动主循环的缓存键（§4 有专门测试钉死）。
5. **裁决映射** —— `approve=true` → `allow`（携带 reasoning）；`approve=false` → `deny`。`executeOne` 收到 `deny` 后**不执行工具**，把结果替换为 `tools.Errf("blocked by hook: %s", reason)` 返回给模型——模型读到否决理由后自行调整方案，turn 不中断。
6. **失败语义分两种** —— `ValidatePro` 网络 / API 错误 → fail-open：`continue` + reason `pro validation skipped: …`（记 `slog.Warn`），落回标准权限语义；但 validator 返回**畸形 JSON / 空回复**时 `ValidatePro` 返回 `approve=false`（fail-closed，reasoning 为 `malformed validator response` 等）。改这里时别把两种失败混为一谈。

### 3.3 与 permissions 的关系

Duet 是权限闸**之后**的第二道独立闸：

- 用户 allowlist（`[permissions] allow_bash`）或宽松模式放行的 destructive 调用，**仍然**要过 pro 复核——Duet 的 deny 无法靠 allowlist 绕开；
- 反过来 Duet 的 approve 也不豁免权限闸——顺序在后，权限闸已先做过决定；
- 两者共享 `secret_path_patterns`：`buildHookRunner` 把 `cfg.Permissions.SecretPathPatterns` 原样传给 Duet 的路径判定，秘密文件的定义只有一份；
- Duet 关闭时（`--no-duet`），destructive 调用只剩标准权限语义（`ClassifyBash` 的 destructive 级别本身就是「always ask、忽略 allowlist」，见 [../reference/bash-validation.md](../reference/bash-validation.md)）。

### 3.4 配置速查

| 键 / 旗标 | 默认 | 作用 |
| --- | --- | --- |
| `[routing] auto_route` / `--auto-route` | `false` | 开启通道①（turn 前路由） |
| `[routing] escalation_model` / `--escalation-model` | 空（`auto_route` 时补 `deepseek-v4-pro`） | pro 目标；非空即启用通道② |
| `[clarify] auto_clarify` / `--auto-clarify` | `false` | 澄清前置闸 |
| `[duet] enabled` / `--no-duet` | `true` | Duet 总开关 |
| `[duet] extra_destructive_patterns` | 空 | 追加 bash destructive 正则 |
| `[duet] retry_on_failure` / `validator_timeout_ms` | `true` / `10000` | 失败重试一次 / 单次验证超时（毫秒），经 `hooks.DuetOptions` 生效 |
| `[permissions] secret_path_patterns` | 见 defaults | 与权限闸共享的秘密文件面 |

## 4. 不变量与测试守卫

改动前先读这几条不变量，每条都有测试钉死：

| 不变量 | 守卫测试 |
| --- | --- |
| 四信号语义：mechanical→flash 不思考；hard→pro+max；repair≥3→pro；粘滞递减不抖动 | `internal/routing/classifier_test.go` |
| 模糊 / 具体 prompt 的澄清分界 | `internal/routing/clarify_test.go` |
| `AutoRoute` 关闭时回退主循环模型与配置 effort；hard prompt 经 `routeTurn` 升级；`ThinkingMode` 覆写优先 | `internal/agent/route_turn_test.go` |
| marker 必须整行才触发；trigger 优先级 marker > 阈值；pro 主循环自跳过；升级默认关闭；**契约文本进指纹输入**；**flash 与 pro 两轮都计费**；**被丢弃的 flash turn 不污染 storm 历史**；marker / repair-errors 均真实重发 pro | `internal/agent/loop_escalation_test.go` |
| 澄清闸默认关；开启时拦截模糊 prompt 且零模型调用 | `internal/agent/clarify_gate_test.go`、`clarify_gate_wiring_test.go` |
| **Duet 验证不改变 agent 缓存状态；验证请求的指纹与主循环不同**（缓存隔离） | `internal/agent/duet_isolation_test.go` |
| 端到端六场景：destructive 否决 / 放行、安全调用直通、pro 主循环自跳过、validator 故障 fail-open、secret 路径否决（mock DeepSeek server，全离线） | `internal/agent/e2e_duet_hook_test.go` |
| `ClassifyBash` 分级表（含链式拆分、git 子命令、force 旗标） | `internal/tools/bash_validate_test.go` |
| `IsDestructiveBash` 与 extra patterns | `internal/permissions/bash_patterns_test.go` |
| hook 聚合语义：`deny` 短路、`ask` 优先于 `allow`、子进程 hook fail-open | `internal/hooks/runner_test.go` |

跑法：

```sh
go test ./internal/routing/... ./internal/agent/... ./internal/hooks/... \
        ./internal/permissions/... ./internal/tools/...
```

全程不碰网络（agent 侧用 `internal/llmtest` 的 mock server）。

## 5. 常见修改场景实操

### 5.1 想加一个新的升级信号

以「连续 N 次工具结果为错误就升级」为例：

1. **`internal/routing/classifier.go`** —— 给 `Signals` 加字段（如 `ToolErrorsLastTurn int`），在 `Classify` 里插入判定分支。想清楚三件事：
   - **优先级位置**：放在 sticky 之后、hard 之前还是之后，决定了它能否被粘滞掩盖；
   - 是否**重新武装** `StickyLeft`；
   - 给 `Reason` 起一个新的稳定字符串（trace 和测试都会断言它）。
2. **`internal/routing/classifier_test.go`** —— 为新信号加正反两个用例：触发时升 pro，未达阈值时仍 mechanical。`routing` 包零依赖，这层测试秒级。
3. **`internal/agent/agent.go` 的 `runStep`** —— 把真实信号值喂进 `routeTurn` 调用。历史教训：`repairErrorsLastTurn` 曾长期传字面量 `0`——**信号在分类器里实现了、调用点没接线，等于没有**（2026-06 已接线）。信号若需要跨步状态，照 `Agent.repairErrorsLastTurn` 的样子加字段并注意 session 生命周期。
4. **`internal/agent/route_turn_test.go`** —— 加一条经 `routeTurn` 的集成断言，防止接线再次退化。
5. **想清楚通道归属** —— 信号若来自「本步已经发生的事实」（模型输出、修复结果），考虑放进 `escalationTrigger`（通道②，turn 内重发）而不是 `Classify`（通道①，turn 前预测）；前者多花一轮 flash 的钱但判定更准，后者免费但只能看 turn 前可见的信号。
6. **缓存红线自查** —— 新信号的判定与生效都不得改前缀字节。若你发现自己想往系统提示里塞东西，先读 [prefix-cache.md](prefix-cache.md) 的 golden 守卫一节，并参考 `escalationContract` 的注入纪律（边界之前、字节稳定、`PromptBuilder` 场景禁注入）。
7. `go test ./internal/routing/... ./internal/agent/...` 全绿后，若信号语义面向用户，同步 [../reference/duet.md](../reference/duet.md) 或本篇。

### 5.2 想扩大 / 收紧 Duet 的 destructive 面

- **只对自己生效**：配置 `[duet] extra_destructive_patterns`（正则，作用于 bash 命令原文），不用改代码。
- **改产品默认**：动 `internal/tools/bash_validate.go` 的 `classifySingle` / `classifyGit`，同步 `bash_validate_test.go` 的分级表用例，并更新 [../reference/bash-validation.md](../reference/bash-validation.md)（它是分级表的单一事实源）。注意这同时影响权限闸的 ask/allow 决策——`ClassifyBash` 是两道闸共用的。
- **非 bash 工具想纳入**：扩 `permissions.IsDestructiveToolCall` 的 switch（当前只有 `write_file` / `edit_file`），并在 `internal/agent/e2e_duet_hook_test.go` 加端到端场景。

### 5.3 想调粘滞与阈值

三个数字都有明确的家：

- `StickyTurns` —— `routeTurn` 调用点硬编码 `2`（`agent.go`，想配置化需先过 `routing.Config` 透传）；
- `escalationRepairThreshold` —— `agent.go` 包级常量 `3`；
- hard 文本的长度阈值 `240` 与 `hardWords` 词表 —— `classifier.go`。

改任何一个，对应更新 `classifier_test.go` / `loop_escalation_test.go` 里写死的期望值——它们故意钉死数值，防止「顺手调参」绕过 review。
