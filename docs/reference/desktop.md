# Desktop App

Native macOS `.app` built from the same dsc binary and agent core, packaged via a self-contained shell script that uses only stock macOS tools (no Wails CLI required).

## Build & run

- `make desktop` → `bin/DeepSeekCode.app`
- Requires the SPA to be built first; the `desktop` Make target depends on `web`, which handles this automatically.
- Only Go and standard macOS tools (`sips`, `iconutil`, `plutil`, `codesign`) are needed — no `wails3` CLI, no `go-task`.

## Relationship to the Web SPA

The desktop shell embeds the same SPA that the gateway serves, compiled into the Go binary via `webapp.DistFS`. See [web.md](web.md) for details on the SPA build.

## Checkpoint & rewind — what is and isn't tracked

The desktop/web rewind controls (per-message RewindMenu → `/v1/rewind`,
`/v1/fork`, `/v1/branch`, `/v1/switch`, `/v1/summarize`) restore two
independent things, and you choose which:

- **Conversation** — the persisted message history (SQLite session store).
  Truncating it cannot fail destructively: it only forgets later turns.
- **Code** — files captured by the agent's *pre-edit snapshots*
  (`.deepseek/snapshots/<session>/<step>/`). A code rewind runs the same
  restore `/undo` uses.

### What is NOT tracked

- Files changed by **bash commands** or by any tool/process outside the
  agent's edit path. The agent only snapshots files it edits by name.
- Anything done **outside the app** (your editor, another terminal, CI).

**git is the source of truth for the working tree.** After a code rewind,
verify with `git diff` / the Changed tab. Prefer committing before risky
operations; a code rewind is a convenience, not a substitute for VCS.

### fork vs branch

- **branch** forks at an explicit message index (rewind-then-explore).
- **fork** forks at the end of the current history (clean continuation).

Both share history via no-copy replay, so branching is cheap regardless of
session depth.
