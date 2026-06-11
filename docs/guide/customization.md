# 定制：让 dsc 长成你的形状

> **目标：** 写出第一个自定义 slash 命令，了解 `.deepseek/` 目录的完整结构，配好通知。
>
> **前提：** 已完成[配置教程](configuration.md)，理解四层配置叠加模型。

---

## 1. `.deepseek/` 目录全景

`dsc` 在项目根目录下识别一个 `.deepseek/` 子目录，用于存放所有项目级定制内容：

```
.deepseek/
├── config.toml          # 项目级配置（覆盖 ~/.deepseek/config.toml）
├── command/             # 自定义 slash 命令（每个 .md 文件 = 一个命令）
├── agent/               # Agent 定义（每个 .md 文件 = 一个 agent）
└── skills/              # Skill 文件（SKILL.md 发现规范）
```

各子目录一句话说明：

| 目录 | 用途 | 参考文档 |
|------|------|---------|
| `command/` | 自定义 slash 命令；`/commit`、`/review` 之类的快捷模板 | [命令参考](../reference/commands.md) |
| `agent/` | Agent 定义；通过 `dsc agent` 子命令管理，当前 frontmatter 路由字段尚未生效 | [命令参考](../reference/commands.md)（Managing agents 节） |
| `skills/` | Skill 文件；自动升格为 slash 命令，命令优先级高于同名 skill | [Skills 参考](../reference/skills.md) |

> `.deepseek/` 下的 `.gitignore` 由 `dsc` 自动写入，确保会话快照和指针文件不污染 `git status`。

---

## 2. 实战：写一个 `/commit` 命令

### 创建命令文件

在项目根目录下创建文件 `.deepseek/command/commit.md`：

```markdown
---
description: stage all, commit, and push
model: deepseek-v4-pro
---
commit and push with a conventional commit message
## DIFF
!`git diff --cached`
## MESSAGE
$ARGUMENTS
```

这个文件与 [命令参考](../reference/commands.md) 中的完整示例完全一致。

### 逐行解释插值语法

```
description: stage all, commit, and push
```
命令描述，出现在 TUI 补全提示中（`/` 后输入时可见）。**当前生效。**

```
model: deepseek-v4-pro
```
执行此命令时临时切换到 `deepseek-v4-pro`。本次命令结束后恢复原模型。**当前生效。**

```
!`git diff --cached`
```
Shell 注入：执行 `git diff --cached`，将输出原地替换插入提示词。有 10 秒超时；失败时替换为 `[command failed: ...]`。

```
$ARGUMENTS
```
将你在 `/commit` 后面键入的所有内容原样插入。例如输入 `/commit fix: resolve auth bug`，`$ARGUMENTS` 就替换为 `fix: resolve auth bug`。

### 生效字段与未生效字段

| 字段 | 当前生效 | 说明 |
|------|---------|------|
| `description` | **是** | 补全提示文字 |
| `model` | **是** | 临时切换模型 |
| `agent` | **否**（Phase 21） | 指定 agent，尚未路由 |
| `subtask` | **否**（Phase 21） | 作为子任务运行，尚未实现 |

> `agent` 和 `subtask` 字段写了不会报错，也不会生效——`dsc` 会静默忽略未知字段。

### 验证

保存文件后，在当前项目目录启动 `dsc`（或已启动的会话中），在输入框输入：

```
/commit fix: 修复空请求体返回 500 的问题
```

`dsc` 会：
1. 执行 `git diff --cached` 获取暂存区 diff
2. 拼接 `$ARGUMENTS` 作为 commit 信息提示
3. 临时切换到 `deepseek-v4-pro` 生成一条 conventional commit message 并提交

### 命令加载优先级

- 同时扫描 `$CWD/.deepseek/command/` 和 `$HOME/.deepseek/command/`
- 同名命令：**项目级优先**，home 版本被跳过
- 内置命令（`/help`、`/clear`、`/quit`、`/models` 等）不可被覆盖

---

## 3. 通知

`dsc` 内置通知接口，在以下两类事件时触发：

| 事件 | 通知标题 | 通知内容 |
|------|---------|---------|
| Agent 任务完成 | DeepSeekCode | Task finished |
| 权限提示出现 | DeepSeekCode | Permission requested |

通知**从不**包含文件路径、命令输出、模型回答或任何凭证——只发送以上固定的标题与内容。

**默认行为：** `dsc` 使用空操作（no-op）通知器，所有通知静默丢弃。无需配置即可"禁用"通知——它们本来就是关闭的。

**启用通知：** 需要实现 `Notifier` 接口并注入自定义实现。适合长任务（如大规模重构、批量 CI 流水线）完成后收到桌面或终端提醒的场景。平台参考：

- **macOS**：Terminal.app / iTerm2 支持 OSC 9 终端通知；也可包装 `terminal-notifier`
- **Linux**：`notify-send` 在多数桌面环境可用
- **Windows**：Windows Terminal 支持未聚焦标签页的 toast 通知

> 完整通知接口说明见 [通知参考](../reference/notifications.md)。

---

## 下一步

- [进阶三件套：Skills、Subagents、Hooks](skills-agents-hooks.md)
