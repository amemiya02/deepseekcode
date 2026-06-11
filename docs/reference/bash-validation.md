# Bash Validation

`dsc` classifies bash commands into four intent levels before deciding
whether to auto-allow, ask the user, or deny. This replaces the previous
regex-based destructive detection with a structured classifier.

## Intent Levels

| Level | Name | Behavior |
|---|---|---|
| 0 | `read` | Auto-allow (no permission prompt) |
| 1 | `safe` | Allow if pattern in `bashAllowlist`; otherwise ask |
| 2 | `destructive` | Always ask (allowlist ignored) |
| 3 | `unknown` | Ask (treated conservatively) |

## Classification Rules

### Read-only commands (auto-allow)

**Standalone verbs:** `ls`, `cat`, `head`, `tail`, `grep`, `egrep`, `fgrep`, `rg`, `find`, `which`, `where`, `whereis`, `wc`, `sort`, `uniq`, `diff`, `echo`, `printf`, `pwd`, `whoami`, `hostname`, `uname`, `date`, `env`, `printenv`, `type`, `file`, `stat`, `du`, `df`, `ps`, `top`, `htop`, `curl`, `wget`, `tree`, `less`, `more`, `jq`, `yq`, `xargs`

**Git read-only subcommands:** `status`, `log`, `diff`, `show`, `blame`, `describe`, `reflog`, `shortlog`, `branch` (listing only), `remote` (listing only), `tag` (listing only), `stash list`

**Go read-only subcommands:** `go list`, `go vet`, `go doc`, `go version`, `go env`, `go test`, `go build`

**Other read-only:** `terraform plan`, `terraform show`, `terraform validate`

### Safe-mutating commands (allowlist-gated)

**Standalone verbs:** `mkdir`, `touch`, `cp`, `ln`, `tar`, `zip`, `unzip`, `gzip`, `pip`, `pip3`, `uv`, `cargo`, `npm`, `pnpm`, `yarn`, `make`, `cmake`, `brew`

**Git safe subcommands:** `add`, `commit`, `push` (without `--force`), `mv`, `rebase`, `cherry-pick`, `merge`, `stash`, `branch <name>`, `remote <name>`, `tag <name>`, `checkout` (without `-- .`), `reset` (without `--hard`), `clean` (without `-f`)

**Go safe subcommands:** `go install`, `go get`, `go mod download`, `go mod tidy`, `go mod verify`, `go generate`, `go run`

**Other safe:** `kubectl apply`, `echo hello > file.txt` (redirect = safe)

> **Note on `kubectl apply`:** `kubectl apply` is classified as safe-mutating (allowlist-gated). In production, `kubectl apply -f delete.yaml` can delete cluster resources. Users should review apply commands manually or add them to `extra_destructive_patterns` if they prefer stricter enforcement.

### Destructive commands (always ask)

**Standalone verbs:** `rm`, `rmdir`, `mv`, `cp -f`/`cp --force`, `sed -i`/`sed --in-place`, `chmod`, `chown`, `chgrp`, `kill`, `pkill`, `killall`, `dd`, `mkfs`, `mount`, `umount`, `shutdown`, `reboot`, `halt`, `poweroff`

**Git destructive:** `git push --force`/`-f`/`--force-with-lease`, `git reset --hard`, `git checkout -- .`, `git clean -f`/`-fd`

**Other destructive:** `docker rm`, `docker rmi`, `docker push`, `kubectl delete`, `npm publish`/`pnpm publish`/`yarn publish`, `terraform apply`, `terraform destroy`, `curl --upload-file`/`-T`

### Unknown commands (conservatively ask)

**Eval/shell:** `eval`, `bash`, `sh`, `zsh`, `source`, `.`

**Write-method HTTP:** `curl -X POST`, `curl --request PUT`

**Unrecognized commands:** anything not in the above lists

## Pipe/Chain Rules

Commands joined by `;`, `|`, `&&`, or `||` are split into segments. Each
segment is classified independently. The **strictest** intent wins:

```
cat file | grep pattern        → read | read   = read
mkdir dir && git status        → safe | read   = safe
cat file | rm -rf dir          → read | destr  = destructive
git status; rm -rf .           → read ; destr  = destructive
```

## Extra Destructive Patterns

Users can add additional regex patterns via `[duet].extra_destructive_patterns`
in config. These are checked after `ClassifyBash` — if the classifier
says non-destructive but the command matches an extra pattern, it is
still treated as destructive for the Duet validator.

## Redirect Handling

Any command containing `>` or `>>` **outside of quotes** is classified as `safe`
(file-writing), even if the base verb is read-only (e.g., `echo hello > file.txt`).
Redirects inside quoted strings (e.g., `echo "a > b"`) are not treated as redirects.

The redirect detection uses a character-level scan that respects shell quoting.
This handles the common case correctly but does not cover every edge case:
heredocs (`<<EOF`), process substitution (`>(cmd)`), and other advanced redirect
forms are not parsed. In these rare cases, the classifier falls back to the base
verb's intent level, which is safe (may be more permissive but never misses a
destructive operation).

## Combined Short Flags

Flags like `-fd` are treated as containing both `-f` and `-d`. This
ensures `git clean -fd` is correctly classified as destructive (contains `-f`).
