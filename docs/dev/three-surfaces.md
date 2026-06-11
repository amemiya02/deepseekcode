# 深入：一核三端——gateway、Web SPA、Desktop 与 ACP

> 本文是 [architecture.md](architecture.md) 的接入层深入篇（六篇之末）。讲同一个 agent 内核如何同时伺候 TUI、Web SPA、桌面端三个界面，外加 ACP 这条无界面的协议通道。TUI 自身的实现见 [tui.md](tui.md)；桌面端的用户视角见 [../reference/desktop.md](../reference/desktop.md)，本文只讲实现（注意 [../reference/web.md](../reference/web.md) 是 `web_fetch`/`web_search` 工具参考，与本文的 Web SPA 不是一回事）。

## 0. 解决什么问题

dsc 是单个 Go 二进制，但要以四种形态出现：终端里的 TUI、浏览器里的 SPA、macOS 上的 `.app`、编辑器可挂接的 headless 协议。如果每个端各自持有一份"会话怎么建、事件怎么流、权限怎么批"的逻辑，四份实现会立刻漂移。这一层要解决三件事：

1. **复用边界划在哪**：哪些状态只能活在内核，哪些必须留给各端自己（渲染、审批交互）；
2. **流式语义过 HTTP 不丢帧**：agent 在后台 goroutine 里喷事件，浏览器的 `EventSource` 却是"先 POST 后订阅"，时序天然错位（§3.2 的 replay buffer 就是为此存在）；
3. **一个 handler 多处挂载**：同一个 `/v1/*` 处理器要同时被 `dsc serve --http` 的监听器、桌面 webview 的资产中间件、桌面回环兜底监听器复用，且共享会话状态。

四条路径的启动命令（均已核实，TUI 即裸 `dsc`）：

```sh
make build-web && ./bin/dsc serve --http 127.0.0.1:7432   # Web：嵌入 SPA 的网关
dsc serve --acp                                           # ACP：stdio JSON-RPC
make desktop                                              # Desktop：bin/DeepSeekCode.app
```

## 1. 复用边界：什么在内核，什么在端上

| 职责 | 归属 | 锚点 |
|---|---|---|
| turn 循环、工具执行、权限**策略** | 内核，所有端共享 | `internal/agent`、`internal/permissions` |
| 会话生命周期（建/查/取消/steer/逐 turn 设置） | `acp.SessionManager`，Web/Desktop/ACP 三路共用 | `internal/acp/session.go` |
| 每会话造一个生产级 agent | `acp.RealAgentFactory` | `internal/acp/factory.go` |
| agent 事件 → 跨进程事件 | `acp.AgentAdapter`（订阅 `agent.Bus`，翻成 `AgentEvent`） | `internal/acp/adapter.go` |
| `/v1/*` HTTP API、SSE 扇出、审批 id 簿记 | gateway | `internal/gateway/gateway.go`、`hub.go`、`turn.go` |
| 会话列表元数据、model/effort/output-style 的 UI 态 | gateway 自己持有（不进内核） | `gateway.Handler` 的 `sessions`/`models`/`outputStyle` 字段 |
| 权限审批的**交互** | 各端各管（TUI 卡片；SPA 收 `permission_request` 再 POST 回来；ACP 交给客户端） | §3.4 |
| 渲染 | 各端各管 | `internal/tui` / `web/src` |

两条结构性事实（读代码前先立住）：

- **TUI 是零网关路径**。`cmd/dsc/main.go` 的 `runTUI` 在进程内直接构造 `agent.Agent` 并订阅它的 `Bus`，完全不经过 gateway 和 acp。所以"三端一核"准确说是：TUI 直连内核；Web、Desktop、ACP 三路骑在 `acp.SessionManager` 上。
- **gateway 不持有 agent**。`gateway.NewHandler(sm, …)` 包着 `*acp.SessionManager`（`internal/gateway/gateway.go`），每个会话经 `RealAgentFactory` 按需创建 agent——与 TUI 的进程内装配等价，只是多了一层会话簿记。

