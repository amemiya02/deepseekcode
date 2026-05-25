# TUI Theme — DeepSeek Ocean

The TUI uses a **DeepSeek Ocean** visual identity built on a deep blue →
light blue gradient palette. There is no runtime theme switching; the
theme is selected at startup from config (`dark` default, `light` alt).

## Palette

| Token | Hex | Usage |
|-------|-----|-------|
| `brandDeep` | `#1D4ED8` | Gradient start, card bar, input border (insert mode) |
| `brandLight` | `#7DD3FC` | Gradient end, wordmark/whale accent |
| `flash` | `#5fd7d7` | Flash model accent, tool call, spinner |
| `pro` | `#c77dff` | Pro model accent |
| `fg` | `#e4e4e4` | Primary text (dark theme) |
| `bg` | `#0d1b2a` | Background (dark theme) |
| `dim` | `#6c7a89` | Secondary text, hints, reasoning |
| `success` | `#5fd75f` | Success indicators |
| `error` | `#ff5f5f` | Error indicators |
| `warn` | `#d7d75f` | Warnings, info |

Light theme uses the same semantic tokens with inverted contrast.

## Gradient

The wordmark (`DEEPSEEKCODE`) and whale mascot are rendered with a
per-grapheme horizontal gradient from `brandDeep` → `brandLight` using
`lipgloss.Blend1D`. Each line is independently gradient-colored (left→
right deep→light). The spinner uses a 16-step HCL round-trip ramp
(`brandDeep → brandLight → brandDeep`) with per-frame offset for a
flowing effect.

## Tool Cards

Tool calls render as left-sidebar cards:

```
▌ ● bash  go test ./...                 ✓ 1.2s
▌   ok  deepseekcode/internal  0.3s
```

- `▌` (BarThick) in brand blue
- Status icons: `●` pending, `✓` success, `×` error
- Body truncated to 10 lines when folded; `… N more lines, press e to expand`

## Spinner

Uses the standard Braille frames (`⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏`) with gradient
coloring. The ramp is lazily initialized from the theme's `BrandDeep`/
`BrandLight` and cached for the session.

## Symbol Library

`internal/tui/symbols.go` defines Unicode constants used across the TUI:

| Constant | Symbol | Usage |
|----------|--------|-------|
| `IconCheck` | `✓` | Success indicator |
| `IconToolPending` | `●` | Tool running |
| `IconToolOk` | `✓` | Tool success |
| `IconToolErr` | `×` | Tool error |
| `BarThick` | `▌` | Card left sidebar |
| `IconModel` | `◇` | Model indicator |
| `IconSkill` | `▲` | Skill indicator |

## Syntax Highlighting

Code blocks in tool cards and markdown use `chroma/v2` with a custom
formatter that maps token types to lipgloss foreground styles. The
background is locked to the card body color for consistent appearance.

## Files

| File | Purpose |
|------|---------|
| `internal/tui/theme.go` | Palette + factory (`buildTheme`) |
| `internal/tui/grad.go` | Per-grapheme gradient rendering |
| `internal/tui/symbols.go` | Unicode symbol constants |
| `internal/tui/highlight.go` | Chroma syntax highlighting glue |
| `internal/tui/chrome.go` | Gradient spinner |
| `internal/tui/welcome.go` | Whale + wordmark banner |
| `internal/tui/items.go` | Tool card rendering |
