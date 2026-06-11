# 前缀缓存：实现深入篇

dsc 的身份主张是"可证明的前缀缓存稳定性"——README 首页的 94.7% 命中率、4.5× 成本优势（prefix A/B 对照，见 [bench/README.md](../../bench/README.md)）都建立在这套机制上。本文是写给贡献者的实现叙事：每个函数、测试、常量都在代码中核实过。三篇相关文档的分工：

- [reference/prefix-cache.md](../reference/prefix-cache.md) —— 用户视角：漂移告警怎么读、怎么排查；
- [model-compatibility.md](model-compatibility.md) —— DeepSeek V4 wire 事实与价格表（贡献者必读）；
- 本文 —— 机制如何实现、不变量由哪些测试守卫、改代码时如何不破坏缓存。

## 1. 解决什么问题：50× 价差与"字节即缓存键"

DeepSeek 对缓存命中的输入 token 定价约为未命中的 **1/50**（`deepseek-v4-flash`：0.02 vs 1.0 ¥/1M，硬编码在 `internal/llm/cache_metrics.go` 的 `Prices` 表；完整价格表见 [model-compatibility.md](model-compatibility.md)，不在此复制）。缓存按**请求体的字节前缀**匹配：从第一个不同的字节开始，后面全部按 miss 计价。

一个多 turn 会话的请求体长这样：

```
├──────────── 静态前缀（跨 turn 字节级稳定）────────────┼──────── 追加式 body ────────┼─ 本 turn 新增 ─┤
│ system prompt（含 skill 元数据索引） │ tools（排序 +  │ 历史 user/assistant/tool    │ 用户输入 +     │
│ ……DynamicContextBoundary 之前的部分  │ schema 规范化）│ 消息，只追加、不重写        │ 动态上下文     │
├──────────────────── 只要字节不变，全部按 0.02 hit 计价 ─────────────────────────────┼─ 1.0 miss 计价 ┤
```

这意味着三件事：

1. 系统提示 + 工具 schema 这段"静态前缀"在多 turn 会话里反复重发，只要字节级稳定，每 turn 只为新增尾部付全价——这是 4.5× 成本优势的来源；
2. 缓存匹配并不止于静态前缀：**追加式的对话 body 同样吃缓存**。只要历史消息不被重写，第 N turn 的请求体是第 N−1 turn 的严格超集，整个旧 body 都按 hit 计价。反过来，重写 body（典型：compaction）等价于换缓存键——这是 §7 历史教训的伏笔；
3. 任何一处非确定性序列化（map 迭代序、JSON 键序、工具声明顺序）都会**静默**烧掉这 50× 折扣——请求照样成功，只是账单变成 50 倍，没有任何报错。

所以整套机制的设计哲学是：**让缓存键成为一等公民**——用同一个序列化器生成 wire 字节和缓存指纹，用 golden 测试把字节钉死，用运行期监测把漂移变成显式事件，用事后工具把每一分钱归因到原因。

## 2. 包地图：谁负责什么

| 包 | 角色 | 核心导出 |
|---|---|---|
| `internal/llm` | **机制层**：生成缓存键本身 | `Request.MarshalCacheStable`（request.go）、`canonicalizeTools` + `StaticPrefix.Fingerprint`（static_prefix.go）、`PrefixMonitor`（prefix_drift.go） |
| `internal/agent/prefix_epoch.go` | **策略层**：前缀的生命周期 | `EpochManager` / `PrefixEpoch`——冻结模型可见前缀；冻结后的能力变更降级为 `PendingChange`，直到显式切换 epoch |
| `internal/cache` | **归因诊断**：纯函数，不含缓存逻辑 | `Attribute(Input) CacheReceipt`（receipt.go）——把每 turn 的缓存行为归因为 `cold_first` / `prefix_mut` / `residual_tail` / `compact_reset` / `steady` |
| `internal/cacheunit` | **对齐优化** | `AlignPadding` / `PadTextConcat`（align.go）——把前缀 pad 到 DeepSeek cache-unit 边界（V4 只复用完整压缩块，尾部不完整块总是重算；unit 需用 `bench/cmd/cacheprobe` 实测，unit≤0 表示禁用） |
| `internal/traceinspect` + `cmd/dsc` | **事后证据** | `dsc trace inspect TRACE.jsonl`（cmd/dsc/trace.go）、`dsc cache explain TRACE.jsonl`（cmd/dsc/cache.go） |

