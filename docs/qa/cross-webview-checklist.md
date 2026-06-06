# Cross-webview QA Checklist (Wave 6)

Run before each platform release. Primary target: macOS (WKWebView). Smoke
Windows (WebView2) and Linux (WebKitGTK) before their releases.

## Per-platform matrix
| Check | macOS WKWebView | Win WebView2 | Linux WebKitGTK |
|-------|-----------------|--------------|-----------------|
| Window pre-paints bg (no white flash) | ☐ | ☐ | ☐ |
| Settings window opens + all 18 sections navigate | ☐ | ☐ | ☐ |
| Theme switch applies live (Graphite/Lumen/Halo) | ☐ | ☐ | ☐ |
| Accent + density change live | ☐ | ☐ | ☐ |
| Onboarding wizard: invalid key blocks, valid advances | ☐ | ☐ | ☐ |
| Doctor view runs + re-runs | ☐ | ☐ | ☐ |
| Update banner opens download in OS browser | ☐ | ☐ | ☐ |
| Monaco diff renders + per-hunk accept/reject (Wave 2) | ☐ | ☐ (risk) | ☐ (risk) |
| CJK glyphs render in transcript, diff, cockpit | ☐ | ☐ | ☐ |
| Non-UTF-8 (GB18030) file viewer round-trips | ☐ | ☐ | ☐ |
| SSE stream: delta/tool/done arrive in order | ☐ | ☐ | ☐ |
| Cancel/Stop mid-turn works | ☐ | ☐ | ☐ |
| Keyboard: ⌘K palette, Esc stop, focus-visible rings | ☐ | ☐ | ☐ |
| High-contrast theme passes WCAG AA | ☐ | ☐ | ☐ |
| Reduced-motion disables spinners/animations | ☐ | ☐ | ☐ |

## Browser-dev mock path
- ☐ `localStorage.dsc.mock='1'` (or `VITE_MOCK=1`) replays the canned turn with no Go backend.
- ☐ All UI states (loading/streaming/tool/done/empty/error) reachable via the mock fixtures.

## Known risks (spec §11)
- Monaco on Linux WebKitGTK: lazy-load + PlainCode/LCS fallback; avoid backdrop-blur + web-fonts on Linux.
- Wails v3 alpha: keep the native shell thin behind a small interface (v2 fallback is a contained change); CI smoke-tests the shell.
