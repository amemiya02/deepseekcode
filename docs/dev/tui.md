# 深入：Bubble Tea TUI

> 本文是 [architecture.md](architecture.md) 的 TUI 深入篇。代码全部在 `internal/tui`（约 1.9 万行，与 agent 并列两座大山）。主题的用户侧配置见 [reference/tui-theme.md](../reference/tui-theme.md)——那里讲怎么配，这里讲怎么实现。

## 0. 解决什么问题

终端 UI 要同时伺候两个节奏完全不同的世界：一边是 agent goroutine 高频喷出的流式事件（reasoning delta、text delta、tool call），一边是用户的键盘输入与终端 resize。Bubble Tea 的答案是单线程消息循环——所有状态变更都收敛到一个 `Update` 函数里按消息折叠，渲染由纯函数 `View` 从状态推导。`internal/tui` 在这个骨架上解决三件事：

1. **跨线程桥接**：agent 事件如何安全进入单线程的 `tea.Program`；
2. **流式节流**：token 洪峰不能逐条触发全屏重排（O(N²) 渲染会卡死滚动）；
3. **无重排布局**：弹层、toast、权限卡的出现与消失不能把输入框顶出屏幕。

包级架构注释（一段话版本）在 `internal/tui/messages.go` 文件头，本文是它的展开。

## 1. 核心数据结构

### 1.1 `App`：根模型（`app.go`）

```go
// app.go
type App struct {
    agent *agent.Agent            // 进程内 agent
    theme Theme                   // 语义化样式集（见 §4）
    vp    viewport.Model          // 滚动正文（charm.land/bubbles/v2）
    input textarea.Model          // 输入框
    mode  appMode                 // 键盘路由模式（见 1.2）

    scrollback *Scrollback        // 聊天历史 + 流式游标 + visual 选区
    chrome     *Chrome            // 活动指示行 + 重绘 ticker 标志
    overlay    *Overlay           // 模态选择器（models/themes/sessions/…）
    permission *PermissionFlow    // 权限卡（模态）
    question   *QuestionFlow      // 提问卡（模态）

    completions completions       // `/` 与 `@` 内联补全弹层（见 §3.3）
    send func(tea.Msg)            // tea.Program.Send，跨 goroutine 回注消息
    // …状态字段还有 status/toast/history/fileIndex/queued 等
}
```

`New(cfg Config) *App` 构造，`Run()` 里 `tea.NewProgram(a)` 启动；一个进程一个 `App`。子模块各自持有自己渲染/变更的状态，`App` 只调用它们的方法编排，不伸手进字段。

### 1.2 `appMode`：键盘路由的唯一开关（`mode.go`）

```go
const (
    modeInsert appMode = iota // textarea 聚焦，按键流入输入框
    modeNormal                // viewport 聚焦，j/k 滚动
    modePermission            // 模态：权限卡吃掉全部按键
    modePager                 // 模态：pager 吃掉全部按键
    modeVisual                // Vim 式行选区
    modeQuestion              // 模态：提问卡
)
```

`setMode` 是模式切换的单一入口，把 `a.mode`、textarea 的 focus/blur、状态栏徽章锁在一起改——`View` 的渲染因此永远不会和 `Update` 的路由打架。

### 1.3 `agentEventMsg`：唯一的事件信封（`messages.go`)

```go
type agentEventMsg struct{ Event agent.Event }
```

agent 的所有事件（十几种 `agent.Event` 具体类型）进 TUI 只走这一个 `tea.Msg` 类型，由 `dispatchAgentEvent` 内部再做类型分发。这取代了早期"每个回调一个 tea.Msg 类型"（9 个类型）的设计。其余自定义消息也都在 `messages.go`：`runStartMsg`（提交后立即点亮 spinner）、`redrawMsg`（节流重绘 tick）、`slashExpandedMsg` / `spawnExpandedMsg` / `spawnResultMsg`（slash 命令模板展开与子 agent 结果）。

### 1.4 `completions`：补全弹层状态机（`completions.go`）

```go
type completions struct {
    active   bool
    trigger  rune        // '/' 或 '@'
    query    string      // 当前过滤串（SetQuery 写入）
    items    []complItem // 当前触发符的全量候选（打开时固定）
    filtered []int       // fuzzy 过滤后的下标，按 score 再按 label 排序
    matches  [][]int     // 每个 filtered 项的匹配 rune 下标（用于加粗）
    cursor   int         // filtered 内的光标
    anchorStart int      // 触发符在输入缓冲里的字节偏移（accept 时从这里替换）
    capped  bool         // layout() 是否设过终端高度天花板
    maxRows int          // 天花板值；0 = 完全抑制弹层
}
```