一句话定位：`internal/llm` 决定"缓存键是什么字节"，`internal/agent` 决定"这些字节何时允许变"，`internal/cache` 解释"这一 turn 的钱花在哪了"，`internal/cacheunit` 榨取最后一块不完整压缩块，`internal/traceinspect` 把以上全部变成可审计的报告。

## 3. 核心数据结构与单一序列化器

### 3.1 `MarshalCacheStable`：wire 字节 = 缓存键

`Request.MarshalCacheStable()`（`internal/llm/request.go`）是发往 DeepSeek 的**唯一**序列化路径——`Client.Stream` 与非流式路径都调它（`internal/llm/client.go`），不存在第二条 `json.Marshal(req)` 的旁路。它产出的字节就是 DeepSeek 给 prompt cache 建键的字节。流程：

1. `SanitizeForDeepSeek()` —— thinking 模式下给"有 tool_calls 但无 thinking block"的 assistant 消息补占位 `reasoning_content`，保证重放稳定；
2. `applyPolicy` —— 应用 reasoning 保留策略（环境变量门控，默认 no-op）；
3. `canonicalizeTools` —— 工具规范化（见 3.2）；
4. `splitToolResults` + `flattenForWire` —— 把 `ContentBlock` 形态的消息摊平成 DeepSeek 的 OpenAI 形态（`content` / `reasoning_content` / `tool_calls`）；
5. 对一个**字段顺序固定**的匿名 struct 做 `json.Marshal`——Go 对 struct 的编码顺序是声明顺序，确定性由此而来。

`ToolFunction.Parameters` 故意声明为 `json.RawMessage` 而非 `map[string]any`：map 会在编码时引入键序问题，RawMessage 把"键序由谁保证"这个问题显式交给 `canonicalJSON`。

### 3.2 `canonicalizeTools`：单一事实源

`canonicalizeTools`（`internal/llm/static_prefix.go`）做两件事：按 `Function.Name` 字典序稳定排序；对每个工具的 JSON-Schema 调 `canonicalJSON`（`request.go`，对象键在每一层递归排序）。它的注释自称"the SINGLE source of truth for cache-stable tool serialization"——**wire 序列化与前缀指纹共用这一个函数**，所以两者不可能在工具顺序或 schema 键序上分叉。历史上这条一致性只靠一句"same as MarshalCacheStable"的注释维护，M1 整合后变成了结构性保证。

错误处理上有个值得注意的双契约：遇到畸形 schema 时它记录第一个错误但**继续处理**，返回完整排序的切片。严格调用方（wire 序列化器）拿到 error 后 fail-closed 直接报错；best-effort 调用方（指纹的 `hashToolsCanonical`，`prefix_drift.go`）忽略错误，仍能对稳定排序的结果做 hash。

**上游任何字段乱序 = 静默失效**：`canonicalizeTools` 只规范化它看得见的东西（工具顺序、schema 键序）。如果某个 tool 的 `Description()` 里嵌了时间戳，或系统提示组装时混入了随机序——序列化器无能为力，字节照样漂。这正是 §6 红线和 §7 运行期监测存在的原因。

### 3.3 `StaticPrefix.Fingerprint()`：只 hash 模型可见字节

```go
// internal/llm/static_prefix.go
type StaticPrefix struct {
    System   string
    Tools    []Tool
    FewShots []Message  // 预留字段，尚未折入指纹
}

func (p StaticPrefix) Fingerprint() PrefixFingerprint {
    sysH := sha256hex(p.System)
    toolsH := hashToolsCanonical(p.Tools)
    return PrefixFingerprint{sysH, toolsH, sha256hex(sysH + ":" + toolsH)}
}
```

指纹**只覆盖模型可见的 System + Tools 字节**，且 tools 走与 wire 完全相同的 `canonicalizeTools`——所以 `CombinedSHA256` 在构造上就等于 DeepSeek 缓存键的等价类：缓存无关的重排（工具顺序、schema 键序）不动指纹，真实变更必动指纹。

