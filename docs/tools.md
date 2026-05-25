# Tools

`deepseekcode` ships with 14 built-in tools. The model uses them via
OpenAI-style function calling.

| Tool | Class | Auto-allowed | Notes |
|------|-------|--------------|-------|
| `read_file` | read | yes | Returns content with `cat -n` line numbers. Optional `start_line`/`end_line` range. 2 MiB cap per file. |
| `write_file` | mutate | inside cwd | Atomic write (tempfile + rename). Snapshotted before write. |
| `edit_file` | mutate | inside cwd | String-replace with unique-match enforcement; `replace_all` opts in to multi-replace. Fuzzy hint when exact match fails (≤3 line-level edits). Snapshotted. |
| `apply_patch` | mutate | inside cwd | Multi-hunk patch in `*** Begin Patch` / `*** End Patch` envelope. Add, Update (4-pass fuzzy match), Delete, Move. Replaces multiple `edit_file` calls. Snapshotted. |
| `question` | read | yes | Asks the user one or more questions with selectable options. Single or multi-select. Blocks until answered. |
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

## Fuzzy matching in edit_file

When `edit_file` cannot find an exact match for `old_string`, it
attempts a fuzzy fallback using line-level Levenshtein distance:

1. The file is split into lines. A sliding window of the same line count
   as `old_string` is moved across the file.
2. Each window is compared to `old_string` using
   `LevenshteinLines` (standard DP, O(n×m) on line tokens).
3. If the closest window is within **3 line edits** (insert, delete, or
   substitute), `edit_file` returns an error hint:

   ```
   old_string not found in path. Closest match at line N (distance D): "preview..."
   ```

4. If the closest match exceeds 3 edits, the plain "not found" error is
   returned instead.

The fuzzy hint **never auto-applies** the change. The model (or user)
must retry with the correct `old_string`. This prevents silent
mis-edits while still providing actionable feedback on near-matches.

## apply_patch envelope format

`apply_patch` uses a multi-hunk envelope format (aligned with the
opencode/Codex convention) that lets the model express several file
operations in a single tool call:

```
*** Begin Patch
*** Add File: path/new.go
+line1
+line2
*** Update File: path/existing.go
*** Move to: path/renamed.go        (optional)
@@ optional context header
 context line
-removed line
+added line
*** Delete File: path/old.go
*** End Patch
```

**When to use `apply_patch` vs `edit_file`:**

- Use `apply_patch` when making changes across **multiple files** or when
  the model naturally generates a patch-like diff. One call replaces
  several `edit_file` invocations, reducing latency and tool-call count.
- Use `edit_file` for a **single surgical string replacement** where
  exact-match semantics are preferred over the 4-pass fuzzy matching
  that `apply_patch` uses.

**Safety:** All paths are validated via `path_safety` (rejects escapes
outside cwd, symlink traversal). Writes are atomic (tempfile + rename).
Pre-tool snapshots cover every affected path so `/undo` reverts all
files touched by one patch atomically.

## question tool

`question` lets the model ask the user one or more questions with
pre-defined options. It blocks until the user answers.

```json
{
  "questions": [
    {
      "question": "Which database should we use?",
      "header": "Database",
      "options": [
        { "label": "pg", "description": "PostgreSQL" },
        { "label": "mysql", "description": "MySQL" }
      ],
      "multiple": false
    }
  ]
}
```

**Parameters:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `questions` | array | yes | List of questions to present |
| `questions[].question` | string | yes | The question text |
| `questions[].header` | string | no | Short label (≤30 chars) shown in the UI |
| `questions[].options` | array | yes | Selectable options (can be empty for skip-only) |
| `questions[].options[].label` | string | yes | Answer label returned to the model |
| `questions[].options[].description` | string | no | Extended description shown alongside the label |
| `questions[].multiple` | boolean | no | Allow multi-select (default: false) |

**Behavior by mode:**

- **TUI**: A modal card with arrow-key navigation, Enter to confirm,
  Space to toggle (multi-select), Esc to cancel (returns empty answer).
- **CLI (`-p` mode)**: Numbered options printed to stderr, user types
  a number (or comma-separated numbers for multi-select) on stdin.
  EOF on stdin returns empty answers so the agent continues without
  hanging.
