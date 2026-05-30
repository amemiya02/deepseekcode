# Configuration

`deepseekcode` reads TOML config from two locations, layered in this
order (later wins):

1. Built-in defaults
2. `~/.deepseek/config.toml` (user-wide)
3. `./.deepseek/config.toml` (project-local, optional)
4. CLI flags

Environment variables referenced as `${VAR}` inside string values are
expanded at load time. If `[api].key` is empty, `DEEPSEEK_API_KEY` is
used as a fallback.

## Example

```toml
# ~/.deepseek/config.toml

[api]
key = "${DEEPSEEK_API_KEY}"
base_url = "https://api.deepseek.com"
first_token_timeout_ms = 45000
chunk_stall_timeout_ms = 20000

[defaults]
model = "deepseek-v4-flash"
thinking = true
reasoning_effort = "max"        # low | medium | high | max
theme = "dark"                  # dark | light
vim_keybindings = true

[duet]
enabled = true
retry_on_failure = true
validator_timeout_ms = 10000
extra_destructive_patterns = [
  # additional regexes — built-ins always apply
  # "^my-deploy-script",
]

[permissions]
# bash patterns auto-allowed (you can append to this by pressing 'a'
# during a permission prompt — the entry is added here).
allow_bash = [
  "git status *",
  "git log *",
  "git diff *",
  "ls *",
  "pwd",
  "cat *",
]
# extra path patterns considered "secret-like" (always prompt to write)
secret_path_patterns = [
  "*.pem",
  "*.key",
  "id_rsa*",
  ".env*",
]

[sessions]
ttl_days = 90
snapshot_keep = 30
auto_resume_age_hours = 24

# MCP servers are NOT enabled by default. Add an entry to opt in.
# [mcp_servers.example]
# command = "node"
# args = ["/path/to/server.js"]
# env = { FOO = "bar" }
# timeout_seconds = 60

[tools]
max_read_bytes = 5_242_880   # 5 MiB
max_write_bytes = 5_242_880  # 5 MiB

# Permission rules: evaluated before the tiered defaults.
# Decision order: deny > ask > allow > tiered default.
# [permissions.rules]
# allow = [{ tool = "read_file", args = ".*" }]
# deny = [{ tool = "bash", args = "rm\\s+-rf" }]
# ask = [{ tool = "write_file", args = ".*secret.*" }]

# Hooks: run shell commands or builtins on agent events.
# [[hooks]]
# event = "PreToolUse"
# type = "subprocess"
# command = "echo '{\"decision\":\"allow\"}'"
# timeout_seconds = 10

# [[hooks]]
# event = "PostToolUse"
# type = "builtin"
# name = "duet"
```

## CLI flags

```
-version              print version and exit
-yolo                 auto-approve all tool calls (DANGEROUS)
-read-only            block all write/edit/bash tools
-ask-all              prompt for every tool, ignoring allowlist
-no-duet              disable the Two-Model Duet (Pro Validator)
-model <id>           override main-loop model for this session
-effort <level>       override reasoning effort (low|medium|high|max)
-new                  force new session, even if a recent one exists
-c                    continue last session in cwd
-r <id>               resume session by ID
-p "prompt"           one-shot: send prompt to the model, print result, exit
```

## Slash commands

Run inside the TUI:

```
/help        keymap + commands
/clear       clear scrollback
/quit        exit
/models      list / switch the main-loop model (also: /models <id>)
/effort      show / set DeepSeek reasoning effort (also: /effort <level>)
/tape        open the Reasoning Tape fullscreen view
/sessions    list this project's sessions, switch to one
/undo        revert the last edit step (also: /undo N)
```

## Storage paths

- `~/.deepseek/config.toml` — user config (this file)
- `~/.deepseek/sessions.db` — global SQLite session store
- `./.deepseek/last_session` — pointer for `dsc -c`
- `./.deepseek/snapshots/<sessionID>/<stepIdx>/` — pre-edit snapshots
- `./.deepseek/.gitignore` — auto-written so `git status` stays clean

## Field reference

### `[api]`

| Field | Type | Default | Description |
|---|---|---|---|
| `key` | string | `""` | API key; falls back to `DEEPSEEK_API_KEY` env var |
| `base_url` | string | `"https://api.deepseek.com"` | API endpoint |
| `user_id` | string | `""` | Optional DeepSeek user_id for abuse monitoring / enterprise attribution |
| `first_token_timeout_ms` | int | `45000` | Timeout for first token (reasoner cold start) |
| `chunk_stall_timeout_ms` | int | `20000` | Timeout between streaming chunks |

### `[defaults]`

