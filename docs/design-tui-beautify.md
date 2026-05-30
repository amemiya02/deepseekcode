# Design: TUI Beautification

Status: Accepted (locked via design interview)
Scope: `internal/tui/**` and a single new config flag (`ui.transparent_background`)
Branch: `feat/tui-beautify`
UI stack: `charm.land/lipgloss/v2` on Bubble Tea v2

This is **presentational** work only. It does not touch the wire / prompt-cache
path. See [Non-goals & guardrails](#non-goals--guardrails).

---

## 1. Overview & principles

deepseekcode's TUI today is a single-column stack: `viewport -> chrome ->
status -> input -> hint`. It renders correctly but reads flat — there is no
clear surface hierarchy, panels and prose share the same canvas, the palette is
a small ad-hoc set of named styles, and color decisions are scattered as raw
hex at call sites.

This design is a **targeted polish**, not a re-layout. We deliberately
**keep the single-column layout** and every existing key flow. What changes is
the *visual system*: a two-layer token palette, an owned canvas with a
background ladder, role-colored gutters, and bg-filled panels limited to
code-bearing surfaces. The reference aesthetic is the `crush` project
(charmbracelet), adapted to DeepSeek's brand and our existing identity.

### Principles

1. **Hierarchy through background tiers, not boxes.** A small ladder of
   background shades (base / well / surface / raised) communicates depth.
   Borders are used sparingly (modals, focus).
2. **Panels only where they earn their keep.** Background fills go on
   *code-bearing* surfaces (tool-result bodies, diffs, reasoning). Assistant and
   user **prose stays airy on the canvas** — no panel chrome around plain text.
3. **One gutter to rule them all.** Every chat item shares a single 2-cell left
   gutter, and a role-colored left bar. Visual rhythm comes from that constant
   left edge.
4. **Semantic tokens at call sites, never raw hex.** Components ask the theme
   for `Panel(tier)`, `Badge(kind)`, `LeftBar(color)` — they never embed hex.
   The single exception is the diff +/- band colors (see §6).
5. **Brand continuity.** DeepSeek blue stays the primary brand color. The input
   chevron `>` and the user `>` glyph stay. We do **not** adopt crush's `:::`
   prompt style.
6. **Graceful degradation is a first-class path, not an afterthought.** Both an
   explicit opt-out (`ui.transparent_background`) and automatic non-truecolor
   detection collapse panels to left-bars / separators with no opaque fills.
7. **Full light/dark parity.** Every tier, panel, badge, and diff color has both
   a dark and a light value. No "dark-only" polish.

---

## 2. Token system

We move from a flat set of named `Theme` styles to a **two-layer** model,
mirroring crush:

- **Layer 1 — raw palette**: a struct of named colors (the "physical" palette).
  Extends the existing Ocean palette. No new dependency.
- **Layer 2 — semantic tokens / composed styles**: `Theme` exposes meaning
  (`Panel`, `Badge`, `LeftBar`, foreground tiers) composed from the raw palette.

The API is **additive**. We do **not** remove or rename existing `Theme`
fields. Existing named styles (`UserPrompt`, `ToolCall`, `CardBar`, etc.) are
**re-derived** from the new tokens so every current call site keeps compiling
unchanged. New code uses the semantic tokens directly.

### 2.1 Raw palette tokens

Values are listed as **DARK / LIGHT**.

| Token         | Dark      | Light     | Role                                              |
|---------------|-----------|-----------|---------------------------------------------------|
| `brandDeep`   | `#1D4ED8` | `#1D4ED8` | Primary; focus borders; user left bar             |
| `brandLight`  | `#7DD3FC` | `#0087af` | Secondary; info                                   |
| `accentFlash` | `#5fd7d7` | `#0087af` | Flash model + tool left bar                       |
| `accentPro`   | `#c77dff` | `#7c3aed` | Pro model + tool left bar                          |
| `bgBase`      | `#0d1b2a` | `#ffffff` | Painted canvas                                    |
| `bgWell`      | `#0a1622` | `#eef1f5` | Code / diff inset (deepest)                       |
| `bgSurface`   | `#12243a` | `#f3f5f8` | Tool / reasoning panels                           |
| `bgRaised`    | `#16314f` | `#e8edf3` | Modals; selected rows                             |
| `fgBase`      | `#e4e4e4` | `#1c1c1c` | Primary text                                      |
| `fgMuted`     | `#9aa7b4` | `#4a5663` | Secondary text                                    |
| `fgSubtle`    | `#6c7a89` | `#878787` | De-emphasized (== today's `Dim`)                  |
| `fgFaint`     | `#4a5663` | `#aab2bd` | Meta, line numbers, hints                         |
| `border`      | `#1c3350` | `#d4dae2` | Panel / modal borders, separators                 |
| `ok`          | `#5fd75f` | `#008f00` | Success / healthy                                 |
| `err`         | `#ff5f5f` | `#c0392b` | Error                                             |
| `warn`        | `#d7d75f` | `#9a6700` | Warning                                           |
| `onAccent`    | `#0d1b2a` | `#ffffff` | Text drawn on filled badges / selection           |

Notes:
- The background ladder `bgBase -> bgWell -> bgSurface -> bgRaised` is the spine
  of the whole design. In dark it climbs *toward* lighter blue-greys; in light
  it is a tight, low-contrast grey ladder so panels read as subtle insets, not
  boxes.
- `fgSubtle` is intentionally pinned to today's `Dim` value so existing
  "dimmed" call sites map cleanly.
- `brandDeep` is identical in both themes — DeepSeek blue is the brand constant.

### 2.2 Diff bands (hard-coded, NOT tokens)

Diff add/delete colors are the **only** sanctioned hard-coded hex in component
code. They are not part of the palette struct because they are a fixed,
universally-recognized green/red semantic that must not drift with theme tuning.

| Theme | Kind | Foreground | Background |
|-------|------|------------|------------|
| Dark  | ADD  | `#7ee787`  | `#12301f`  |
| Dark  | DEL  | `#ff7b72`  | `#2d1517`  |
| Light | ADD  | `#22863a`  | `#e6ffec`  |
| Light | DEL  | `#b31d28`  | `#ffeef0`  |

### 2.3 Helpers exported on `Theme`

The foundation layer must create and export these helpers. Components rely on
them and never reconstruct background/badge logic locally.

| Helper                | Returns / behavior                                                                                                                                          |
|-----------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Theme.Panel(tier)`   | A `lipgloss.Style` backgrounded with the given tier color. Returns **no background** when in transparent-mode or non-truecolor (degrades to plain).          |
| `Theme.Badge(kind)`   | A filled chip style: `Background` = semantic color (`ok`/`err`/`warn`/`info`/`brandDeep`), `Foreground` = `onAccent`, `Padding(0,1)`, `Bold`.                |
| `Theme.LeftBar(color)`| The role-bar style rendering the existing thick bar glyph in the given color.                                                                                |
| `Theme.Gutter`        | The single 2-cell left gutter string/const used by **every** chat item.                                                                                     |

`Theme` additionally holds two booleans:

- `transparent bool` — from `ui.transparent_background` config.
- `truecolor bool` — from color-profile detection at startup.

`Panel()` consults **both**: if either is set against full-color rendering
(transparent == true, or truecolor == false), it returns a style with no
background fill so the caller's fallback (left-bar / separator) governs.

---

## 3. Canvas & rendering

> **Amended 2026-05-30 (see ADR-0002):** the full-screen canvas paint described
> in §3.1 was **reverted**. An outer `Background` cannot fill behind glamour /
> viewport output (their ANSI resets snap the bg to the terminal default
> mid-line → ragged black gaps). We no longer paint the frame; prose/reasoning
> sit on the terminal's own background. The bg-tier **panel ladder** (§3.2,
> `bgWell`/`bgSurface`/`bgRaised`) is **kept** — those render their own
> full-width per-line fills and never bleed, reading as cards on the terminal
> background. `ui.transparent_background` now disables the panel fills too.

### 3.1 Owning the background

Today the TUI inherits the terminal's background. The new design **owns** the
canvas: the root view paints `bgBase` (`#0d1b2a` dark / `#ffffff` light) across
the full frame. Everything composes on top of that owned base.

On top of the canvas, the background ladder gives depth:

```
bgBase     painted canvas (root)
  bgWell     code / diff inset (deepest panel)
  bgSurface  tool / reasoning panels
  bgRaised   modals, selected picker rows
```

### 3.2 Opt-out: `ui.transparent_background`

A new config key `ui.transparent_background` (bool, default `false`). When
`true`:

- The canvas is **not** painted — the terminal background shows through.
- All `Theme.Panel(tier)` calls return no-background styles. Panels degrade to
  their left-bar / separator presentation (see §3.3).
- Badges and the diff bands still render (they are intentional foreground
  emphasis, not ambient surface), but no large opaque fills appear.

This is for users on themed/transparent terminals who do not want an opaque
app background.

### 3.3 Color-profile degrade (non-truecolor)

At startup we detect the terminal color profile. When the terminal is **not
truecolor**, `truecolor` is set `false` and we degrade automatically with the
exact same code path as the opt-out:

- No canvas paint, no panel background fills.
- Panels collapse to a **left-bar** (a colored vertical bar in the gutter) plus,
  where a panel previously had a top/bottom edge, a thin separator rule.
- Semantic colors are still applied to foregrounds (they resolve to the nearest
  ANSI-256 color via lipgloss), so role/health coloring survives; only the
  opaque background tiers are dropped.

This keeps the layout and information hierarchy intact on basic terminals
without smeared or mis-rendered background blocks.

---

## 4. Chat items

All chat items (`itemUser`, `itemAssistant`, `itemToolCall`, `itemToolResult`,
`itemReasoning`, error/note lines) share a unified structure:

```
<2-cell gutter><left bar><content>
```

- **Unified 2-cell gutter** everywhere via `Theme.Gutter` — one constant left
  edge across the whole transcript.
- **Role-colored left bar** via `Theme.LeftBar(color)`:
  - user → `brandDeep` (blue)
  - tool → model accent: `accentFlash` for flash, `accentPro` for pro
  - error → `err` (red)
  - assistant → muted/neutral bar (prose stays calm)
- **Background panels ONLY on code-bearing surfaces**: the tool-result body,
  diffs, and reasoning get a `Panel(...)` background. **Assistant and user prose
  stay airy on the canvas** — no panel fill, just gutter + bar + text.
- **Inline badges** for `ERROR` and `NOTE` via `Theme.Badge(...)` — a filled
  chip in the line rather than a separate boxed block, keeping the airy prose
  rhythm.

This is the core readability move: the eye tracks one left edge, color tells you
*who/what* is speaking, and only the surfaces that contain code get visually
"boxed in" so prose never feels caged.

---

## 5. Reasoning

- **Keep the existing 2-state fold.** The toggle keys remain **`ctrl+r`** and
  **`ctrl+t`** exactly as today. They are never rebound to plain letters (plain
  letters collide with first-character typing).
- The reasoning block is wrapped in a `Panel(bgSurface)` so it reads as a
  distinct, code-bearing surface (it is model thinking, treated like an inset).
- **Expanded height cap (~16 lines).** When expanded reasoning exceeds the cap,
  it is clipped to ~16 lines and a faint hint line is appended:
  `... N more (ctrl+t)`, rendered in `fgFaint`. This prevents a long chain of
  thought from flooding the viewport while still signaling there is more and how
  to see it.

In transparent / non-truecolor mode the `bgSurface` panel degrades to a left-bar
in `fgMuted` (per §3.3); the fold behavior and the height cap are unchanged.

---

## 6. Diffs

- **Unified diff only.** No split view.
- The diff body is rendered inside a `Panel(bgWell)` inset.
- A **line-number gutter** runs down the left of the diff body, rendered in
  `fgFaint`.
- Add/delete lines get **hard-coded green/red bands** (the only sanctioned
  hard-coded hex — see §2.2): a forced foreground + a forced full-width
  background band per line.
- **Syntax highlighting via chroma** is applied on top of the forced band
  background, so highlighted code still sits on the correct green/red band
  rather than on the panel base. The band background wins; chroma supplies the
  per-token foreground.

Degrade behavior: in transparent / non-truecolor mode the `bgWell` panel base is
dropped, but the **diff bands themselves still render** (they are semantic, not
ambient surface). On non-truecolor terminals the band hex resolves to nearest
ANSI; the +/- sign column plus foreground color keeps add/delete legible even if
the band fill is approximate.

---

## 7. Status line

The status line **content is unchanged** — same fields, same order. Only the
visual treatment changes:

- **Brightness hierarchy.** Primary fields use `fgBase`, secondary use
  `fgMuted`, meta uses `fgFaint`, so the eye lands on what matters first.
- **Section accents.** Logical sections are tinted with their semantic accent
  (e.g. model section uses the model's accent — `accentFlash` / `accentPro`).
- **Health-colored cache% / cost.** The cache-hit percentage and cost render in
  `ok` / `warn` / `err` based on health thresholds (good hit rate / cost → `ok`;
  degrading → `warn`; bad → `err`).
- **Background badges ONLY for transient state.** A filled `Badge(...)` appears
  only for transient conditions — cache-invalidation, compaction events, error
  states. Steady-state fields never get a bg badge; this keeps the status line
  calm and makes the badge genuinely attention-grabbing when it appears.

---

## 8. Modals & pickers

- **Keep replace-in-layout.** Modals/pickers continue to replace content in the
  single-column layout. **No dimmed backdrop / overlay.**
- The modal/picker is a `Panel(bgRaised)` with:
  - a **rounded border** (in `border`, or `brandDeep` when focused),
  - `Padding(1, 2)`,
  - a **gradient `///` accent title** — a short gradient-rendered `///` glyph
    run leading the title, giving modals a distinct, branded header without a
    heavy title bar.
- **Selected picker rows get a bg-fill** (`bgRaised` or a brand-tinted selection
  with `onAccent` foreground) so the current selection is unmistakable.

Degrade behavior: in transparent / non-truecolor mode the `bgRaised` panel fill
and selection fill are dropped; the rounded border + the `///` accent title
remain as the structural cue, and the selected row falls back to a left-bar /
inverse marker instead of a fill.

---

## 9. Smaller surfaces

- **Input.** Keep the `>` chevron prompt glyph. The input row uses `fgBase`
  text on the canvas; the focus state tints the chevron / a thin focus rule in
  `brandDeep`. We do **not** adopt the crush `:::` prompt.
- **Scrollbar.** Track in `border`, thumb in `fgMuted` (or `brandDeep` while
  actively scrolling). Thin, unobtrusive, on the canvas.
- **Logo.** Re-tinted with the brand gradient (`brandDeep -> brandLight`) so the
  splash/header reads as DeepSeek-branded; no layout change.
- **Glamour (markdown).** The glamour style is re-derived from the new tokens —
  body in `fgBase`, headings tinted with brand/accent, code spans/blocks on
  `bgWell`, links in `brandLight`, blockquote bar in `fgFaint`. This keeps
  rendered markdown consistent with the rest of the surface system instead of
  glamour's default palette.

---

## 10. Theme parity

**Full light/dark parity is mandatory.** Every value introduced by this design —
every background tier, foreground tier, panel, badge kind, diff band, accent,
selection fill, and degrade fallback — has **both** a dark and a light value (see
§2.1 and §2.2). There is no dark-only or light-only path. Both themes are
defined in the same palette construction and selected at theme-init time.

Light theme intent: the background ladder is a tight, low-contrast grey set so
panels read as subtle insets rather than heavy boxes, while brand/accent/health
colors are darkened versions chosen for contrast on a white canvas.

---

## 11. Files touched

All work is in `internal/tui/` plus one config flag. Indicative file map (edit
in place on `feat/tui-beautify`):

- `internal/tui/theme.go` — the big one. Add the raw palette struct, both
  dark/light palettes, the semantic-token layer, the `Panel` / `Badge` /
  `LeftBar` / `Gutter` helpers, and the `transparent` / `truecolor` fields.
  Re-derive existing named styles (`UserPrompt`, `ToolCall`, `CardBar`, etc.)
  from the new tokens.
- `internal/tui/app.go` — paint the owned canvas (`bgBase`) at the root view;
  wire color-profile detection into `truecolor`; thread the
  `ui.transparent_background` config value into the theme.
- Chat-item rendering (the item renderers, e.g. `internal/tui/render*.go` /
  the file(s) building `itemUser` / `itemAssistant` / `itemToolCall` /
  `itemToolResult`) — unified gutter, role left bars, panels on code-bearing
  surfaces only, inline `ERROR`/`NOTE` badges.
- Reasoning renderer — `Panel(bgSurface)` wrap + ~16-line cap + `... N more
  (ctrl+t)` hint. Fold toggles untouched.
- Diff renderer — `Panel(bgWell)`, line-number gutter, hard-coded bands, chroma
  on band bg, unified-only.
- Status line renderer — brightness hierarchy, section accents, health-colored
  cache%/cost, transient-only badges.
- Modal / picker renderer(s) — `bgRaised` panel, rounded border, `Padding(1,2)`,
  gradient `///` title, selected-row fill.
- Input / scrollbar / logo / glamour styling — re-tinted from tokens (§9).
- `internal/tui/render_cache.go` — review only; see §12. No behavioral change
  expected (cache keys already include `theme`).
- Config: add `ui.transparent_background` to the config struct + loader (the
  `config` package), default `false`, threaded into the theme at TUI init.

(Exact filenames are confirmed against the tree at implementation time; the
constraint is that **only** the TUI package and the one config flag change.)

---

## 12. Testing & verification

### 12.1 Cache goldens are NOT touched

This is presentational, TUI-only work. It must **never** touch the wire /
prompt-cache path:

- No edits to `internal/llm` request serialization, `MarshalCacheStable`, the
  static prefix, or anything upstream of them.
- `TestCacheStableGolden` and `TestFingerprintTracksWireStaticHead` **must keep
  passing with no `-update`**. These goldens are explicitly out of scope and
  must not move.

### 12.2 Render goldens strip ANSI

The TUI render goldens **strip ANSI escape codes** before comparison. Therefore
**color-only changes are golden-safe** — recoloring text, adding background
fills, tinting accents, etc., produce no diff in the stripped golden.

What *will* legitimately change the render goldens is **structure**: the unified
2-cell gutter, the left-bar glyph column, the reasoning height cap + `... N more`
hint line, the diff line-number gutter, inline `ERROR`/`NOTE` badge text, and the
modal `///` title text. Those goldens are regenerated as part of this one
big-bang PR (`-update`), and the regenerated stripped output is reviewed to
confirm only the intended structural changes appear.

### 12.3 Degrade-path coverage

Add/extend tests asserting the two degrade conditions produce no background
fills:

- `ui.transparent_background == true` → `Panel(tier)` yields a style with no
  background set.
- `truecolor == false` → same no-background behavior; panels fall back to
  left-bar / separator.

These can assert on the resolved `lipgloss.Style` (no background) and/or on the
ANSI-stripped structural output (left-bar present, no full-width band fills where
a panel would have been). Diff bands and badges still render in degrade mode.

### 12.4 Build & format gate

Before the work is considered done:

- `gofmt -w` is run on every changed file.
- `go build ./...` passes; if the TUI package was touched, `go build
  ./internal/tui/` passes too.
- `make test` is green, including the untouched cache goldens (§12.1) and the
  regenerated render goldens (§12.2).

Do not conclude on a broken build.

---

## Non-goals & guardrails

- **No layout redesign.** Single-column stack stays.
- **No new dependency.** The palette is extended in-package; no new color or UI
  library.
- **No wire/cache edits.** See §12.1.
- **No key rebinds.** `ctrl+r` / `ctrl+t` reasoning fold keys stay; the `>`
  input chevron and `>` user glyph stay; no `:::`.
- **No raw hex at call sites** except the diff +/- band colors (§2.2).
- **Respect the opt-out and the non-truecolor path** as equal, first-class
  degrade routes (§3.2, §3.3).
