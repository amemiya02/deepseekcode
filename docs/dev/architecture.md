# 架构总览

deepseekcode（`dsc`）是一个为 DeepSeek 模型量身打造的终端原生编码代理：单个 Go 二进制，三个交互端（TUI / Web SPA / 桌面端），共用同一个 agent 内核。本文是 dev/ 开发文档的地图——读完后你应该能回答"X 功能在哪个包、从哪个文件读起"：

- 想知道**三端如何接到同一个内核** → §1
- 想跟踪**一条消息从输入到落库的全过程** → §2
- 想查**某个包是干什么的** → §3 的 37 包速览表
- 想知道**第一周按什么顺序读代码** → §4

本文只描述已实现、已核实的代码；引用一律给出文件与函数名（行号会漂移，函数名更稳）。

## 1. 三端一核

```
┌──────────────────┐  ┌───────────────────────┐  ┌──────────────────────────┐
│ TUI              │  │ Web SPA               │  │ Desktop                  │
│ internal/tui     │  │ web/（React+Vite）    │  │ desktop/（Wails v3）     │
│ Bubble Tea       │  │ fetch /v1/* + SSE     │  │ webview 加载嵌入式 SPA   │
└────────┬─────────┘  └───────────┬───────────┘  └────────────┬─────────────┘
         │                        │ HTTP + SSE                │ 资产中间件挂 /v1/*
         │                        │                           │ + 回环 127.0.0.1:7432
         │             ┌──────────▼───────────────────────────▼──────────┐
         │             │ internal/gateway —— /v1/* HTTP 处理器            │
         │             │ NewHandler / ServeHandler；hub = 每会话 SSE      │
         │             │ 广播 + 每 turn 重放缓冲（hub.go）                │
         │             └──────────────────────┬──────────────────────────┘
         │                        ┌───────────▼────────────┐
         │                        │ internal/acp           │ ←─ dsc serve --acp
         │                        │ SessionManager +       │    （stdio JSON-RPC，
         │                        │ RealAgentFactory       │     供编辑器接入）
         │                        └───────────┬────────────┘
┌────────▼────────────────────────────────────▼────────────┐
│ internal/agent —— 同一个 ReAct 内核（Agent.Run）          │
│ turn 边界 · 工具调度 · 权限/沙箱关卡 · 压缩 · 事件总线    │
└──────────────────────────┬────────────────────────────────┘
                ┌──────────▼──────────┐
                │ internal/llm        │ ──SSE──▶ DeepSeek API
                │ Client.Stream       │          （OpenAI 兼容 wire）
                └─────────────────────┘
```

四条接入路径，一个内核。记住一条铁律：各端只负责"呈现与输入"，所有智能都在 `internal/agent` 及其下游——改行为去内核，改外观去对应端。

启动命令速查：

```sh
dsc                       # TUI（主端）
dsc -p "解释这个 panic"    # 一次性模式，结果到 stdout
dsc serve --http :7432    # HTTP+SSE 网关（Web SPA 后端）
dsc serve --acp           # stdio JSON-RPC（编辑器接入）
make desktop              # 构建桌面端 .app（先构建 web/ 再打包）
```

### TUI（主端）

- **入口**：`cmd/dsc/main.go`。无参数运行 `dsc` 走 `runTUI`，在进程内直接构造 `agent.Agent`（不经过 gateway/acp），再 `tui.New(tui.Config{...})`（`internal/tui/app.go` 的 `New`）→ `app.Run()` 启动 Bubble Tea 程序。
- **一次性模式**：`dsc -p "prompt"` 走 `runOneShot`（同文件），同一套装配，结果打到 stdout 后退出。
- **启动命令**：`dsc`（交互）、`dsc -p "..."`（一次性）。
- `cmd/dsc/main.go` 还分发其余子命令：`dsc init`（→ `internal/bootstrap`）、`dsc doctor`、`dsc upgrade`、`dsc trace`、`dsc cache`、`dsc serve`，以及内部重入口 `dsc __sandbox_run`（见 §2-⑥）。

### Web SPA