## 2. 核心数据结构

### 2.1 `acp.AgentEvent`：跨进程的事件词汇表（`internal/acp/session.go`）

`EventKind` 共 17 种：`TextDelta`/`Info`/`Done` 三个基础种类，`Permission`/`Ask` 两个**交互**种类（payload 里带 `Respond func(PermissionDecision)` / `Answer` 回调闭包，见 §3.4），以及 `ToolStart`/`ToolEnd`/`ToolDelta`/`Cache`/`Cost`/`Routing`/`Job`/`Retry`/`Thinking`/`Plan`/`Duet`/`CompactionWarning` 等实时种类。`PermissionDecision` 是四档：`Deny`/`AllowOnce`/`AllowSession`/`AllowAlways`。

### 2.2 `gateway.Handler`：`/v1/*` 的复合处理器（`internal/gateway/gateway.go`)

```go
type Handler struct {
    mux  *http.ServeMux
    sm   *acp.SessionManager
    hub  *hub                            // 每会话 SSE 扇出 + replay buffer
    pendingPerm map[string]pendingPermission // 在途审批：id -> Respond 闭包
    pendingAsk  map[string]pendingAsk
    activeTurns map[string]bool          // 双发守卫：在跑的 turn 把第二个 prompt 转成 Steer
    sessions    *sessionStore            // 会话栏元数据（标题/时间/turn 数）
    models      *modelState              // /v1/model、/v1/effort 维护的 UI 态
    // …store(checkpoint)、snaps(代码回滚)、root(工作区)、mcpReg 等可选件
}
```

`NewHandler` 注册约 40 个 `/v1/*` 路由，主要分组（全部在 `gateway.go` 的 `HandleFunc` 清单里，可验证）：

- **turn**：`POST /v1/prompt`、`GET /v1/events`（SSE）、`/v1/cancel`、`/v1/steer`、`/v1/permission`、`/v1/answer`；
- **会话**：`/v1/sessions`、`/v1/sessions/{id}`；**checkpoint**：`/v1/rewind`、`/v1/fork`、`/v1/branch`、`/v1/switch`、`/v1/summarize`；
- **遥测**：`/v1/cache`、`/v1/cache/ledger`、`/v1/balance`；**模型**：`/v1/models`、`/v1/model`、`/v1/effort`、`/v1/capabilities`；
- **工作区**：`/v1/files`、`/v1/file`、`/v1/changed`、`/v1/diff`、`/v1/upload`、`/v1/add-to-chat`、`/v1/git/branches`、`/v1/git/checkout`；
- **配置面**：`/v1/config`、`/v1/mcp`(+`/{name}`)、`/v1/hooks`、`/v1/skills`、`/v1/memory`、`/v1/doctor`、`/v1/onboarding`、`/v1/update` 等。

最后一行是关键：`h.mux.Handle("/", webapp.Handler())`——凡不在 `/v1` 下的路径都落到嵌入的 SPA（`ServeMux` 里 `/` 优先级最低，`/v1/*` 永远先赢）。

### 2.3 `hub`：每会话 SSE 扇出 + 每 turn 重放缓冲（`internal/gateway/hub.go`）

```go
type hub struct {
    mu      sync.Mutex
    clients map[string][]*subscriber // sessionID -> 订阅者
    buffers map[string][]sseEvent    // sessionID -> 当前 turn 的帧（供重放）
}
const maxTurnBuffer = 512
```

同文件的 `mapAgentEvent` 是 `acp.AgentEvent` → SSE 命名事件的唯一翻译点：`message_delta`、`thinking_delta`、`tool_start/_delta/_end`、`plan_update`、`routing`、`duet`、`cache_update`、`cost_update`、`job_update`、`retry`、`compaction_warning`、`turn_done`。注意 `permission_request`/`ask_request` **不在**这里产生——gateway 要先在 pending 表里登记并分配 id，所以由 `handlePrompt` 的 onEvent 自己广播（§3.4）。