这是 [adr/0001-prefix-fingerprint-is-model-visible-bytes-only.md](adr/0001-prefix-fingerprint-is-model-visible-bytes-only.md) 的核心决策：早期的 6 分量组合 hash（system : tools : skill_dir : mcp_schema : agent_profile : few_shots）被否决，因为它重复计数（skill 目录已渲染进 system）、对键序敏感的 `mcp_schema` 分量会在 MCP 重连时产生**幻影漂移**、且会因模型根本看不见的潜在状态（未激活的 MCP、profile 名）强制缓存失效。潜在能力状态被剥离成 `agent.CapabilitySet`，由 `EpochManager` 单独追踪。

`ComputeFingerprint`（`prefix_drift.go`）现在只是 `StaticPrefix.Fingerprint()` 的薄壳，调用方在向后者迁移。

### 3.4 `PrefixEpoch`：冻结与 PendingChange

`internal/agent/prefix_epoch.go` 的 `PrefixEpoch` 是一份冻结的前缀快照：`FreezeEpoch` 之后，`runStep` 与 `maybeCompact` **必须**使用 `FrozenSystem` / `FrozenTools` 而非实时值（结构体注释原文："MUST use these instead of the live values to guarantee cache-stable prefixes"）。冻结后检测到的工具/技能/MCP/系统提示变更不会进入请求，而是记为 `PendingChange`（`PendingToolAdded`、`PendingSkillBodyChanged` 等 12 种），在收据中可见、对模型不可见，直到显式 epoch 切换。`StaticPrefixHash` 字段就是 3.3 的指纹——缓存键与策略身份（`Capability`）在结构体里也是分开的两个字段。

### 3.5 `CacheReceipt`：每 turn 的归因判决

`internal/cache` 的包注释开宗明义："pure diagnostics ... contains no cache logic of its own"。`Attribute(Input)` 是一棵纯函数决策树，输入是观察到的状态（上一 turn 与本 turn 的 `StaticPrefix`、compaction epoch、usage），输出 `CacheReceipt`：归因 `Cause`、命中/未命中 token、成本与节省（¥）。五种 `Cause`：

- `cold_first` —— 无前序基线，首 turn 必付全量 prefill，预期内；
- `prefix_mut` —— 系统提示或工具集在 turn 之间变了，收据的 `Drift` 字段（`llm.PrefixDiff`）给出是哪一部分；
- `residual_tail` —— 结构性地板：V4 的尾部不完整压缩块总是重算，浪费量为 `ResidualEst`（= `TailTokens % Unit`，unit 未测得时禁用）；
- `compact_reset` —— compaction epoch 跳变。`cache.Epoch` 是"**只在 compaction 时**递增"的单调计数器，epoch 一跳，无论前缀内容如何都归因为压缩重置（见 §7 的历史教训）；
- `steady` —— 快乐路径：前缀稳定、同 epoch、hit 占主导。

`ReceiptLine`（emit.go）把收据渲染成稳定的单行格式（`turn=… epoch=… cause=… hit=… miss=… residual=… cost=¥… saved=¥…`），格式稳定到可以在测试里直接断言，供 `dsc trace inspect` 与 Cost HUD 使用。

### 3.6 `cacheunit`：把前缀对齐到压缩块边界

`internal/cacheunit/align.go` 处理一个更细的颗粒度问题：DeepSeek V4 只复用已存前缀的**最后一个完整压缩块**之前的部分，尾部不完整块总是重算（包注释引 DeepSeek-V4 report §3.5.2）。把静态前缀 pad 到 unit 的整数倍，能让"完整持久化、可复用"的部分最大化。

- `AlignPadding(prefixTokens, unit)` —— 算出需要补多少 token；unit≤0（未实测）或已对齐时返回 0；
- `PadTextConcat(prefix, unit, count)` —— 生成确定性填充（固定的 `<!-- cache-alignment padding -->` 注释串），**度量的是拼接后的整体** `count(prefix+pad)`；
- 旧的 `PadText` 已标记 DEPRECATED：它假设 `count(prefix)+count(pad)==count(prefix+pad)`，而真实 tokenizer 会在拼接边界合并 token，该假设不成立——这个注释本身就是一课：对齐计算必须和指纹用同一个 tokenizer、同一种度量方式。

unit 的值必须用 `bench/cmd/cacheprobe` 实测，不能拍脑袋；测不出来就保持 0（禁用 padding），宁可少省一点也不引入错误的字节。