- **前端**：`web/`（React + Vite），通过 `/v1/*` REST + SSE 与后端通信，入口 `web/src/main.tsx`。
- **后端**：`dsc serve --http :7432` 启动（`cmd/dsc/serve.go` 的 `runServe` → `buildServeHandler` → `gateway.NewHandler`）。
- gateway 不直接持有 agent，而是包着 `acp.SessionManager`——每个会话经 `acp.RealAgentFactory`（`internal/acp/factory.go`）按需创建一个 agent，与 TUI 的进程内装配等价（注释称之为"production agent factory"）。
- **安全默认**：只绑回环地址；非回环需显式 `--http-allow-remote`，且自动套 Bearer token 鉴权（`serve.go` 的 `requireBearer`，token 由 `newServeToken` 随机生成）。
- 详见 [../reference/web.md](../reference/web.md)。

### Desktop

- **入口**：`desktop/main.go`，Wails v3 包装（`application.New`），把编译好的 SPA 从 `webapp` 包嵌入 webview。
- **网关接线**（同一个 handler 走两条路，见 `desktop/main.go` 顶部注释）：
  - webview 主路径：gateway 作为 Wails 资产中间件挂在 webview 同源，SPA 的相对 `/v1` 请求同源解析，无端口、无 CORS；
  - 回环备份：`App.ServiceStartup` 同时经 `gateway.ServeHandler` 在 `127.0.0.1:7432`（`defaultGatewayPort`）提供浏览器直连，与 webview 共享会话状态。
- **构建命令**：`make desktop`（Makefile 的 `desktop: web` 目标，先构建 SPA 再打包 `.app`）。
- 详见 [../reference/desktop.md](../reference/desktop.md)。

### ACP stdio（第四条路径）

- `dsc serve --acp` 启动 `acp.NewACPServer(sm, stdin, stdout)`（`internal/acp/server.go`），stdin/stdout 上的 JSON-RPC 服务，供编辑器等外部客户端接入；`--acp` 与 `--http` 互斥（`runServe` 校验）。
- HTTP 与 stdio 两种 transport 之下是同一个 `acp.SessionManager`（`internal/acp/session.go` 的 `NewSessionManager`），所以会话语义完全一致。

## 2. 一次请求的生命周期

以"用户发出一条消息，模型改了一个文件"为例：

```
输入(①) → ReAct 循环(②) → prompt 组装(③) → 路由(④) → LLM 流式(⑤)
   → 工具调度+安全关卡(⑥) → 快照(⑦) → 持久化+收尾(⑧)  ↺ 回到⑤直到停止条件
```

### ① 输入接收

TUI 在 `internal/tui` 捕获输入后调用进程内 agent。Web/Desktop 由 SPA `POST` 到 `/v1/*`，落到 `acp.HTTPGateway.handlePrompt`（`internal/acp/http.go`）；流式回包走同文件的 `handleStream`，事件由 `internal/gateway/hub.go` 的 `hub` 做每会话 SSE 广播。`hub` 带每 turn 重放缓冲（`subscribe` 返回订阅前已发生的事件，`resetTurn` 在新 turn 清空）——订阅晚到也不会丢 `turn_done`，这是桌面端"第二条消息死锁"级 bug 的根治手段。

### ② 进入 ReAct 循环

`agent.New(client, reg, pol, model)`（`internal/agent/agent.go`）构造 `Agent`，`Run(ctx, userPrompt)` 启动一个 turn。内部反复执行 `runStep`（一步 = 一次 LLM 往返 + 工具执行），直到 `stop_conditions.go` 判定停止。事件经 `Bus`（`bus.go`）扇出给所有订阅方（TUI 直接订阅，gateway 转成 SSE）。循环中还有两个旁路：

- **steer**：用户可在 turn 进行中追加指令，`Agent.Steer` 入队、`drainSteer` 在步间消费；
- **plan 模式**：`Agent.EnterPlan`（`plan.go`）切到只读工具 + `question` + `plan_exit` 的受限集合。

### ③ Prompt 组装

系统提示来自 `internal/prompt` 的 `SystemPromptBuilder.Build()`（`builder.go`）。关键设计：`prompt.DynamicContextBoundary` 把提示切成两段——