### 2.4 `webapp` 包：SPA 的嵌入点（`webapp/embed.go`）

`//go:build withwebapp` 之下 `//go:embed dist`；`Handler()` 返回静态文件服务器（给 gateway 的 `/` 兜底），`DistFS()` 返回 `fs.FS`（给 Wails 资产服务器）。不带 tag 时 `handler_stub.go` 提供占位实现，保证新克隆下 `go build ./...` 不依赖前端产物。

## 3. 控制流走查：Web 端一条消息的旅程

### 3.1 `POST /v1/prompt`（`gateway.go` 的 `handlePrompt`）

1. 解析 `{prompt, session_id?, mode?}`；无会话或会话不存在则 `sm.NewSession`；
2. **双发守卫**：`activeTurns[sid]` 已置位时不开第二个 run goroutine（会跟在跑的 agent 竞态），而是 `sm.Steer(sid, prompt)` 做 turn 中改向，直接返回；
3. `sm.ApplySettings(sid, acp.TurnSettings{Model, Effort, PermissionMode})`——composer 上选的模型/effort/审批模式在 run 启动**前**写进该会话的 agent（turn 间裸字段写是安全的）；
4. `h.hub.resetTurn(sid)` 清空重放缓冲，然后 `go func(){ … h.sm.Prompt(runCtx, sid, prompt, onEvent) }()` 后台起跑——**runCtx 取自 `sm.SessionCtx(sid)` 而不是请求 ctx**，POST 连接关了 agent 也继续跑；
5. 立即返回 `{request_id, session_id}`。流式内容不在这条响应里，要靠下一步的 SSE；
6. **错误也要收尾**：`sm.Prompt` 返回错误时（如会话已消失），run goroutine 兜底广播一帧 `turn_done{stop_reason: "error: …"}`——SPA 的 onDone 在失败路径上同样会触发，composer 不会因为后端报错而卡死。

### 3.2 `GET /v1/events` 与 replay buffer：被它根治的竞态

SPA 的时序是"`POST /v1/prompt` 返回**之后**才 `new EventSource('/v1/events?session_id=…')`"（`web/src/lib/api.ts` 的 `GatewayClient.openEventStream`）。而 run goroutine 在 POST 返回前就已起跑——一个快 turn（或纯文本短答）可能在订阅建立前就把 `turn_done` 广播完了。没有缓冲时这帧直接丢失，SPA 的 onDone 永远不触发，composer 永久锁死——这就是桌面端"第二条消息发不出去/回答卡在 Thinking"级 bug 的根因。

hub 的根治手段是**让广播与订阅在同一把锁下对账**（`hub.go`，逐行可验证）：

- `broadcast`：在锁内先 `appendBounded(buffers[sid], ev)` 进当前 turn 缓冲，再快照订阅者列表，锁外非阻塞投递（满 channel 即丢，不背压 agent）；
- `subscribe`：在**同一把锁**内把订阅者加进 `clients` 并快照 `buffers` 作为 backlog 返回——锁保证每帧恰好走一条路：要么已在 backlog 里，要么进 channel，不会两边都有或两边都无；
- `handleEvents`（`gateway.go`）先把 backlog 逐帧写出（遇 `turn_done` 直接结束流），再进 live 循环；
- `resetTurn` 在每个 prompt 开跑前清缓冲，晚到订阅者只重放**当前** turn；
- `maxTurnBuffer = 512` 封顶，超限丢最旧帧、**尾部（含 `turn_done`）永远保留**。

守卫测试：`hub_replay_test.go` 的 `TestHubReplaysBufferedTurnToLateSubscriber`（注释明言这是 dead-2nd-send 的根因守卫）与 `TestHubResetTurnClearsBuffer`。

### 3.3 权限审批往返