## 4. 控制流走查：一次 turn 的前缀之旅

以下锚点按执行顺序排列（行号会漂移，认函数名）：

1. **组装**：`runStep`（`internal/agent/agent.go`）确定本 turn 的 `staticSys` 与 `req.Tools`；若 epoch 已冻结，替换为 `epoch.FrozenSystem` / 冻结工具集（§3.4）。
2. **漂移检查**：`a.prefixMon.Check(llm.StaticPrefix{System: staticSys, Tools: req.Tools})`——`PrefixMonitor.Check`（`internal/llm/prefix_drift.go`）首 turn 钉基线（防御性拷贝，防调用方事后改切片）；后续 turn 只比较 `CombinedSHA256`，稳定路径只做 hash，**漂移时才**调 `Diff` 算出类型化的 `PrefixDiff`（system 变了？哪些工具增删/schema 变了？）并重新钉基线。
3. **事件**：漂移时发 `EventInfo{Text: "prefix cache invalidated: sys|tools|sys+tools"}`；若此时 epoch 处于冻结态，额外发 `EventDriftBlocked`（`internal/agent/events.go`）——冻结后的漂移按 bug 对待，不是正常的 pending change。
4. **上线**：`client.go` 的 `Client.Stream` 调 `req.MarshalCacheStable()` 取得 wire 字节发出——与第 2 步 hash 的是同一套 canonical 字节（§3.2）。
5. **预算投影**：发流之前，`ProjectedTurnCostCNY(...)` 按**滚动会话缓存命中率**折扣输入成本来预估本 turn 花费（冷启动命中率为 0，首 turn 按全 miss 计价）——缓存机制甚至参与预算关卡的定价。
6. **回账**：SSE 终帧的 usage 带回 `PromptCacheHitTokens` / `PromptCacheMissTokens`；`runStep` 末尾构造本 turn 的 `curStaticPrefix`（同样防御性拷贝），连同 compaction epoch、尾部 token 估计一起喂给 `cache.Attribute`，产出本 turn 的 `CacheReceipt`。
7. **跨会话**：`Agent.StaticPrefixFingerprint()` 暴露 `CombinedSHA256` 供持久化；下次启动用 `agent/warm.go` 的 `IsLikelyWarm(lastFP, curFP, sinceLastUse, ttl)` 判断 DeepSeek 侧缓存是否还热。`PrefixMonitor.StabilityRatio()` 则给出本会话"未漂移 turn 占比"的汇总指标。

整条链路里同一组字节被消费三次——hash（第 2 步）、上线（第 4 步）、归因（第 6 步）——三处全部经由 `canonicalizeTools` 这一个函数，这就是"单一序列化器"在控制流层面的含义。

## 5. 不变量与测试守卫

每条不变量都有具名测试钉死，全部在 `internal/llm/`：

| 测试 | 守卫的不变量 |
|---|---|
| `TestCacheStableDeterminism`（e2e_cache_stable_test.go） | 同一请求 marshal 10 次，hash 唯一——无 map 迭代序泄漏 |
| `TestCacheStableGolden`（同上） | 代表性请求（系统提示 + 2 工具 + thinking + 工具往返）的**精确字节**钉在 `testdata/cache_stable.golden.json`，逐字节比较 |
| `TestMarshalCacheStable_GoldenLock`（golden_lock_test.go） | 第二把独立字节锁：极简多轮文本请求钉在 `golden/marshal_cache_stable.golden` |
| `TestDeepSeekMarshalGoldenUnchanged`（provider_deepseek_golden_test.go） | provider 抽象引入后，DeepSeek wire 与冻结时的 golden 逐字节一致——"If this test fails the frozen wire has drifted" |
| `TestMarshalCacheStableIsToolOrderIndependent`（cache_stable_property_test.go） | 工具声明顺序不影响 wire 字节（性质测试） |
| `TestFingerprintTracksWireStaticHead`（fingerprint_wire_linkage_test.go） | **ADR-0001 的 P1 契约**：指纹与 wire 静态头同进退——缓存无关的重排（工具乱序 + schema 键置换）两者都不许动；真实变更（改一个 tool description）两者必须一起动 |
| `TestParityGolden` + `TestParityConsistency`（parity_test.go / parity_consistency_test.go） | 9 个具名场景的 wire 字节各钉一个 golden；一致性测试强制场景表、`ParityScenarios()`、manifest、golden 文件集四向同步——场景清单与"有意为之的浅覆盖"哲学见 [parity.md](parity.md) |