- **冻结前缀**：基础系统提示 + 工具 schema + skills 元数据索引（`internal/skills`，body 懒加载、不进前缀），跨 turn 字节级稳定，是 DeepSeek 前缀缓存命中的前提；
- **动态尾部**：git 状态（`internal/gitctx` 的 `Reader`，agent 在 `refreshGitContext` 中按 cwd 懒构造）、长期记忆召回（`prompt.InjectRecalled`）等，永远落在边界之后。

前缀稳定性是全项目的最高约束：`Agent.StaticPrefixFingerprint` 给前缀算指纹，`internal/llm/prefix_drift.go` 在运行期监测漂移，`internal/cacheunit.AlignPadding` 把前缀 padding 到 DeepSeek cache-unit 边界。机制深入见 [prefix-cache.md](prefix-cache.md)；背景与 wire 约束见 [model-compatibility.md](model-compatibility.md) 与 [../reference/prefix-cache.md](../reference/prefix-cache.md)。

### ④ 路由决策

每 turn 发请求前 `Agent.routeTurn` 调 `routing.Classify(Signals, Config, prev)`（`internal/routing/classifier.go`）——纯启发式、零额外 LLM 调用，依据用户文本信号与上一轮工具修复错误数在 Flash/Pro 间切换（`StickyTurns: 2` 防抖），同时决定 thinking 开关（`selectTurnThinking`，对琐碎消息经 `llm.IsTrivialMessage` 关闭）与 reasoning effort。模型与 effort 不进前缀，所以路由切换不会引起缓存失效（`routeTurn` 的文档注释明确了这一不变量）。信号全列表、turn 内升级通道与 Duet 验证器深入见 [routing.md](routing.md)。

### ⑤ LLM 流式

`llm.NewClient(apiKey, baseURL)` 的 `Client.Stream(ctx, req)`（`internal/llm/client.go`）发起 SSE 请求。`readSSE` 实施两级超时：

- `FirstTokenTimeout`（默认 45s）：从发请求到首个 token；
- `ChunkStallTimeout`（默认 20s）：流中相邻事件的最大间隔。

两者对应哨兵错误 `ErrFirstTokenTimeout` / `ErrChunkStall`；HTTP 层不设全局超时（`transport.go`，拨号 15s、TLS 握手 10s，按注释是"China-latency-appropriate"）。agent 侧由 `streamWithReissue` 包一层断流重发，`consumeStream` 聚合事件流。多 provider 适配（DeepSeek / OpenAI 原生 / Anthropic / OpenAI-compat）在 `provider*.go`，矩阵见 [../reference/providers.md](../reference/providers.md)。

### ⑥ 工具调度与安全关卡

模型返回 tool call 后 `runToolCalls` → `executeOne`（`agent.go`）。每个调用顺序过三道关：

1. **权限**：`a.Permissions.Decide(permissions.Check{Tool, Args})`（`internal/permissions/policy.go`）按工具/路径/命令模式决定放行、询问或拒绝；子代理经 `Policy.DeriveChild` 拿到收紧后的子策略。
2. **hook**：`executeOne` 在执行前跑 `hooks.EventPreToolUse`（可否决），执行后由 `firePostHook` 跑 `EventPostToolUse` / `EventPostToolUseFailure`（`internal/hooks` 的 `Runner.Run`，内置函数与子进程两种 hook 统一调度；全部五个事件见 `hooks.go`）。
3. **沙箱**：`bash` 工具经 `internal/sandbox` 包裹——macOS 用 seatbelt（`sandbox_darwin.go` 的 `Wrap`），Linux 用 Landlock（`sandbox_linux.go`），经 `dsc __sandbox_run` 重入子进程（`main.go` 最先分发的就是它）。

畸形 tool call 由 `internal/repair` 修复（schema 纠偏、截断恢复等）；修复失败次数会作为 `RepairErrorsLastTurn` 信号反馈给 ④ 的路由。读多写少的工具并行执行，快照（⑦）在并行开跑前串行完成。

### ⑦ 快照

任何改文件的工具执行前，agent 调 `a.Persister.TakeSnapshot(stepIdx, paths)`，落到 `snapshots.Manager.Take`（`internal/snapshots/manager.go`）保存改动前副本；`/undo` 走 `Manager.Undo` 逐步回滚，`Manager.Prune` 随会话清理。