1. 内核要批准时，`AgentEvent{Kind: EventKindPermission, Respond: func(PermissionDecision)}` 到达 `handlePrompt` 的 onEvent；
2. gateway 分配 `perm-N` id，把 `Respond` 闭包存进 `pendingPerm`，广播 `permission_request{id, tool, args, options}`（options 即 `permissionOptions()` 的 deny/once/session/always 四档）；
3. SPA 弹审批卡，用户选择后 `POST /v1/permission {id, decision}`；
4. `turn.go` 的 `handlePermission` 取出闭包调用 `p.respond(decodePermissionDecision(…))`——**未知字符串一律按 deny 处理（fail-closed）**。`ask_request`/`POST /v1/answer` 是同构的问答版。

整个往返里 agent goroutine 阻塞在 `Respond` 上等答案；策略（哪些工具要批）在内核，**决定权的传输与 UI 在端上**。

### 3.4 鉴权与 loopback 约束（`internal/gateway/auth.go`、`cmd/dsc/serve.go`）

- `withAuth` 两道闸：① 远端地址必须是回环 IP，否则 403（绑定 127.0.0.1 之外的纵深防御）；② `DSC_GATEWAY_TOKEN` 非空时要求 `Authorization: Bearer`（常数时间比较），空 token 只免 bearer、回环检查仍在；
- `dsc serve --http` 的 `resolveBindAddr`：默认把通配/非回环 host **强制改写**为 127.0.0.1；只有 `--http-allow-remote` 显式放行，且此时 `buildServeHandler` 生成随机 32 字节 token 包上 `requireBearer` 并打印到 stderr；
- 桌面端见 §5——webview 路径故意**绕过** `withAuth`。

## 4. Web SPA（`web/`）

**技术栈**（`web/package.json`，name `dsc-web`，已核实）：React 18 + TypeScript + Vite；状态用 zustand；渲染件有 react-markdown(+remark-gfm/math、rehype-highlight/katex)、monaco-editor、highlight.js、lucide-react、cmdk；测试 vitest + Playwright。不是 Svelte（`webapp/embed.go` 的包注释写着"Svelte SPA"，是陈旧注释，以 package.json 为准）。

**与 gateway 的契约**全部收口在 `web/src/lib/api.ts`（文件头自称"the SINGLE gateway client"）：REST 部分是一批 typed `fetch('/v1/…')` 函数；事件流部分是 `GatewayClient.openEventStream(sessionId, handlers)`，对 §2.3 的每个命名事件 `addEventListener`。开发态 `vite.config.ts` 把 `/v1` 代理到 `http://localhost:7432`；生产态 SPA 与 gateway 同源（嵌入或同进程），相对路径直接生效——三种宿主（浏览器、webview、回环兜底）零分支。

**嵌入链路**（Makefile，已核实）：

```
make web        # cd web && npm install && npm run build；cp web/dist → webapp/dist
make build-web  # 依赖 web；go build -tags withwebapp ./cmd/dsc
```

不带 `-tags withwebapp` 构建出的 dsc 一切照常，只是 `/` 返回"SPA not embedded"占位页（`webapp/handler_stub.go`）。

## 5. Desktop（`desktop/`）

### 5.1 打包：stock 工具链，无 wails3 CLI

`make desktop`（依赖 `web`）→ `desktop/package-darwin.sh`，全程只用 Go + macOS 自带工具（与 [../reference/desktop.md](../reference/desktop.md) 一致）：

1. `go build -tags withwebapp ./desktop/`——桌面壳是普通 Go 包，Wails v3 只是库依赖；
2. 手工组装 `bin/DeepSeekCode.app/Contents/{MacOS,Resources}`：拷 `desktop/packaging/Info.plist`（`plutil -lint` 校验）、写 `PkgInfo`，`sips` 生成 10 档 iconset 再 `iconutil -c icns`；
3. ad-hoc `codesign`（本机可启动；分发签名/公证不在脚本范围）。