候选行 `complItem{insert, label, detail, kind}`；`kind` 来自 `commands_registry.go` 的 `cmdKind`（`builtinCmd` / `customCmd` / `skillCmd` / `fileCmd`）。

## 2. 控制流走查

### 2.1 启动与事件泵

```
cmd/dsc runTUI → tui.New(cfg) → app.Run()
  Run:  prog := tea.NewProgram(a); a.send = prog.Send
        go a.pumpEvents()          // 桥接 goroutine
        prog.Run()
```

`pumpEvents`（`app.go`）就是一个 for-range：

```go
for ev := range a.agent.Events() {   // internal/agent/agent.go: Events()
    a.send(agentEventMsg{Event: ev})
}
```

`agent.Events()` 返回 agent 终生事件 channel（带环形缓冲，满了丢最旧）；`prog.Send` 是 Bubble Tea 提供的线程安全注入口。这就是"agent 事件到达 TUI"的全部机制——一个消费者、一个信封，没有锁、没有共享可变状态。

`Init()` 做三件事：往 scrollback 写欢迎横幅与启动通知、`textarea.Blink`、并在 `readableDir(a.cwd)` 成立时发起 `indexFilesCmd(a.cwd)` 在后台 goroutine 里扫 `@` 菜单的文件索引（完成时投递 `fileIndexMsg`，期间 `@` 菜单显示"(indexing files…)"占位行）。

### 2.2 `Update`：消息分发结构（`app.go`）

`Update` 的形状是"模态拦截 → 类型 switch → 转发子模型"三段：

1. **模态拦截**：`modePermission` / `modeQuestion` 在最顶上把 `tea.KeyPressMsg` 整个吃掉（`handlePermissionKey` / `handleQuestionKey`），下面的 switch 不必关心。
2. **类型 switch**：`tea.WindowSizeMsg`→`layout()`；`tea.PasteMsg`→大段粘贴折叠成"[pasted N lines]"芯片；`tea.KeyPressMsg`→`handleKey`（拦截则返回，否则落到第 3 段）；`runStartMsg`→点亮 chrome spinner + `ensureTick()`；`redrawMsg`→节流重绘（见 2.4）；`agentEventMsg`→`dispatchAgentEvent`；以及 toast 过期、^C 双击解除、鼠标事件等。
3. **转发子模型**：只有 `modeInsert` 才把按键转给 `a.input.Update`（textarea），随后调 `syncCompletions()` 从键后缓冲重derive弹层；鼠标事件只给 viewport（给 textarea 会把 SGR 转义序列当文字插进输入框）。

`dispatchAgentEvent` 是第二层 type-switch，把每种 `agent.Event` 路由到子模块：reasoning/text delta 进 `Scrollback` 的流式游标，tool call 进 `AppendToolCall/AppendToolResult`，`EventStepFinish` 累计用量与费用进状态栏，`EventCacheReceipt` 按四因（cold_first / prefix_mut / compact_reset / steady）累计 miss tokens 喂 HUD，`EventPermissionAsk` 关掉一切 overlay 后弹权限卡并切 `modePermission`，`EventDone` 是本轮终止符——它在 agent 侧被 defer 发出、必然排在所有尾部 delta 之后，所以在这里 `EndStreams()` + `chrome.Reset()` 不会截断流。

### 2.3 一次提问的完整路径

```
Enter（modeInsert, handleInsertKey）
  → submitInput → submitPromptCmd(text)        // messages.go
      闭包内：go a.runAgent(prompt)             // agent 在自己的 goroutine 跑
      同步返回 runStartMsg                      // UI 先点亮 spinner
  → agent goroutine 喷事件 → Events() channel
  → pumpEvents 包成 agentEventMsg → Update → dispatchAgentEvent
  → EventDone：收流、重置 chrome、排空 G11 提交队列（运行中输入的 prompt FIFO 续跑）
```

### 2.4 流式 delta 的节流渲染

