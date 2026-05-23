# Tools

`deepseekcode` ships with 12 built-in tools. The model uses them via
OpenAI-style function calling.

| Tool | Class | Auto-allowed | Notes |
|------|-------|--------------|-------|
| `read_file` | read | yes | Returns content with `cat -n` line numbers. Optional `start_line`/`end_line` range. 2 MiB cap per file. |
| `write_file` | mutate | inside cwd | Atomic write (tempfile + rename). Snapshotted before write. |
| `edit_file` | mutate | inside cwd | String-replace with unique-match enforcement; `replace_all` opts in to multi-replace. Snapshotted. |
| `bash` | execute | per-pattern | Runs in user shell with 120s default timeout. Permission gate per `bashPattern` matcher. Destructive bash triggers the Duet validator. |
| `glob` | read | yes | Doublestar pattern (`**/*.go`). Skips `.git`, `node_modules`, `.venv`, etc. |
| `grep` | read | yes | Regex search. Shells out to `rg` when present; falls back to a stdlib walker. |
| `ls` | read | yes | Lists immediate children with `[d]`/`[f]`/`[l]` type indicators. |
| `todo_write` | bookkeeping | yes | Manages the session's structured plan. One `in_progress` item at a time. |
| `git_diff` | read | yes | Unified diff between refs or working tree. |
| `git_show` | read | yes | File content at a ref, or a commit's metadata + diff. |
| `git_blame` | read | yes | Reformatted per-line: `path:line SHA author date content`. |
| `git_log` | read | yes | Compact: `SHA date author: subject`. Default 20 commits. |

## Permission tiers

Read-only tools never prompt. Mutating tools auto-allow inside the
working directory; outside-cwd, `.git/`, `.env*`, or other secret-like
paths always prompt. Bash prompts per unique command pattern with
choices:

- `o` once
- `s` session
- `a` always (persists to `~/.deepseek/config.toml`)
- `d` deny

Override at startup:

- `dsc --yolo` — auto-approve everything (don't)
- `dsc --read-only` — block all mutate/execute tools
- `dsc --ask-all` — prompt for every tool

## Snapshots & /undo

Before each step that includes a mutating tool, affected files are
snapshotted under `.deepseek/snapshots/<sessionID>/<stepIdx>/`. `/undo`
restores the last step's snapshots; `/undo N` restores N steps.

Bash effects are not statically known, so bash does not auto-snapshot.
Use `git status` + the Duet validator to catch destructive bash output
before it happens.

## Extending: MCP

MCP (Model Context Protocol) servers are supported via config:

```toml
[mcp_servers.example]
command = "node"
args = ["/path/to/server.js"]
env = { FOO = "bar" }
```

Servers are spawned lazily on first tool-call referencing them. **None
are enabled by default** — the binary ships with the surface above.

> MCP integration ships partially in v0.1 — configuration is parsed but
> the runtime bridge is on the v0.2 roadmap. Until then, the built-in
> tool surface is the working set.
