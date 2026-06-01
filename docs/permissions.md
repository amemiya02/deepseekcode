# Permission Rules Engine

deepseekcode 的权限决策遵循固定优先级。理解这个顺序能帮助你配置更精确的安全策略。

## 决策顺序

每个工具调用按以下顺序评估，第一个匹配的决策生效：

```
1. Global Mode      (--yolo / --read-only / --ask-all CLI flags) — absolute override
2. Rule Engine      ([permissions.rules] in config.toml) — only active in ModeDefault
3. Tool requirements (MinModeFor) — hard gate for tools needing higher privilege
4. Tiered defaults  (read-only auto-allow, bash allowlist, cwd/safety checks)
```

### 1. Global Mode

CLI flags 覆盖所有其他逻辑：

| Flag | 效果 |
|------|------|
| `--yolo` | 全部自动允许 |
| `--read-only` | 只读工具自动允许，写入/执行类工具全部拒绝 |
| `--ask-all` | 每个工具调用都弹出确认提示 |

### 2. Rule Engine

规则在 `config.toml` 的 `[permissions.rules]` 段定义。规则按 **Deny → Ask → Allow** 的顺序检查，第一个匹配的规则生效。

```toml
[permissions.rules]
deny = [
  { tool = "bash", args = "rm\\s+-rf" },
  { tool = "write_file", args = "/etc/" },
]
ask = [
  { tool = "bash", args = ".*" },
]
allow = [
  { tool = "read_file", args = ".*" },
  { tool = "grep", args = ".*" },
  { tool = "glob", args = ".*" },
  { tool = "ls", args = ".*" },
]
```

- `tool` 字段支持精确名称、`*` 通配、或 shell glob 模式（如 `read_*`）
- `args` 字段是正则表达式，匹配工具参数的 JSON 字符串。留空匹配所有参数
- Deny 优先级最高 — 即使有匹配的 Allow 规则，Deny 匹配也会先执行

### 3. Tool Requirements

`MinModeFor(toolName)` 定义每个工具所需的最低权限级别。目前所有内置工具默认为 `ModeDefault`，即被 mode flags 和 rules 控制。

### 4. Tiered Defaults

当没有规则匹配且没有 mode flag 覆盖时，使用分层默认策略：

- **只读工具**（read_file, grep, glob, ls, git_diff, git_show, git_blame, git_log）→ 自动允许
- **文件写入工具**（write_file, edit_file）→ 检查路径是否在 cwd 内且非敏感路径
- **bash** → 按 bash allowlist 模式匹配；未匹配则 Ask

## 完整配置示例

```toml
[permissions]
allow_bash = [
  "git status *",
  "git log *",
  "git diff *",
  "ls *",
  "pwd",
  "cat *",
]
secret_path_patterns = [
  "*.pem",
  "*.key",
  "id_rsa*",
  ".env*",
]

[permissions.rules]
deny = [
  { tool = "bash", args = "rm\\s+-rf" },
  { tool = "bash", args = "sudo" },
  { tool = "write_file", args = "/etc/" },
]
ask = [
  { tool = "bash", args = ".*" },
]
allow = [
  { tool = "read_file", args = ".*" },
  { tool = "grep", args = ".*" },
  { tool = "glob", args = ".*" },
]
```

当规则触发时，`dsc doctor` 会显示已加载的规则数量，TUI 中会展示拒绝原因。

## `/permissions` Overlay

Type `/permissions` in the TUI to open a read-only overlay showing the
effective policy at a glance: current mode, bash allowlist size, secret
pattern count, and whether the rule engine is active.

```
/// /permissions
  Mode ............. default
  Bash allowlist ... 6 patterns
  Secret patterns .. 4 patterns
  Rule engine ...... active
```

The overlay is informational — it does not allow editing the policy. Close
with `esc`.

## Doctor 输出

```
deepseekcode doctor
──────────────────────────────────────────────────
  ✓ permission rules    3 allow, 2 deny, 1 ask
```