delta 到达时**不重绘**：`Scrollback.AppendText`（`scrollback.go`）只把文字接到流式游标指向的块上并 `bump()` 递增 `seq` 计数器。真正的重绘在 `redrawMsg` tick 里做：`scheduleRedraw` 以 80ms（约 12 fps）一拍，`Update` 收到 tick 时比较 `scrollback.Seq() != a.lastRenderSeq`，有漂移才 `refreshView()`；spinner 帧则每拍都走。`ensureTick` 幂等，chrome 不活跃且 run 结束后 ticker 自停。`Scrollback.Render` 还有按 `(width, seq)` 键的行缓存（`render_cache.go`），同帧重复渲染不重排版。

### 2.5 `View`：组合方式（`app.go`）

`View() tea.View` 是纯组装，三个出口：

- overlay 打开 ⇒ 整屏交给 `renderOverlay()`（modeTape/modeModels/modeSessions/modePalette/modeHelp/modeThemes/modeMCP/modeLSP/modePermissions/modeQuitConfirm/modeEffort/modeFilePicker 各一个渲染分支）；
- 权限卡 / 提问卡活跃 ⇒ 卡片**替换**输入框行：`header, body, chrome, permView, divider, status`；
- 常规帧 ⇒ `lipgloss.JoinVertical(header, body, chrome, divider, status, [toast], inputBox, hint)`，其中 body 是 viewport（或 pager 模态替换正文），chrome 永远保留 1 行（空时填空格防止状态翻转引起重排）。

注意 `View` **不画全屏背景**：glamour/viewport 输出里的 ANSI reset 会把外层背景切回终端默认色留下黑条（[ADR-0002](adr/0002-own-rendered-background.md)），背景只活在显式的 panel/badge/diff 带上。

### 2.6 补全弹层：浮层合成与自适应高度

弹层不占布局行——基础帧按"没有菜单"排好后，`View` 末尾把卡片**合成**上去：

```go
// app.go View() 末尾
if pop := a.completions.View(a.theme, a.width); pop != "" {
    content = overlayPopup(content, pop, bottomRows)
}
```

`overlayPopup(frame, pop, bottomRows)` 把卡片整行覆盖到帧的倒数第 `bottomRows` 行之上（底部锚定，紧贴 chrome/divider/status/输入框集群的上沿），且永不覆盖第 0 行的 header——帧太矮时宁可丢卡片顶部的行。

高度是**自适应**的，全部逻辑在 `completions.go` 的 `visibleRows()`：

```go
func (c *completions) visibleRows() int {
    if !c.active || len(c.filtered) == 0 { return 0 }   // 零匹配 ⇒ 隐藏
    limit := complMaxRows                                // 固定窗口 10 行
    if c.capped && c.maxRows < limit { limit = c.maxRows } // 终端高度天花板
    if len(c.filtered) > limit { return limit }
    return len(c.filtered)                               // 收缩贴合匹配数
}
```

- **打开时最大**：query 为空，`filtered` 是全量候选，卡片张到 `min(候选数, 10, 天花板)`；
- **随匹配收缩**：每次按键 `SetQuery` 重新 fuzzy 过滤（`fuzzy.go` 的 `fuzzyMatch`，和各 picker 共用一个匹配器），匹配变少卡片就矮下去——因为是底部锚定的浮层，收缩只是露出更多 transcript，帧上其他任何行都不动；
- **零匹配隐藏**：`visibleRows()==0` 时 `View` 返回 `""`，整张卡消失；
- **天花板**：`layout()` 用 `popupBudget = a.height − header − chrome − divider − status − input − hint − toast` 算出 transcript 带的高度，`SetMaxRows(popupBudget−2)`（减边框）——矮终端上卡片被钳住内部滚动（右缘滚动条），预算为 0 时整个弹层被抑制，绝不把输入框顶出屏幕。

`Lines() = visibleRows() + 2`（边框）与 `View` 同源于 `visibleRows`，两者按构造不可能漂移；`a.popupLines` 只是给测试与遥测的镜像。

候选从哪来：`syncCompletions`（`app.go`）在每次 insert 模式按键后从"光标左侧最近 token"找触发符（`triggerToken`：`/` 或 `@` 必须在词边界，路径里的 `a/b` 不触发），`completionItems` 对 `/` 走 `allCommands(customCmds, agent.Skills.List())`、对 `@` 走缓存的文件索引。接受（Tab，或 Enter 在非完整命令时）走 `acceptCompletion`：从 `anchorStart` 到光标替换为 `sel.insert`，光标停在插入文本之后；`exactSlashCompletionReady` 让敲全 `/models` 后的 Enter 直接执行而非再补全一次。

