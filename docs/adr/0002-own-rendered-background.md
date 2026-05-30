# Own the rendered background (paint the canvas)

**Status:** amended (2026-05-30) — the full-screen canvas paint was **reverted**; the bg-tier panel ladder was **kept**. See _Update_ below. Originally accepted 2026-05-29.

## Update (2026-05-30): full-screen paint reverted, panels retained

Painting the whole screen with `bgBase` did not survive contact with real
output. An outer lipgloss `Background` cannot reliably fill *behind* content
rendered by glamour and the viewport, because those emit ANSI resets (`\x1b[0m`)
throughout — each reset snaps the background to the terminal default mid-line,
and the outer style cannot reassert itself across it. The result was ragged
**black gaps** wherever a reset fired (prose and reasoning especially) — exactly
the "fighting an unknown backdrop" failure this ADR set out to avoid, now coming
from *within* our own content stream.

Decision: **stop painting the full-screen canvas.** Prose / reasoning / status
sit on the terminal's own background (uniform, no gaps). The valuable half — the
**`bgWell` / `bgSurface` / `bgRaised` panel ladder** for tool results, diffs,
reasoning panels, badges, and the role-bars — is **kept**: those render their
own full-width per-line fills, contain no mid-line resets, and so never bleed.
They read as "cards on the terminal background," which achieves the soft-panel
depth goal without the fragile global paint. `ui.transparent_background` now
disables the *panel* fills too (fully flat / fg-only); there is no longer any
full-screen fill to opt out of.

The original decision and its reasoning are preserved below for history.

---

The TUI **owns the rendered background**: the root canvas is painted with the
theme's `bgBase`, and panels compose against a small **bg-tier ladder**
(`bgWell` / `bgSurface` / `bgRaised`) layered on top of it. Today the TUI
renders *transparent* — the `#0d1b2a` `Background` palette value is defined but
never painted, and only chroma syntax tokens and a single button carry a `bg`
fill. The beautification work needs crush-style subtle panels, and a subtle
panel only reads correctly against a **known** base color: a faint surface fill
is invisible (or inverts) when the actual backdrop is whatever the user's
terminal happens to be. So we paint the canvas and build depth from there. A
config opt-out (`ui.transparent_background`) restores the old transparent
rendering; the same degraded path engages automatically on non-truecolor
terminals. See [`/CONTEXT.md`](../../CONTEXT.md) for the vocabulary.

## Considered options

- **Stay transparent, depth from left-bars and separators only:** keep
  rendering transparent and convey structure with accent left-bars, rules, and
  spacing rather than filled panels. **Rejected** — it forgoes the soft-panel
  depth that is the #1 visual gap the beautification is meant to close; left-bars
  alone read as flat and cannot reproduce the layered crush-style surfaces.
- **Adopt charmtone wholesale:** swap the palette for the upstream charmtone
  background/surface ramp and inherit its panel tiers directly. **Rejected** —
  it abandons the DeepSeek-blue brand identity that the theme is built around and
  pulls in a dependency, for a result we can match with our own three-tier ladder.
- **Own the rendered background (chosen):** paint the root canvas with `bgBase`
  and introduce the `bgWell` / `bgSurface` / `bgRaised` tier ladder for panels;
  every fill is a semantic theme token, gated behind the opt-out and the
  truecolor check so transparent rendering remains a first-class mode.

## Consequences

- Panels, diffs, and badges render predictably against a known base, so subtle
  surface fills compose correctly instead of fighting an unknown backdrop.
- The previously-dead `Background` palette value is now actually used; the
  defined-but-unpainted state that motivated this ADR is resolved.
- By default we override the user's terminal background and any transparency
  they had configured — hence the `ui.transparent_background` opt-out, which
  restores transparent rendering and degrades the bg-tier panels to left-bars /
  separators with no opaque fills.
- The same degraded (left-bar) path engages on non-truecolor terminals, where
  the tier ladder cannot render distinguishable fills.
- Requires full light-theme parity before shipping: a half-tuned light canvas
  would look worse than flat transparent rendering, so owning the background is
  an all-or-nothing commitment per theme.
- Color-safe for the test suite: render goldens strip ANSI, so the new fills do
  not move them, and the cache goldens are unaffected — this is TUI-only
  presentational work that never touches the wire / prompt-cache path.