### ⑧ 持久化与收尾

`session.NewPersister(store, snaps, sessionID)`（`internal/session/persister.go`）把用户/助手消息按内容块写入 SQLite（`session.Store`，`store.go` 的 `Open`）；agent 侧只依赖 `internal/agent/persistence.go` 定义的 `Persister` 接口，保持内核与存储解耦。turn 收尾时：

- `cache.Attribute`（`internal/cache`）生成本 turn 的缓存收据（命中/未命中归因），经 `cache.ReceiptLine` 上报 UI；
- 上下文逼近预算时 `maybeCompact` 触发压缩（`compact.go` / `semantic_compact.go`），压缩只动边界之后的内容，前缀代际由 `prefix_epoch.go` 管理；
- 每 turn 按 `internal/traceschema.Record` 追加一行 JSONL trace，供 `dsc trace`（`internal/traceinspect`）离线分析。

## 3. 37 包速览

`internal/` 共 37 个包，按依赖方向大致分四层：接入端（tui / gateway / acp）→ 内核（agent）→ 能力（llm / tools / prompt / ...）→ 基础设施（config / logging / ...）。

| 包名 | 一句话职责 | 关键类型 / 入口 |
|---|---|---|
| `acp` | ACP 接入层：stdio JSON-RPC 服务 + HTTP/SSE 网关共用的会话管理与 agent 工厂 | `SessionManager`、`ACPServer`、`HTTPGateway`、`RealAgentFactory` |
| `agent` | ReAct 主循环：turn 边界、工具调度、事件扇出、停止条件、压缩、路由接线 | `Agent`、`Run`、`runStep`、`Bus` |
| `agents` | 子代理定义（`.deepseek/agent/<name>.md`）与内置 profile | `AgentDef`、`AgentProfile`、`Load`、`DefaultProfiles` |
| `bootstrap` | `dsc init` 项目脚手架：生成 DEEPSEEK.md 与 `.deepseek/config.toml`（含语言检测） | `InitOptions`、`Run` |
| `cache` | 前缀缓存命中/未命中归因诊断（纯函数，无缓存逻辑） | `CacheReceipt`、`Attribute`、`ReceiptLine` |
| `cacheunit` | 计算前缀 padding，使 Static Prefix 对齐 DeepSeek cache-unit 边界 | `AlignPadding`、`PadText` |
| `codegraph` | 代码符号知识图谱索引与 PageRank 中心度排序 | `Store`、`RankByPageRank` |
| `commands` | 自定义 slash 命令的加载与解析 | `Command`、`ParseCommand`、`Load` |
| `config` | TOML 配置加载，优先级 CLI > 项目 > 用户 > 内置默认 | `Config`、`Load`、`Default` |
| `doctor` | `dsc doctor` 健康检查（key 有效性、网络等） | `CheckResult`、`RunChecks`、`Run` |
| `gateway` | `/v1/*` HTTP+SSE 网关：会话、turn、文件、checkpoint、MCP 管理等端点 | `NewHandler`、`ServeHandler`、`hub` |
| `gitctx` | 读取 cwd 的 git 状态，供 prompt 动态上下文段 | `Reader`、`Snapshot` |
| `hooks` | 生命周期 hook 系统（五个事件），内置函数 + 子进程两种执行方式 | `Runner`、`HookConfig`、`BuiltinHook` |
| `i18n` | 轻量消息目录，`DEEPSEEKCODE_LANG`/`LANG` 决定 locale | `T`、`ReloadLocale` |
| `llm` | 手写 DeepSeek API 客户端（OpenAI 兼容 function calling over SSE），两级流超时 | `Client`、`Stream`、`Request`、`Event` |
| `llmtest` | 离线确定性 mock DeepSeek server，端到端测试 agent 循环用 | `Server`、`Turn`、`NewServer` |
| `logging` | 结构化日志，debug 关闭时丢弃；敏感信息脱敏 | `Setup`、`RedactAPIKey` |
| `lsp` | 轻量 LSP 客户端（JSON-RPC over stdio），语言服务器检测与诊断接入 | `Registry`、`DetectServers` |
| `mcp` | MCP 客户端：spawn 子进程 / SSE 连接，握手并桥接外部工具 | `Registry`、`Connect`、`ConnectSSE` |
| `memory` | BM25 索引的长期记忆，JSONL 存储、SHA 去重、近重复合并 | `Store`、`JSONLStore`、`BM25Index` |
| `onboarding` | 首跑检测与交互式配置引导（API key 校验、写配置） | `NeedsOnboarding`、`Run`、`ValidateKey` |
| `permissions` | 分层审批模型：按工具/路径/命令模式决定放行、询问或拒绝 | `Policy`、`Decide`、`DeriveChild` |
| `prompt` | 缓存稳定的系统提示组装；冻结前缀与动态尾部的边界在此定义 | `SystemPromptBuilder`、`Build`、`DynamicContextBoundary` |
| `repair` | 工具调用修复：畸形 JSON 纠偏、截断恢复、修复语料回归 | `RunCorpus`（+ `dsml.go`/`scavenge.go`/`truncation.go`） |
| `routing` | 每 turn 无 LLM 调用的模型/effort 路由（Flash↔Pro，带粘滞） | `Classify`、`Signals`、`Decision` |
| `sandbox` | OS 沙箱：macOS seatbelt / Linux Landlock，包裹 bash 子进程 | `Sandbox`、`Profile`、`RunSandboxedChild` |
| `session` | SQLite 会话存储与逐块持久化 | `Store`、`Open`、`Persister` |
| `skills` | 缓存稳定的技能元数据索引，技能 body 懒加载（不进前缀） | `Skill`、`Store`、`Load` |
| `snapshots` | 编辑前文件快照与回滚（`/undo`） | `Manager`、`Take`、`Undo` |
| `structsearch` | 结构化代码搜索（按函数等结构单元） | `Query`、`Search` |
| `tokenizer` | 纯 Go 的 DeepSeek V4 BPE tokenizer，仅用于精确计数（不上 wire） | `CountExact` |
| `tools` | 内置工具面：read_file/edit_file/bash/grep 等及其注册表；MCP 工具桥接进来 | `Tool`（接口）、`Registry`、`New` |
| `traceinspect` | trace JSONL 离线分析：逐 turn 账本、请求体 diff | `TurnLedger`、`ExplainFile`、`DiffBytes` |
| `traceschema` | trace 记录的单一事实源 schema（agent 写、traceinspect 与基准读） | `Record` |
| `tui` | Bubble Tea 终端 UI：单 `tea.Program` 持有全部界面状态 | `App`、`New`、`Run` |
| `version` | 构建期版本标识（-ldflags）与安装方式/新版本检测 | `DetectInstallMethod`、`Display` |
| `worktree` | git worktree 管理（shell 出 `git`，无 go-git 依赖） | `Manager`、`Worktree`、`BranchLock` |

