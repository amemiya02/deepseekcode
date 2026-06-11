# 配置：从默认值到项目级定制

> **目标：** 建立分层配置的心智模型，能够写出符合自己需求的 `config.toml`，理解模型选择与 effort 的含义，知道如何切换界面语言与主题。
>
> **前提：** 已完成[日常工作流](core-workflow.md)，`dsc` 能正常启动。

---

## 1. 四层叠加

`dsc` 的配置由四层按优先级从低到高依次叠加：

```
内置默认值
    ↓（被覆盖）
~/.deepseek/config.toml      ← 用户级，对所有项目生效
    ↓（被覆盖）
./.deepseek/config.toml      ← 项目级，仅对当前目录生效（可选）
    ↓（被覆盖）
CLI flags（如 -model、-effort）← 本次启动生效，不持久化
```

**具体演示：** 以 `defaults.reasoning_effort` 为例。

内置默认值是 `"max"`。假设你在 `~/.deepseek/config.toml` 写了：

```toml
[defaults]
reasoning_effort = "high"
```

在某个对延迟敏感的项目里，`./.deepseek/config.toml` 进一步写：

```toml
[defaults]
reasoning_effort = "low"
```

最终对该项目生效的是 `"low"`。如果你在启动时附加 `-effort medium`，本次会话实际使用 `"medium"`——但下次启动仍恢复 `"low"`（CLI flag 不持久化）。

> 完整分层说明见 [配置参考](../reference/config.md)。

---

## 2. 最小可用配置

以下是开始一个新项目前，你最可能需要的 `~/.deepseek/config.toml` 骨架：

```toml
[api]
key = "${DEEPSEEK_API_KEY}"          # 从环境变量读取；也可以直接写 key 字符串
base_url = "https://api.deepseek.com"

[defaults]
model = "deepseek-v4-flash"          # 默认模型；可改为 deepseek-v4-pro
thinking = true                      # 开启推理模式
reasoning_effort = "max"             # low | medium | high | max
theme = "dark"                       # dark | light
vim_keybindings = true               # 不喜欢 Vim 键位可改为 false
```

这个配置与内置默认完全一致——它的意义在于**让你有一个显式的起点**，改动时知道自己在改哪一层。

> - 查看所有可用配置项：[配置参考](../reference/config.md)
> - 了解 provider 接入选项：[Provider 矩阵](../reference/providers.md)

---

## 3. 模型与 effort

### 可用模型

`dsc` 支持两个官方 DeepSeek 模型：

| 模型 | 说明 |
|------|------|
| `deepseek-v4-flash`（默认） | 速度更快，适合日常改 bug、代码审查、快速问答 |
| `deepseek-v4-pro` | 推理更深，适合架构设计、复杂重构、多文件协同任务 |

两个模型均支持 100 万 token 上下文窗口。

在 `config.toml` 里设置默认模型：

```toml
[defaults]
model = "deepseek-v4-flash"
```

### effort 级别

`reasoning_effort` 控制 DeepSeek 推理模式的深度，影响响应速度与推理质量的权衡：

| 值 | 适用场景 |
|----|---------|
| `low` | 简单问答、格式转换、单行修改 |
| `medium` | 普通 bug 修复、代码解释 |
| `high` | 跨文件重构、调试复杂逻辑 |
| `max`（默认）| 架构设计、安全审查、高风险变更 |

### TUI 内即时切换

不需要重启 `dsc` 也能切换——在 TUI 输入框中直接输入：

```
/models              # 弹出模型列表，选中即切换
/models deepseek-v4-pro   # 直接切换到指定模型

/effort              # 显示当前 effort 并弹出选择
/effort low          # 直接设置为 low
```

切换仅对当前会话生效；下次启动仍使用 `config.toml` 里的值。

### `auto_reasoning`（可选）

如果你希望 `dsc` 根据消息内容自动决定是否开启推理，可以启用：

```toml
[defaults]
auto_reasoning = true
```

启用后，含有"debug"、"调试"、"error"等关键词的消息自动开启 thinking；含有"搜索"、"查找"等低强度关键词时自动关闭。未命中关键词则沿用 `thinking` 字段的默认值。

---

## 4. 界面语言与主题

### 界面语言

通过环境变量控制 TUI 显示语言：

```sh
export DEEPSEEKCODE_LANG=zh-CN   # 中文
export DEEPSEEKCODE_LANG=en      # 英文
```

建议写入 `~/.zshrc` 或 `~/.bashrc` 使其持久生效。未设置时，`dsc` 会尝试回退到系统 `LANG` 环境变量。

### 主题切换

TUI 默认使用深色主题（`dark`）。有两种方式切换：

**方式一：** 在 TUI 内即时切换（当前会话生效）

```
/theme
```

弹出主题选择面板，选中即切换。

**方式二：** 在 `config.toml` 中持久化

```toml
[defaults]
theme = "light"   # dark | light
```

> 主题系统的调色板、渐进着色规则及扩展指南见 [TUI 主题参考](../reference/tui-theme.md)。

---

## 下一步

- [定制：让 dsc 长成你的形状](customization.md) — 自定义 slash 命令、`.deepseek/` 目录全景、通知配置
