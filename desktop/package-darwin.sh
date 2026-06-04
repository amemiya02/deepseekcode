#!/usr/bin/env bash
#
# Self-contained macOS .app packager for the DeepSeekCode Wails v3 desktop shell.
#
# Why this exists: `wails3 build` / `wails3 package` need the wails3 CLI plus
# go-task on PATH and the full `wails3 init` project template (a generated
# Taskfile whose `darwin:build` task assumes a frontend/ npm build). This repo
# embeds the SPA via Go (webapp.DistFS) and ships as a single Go binary, so we
# package the app with nothing but Go + stock macOS tools (sips, iconutil,
# plutil, codesign). This is the path `make desktop` uses; the wails3 CLI path
# stays available via desktop/Taskfile.yml for anyone who has the CLI installed.
#
# Usage (from the repo root):  make desktop
#   or directly:               bash desktop/package-darwin.sh
#
# Prereq: the embedded SPA must already be built into webapp/dist. The Makefile
# `desktop:` target depends on `web`, which does that; if you run this script
# standalone, run `make web` first.

set -euo pipefail

# Resolve the repo root (this script lives in desktop/) and run from there so the
# Go build sees ./desktop/ and the webapp/dist embed.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

APP_NAME="DeepSeekCode"
EXE_NAME="DeepSeekCode"
PKG_DIR="desktop/packaging"
OUT_DIR="bin"
APP="$OUT_DIR/$APP_NAME.app"

if [ ! -d "webapp/dist" ] || [ -z "$(ls -A webapp/dist 2>/dev/null)" ]; then
	echo "error: webapp/dist is empty — build the SPA first (run 'make web')." >&2
	exit 1
fi

echo "==> Building desktop binary (go build -tags withwebapp)…"
mkdir -p "$OUT_DIR"
go build -tags withwebapp -ldflags "-s -w" -o "$OUT_DIR/.${EXE_NAME}.bin" ./desktop/

echo "==> Assembling $APP …"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
mv "$OUT_DIR/.${EXE_NAME}.bin" "$APP/Contents/MacOS/$EXE_NAME"
chmod +x "$APP/Contents/MacOS/$EXE_NAME"

# Info.plist (tracked template) + integrity lint.
cp "$PKG_DIR/Info.plist" "$APP/Contents/Info.plist"
/usr/bin/plutil -lint "$APP/Contents/Info.plist" >/dev/null

# Classic PkgInfo file (APPL + no signature).
printf 'APPL????' > "$APP/Contents/PkgInfo"

# Icon: PNG -> .iconset (exact Apple sizes) -> icon.icns, using only stock tools.
echo "==> Generating icon.icns…"
SRC="$PKG_DIR/appicon.png"
if [ ! -f "$SRC" ]; then
	echo "error: $SRC not found." >&2
	exit 1
fi
ICONSET="$(mktemp -d)/icon.iconset"
mkdir -p "$ICONSET"
# name -> pixel size (Apple-required iconset members).
gen() { /usr/bin/sips -z "$2" "$2" "$SRC" --out "$ICONSET/$1" >/dev/null; }
gen icon_16x16.png        16
gen icon_16x16@2x.png     32
gen icon_32x32.png        32
gen icon_32x32@2x.png     64
gen icon_128x128.png      128
gen icon_128x128@2x.png   256
gen icon_256x256.png      256
gen icon_256x256@2x.png   512
gen icon_512x512.png      512
gen icon_512x512@2x.png   1024
/usr/bin/iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/icon.icns"

# Ad-hoc code-sign so Gatekeeper lets the operator launch their own build
# without a "damaged / unidentified developer" error. (Distribution to other
# machines still needs a Developer ID signature + notarization — out of scope.)
echo "==> Ad-hoc code-signing…"
if /usr/bin/codesign --force --deep --sign - "$APP" 2>/dev/null; then
	/usr/bin/codesign --verify --deep --strict "$APP" 2>/dev/null \
		&& echo "    signature: ad-hoc, verified" \
		|| echo "    signature: ad-hoc applied (strict verify skipped)"
else
	echo "    warning: codesign unavailable — the app still runs locally."
fi

SIZE="$(du -sh "$APP" | cut -f1)"
echo ""
echo "Built $APP ($SIZE)"
echo "  Open:  open \"$APP\""
echo "  Run :  \"$APP/Contents/MacOS/$EXE_NAME\""