几条跨包的阅读提示：

- **体量分布**（`wc -l` 数量级）：`tui` 与 `agent` 各约 1.9 万行是两座大山；`tools` 约 1.3 万行；`llm`、`gateway` 各约 0.7 万行；其余多在数百到数千行。小包（`structsearch` 72 行、`traceschema` 75 行）往往是刻意收窄的单一职责。
- **工具清单与权限语义**不在此展开，见 [../reference/tools.md](../reference/tools.md)、[../reference/permissions.md](../reference/permissions.md)、[../reference/sandbox.md](../reference/sandbox.md)。
- **子代理**：`tools` 暴露 spawn 工具，由 `internal/agent/spawn_dispatch.go` 的 `LoopSpawner.Spawn` 实现——子代理就是又一个 `Agent`，继承收紧的权限（`Policy.DeriveChild`）与裁剪的系统提示（`childSystem`）。
- **背景任务**：`internal/agent/jobs.go`（`JobKind`/`JobState`）支撑 background bash 等长任务。

## 4. 代码导读路线（第一周读什么）

### Day 1 —— 入口与装配

从 `cmd/dsc/main.go` 读起：

1. `main()`：最先分发 `__sandbox_run` 沙箱重入，然后进 `run()`；
2. `run()`：逐个分发子命令（`init` / `doctor` / `upgrade` / `trace` / `cache` / `serve`），解析 flag，最后落到 `runTUI` 或 `runOneShot`；
3. 装配全在这两个函数里：`config.Load()` → 连 MCP（`cfg.ActiveMCPServers()` + `mcp.Registry.Connect`）→ 探测 LSP（`lsp.DetectServers`）→ `reg.Register(...)` 注册工具 → `session.NewPersister` 接持久化 → `tui.New(...)` 进 UI。

