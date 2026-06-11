# Custom Slash Commands (`.deepseek/command/*.md`)

dsc 支持用户自定义 slash 命令。在项目或 home 目录下创建 `.deepseek/command/*.md` 文件，
即可在 TUI 中用 `/命令名` 触发。

## 命令文件格式

```markdown
---
description: short description of the command
model: deepseek-v4-pro
agent: planner
subtask: true
---
Your template here. Use $1, $2 for positional args.
Use $ARGUMENTS for the full argument string.
Use !`shell command` for shell injection.
Use @path/to/file for file content injection.
```

### Frontmatter 字段

| 字段 | 类型 | 说明 | 当前生效 |
|---|---|---|---|
| `description` | string | 命令描述（用于补全提示） | 是 |
| `model` | string | 执行时临时切换的模型 | **是** |
| `agent` | string | 指定 agent（未生效） | 否（Phase 21） |
| `subtask` | bool | 是否作为子任务运行 | 否（Phase 21） |

- frontmatter 是可选的。无 `---` 分隔符时，整个文件内容作为模板。
- 只识别以上四个 key；未知 key 被忽略。
- value 可带或不带引号（`model: "deepseek-v4-pro"` 或 `model: deepseek-v4-pro`）。

## 插值语法

按以下顺序处理：

| 语法 | 说明 | 示例 |
|---|---|---|
| `$1`, `$2`, … | 位置参数（从 `$1` 开始） | `/fix bug auth.go` → `$1=bug`, `$2=auth.go` |
| `$N` 吸收 | 最高位占位符吸收其后所有剩余参数（空格连接） | `/review a b c` → `$1="a b c"` |
| `$ARGUMENTS` | 原始参数串 | `/run x y` → `$ARGUMENTS="x y"` |
| 无占位符 | 若模板无 `$N` 也无 `$ARGUMENTS` 且有参数，追加到新行 | 模板 `summarize` + 参数 `pkg/x` → `summarize\npkg/x` |
| `` !`cmd` `` | 执行 shell 命令，替换为 stdout | `` !`git diff` `` → diff 输出 |
| `@path` | 读取文件内容替换 | `@README.md` → 文件内容 |

- shell 命令有 10 秒超时；失败时替换为 `[command failed: ...]`。
- `@path` 文件不存在时保留原样 `@path`，不报错。

## 命令名推导

- 命令名由相对 `.deepseek/command/` 的路径去掉 `.md` 后缀得到。
- 分隔符统一为 `/`：`.deepseek/command/git/sync.md` → 命令名 `git/sync`。
- 命令名区分大小写。

## 加载优先级

- 扫描两个目录：`$CWD/.deepseek/command` 和 `$HOME/.deepseek/command`。
- 同名命令：**cwd 优先**，home 版本被跳过。
- 文件超过 64 KiB 自动跳过。
- 缺少目录不报错。

## 完整示例

`.deepseek/command/commit.md`：

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

使用：在 TUI 中输入 `/commit fix: resolve auth bug`。

## 内置命令

以下内置命令不可被覆盖：`/help`, `/clear`, `/quit`, `/models`, `/tape`, `/sessions`,
`/export`, `/undo`, `/compact`。