脚本头注释解释了为什么绕开 `wails3 build`：那条路要 wails3 CLI + go-task + 模板 Taskfile，而本仓库经 `webapp.DistFS` 用 Go 嵌入 SPA，单二进制自足。CLI 路径仍保留在 `desktop/Taskfile.yml` 供装了工具的人用。

### 5.2 运行时：一个 handler，两条挂载路径（`desktop/main.go`、`app.go`）

- `App.gatewayHandler()` 经 `sync.Once` 调 `gateway.DefaultHandler()` 构造**唯一**的 gateway 实例（生产 SessionManager + session.Store + snapshots + 工作区根）；
- **webview 主路径**：`AssetOptions.Handler = AssetFileServerFS(webapp.DistFS())` 伺服 SPA，`Middleware = gatewayMiddleware(gw)` 把 `/v1/*` 前缀截给 gateway——SPA 的相对 `/v1` 请求在 webview 同源解析，无端口、无 CORS、SPA 零改动。此路径**不包** `withAuth`：Wails 给 webview 请求伪造非回环 RemoteAddr（TEST-NET 192.0.2.1），包了必 403（`main.go` 注释明言）；
- **回环兜底**：`App.ServiceStartup` 用 `gateway.ServeHandler(ctx, 7432, h)` 把**同一个** handler 挂到 `127.0.0.1:7432`（这条路有 `withAuth`），浏览器开 `http://127.0.0.1:7432` 与窗口内 webview 共享全部会话状态。

### 5.3 壳桥：`web/src/lib/desktopBridge.ts`

SPA 在浏览器和 webview 里渲染同一份代码；桌面专属能力全部隔离在这个薄适配层里：`isDesktop()` 探测 `window._wails`，绑定方法经 Wails v3 的 `Call.ByName('App.GetPort'/'App.GetVersion'/'App.OpenFileDialog', …)` 调用（Go 侧在 `desktop/app.go`），开链接走 `Browser.OpenURL`；运行时缺席时全部优雅降级，因此 SPA 对 `@wailsio/runtime` 无硬依赖、浏览器构建与单测不受影响。

## 6. ACP（`internal/acp`）

ACP 是无界面的接入协议，供编辑器等外部客户端 headless 驱动 agent。**当前可用的客户端模式只有 stdio 一种**：

- `dsc serve --acp` → `acp.NewACPServer(sm, stdin, stdout)`（`server.go`），stdin/stdout 上的 JSON-RPC 2.0，`FrameReader/FrameWriter`（`transport.go`）做帧编解码；`--acp` 与 `--http` 互斥（`cmd/dsc/serve.go` 校验）；
- 方法表（`server.go` 的 dispatch，已核实）：`session/new`、`session/prompt`（流式 notification 回推）、`session/cancel`。

包内另有一个 `HTTPGateway`（`http.go`：`POST /session`、`POST /session/{id}/prompt`、`GET /session/{id}/stream` SSE、`DELETE /session/{id}`，自带随机 bearer token）——**目前没有任何 CLI 入口挂载它**（全仓库非测试引用为零，已核实），属于库级表面 + 测试覆盖的预留件；`internal/gateway/hub.go` 的广播纪律即借鉴自它。不要把它和 `/v1/*` gateway 混为一谈：SPA 走的是 `internal/gateway`，不是这个。

更重要的是：HTTP `/v1` 路与 stdio 路**之下是同一个 `SessionManager`**，会话语义（创建、取消、steer、逐 turn 设置）完全一致——这就是"三路骑一核"的支点。

## 7. 不变量与测试守卫

