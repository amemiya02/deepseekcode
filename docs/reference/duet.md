# Two-Model Duet — Pro Validator

> Implementation deep dive (for contributors): [dev/routing.md](../dev/routing.md)

`deepseekcode`'s second headline feature. Flash drives the main loop;
Pro adjudicates the dangerous moments. Pro never runs on every turn —
that would halve the cache-hit rate and double cost for no quality gain.
Pro is invoked **surgically**, at moments where its premium is justified.

## When Pro fires

Two automatic triggers:

1. **Destructive-tool gate.** Before any of:
   - `write_file` / `edit_file` outside cwd, in `.git/`, in `.env*`, or
     matching the user's `secret_path_patterns`
   - `bash` matching destructive patterns: `rm`, `rm -rf`, `git push`,
     `git reset --hard`, `git checkout .`, `git clean -f`,
     `curl -X POST|PUT|DELETE|PATCH`, `kubectl delete`,
     `kubectl apply -f`, `terraform apply`, `terraform destroy`,
     `psql/mysql/sqlite3` with `DROP/DELETE/TRUNCATE`,
     `npm/pnpm/yarn publish`, `docker push`
   - Plus anything in `[duet].extra_destructive_patterns`

2. **`failure_retry_with_pro`.** When the main loop attempts the same
   fix twice and both fail (same tool name + arg-hash + non-zero exit),
   the next single attempt is routed to Pro internally. Per-attempt
   only; does not persist.

## What you see

```
◆ pro check (1.2s · 800 tok): approved
◆ pro check (3.4s · 1.2k tok): blocked — would delete uncommitted changes in pkg/auth/
```

Distinct color from the flash voice (magenta vs cyan). Validator output
is folded by `r` / `R` along with regular reasoning blocks.

## When Pro blocks

You get a choice:

```
◆ pro check (3.4s): blocked — <reason>
  [o]nce override · [e]dit · [c]ancel
```

- `o` — proceed anyway. Override is logged so you can review later.
- `e` — edit your prompt and try again.
- `c` — cancel; the model receives a "user cancelled after pro
  validation block" tool result and adapts.

## Failure modes

| Pro failure | Behavior |
|-------------|----------|
| 10s timeout | Skip with `◆ pro validation skipped: timeout` → fall through to standard permission prompt |
| Network error | Same as timeout |
| Malformed JSON | Treat as `block` with override choice |
| Rate limited | Skip with note → fall through to permission prompt |

**Pro never auto-approves a destructive op because the validator
failed.** You always get the final say.

## Edge case: `/models deepseek-v4-pro`

If you've switched the main-loop model to Pro, the destructive-tool
gate becomes a silent no-op (Pro can't meaningfully validate itself).
The standard permission prompt still gates destructive paths.

## Disabling

- CLI: `dsc --no-duet`
- Config: `[duet] enabled = false`

When disabled, destructive operations go through the standard
permission prompt without Pro intervention.

## Cost

The Cost HUD splits into two lines when the validator has fired this
session:

```
flash · 7 steps · cache 91% · ¥0.04
pro   · 2 calls · cache 73% · ¥0.02
```

Pro's cache-hit rate is naturally lower (validator prompts are
constructed fresh per call), but each call is bounded to recent
transcript + the proposed tool args, so total cost typically stays
well under what an always-on Pro main loop would burn.