golden 再生成机制——序列化变更若是有意的：

```sh
UPDATE_GOLDEN=1 go test -run TestCacheStableGolden ./internal/llm/   # 字节锁
UPDATE_GOLDEN=1 go test -run TestParityGolden ./internal/llm/        # 9 场景 golden + manifest
```

重新生成后**连 golden 文件一起提交**——让"全体用户升级后缓存一次性失效"这件事在 diff 里显式可见、可 review，而不是悄悄发生。注意 `TestFingerprintTracksWireStaticHead` 没有 golden、没有再生成出口：它断言的是两个产物之间的**关系**（同进退），关系破了只能改代码，不能改 fixture——这是矩阵里唯一一条"不可协商"的守卫。

## 6. 贡献者红线

以下字节都在静态前缀里，动它们 = 全体用户的缓存失效。[model-compatibility.md](model-compatibility.md) 列了结论，这里给出处：

1. **工具的 `Description()` / `Parameters()` 字节**（`internal/tools/*.go`）不可为了改变运行时行为而修改——想约束模型行为，让工具在执行期返回错误（运行时错误会进对话尾部，不进前缀；改 schema 字节则烧掉整个前缀）。
2. **`DefaultSystemPrompt`**（`internal/agent/agent.go`）的文档注释就是规约原文："It must not change between turns; that would invalidate the prompt cache and blow the cost story. Versioned by binary release, not by session."——它按二进制版本演进，不按会话、更不按 turn。
3. **epoch 内系统提示字节稳定**：动态内容（git 状态、记忆召回）必须落在 `prompt.DynamicContextBoundary` 之后（见 [architecture.md](architecture.md) §2-③）；冻结 epoch 后连"合法"的前缀变更都要走 `PendingChange` + epoch 切换，直接绕过会触发 `EventDriftBlocked`。
4. **skills 只有元数据索引进前缀**：skill body 通过 `skill_read` 工具懒加载（`internal/tools/skill_read.go` 注释："It exists so the cache-stable static prefix can carry only..."）。给前缀加大块可变内容前，先想想能不能走同样的懒加载模式。
5. **改序列化器前先跑 §5 全家**：任何 golden 变红都意味着缓存键变了——要么是 bug，要么需要在 PR 里显式承认升级成本。

## 7. 历史教训：冻结前缀不保护 body

2026-06-03 的同模型 head-to-head 对照实验（与 reasonix，编辑任务 6/6 平手）暴露过一个反直觉事实：dsc 的计费一度高出约 5×，根因**不是**前缀不稳，而是 **compaction 重写了前缀**——前缀指纹守住了"组装出的前缀字节稳定"，但 compaction 把整个消息历史（含前缀之后的 body）重写，DeepSeek 侧等于换了一把缓存键，每次压缩都付一次全量 re-prefill。更反直觉的是：为省钱给 body"瘦身"反而净负面（省下的 token 不抵多付的 miss）。

这就是 §3.5 中 `cache.Epoch` "bumped ONLY at compaction" 与 `CauseCompactReset` 的来历——既然压缩的代价无法绕过，至少把它显式记账，让 `dsc cache explain` 能指着某一 turn 说"这次贵是因为压缩"。实验报告是 gitignored 的内部材料（不随仓库分发），路径备查：`docs/internal/competitive/2026-06-03-controlled-experiments-compendium.md` 与 `docs/internal/competitive/2026-06-03-cache-cost-head-to-head.md`。

## 8. 常见修改场景实操

### 8.1 新增一个 tool：如何确认没破坏缓存

假设你给 `internal/tools/` 加了一个新工具：