| 守卫 | 不变量 |
|---|---|
| `internal/gateway/hub_replay_test.go` | 晚到订阅者必收到整个 turn（含 `turn_done`）；`resetTurn` 清空缓冲 |
| `internal/gateway/auth_test.go` | 非回环 403；坏 token 401;回环+对 token 放行；空 token 只免 bearer |
| `internal/gateway/gateway_test.go` | `/` 兜底出 SPA；`/v1/prompt` 返回双 id；events 流以 done 收尾；权限请求会以 `permission_request` 出现在流上 |
| `internal/acp/server_test.go` | `session/new`→`session/prompt` 流式→`session/cancel` 全回路；未知方法回标准 JSON-RPC 错误 |
| `internal/acp/transport_test.go` | 帧读写往返、EOF、坏头处理 |
| `internal/acp/http_test.go` | HTTPGateway 的 token 闸（缺/错 token 拒绝）与 SSE 流 |
| `internal/acp/session_kinds_test.go` | EventKind 枚举互异；交互事件确实携带 Respond/Answer 字段 |
| `desktop/middleware_test.go` | `/v1` 前缀进 gateway、方法不变形、gw 为 nil 时落回资产链 |
| `desktop/app_test.go`、`gateway_integration_test.go` | ServiceStartup 拉起回环 gateway；`/v1/cache` 可达；默认端口 7432 |
| `webapp/embed_test.go` | `Handler()` 出 HTML；`DistFS()` 非 nil、有 index.html、未知路径 `fs.ErrNotExist`（stub 与真嵌入同契约） |
| `web/src/lib/api.test.ts`、`api-contract.test.ts` | SPA 侧对每个命名事件/REST 形状的消费契约（配 `mockGateway.ts`） |

改这一层时的红线：**`turn_done` 是流的终结协议**——`handleEvents` 靠它关流、SPA 靠它解锁 composer，任何新事件都不得在它之后发；**`subscribe` 与 `broadcast` 必须共一把锁**，拆开就把 §3.2 的竞态请回来了。

## 8. 修改场景实操：加一个新的 `GET /v1/quota` 端点

1. **Go（必做）**：新建 `internal/gateway/quota.go` 写 `func (h *Handler) handleQuota(w, r)`，在 `gateway.go` 的 `NewHandler` 路由清单里加一行 `h.mux.HandleFunc("/v1/quota", h.handleQuota)`；配 `quota_test.go`（参考 `doctor_test.go` 这类纯读端点）。鉴权**自动生效**——回环监听器的 `withAuth`、`dsc serve` 远程绑定的 bearer 都包在 handler 外层，新路由无需自理。
2. **Web SPA（必做）**：在 `web/src/lib/api.ts` 加 typed 的 `fetchQuota()`，配 `api.test.ts` 用例；组件或 zustand store 消费它。开发态 vite 代理、生产态同源都无需配置。
3. **Desktop：零改动**。`gatewayMiddleware` 按 `/v1/` 前缀整体放行（`desktop/main.go`），新端点在 webview 和回环兜底两条路上自动可用。
4. **TUI：通常零改动**。TUI 不经过 gateway；若 TUI 也要同能力，在 `internal/tui` 直接调内核包（gateway 的 handler 本身也应当只是内核能力的薄封装，逻辑下沉）。
5. **ACP：不自动获得**。stdio 协议没有 `/v1` 概念；确有外部客户端需要时，在 `internal/acp/server.go` 的 dispatch 加 JSON-RPC 方法并配 `server_test.go`。
6. **若数据产生在 turn 过程中**（要流式推送而非请求/响应）：那不是新 REST 端点，而是新 SSE 事件——五处一起动：`internal/acp/session.go` 加 `EventKind` 与字段 → `adapter.go` 从 `agent.Bus` 翻译 → `hub.go` 的 `mapAgentEvent` 定事件名与 payload → `api.ts` 的 `openEventStream` 加 listener → `session_kinds_test.go` 与 SPA 契约测试同步。事件名一旦发布就是契约（SPA 按名字 `addEventListener`），不要改名复用。

---

相关阅读：总图 [architecture.md](architecture.md)；内核循环 [agent-loop.md](agent-loop.md)；TUI 端 [tui.md](tui.md)；桌面端用户视角 [../reference/desktop.md](../reference/desktop.md)。
