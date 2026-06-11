# 进阶三件套：Skills、Subagents、Hooks

> **目标：** 各写一个最小可用例子——一个 SKILL.md、一个 subagent 定义、一个拦截危险操作的 hook。
>
> **前提：** 已完成[定制教程](customization.md)，了解 `.deepseek/` 目录结构。

---

## 1. 写第一个 SKILL.md

### 文件位置

`dsc` 按固定优先级在以下目录下扫描 `SKILL.md`（每个子目录内放一个）：

**项目级**（相对于 `$CWD`）：
1. `.deepseek/skills/<name>/SKILL.md`
2. `skills/<name>/SKILL.md`
3. `.opencode/skills/<name>/SKILL.md`
4. `.claude/skills/<name>/SKILL.md`
5. `.agents/skills/<name>/SKILL.md`

**Home 级**（相对于 `$HOME`）：同上五个路径，优先级低于项目级。

同名 skill 以第一个发现的为准（项目级 > home 级；`.deepseek` > `.claude`）。

完整说明见 [Skills 参考](../reference/skills.md)。

### 最小示例

在项目根目录创建 `.deepseek/skills/pr-summary/SKILL.md`：

```markdown
---
name: pr-summary
description: 生成一份简洁的 PR 摘要，列出变更点和测试建议
---
# PR 摘要

用中文回答。

1. 先调用 `git_diff` 获取与 main 的差异；
2. 列出变更文件和核心逻辑改动（不超过 5 条）；
3. 给出 2～3 条有针对性的测试建议。
```

frontmatter 规则（源自 `internal/skills/store.go`）：
- `name`：必填，用于索引和 slash 命令名；缺失时回退到目录名。
- `description`：可选，但**建议填写**——空 description 的 skill 不会出现在 `/` 补全列表。
- 其余可选字段：`run_mode`（默认 `direct`）、`allowed-tools`（逗号分隔）。

### 发现机制与 slash 命令自动升格

`dsc` 在**会话启动时**扫描一次，将 skill 元数据（名称、描述、路径）注入系统提示的静态前缀。发现后自动升格为 slash 命令：在 TUI 输入框输入 `/pr-summary` 即可触发。

> 注意：中途修改 SKILL.md 不会热更新——修改在**下一次会话**才生效。如需当前会话立即看到变更，可使用内置命令 `/reload-skills`。

用户自定义 slash 命令（`.deepseek/command/`）优先级高于同名 skill。

### 验证

```
# 启动 dsc 后，在输入框输入 /
# 补全列表中应出现 pr-summary
/pr-summary
```

若不出现，先确认目录结构正确（子目录名任意，但 SKILL.md 必须在子目录内），再确认 description 非空。

---

## 2. 定义一个 subagent

### 文件格式

Subagent 定义放在 `.deepseek/agent/<name>.md`，与 SKILL.md 格式类似——frontmatter 加 body（源自 `internal/agents/def.go`）：

```markdown
---
description: 只读代码探索专员，不写文件
mode: subagent
model: deepseek-v4-flash
tools: read_file, glob, grep, ls, git_diff, git_show, git_blame, git_log
---
你是一个只读代码探索专员。
你的任务是理解代码结构、梳理调用链，不允许写入任何文件。
回答时给出文件路径和行号作为依据。
```

frontmatter 可用字段（以代码为准）：

| 字段 | 说明 |
|------|------|
| `description` | 描述（选填） |
| `mode` | `subagent`（默认）或 `plan` |
| `model` | 指定模型；空 = 继承父级 |
| `tools` | 逗号分隔的工具白名单；空 = 继承父级全集 |
| `worktree` | `true` = 在独立 git worktree 中运行 |
| `max_steps` | 最大步数上限 |
| `omit_project_context` | `true` = 不向子 agent 传递 git diff 等项目上下文 |

### 内置 agent profile 一览

`internal/agents/def.go` 的 `DefaultProfiles()` 定义了五个开箱即用的 profile：

| Profile | 模型 | Reasoning | 用途 |
|---------|------|-----------|------|
| `coding-default` | flash | medium | 默认主循环 |
| `explore` | flash | low | 只读探索（read/glob/grep/git/web） |
| `implement` | flash | high | 代码写入（read/write/edit/bash/git） |
| `review` | flash | medium | 代码审查（只读 + git） |
| `autonomous` | flash | high | 完全自主（全工具，auto-approve，可派生子 agent） |

### 何时用自定义 subagent

- 需要把某个专项任务（纯探索、纯审查）的工具集限定在最小集合；
- 需要固定某个子任务使用特定模型或 reasoning 强度；
- 需要在独立 worktree 里安全地进行破坏性实验（`worktree: true`）。

---

## 3. 用 hook 拦一次危险操作

### 机制简介

`dsc` 支持五类生命周期事件（`docs/reference/hooks.md`）：

| 事件 | 触发时机 |
|------|---------|
| `PreToolUse` | 权限检查通过后、工具执行前 |
| `PostToolUse` | 工具成功执行后 |
| `PostToolUseFailure` | 工具执行出错后 |
| `SessionStart` | agent 会话创建时 |
| `SessionEnd` | agent 循环退出时 |

Hook 通过 stdin/stdout 以 JSON 交互，返回 `allow`、`deny`、`ask` 或 `continue` 之一。

**Fail-open**：hook 崩溃、超时或输出非法 JSON 时，一律视作 `continue`——工具照常执行，不会因 hook 故障而卡死流程。

### 完整示例：拦截 `rm -rf`

在 `.deepseek/config.toml` 中添加：

```toml
[[hooks]]
event = "PreToolUse"
type = "subprocess"
timeout_seconds = 5
command = '''
  python3 -c "
import json, sys
inp = json.load(sys.stdin)
if inp.get('tool_name') == 'bash':
    import json as j
    args = j.loads(inp.get('tool_input', '{}')) if isinstance(inp.get('tool_input'), str) else inp.get('tool_input', {})
    cmd = args.get('command', '')
    if 'rm -rf /' in cmd or 'mkfs' in cmd:
        print(j.dumps({'decision': 'deny', 'reason': '拦截：检测到高危删除命令'}))
        sys.exit(0)
print(j.dumps({'decision': 'allow'}))
"
'''
```

hook 接收的 JSON（`HookInput`）结构：

```json
{
  "event": "PreToolUse",
  "tool_name": "bash",
  "tool_input": {"command": "rm -rf /tmp/test"},
  "cwd": "/home/user/project",
  "session_id": "abc123..."
}
```

hook 输出的 JSON（`HookOutput`）结构：

```json
{
  "decision": "deny",
  "reason": "拦截：检测到高危删除命令"
}
```

完整 hook 配置语法见 [Hooks 参考](../reference/hooks.md)。

### 验证

1. 保存 `.deepseek/config.toml` 后重启 `dsc`；
2. 在会话中提示模型执行 `rm -rf /some/path`；
3. 若 hook 生效，工具调用会被 `deny` 并显示拦截原因，模型收到拒绝信号后不会继续执行该命令。

---

## 下一步

- [外部集成：MCP、LSP、CodeGraph](integrations.md)
