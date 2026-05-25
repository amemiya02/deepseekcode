# Bubble Tea v2 API Notes

> 实测结论，来自 `charm.land/bubbletea/v2@v2.0.6`、`charm.land/lipgloss/v2@v2.0.3`、`charm.land/bubbles/v2@v2.1.0`、`charm.land/glamour/v2@v2.0.0`。探针测试：`internal/tui/v2probe_test.go`。

## KeyPressMsg.String

`tea.KeyPressMsg.String()` 存在，委托给 `Key.String()`（内部走 `uv.Key`）。返回格式与 v1 兼容：

- `"enter"`, `"esc"`, `"tab"`, `"space"`
- `"ctrl+r"`, `"ctrl+c"`, `"ctrl+d"`, `"ctrl+t"`, `"ctrl+u"`, `"ctrl+f"`
- `"shift+enter"`, `"alt+enter"`
- 单字符可打印键返回该字符：`"a"`, `"j"`, `"k"` 等

**结论：走 D13 方案（保留 `.String()` 匹配），不需要 `key.Matches` 回退。**

## tea.View 字段

```go
type View struct {
    Content         string
    OnMouse         func(msg MouseMsg) Cmd
    Cursor          *Cursor
    BackgroundColor color.Color
    ForegroundColor color.Color
    WindowTitle     string
    ProgressBar     *ProgressBar
    AltScreen       bool
    MouseMode       MouseMode
}
```

- `Cursor` 是 `*tea.Cursor`，通过 `tea.NewCursor(x, y)` 构造
- `tea.Cursor` 含 `Position{X,Y}`、`Color`、`Shape`、`Blink`
- `AltScreen` 替代 v1 的 `tea.WithAltScreen()` 选项
- `MouseMode` 替代 v1 的 `tea.WithMouseCellMotion()` 选项
- `MouseModeCellMotion` = 点击/释放/滚轮；`MouseModeAllMotion` = 含移动

## Mouse 类型名

四个类型（均为 `Mouse` 的类型别名）：

| v2 类型 | 说明 |
|---------|------|
| `tea.MouseClickMsg` | 按钮按下 |
| `tea.MouseReleaseMsg` | 按钮释放 |
| `tea.MouseMotionMsg` | 移动（需 `MouseModeAllMotion`） |
| `tea.MouseWheelMsg` | 滚轮 |

每个类型都有 `.Mouse() Mouse` 方法和 `.String()` 方法。`Mouse` 结构含 `X`, `Y`, `Button`, `Action` 等字段。

## Blend1D 签名

```go
func Blend1D(steps int, stops ...color.Color) []color.Color
```

注意：v2 签名改为 **variadic** `stops`（v1 是固定两个参数）。调用方式不变：`lipgloss.Blend1D(3, c1, c2)`。

## ExecProcess

```go
func ExecProcess(c *exec.Cmd, fn ExecCallback) Cmd
type ExecCallback func(error) Msg
```

与 v1 完全相同，无需适配。

## textarea/v2

- `textarea.New()` 返回 `Model`
- `Cursor() *tea.Cursor` 方法存在（用于暴露给 `tea.View.Cursor`）
- `Focus() tea.Cmd`、`Value() string`、`SetValue(string)`、`Reset()` 不变
- `SetWidth(int)`、`SetHeight(int)`、`Height() int` 方法存在
- 字段：`ShowLineNumbers`、`CharLimit`、`Prompt`、`Placeholder`

## viewport/v2

- `viewport.New(opts ...Option)` — 用 `WithWidth(w)` / `WithHeight(h)` 选项构造
- `SetWidth(int)` / `SetHeight(int)` 方法
- `SetContent(string)` 方法
- `YOffset() int` / `SetYOffset(int)` 方法
- `AtBottom() bool` 方法
- `GotoTop()` / `GotoBottom()` 方法
- **滚动方法改名**：`LineUp`/`LineDown` → `ScrollUp(n int)` / `ScrollDown(n int)`
- `HalfViewUp`/`HalfViewDown` → `HalfPageUp()` / `HalfPageDown()`
- `PageUp()` / `PageDown()` 方法
- `MouseWheelEnabled` 字段保留

## key/v2

- `key.NewBinding(opts ...BindingOpt) Binding` — 签名不变
- `key.WithKeys(keys ...string) BindingOpt` — 不变
- `key.WithHelp(key, desc string) BindingOpt` — 不变

## lipgloss/v2

- `lipgloss.Color`、`NewStyle`、`Border`、`RoundedBorder`、`JoinVertical`、`JoinHorizontal`、`Width`、`MaxHeight`、`Render`、`GetForeground` — 全部兼容
- 新增 `Blend1D(steps int, stops ...color.Color) []color.Color`
