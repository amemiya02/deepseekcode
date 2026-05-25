# Upgrade

`dsc upgrade` checks for new releases and helps you update.

## Usage

```sh
dsc upgrade              # check + print upgrade command
dsc upgrade --check      # only check, no command printed
dsc upgrade --apply      # execute the upgrade command
```

## Safety default

`dsc upgrade` **prints** the recommended upgrade command but does **not**
execute it. Add `--apply` to run the command automatically. This
prevents accidental upgrades in scripts or CI.

## Install method detection

The binary detects how it was installed by inspecting its own path:

| Method | Detection | Upgrade command |
|--------|-----------|-----------------|
| Homebrew | path contains `Cellar/` or `homebrew` | `brew upgrade deepseekcode` |
| curl \| sh | binary under `~/.local/bin` | `curl -fsSL https://deepseekcode.dev/install.sh \| sh` |
| go install | binary under `$GOPATH/bin` | `go install github.com/amemiya02/deepseekcode/cmd/dsc@latest` |
| manual | anything else | download from GitHub Releases |

## Dev builds

If `version.Version` is `"dev"` (built from source without `-ldflags`),
`dsc upgrade` always reports an update is available and suggests
installing from a release or source.

## Doctor integration

`dsc doctor` includes a best-effort update check (2-second timeout).
If the check fails (offline, rate-limited), it shows "update check
skipped" without affecting the overall doctor result.

## Examples

```sh
# Check if an update is available
$ dsc upgrade --check
current=v0.1.0  latest=v0.2.0  method=brew
update available: https://github.com/amemiya02/deepseekcode/releases/tag/v0.2.0

# Print the upgrade command
$ dsc upgrade
current=v0.1.0  latest=v0.2.0  method=brew
update available: https://github.com/amemiya02/deepseekcode/releases/tag/v0.2.0
run: brew upgrade deepseekcode    (or: dsc upgrade --apply)

# Execute directly
$ dsc upgrade --apply
current=v0.1.0  latest=v0.2.0  method=brew
update available: https://github.com/amemiya02/deepseekcode/releases/tag/v0.2.0
running: brew upgrade deepseekcode
```
