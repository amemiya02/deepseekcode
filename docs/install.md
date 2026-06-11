# Install

`deepseekcode` ships as a single static binary (`dsc`) with no runtime
dependencies. Pick whichever channel suits your environment.

## Homebrew (macOS / Linux)

```sh
brew install amemiya02/deepseekcode/deepseekcode
```

> The tap is configured but not yet published; until v0.1.0 cuts,
> use one of the other channels below.

## curl | sh (any Unix)

```sh
curl -fsSL https://raw.githubusercontent.com/amemiya02/deepseekcode/main/install.sh | sh
```

Pins to a specific version:

```sh
curl -fsSL https://raw.githubusercontent.com/amemiya02/deepseekcode/main/install.sh | DSC_VERSION=v0.1.0 sh
```

Custom install prefix (default `~/.local/bin`):

```sh
curl -fsSL https://raw.githubusercontent.com/amemiya02/deepseekcode/main/install.sh | PREFIX=/usr/local sh
```

## Go install

If you already have Go ≥ 1.23:

```sh
go install github.com/amemiya02/deepseekcode/cmd/dsc@latest
```

Binary lands in `$(go env GOPATH)/bin`.

## GitHub Releases

Prebuilt binaries for every release live at
<https://github.com/amemiya02/deepseekcode/releases>. Download the
archive for your platform, extract, and place `dsc` somewhere on your
`$PATH`.

Supported targets:

- darwin-arm64
- darwin-amd64
- linux-amd64
- linux-arm64
- windows-amd64

## Build from source

```sh
git clone https://github.com/amemiya02/deepseekcode
cd deepseekcode
make build              # → ./bin/dsc
make install            # → $GOPATH/bin/dsc
```

## Verify

```sh
dsc -version
```

## First run

Set your DeepSeek API key, then run with no arguments to launch the TUI:

```sh
export DEEPSEEK_API_KEY=sk-your-key
dsc
```

Or hit the model with a one-shot prompt and exit:

```sh
dsc -p "explain the auth flow in pkg/auth"
```

See [config.md](reference/config.md) for permanent configuration via
`~/.deepseek/config.toml`.

## Upgrade

```sh
dsc upgrade            # check for updates + print upgrade command
dsc upgrade --apply    # execute the upgrade directly
```

See [upgrade.md](upgrade.md) for details on install-method detection
and the safety default (print-only without `--apply`).

## Uninstall

```sh
rm "$(command -v dsc)"           # remove the binary
rm -rf ~/.deepseek               # remove all sessions, snapshots, config
rm -rf .deepseek                 # in each project, removes pointer + snapshots
```