## 3. 组件树

按文件即组件的约定（`internal/tui/` 文件名自解释）：

| 组件 | 文件 | 职责 |
|---|---|---|
| 滚动正文 | `scrollback.go` + `items.go` | 聊天缓冲、流式游标、visual 选区；`chatItem` 各类型的渲染 |
| 工具卡片 | `tool_card.go`、`tool_renderers.go` | tool call/result 的折叠卡片与按工具定制的渲染 |
| 活动指示 | `chrome.go` | thinking/writing/tool 阶段的 spinner 行 + 重绘 ticker 标志 |
| 状态栏 | `status.go`、`status_hud.go` | 模型/用量/费用/步数；四因 cache-miss HUD |
| 输入区 | `app.go`（textarea + 边框）+ `placeholder.go`、`queue_paste.go` | 输入框、动态占位语、粘贴折叠、运行中排队 |
| 补全弹层 | `completions.go` + `fuzzy.go` + `commands_registry.go` + `fileindex.go` | 本文 §2.6 |
| 历史 | `history.go` | 跨会话 prompt 召回环（↑/↓ 在边界行触发） |
| 模态层 | `overlay.go`、`permission.go`、`question.go`、`pager.go` | 选择器全家桶、权限卡、提问卡、$PAGER 式正文 |
| 视觉基建 | `theme.go`、`grad.go`、`markdown.go`、`highlight.go`、`diffview.go`、`logo.go`、`welcome.go`、`toast.go`、`scrollbar.go` | 主题、渐变、Markdown/代码高亮、diff 视图、横幅、toast |

## 4. theme 系统（实现侧）

两层结构，都在 `theme.go`：

1. **`palette`——原始 token 层**：每个主题一袋裸色值（`brandDeep`、`selBg/selFg`、四级背景 `bgBase/bgWell/bgSurface/bgRaised`、`ok/errc/warn`…）。当前五套：`oceanPalette`（dark，默认）、`lightPalette`、`midnightPalette`、`nebulaPalette`、`auroraPalette`，各配一套 `diffBands`。
2. **`buildTheme(name, palette, diffBands) Theme`——语义层**：把 token 组合成具名样式与方法（`Panel(tier)` 四级表面、`Badge(kind)` 五种徽章、`LeftBar`、`Gutter`…），调用方永远不碰裸 hex。

启动选择：`PickTheme(name)`（`cmd/dsc` 从 config 传入）→ `themeByID` + `detectTruecolor()`（看 `COLORTERM`；非真彩终端与 `ui.transparent_background` 都会让 `fillsEnabled()` 为假，全部背景填充退化为左竖条/分隔线）。

**运行时切换**是真实存在的：`/theme` → `handleSlash` 的 `case "/theme"` → `openThemes()` 先把当前主题存进 `a.themePreviewOrig`，再 `overlay.OpenThemes(a.theme.Name)` 打开可过滤的选择器；光标移动即**实时预览**，Esc 取消时用 `themePreviewOrig` 复原，Enter 提交走 `applyThemeSwitch(id)`——校验 id 在 `availableThemes()` 里、`setActiveTheme`、再经 `a.session.setTheme`（`Config.SetThemeFn`，由 cmd/dsc 接到 config 持久化）落盘，持久化失败不致命只提示。

分工：本节讲实现；palette 长什么样、配置键怎么写，见 [reference/tui-theme.md](../reference/tui-theme.md)。

## 5. 不变量与测试守卫

tui 包的测试分四个层级，改动时按由近及远跑：