1. **写 schema 时就当它已冻结**——`Name` / `Description()` / `Parameters()` 一旦发布就是缓存键的一部分，措辞斟酌好再合入，别打算"下个 PR 润色一下描述"。
2. **跑序列化守卫**：`go test ./internal/llm/`。预期**全绿**——新增工具不出现在 golden 的固定请求里，不该动任何 golden；parity 也不需要新场景，除非你引入了新的排序/规范化路径（[parity.md](parity.md) 的明文规则）。如果 golden 变红，说明你动到了序列化器本身，回到 §5 的再生成流程。
3. **跑一次真实会话取证**：`dsc -p "..." -trace-jsonl /tmp/t.jsonl`（one-shot 模式的 trace 旗标，`cmd/dsc/main.go`），然后 `dsc trace inspect /tmp/t.jsonl`，看证据行：

   ```
   cache 99.6% | hit 48700000 | miss 190000 | saved CNY 47.73 | prefixes 1 | expected_miss 1
   ```

   判据是 **`prefixes 1`**——整个 trace 只出现一个 `static_prefix_hash`；`expected_miss` 应等于 epoch 创建数（每个 epoch 首 turn 必 miss 一次，那是冷启动不是漂移）。`prefixes 2+` 说明你的工具让前缀在会话中途变了字节——常见原因是 `Description()`/`Parameters()` 返回值不是纯常量。
4. **逐 turn 归因**：`dsc cache explain /tmp/t.jsonl` 看每 turn 的 `CacheReceipt`——稳态应该是 `cause=steady`；出现 `cause=prefix_mut` 时 `Drift` 字段会告诉你是 sys 还是哪个工具的 schema 动了。
5. **看运行期信号**：会话中若 TUI 冒出 `prefix cache invalidated: tools` 或状态栏 `⚠ cache:tools` 徽标（含义见 [reference/prefix-cache.md](../reference/prefix-cache.md)），就是 §4 第 2-3 步的 `PrefixMonitor` 在叫。
6. **会话中途注册工具是另一回事**：那是预期内的一次 tools 漂移（缓存 miss 一次后重新钉基线）；若 epoch 已冻结，它会变成 `PendingToolAdded` 的 pending change，对模型不可见直到 epoch 切换——这是策略层（§3.4）的职责，不是你的工具的 bug。

两条对照命令收尾：改动前后各跑一次相同的 one-shot 任务，`dsc trace inspect` 的 cache% 与 `prefixes` 不该有肉眼可见的差异；有差异，先查自己的 schema 字节，再查序列化器。

### 8.2 修改系统提示：按版本，不按心情

`DefaultSystemPrompt` 是合法可改的——但只在**二进制版本边界**改（§6 第 2 条的规约）。后果要心里有数：发版后所有用户的第一个会话付一次全量 prefill，之后恢复稳态。所以：

- 措辞修订要**攒批**：十次各改一个词 = 十次全员缓存失效；
- 想给系统提示加"动态"内容（当前目录、日期、git 状态），答案永远是放到 `prompt.DynamicContextBoundary` 之后，而不是改静态段；
- 改完跑 `go test ./internal/agent/ ./internal/llm/`——前缀相关断言会告诉你有没有把动态内容漏进静态段。

### 8.3 动 reasoning 保留策略：body 字节同样是字节

`MarshalCacheStable` 的第 2 步会应用 `ReadPolicy()` 读到的 reasoning 保留策略（`DEEPSEEKCODE_REASONING_DROP` / `DEEPSEEKCODE_REASONING_RETAIN`，`internal/llm/reasoning_policy.go`；两者互斥，默认 no-op）。它改写的是**历史 assistant 消息**——也就是 §1 里"追加式 body"的部分。会话中途切换这两个环境变量，等于把已缓存的 body 字节改掉，缓存断点前移到第一条被改写的消息。golden 测试（`TestMarshalCacheStable_GoldenLock` 等）开头显式 `t.Setenv` 清空这两个变量，就是为了让字节锁不被本机环境污染——你写新的序列化测试时照抄这个开场。

## 快速索引

- 一键回归：`go test ./internal/llm/ ./internal/agent/ ./internal/cache/ ./internal/cacheunit/`
- 缓存键怎么来的 → §3.1–3.2（`request.go` / `static_prefix.go`）
- 指纹为什么只看模型可见字节 → §3.3 + [ADR-0001](adr/0001-prefix-fingerprint-is-model-visible-bytes-only.md)
- 我的改动会不会破坏缓存 → §6 红线 + §8 实操
- 用户报告 `⚠ cache:*` 徽标 → [reference/prefix-cache.md](../reference/prefix-cache.md) 的排查表
- 账单为什么贵 → `dsc cache explain`（§3.5）+ §7 的 compaction 教训