| Field | Type | Default | Description |
|---|---|---|---|
| `model` | string | `"deepseek-v4-flash"` | Main-loop model |
| `thinking` | bool | `true` | Enable reasoning/thinking mode |
| `reasoning_effort` | string | `"max"` | DeepSeek reasoning effort: `low`, `medium`, `high`, `max` |
| `theme` | string | `"dark"` | TUI theme |
| `vim_keybindings` | bool | `true` | Enable Vim key bindings |
| `auto_reasoning` | bool | `false` | Per-turn thinking selection based on message keywords (opt-in) |

When `auto_reasoning` is enabled, the agent inspects the user's most
recent message each turn and decides whether to enable thinking:

- **High-effort keywords** (thinking ON): `debug`, `error`, `调试`,
  `错误`, `报错`, `出错`, `崩溃`, `調試`, `錯誤`, `デバッグ`, `エラー`, `バグ`
- **Low-effort keywords** (thinking OFF): `search`, `lookup`, `搜索`,
  `查找`, `查询`, `検索`
- **No match**: uses the `thinking` default value

Matching is case-insensitive for ASCII; CJK matched verbatim. High
keywords take priority over low. Note: `bug` alone does NOT trigger
high-effort (only `debug` / `error` and the CJK equivalents do).

### `[tools]`

| Field | Type | Default | Description |
|---|---|---|---|
| `max_read_bytes` | int64 | `5242880` (5 MiB) | Max bytes for read_file |
| `max_write_bytes` | int64 | `5242880` (5 MiB) | Max bytes for write_file |

### `[duet]`

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `true` | Enable Two-Model Duet (Pro validator) |
| `retry_on_failure` | bool | `true` | Retry on Pro validation failure |
| `validator_timeout_ms` | int | `10000` | Pro validation timeout |
| `extra_destructive_patterns` | []string | `[]` | Additional destructive command patterns |

### `[permissions]`

| Field | Type | Default | Description |
|---|---|---|---|
| `allow_bash` | []string | git/ls/pwd/cat etc. | Bash auto-allow patterns |
| `secret_path_patterns` | []string | `*.pem`, `*.key` etc. | Sensitive path patterns |

### `[permissions.rules]`

Each rule item has `tool` (glob pattern, e.g. `*`, `read_*`) and `args` (regex on JSON string).

| List | Decision | Priority |
|---|---|---|
| `allow` | Auto-allow | Lowest |
| `ask` | Prompt user | Medium |
| `deny` | Auto-deny | Highest |

### `[sessions]`

| Field | Type | Default | Description |
|---|---|---|---|
| `ttl_days` | int | `90` | Session retention days |
| `snapshot_keep` | int | `30` | Snapshots to keep |
| `auto_resume_age_hours` | int | `24` | Auto-resume window |

### `[[hooks]]`

| Field | Type | Default | Description |
|---|---|---|---|
| `event` | string | required | `PreToolUse` / `PostToolUse` / `PostToolUseFailure` / `SessionStart` / `SessionEnd` |
| `type` | string | `"subprocess"` | `subprocess` / `builtin` |
| `command` | string | — | Shell command (subprocess type) |
| `name` | string | — | Hook name (builtin type, e.g. `duet`) |
| `timeout_seconds` | int | `30` | Hook timeout |

### `[mcp_servers.<name>]`

| Field | Type | Default | Description |
|---|---|---|---|
| `command` | string | required | Command to start the MCP server |
| `args` | []string | `[]` | Command arguments |
| `env` | map | `{}` | Environment variables |
| `timeout_seconds` | int | `60` | tools/call timeout |

## Validation rules

`dsc` runs `ValidateStrict` at startup. The following cause exit with error:

- `api.first_token_timeout_ms` must be > 0
- `api.chunk_stall_timeout_ms` must be > 0
- MCP server name must not be empty
- MCP server `command` must not be empty
- Hook `event` must be a known enum value
- Hook `type` must be `subprocess` or `builtin`
- Builtin hooks must have a `name`
- Subprocess hooks must have a `command`
- Permission rule `tool` must not be empty

- `~/.deepseek/config.toml` — user config (this file)
- `~/.deepseek/sessions.db` — global SQLite session store
- `./.deepseek/last_session` — pointer for `dsc -c`
- `./.deepseek/snapshots/<sessionID>/<stepIdx>/` — pre-edit snapshots
- `./.deepseek/.gitignore` — auto-written so `git status` stays clean

## Managing agents

Agent definitions live in `.deepseek/agent/*.md`. The `dsc agent` subcommand
suite manages them from the CLI:

- `dsc agent list` — list all agents (tab-separated name + description), excluding hidden agents.
- `dsc agent show NAME` — print the raw `.deepseek/agent/NAME.md` file.
- `dsc agent new NAME` — scaffold a new agent definition with a starter template.
- `dsc agent validate` — parse all agent definitions and report errors.
