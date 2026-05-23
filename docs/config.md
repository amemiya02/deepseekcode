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
```

## CLI flags

```
-version              print version and exit
-yolo                 auto-approve all tool calls (DANGEROUS)
-read-only            block all write/edit/bash tools
-ask-all              prompt for every tool, ignoring allowlist
-no-duet              disable the Two-Model Duet (Pro Validator)
-model <id>           override main-loop model for this session
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