1. **组件单测**（`completions_test.go`）：直接驱动 `completions` 状态机，不经 App。`sampleItems()` 固定横跨三种 `cmdKind` 的候选集；`TestViewAdaptiveHeightWhileFiltering` 钉死"打开最大→收缩贴合→零匹配为空"的尺寸契约，`TestViewCapsAtTenRowsAndScrolls` 钉 10 行窗口+滚动条，`TestSetMaxRowsClampsHeight` 钉天花板。
2. **键流集成测**（`keyflow_test.go` 提供 harness：`newKeyflowApp` / `sizeApp` / `press` / `drive` / `keyEscape`，把真实 `tea.KeyPressMsg` 喂进 `Update`）。`popup_height_test.go` 用它钉**帧锚定不变量**：`TestPopupOverlayKeepsFrameAnchored` 走完"打开→收窄→零匹配→Esc 关闭"全程，断言 header、transcript 顶、chrome/divider/status、输入框、hint 的每一行都纹丝不动；`TestPopupClampedOnShortTerminalKeepsInputVisible` 在 16 行终端上断言卡片被钳、输入提示符存活、header 不被盖。
3. **golden 渲染测**（`render_golden_test.go` + `testdata/render`、`testdata/render-ansi`）：宽度敏感的 `chatItem` 渲染跨多宽度钉死——渲染缓存按宽度取键，宽度相关的布局 bug 在缓存命中时会原样复现，所以必须 golden。
4. **快照 harness**（`qa_harness_test.go` 的 `QAFrame` / `CaptureScrollbackFrame`）：归一化尾部空白的 scrollback 快照，供回归比对。

值得记住的不变量：

- `completions.Lines()` 与 `View()` 同源于 `visibleRows()`，高度预留与实际渲染按构造一致；
- 弹层/Toast/权限卡的出现与消失**不得**移动帧上其他行（弹层靠浮层合成，chrome 行空时填空格占位，toast 在 `layout()` 里预留行）；
- 流式 delta 不直接触发渲染，只 bump `Seq()`，重绘统一走 80ms tick；
- 只有 `modeInsert` 的按键能到 textarea（违反它会复现"/clear 后 j/k 变成字面输入"的老 bug）；
- `commands_registry_test.go` 的 `TestBuiltinCommandsMirrorHandleSlash` 把 `builtinCommands()` 与 `handleSlash` 的 case 集钉成镜像——两边改一不改二会红。

## 6. 修改场景实操：加一个新 slash 命令（带补全项）

以新增内建命令 `/foo` 为例，动三处、跑两组测试：

1. **注册候选**：`internal/tui/commands_registry.go` 的 `builtinCommands()` 加一行：

   ```go
   {Name: "foo", Summary: "do the foo thing", Kind: builtinCmd},
   ```

   这一处同时喂 `/` 补全菜单和 `/help` overlay（两者都经 `allCommands` 合并，按构造不会漂移）。`Name` 不带斜杠；有别名就填 `Aliases`。

2. **实现分发**：`internal/tui/app.go` 的 `handleSlash` 加 `case "/foo":`。纯 UI 操作直接改状态 + `refreshView()`；要跑 agent 的话返回 `a.submitPromptCmd(...)`；要开 overlay 就调 `a.overlay.OpenXxx(...)`。

3. **更新镜像测试**：`internal/tui/commands_registry_test.go` 的 `TestBuiltinCommandsMirrorHandleSlash` 把 `"foo"` 加进 `wantNames`（计数断言会逼你来改这里）；有别名再补 `wantAliases`。

4. **验证**：

   ```sh
   go test ./internal/tui/ -run 'TestBuiltinCommands|TestAllCommands|Completion|Popup'
   go test ./internal/tui/
   ```

   补全行为本身（fuzzy 过滤、自适应高度、accept 拼接）不需要新测试——你只是往既有候选集加了一行数据。

顺带一提：**用户自定义命令不需要碰任何 Go 代码**——`.deepseek/command/*.md` 落盘即被 `Config.Commands` 装进 `customCmds`，自动进补全菜单（`customCmd` kind）；skill 同理经 `agent.Skills.List()` 进来并在 label 后缀 `(skill)`。只有需要内建分发逻辑的命令才走上面的三步。

## 快速索引

| 我想找 | 去哪 |
|---|---|
| 根模型 / Update / View | `app.go`：`App`、`Update`、`View` |
| agent 事件桥 | `app.go`：`pumpEvents`；`messages.go`：`agentEventMsg` |
| 事件→子模块分发 | `app.go`：`dispatchAgentEvent` |
| 流式节流 | `app.go`：`scheduleRedraw` / `ensureTick`；`scrollback.go`：`Seq` |
| 弹层自适应高度 | `completions.go`：`visibleRows` / `SetMaxRows`；`app.go`：`overlayPopup` / `layout` |
| 补全触发与接受 | `app.go`：`syncCompletions` / `triggerToken` / `acceptCompletion` |
| 主题定义与切换 | `theme.go`：`palette` / `buildTheme` / `PickTheme`；`app.go`：`applyThemeSwitch` |
| 帧锚定守卫 | `popup_height_test.go`；组件契约在 `completions_test.go` |