注意：`internal/bootstrap` **不是**启动装配——它只是 `dsc init` 的脚手架（`bootstrap.Run` 生成 DEEPSEEK.md 与配置模板）；真正的运行期装配就在 `cmd/dsc/main.go`，没有独立的 wiring 包。

### Day 2–3 —— agent 的 turn 循环

读 `internal/agent/agent.go`，抓住主线三个函数：

- `Run` —— turn 入口：steer 排队、停止原因、压缩触发点（逐步走查见 [agent-loop.md](agent-loop.md)）；
- `runStep` —— 一步 = 一次 LLM 往返：组消息（`fullMessages`）、路由（`routeTurn`）、收流（`streamWithReissue` / `consumeStream`）；
- `runToolCalls` / `executeOne` —— 权限关卡、PreToolUse/PostToolUse hook、并行执行、快照。

主线之外按需读旁支：`bus.go`（事件扇出）、`compact.go` + `semantic_compact.go`（压缩）、`prefix_epoch.go`（前缀代际）、`stop_conditions.go`、`spawn_dispatch.go`（子代理）、`plan.go`（plan 模式）、`jobs.go`（背景任务）。这个包约 1.9 万行，但旁支都是主线的支流，迷路时回到 `runStep`。

### Day 4 —— llm 与缓存纪律

- `internal/llm/client.go`：`Stream`、`readSSE`、两级超时；
- `internal/llm/static_prefix.go` 与 `prefix_drift.go`：前缀稳定性的运行期防线；
- `internal/prompt/builder.go`：`DynamicContextBoundary` 如何切冻结前缀与动态尾部；
- 配套读 [model-compatibility.md](model-compatibility.md)——DeepSeek V4 的 wire 约束是很多"看起来奇怪"的代码存在的原因；改任何请求形状前先读 [parity.md](parity.md)，了解 parity 测试如何把行为锁死在四个方向上。

### Day 5 —— 按兴趣分叉

- **TUI**：`internal/tui/app.go`（`New` / `Run`、`tea.Program` 消息循环）入手，组件按文件名自解释（`completions.go`、`diffview.go`、`permission.go`、`history.go`…）。
- **三端/网关**：`internal/acp/http.go`（`handlePrompt` / `handleStream`）→ `internal/gateway/gateway.go` + `hub.go` → `cmd/dsc/serve.go` → `desktop/main.go`，正好走完 §1 的图。
- **工具与安全**：`internal/tools/registry.go`（`Tool` 接口、tier 体系）→ 挑一个工具如 `bash.go` 顺着读 `internal/permissions/policy.go` 与 `internal/sandbox/`。

两个贯穿全程的调试手段：

- **trace**：设 `DEEPSEEKCODE_TRACE_JSONL` 指向一个文件（gateway 路径在 `cmd/dsc/serve.go` 读这个变量），每 turn 落一行 `traceschema.Record`，再用 `dsc trace` 离线分析（`traceinspect.ExplainFile` 输出逐 turn 账本，含缓存命中归因）；
- **离线测试**：`internal/llmtest` 提供确定性 mock DeepSeek server，`internal/agent` 的循环测试（如 `loop_mock_test.go`）全程不碰网络；`go build ./...` 秒级完成，改完即可验证。

各方向的深入篇（agent-loop / prefix-cache / routing / tools / tui / three-surfaces）会陆续落在本目录，已就位的第一篇是 [agent-loop.md](agent-loop.md)；此外 [model-compatibility.md](model-compatibility.md)、[parity.md](parity.md) 与 [adr/](adr/) 也是 dev/ 下已可用的材料。
