package tui

import (
	"image/color"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
)

// Gutter is the single 2-cell left gutter prepended to EVERY chat item.
// Components reference Theme.Gutter (a method returning this constant) so
// the indent is defined in exactly one place.
const Gutter = "  "

// leftBarGlyph is the thick vertical bar drawn by Theme.LeftBar — the role
// marker that opens user / tool / reasoning blocks.
const leftBarGlyph = "▌"

// PanelTier selects which background surface Theme.Panel fills. The tiers
// climb from the painted canvas up through inset wells, panels, and raised
// modals. Panel() degrades every tier to no-background when fills are
// disabled (transparent mode or non-truecolor terminal).
type PanelTier int

const (
	// TierBase is the painted canvas background (bgBase).
	TierBase PanelTier = iota
	// TierWell is the recessed code/diff inset (bgWell).
	TierWell
	// TierSurface backs tool / reasoning panels (bgSurface).
	TierSurface
	// TierRaised backs modals and selected rows (bgRaised).
	TierRaised
)

// BadgeKind selects the fill color of a filled chip rendered by Theme.Badge.
type BadgeKind int

const (
	// BadgeOk is a success chip (ok color).
	BadgeOk BadgeKind = iota
	// BadgeErr is an error chip (err color).
	BadgeErr
	// BadgeWarn is a warning chip (warn color).
	BadgeWarn
	// BadgeInfo is an informational chip (brandLight color).
	BadgeInfo
	// BadgeBrand is a primary/brand chip (brandDeep color).
	BadgeBrand
)

// palette is the RAW token layer: a flat bag of colors per mode (dark /
// light). buildTheme composes these into the semantic Theme. Every named
// Theme style is re-derived from these tokens so call sites never touch
// raw hex.
type palette struct {
	// Brand + accents.
	brandDeep   color.Color // primary; focus borders; user bar
	brandLight  color.Color // secondary; info
	accentFlash color.Color // flash model + tool bar
	accentPro   color.Color // pro model + tool bar

	// Selection pairing — the focused row in every picker / palette / popup.
	// Kept SEPARATE from brand so the band can be a calm, recessive surface
	// (deep indigo, not the bright brand) carrying a high-luminance text. A
	// full-bleed brand bar with near-black text reads as a harsh, vibrating
	// slab; a deep band with bright text reads as a confident, classy highlight.
	selBg color.Color // focused-row background (a deep, low-chroma indigo)
	selFg color.Color // focused-row foreground (a bright, cool near-white)

	// Background tiers (canvas → raised).
	bgBase    color.Color // painted canvas
	bgWell    color.Color // code/diff inset
	bgSurface color.Color // tool/reasoning panels
	bgRaised  color.Color // modals, selected rows

	// Foreground ramp (base → faint).
	fgBase   color.Color
	fgMuted  color.Color
	fgSubtle color.Color // == today's Dim
	fgFaint  color.Color // meta, line numbers, hints

	// Lines + semantics.
	border color.Color
	ok     color.Color
	errc   color.Color
	warn   color.Color

	// Text drawn on top of a filled badge / selection.
	onAccent color.Color
}

// oceanPalette is the default dark identity — "DeepSeek Ocean". It is a
// deliberately COOL, low-chroma scheme: one azure brand, two desaturated
// analogous accents (cyan/violet that read as tints of the same family, not
// competing primaries), and neutrals on a faintly blue-tinted gray axis. Full
// chroma is reserved for tiny semantic chips (ok/warn/err). The old palette's
// flaw was three high-saturation hues (royal blue, neon teal, hot violet) all
// fighting at once; restraint is what makes it read as coordinated.
var oceanPalette = palette{
	brandDeep:   lipgloss.Color("#4d7fe0"), // azure — gradients, cursor, user bar, headers
	brandLight:  lipgloss.Color("#8fc7ff"), // light azure — gradient end, scrollbar, info
	accentFlash: lipgloss.Color("#5cc9d6"), // muted cyan — flash model / tool bar / input border
	accentPro:   lipgloss.Color("#a98cf0"), // muted violet — pro model / tool bar

	// Calm deep-indigo band + bright cool-white text (see palette.selBg).
	selBg: lipgloss.Color("#2c4a7d"),
	selFg: lipgloss.Color("#eaf1fc"),

	bgBase:    lipgloss.Color("#0e151f"), // canvas — deep cool slate
	bgWell:    lipgloss.Color("#0a0f17"), // code/diff inset — deepest
	bgSurface: lipgloss.Color("#151d29"), // tool/reasoning panels
	bgRaised:  lipgloss.Color("#1b2636"), // modals/selected-row surface — recedes calmly

	fgBase:   lipgloss.Color("#e6ebf2"),
	fgMuted:  lipgloss.Color("#a2b1c4"),
	fgSubtle: lipgloss.Color("#717f92"),
	fgFaint:  lipgloss.Color("#5c6b7e"), // raised vs old #4a5663 so notes read on bgRaised

	border: lipgloss.Color("#29384f"), // a touch brighter so rounded modal edges read
	ok:     lipgloss.Color("#5ad48a"), // softer green (was neon #5fd75f)
	errc:   lipgloss.Color("#ef6a72"), // softer red
	warn:   lipgloss.Color("#e0b24a"), // warm amber (was acid yellow #d7d75f)

	onAccent: lipgloss.Color("#0b1119"), // dark text on filled badges
}

var lightPalette = palette{
	brandDeep:   lipgloss.Color("#1D4ED8"),
	brandLight:  lipgloss.Color("#0087af"),
	accentFlash: lipgloss.Color("#0087af"),
	accentPro:   lipgloss.Color("#7c3aed"),

	// On a light canvas a saturated band with white text is the expected,
	// legible "selected" look — softened a step from the brand royal.
	selBg: lipgloss.Color("#2563eb"),
	selFg: lipgloss.Color("#ffffff"),

	bgBase:    lipgloss.Color("#ffffff"),
	bgWell:    lipgloss.Color("#eef1f5"),
	bgSurface: lipgloss.Color("#f3f5f8"),
	bgRaised:  lipgloss.Color("#e8edf3"),

	fgBase:   lipgloss.Color("#1c1c1c"),
	fgMuted:  lipgloss.Color("#4a5663"),
	fgSubtle: lipgloss.Color("#878787"),
	fgFaint:  lipgloss.Color("#aab2bd"),

	border: lipgloss.Color("#d4dae2"),
	ok:     lipgloss.Color("#008f00"),
	errc:   lipgloss.Color("#c0392b"),
	warn:   lipgloss.Color("#9a6700"),

	onAccent: lipgloss.Color("#ffffff"),
}

// Diff band colors are HARD-CODED per mode (not palette tokens). They are
// the one sanctioned exception to "no raw hex at call sites": expose them
// as Theme fields so components read theme.DiffAdd* / theme.DiffDel*.
type diffBands struct {
	addFg, addBg color.Color
	delFg, delBg color.Color
}

var oceanDiff = diffBands{
	addFg: lipgloss.Color("#7ee787"), addBg: lipgloss.Color("#12301f"),
	delFg: lipgloss.Color("#ff7b72"), delBg: lipgloss.Color("#2d1517"),
}

var lightDiff = diffBands{
	addFg: lipgloss.Color("#22863a"), addBg: lipgloss.Color("#e6ffec"),
	delFg: lipgloss.Color("#b31d28"), delBg: lipgloss.Color("#ffeef0"),
}

// Theme holds the lipgloss styles used across the TUI, composed from a raw
// palette. We ship a dark theme as default and a light alt. Theme is
// selected at startup from config.
//
// The layout is two-layer (crush-style): the unexported palette holds raw
// tokens; the exported fields below are semantic styles/colors derived from
// them. New components should prefer the token colors + the Panel/Badge/
// LeftBar/Gutter helpers over the legacy named styles.
type Theme struct {
	Name string

	// transparent disables canvas painting + bg-tier fills (config opt-out).
	// truecolor is false on terminals that can't render 24-bit color, which
	// also degrades fills. Both gate Panel().
	transparent bool
	truecolor   bool

	// --- Raw token colors, exported for call sites (never hex inline). ---
	BrandDeep   color.Color
	BrandLight  color.Color
	AccentFlash color.Color
	AccentPro   color.Color

	// Selection pairing — the focused-row band color + its text (see selBg).
	SelBg color.Color
	SelFg color.Color

	BgBase    color.Color
	BgWell    color.Color
	BgSurface color.Color
	BgRaised  color.Color

	FgBase   color.Color
	FgMuted  color.Color
	FgSubtle color.Color
	FgFaint  color.Color

	BorderColor color.Color
	OkColor     color.Color
	ErrColor    color.Color
	WarnColor   color.Color
	OnAccent    color.Color

	// Diff band colors (hard-coded per mode — see diffBands).
	DiffAddFg color.Color
	DiffAddBg color.Color
	DiffDelFg color.Color
	DiffDelBg color.Color

	// --- Legacy semantic styles (re-derived from tokens). ---
	UserPrompt    lipgloss.Style
	AssistantText lipgloss.Style
	Reasoning     lipgloss.Style
	ReasoningFold lipgloss.Style

	ToolCall lipgloss.Style
	ToolOk   lipgloss.Style
	ToolErr  lipgloss.Style
	ToolBody lipgloss.Style

	HookInfo lipgloss.Style
	HookDeny lipgloss.Style

	Repair lipgloss.Style

	Status      lipgloss.Style
	StatusModel lipgloss.Style
	StatusGood  lipgloss.Style
	StatusBad   lipgloss.Style

	Info  lipgloss.Style
	Error lipgloss.Style
	Hint  lipgloss.Style

	InputBorder    lipgloss.Style
	InputBorderDim lipgloss.Style // Normal mode: dim border around textarea
	PermPrompt     lipgloss.Style
	PermFocus      lipgloss.Style // focused permission-card button (inverted)
	PermButton     lipgloss.Style // unfocused permission-card button

	// Ocean visual identity — brand gradient endpoints + card styles.
	CardBar    lipgloss.Style // left sidebar bar (foreground BrandDeep)
	CardHeader lipgloss.Style
	CardBody   lipgloss.Style

	// pal/diff retained so helper methods can reach raw tokens without a
	// dozen extra exported fields.
	pal  palette
	diff diffBands
}

// Panel returns a lipgloss.Style backgrounded with the given tier's surface
// color. It returns NO background (a plain foreground-only style) when the
// Theme is in transparent mode OR the terminal is not truecolor — callers
// then degrade to left-bars / separators rather than opaque fills.
func (t Theme) Panel(tier PanelTier) lipgloss.Style {
	base := lipgloss.NewStyle().Foreground(t.pal.fgBase)
	if !t.fillsEnabled() {
		return base
	}
	var bg color.Color
	switch tier {
	case TierWell:
		bg = t.pal.bgWell
	case TierSurface:
		bg = t.pal.bgSurface
	case TierRaised:
		bg = t.pal.bgRaised
	default:
		bg = t.pal.bgBase
	}
	return base.Background(bg)
}

// Badge returns a filled chip style: background = the semantic color for the
// kind, foreground = onAccent, Padding(0,1), Bold. Unlike Panel, a badge is
// a deliberate accent and stays filled even in transparent / degraded modes
// (it's a small chip, not a surface) so status semantics survive.
func (t Theme) Badge(kind BadgeKind) lipgloss.Style {
	var bg color.Color
	switch kind {
	case BadgeErr:
		bg = t.pal.errc
	case BadgeWarn:
		bg = t.pal.warn
	case BadgeInfo:
		bg = t.pal.brandLight
	case BadgeBrand:
		bg = t.pal.brandDeep
	default:
		bg = t.pal.ok
	}
	return lipgloss.NewStyle().
		Background(bg).
		Foreground(t.pal.onAccent).
		Padding(0, 1).
		Bold(true)
}

// LeftBar returns the role-bar style: the thick bar glyph rendered in the
// given color. Callers render the glyph via this style (e.g.
// theme.LeftBar(theme.BrandDeep).Render(tui.LeftBarGlyph())).
func (t Theme) LeftBar(c color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c)
}

// LeftBarGlyph returns the thick bar glyph the LeftBar style is meant to
// render. Exposed so components don't re-declare the rune.
func LeftBarGlyph() string { return leftBarGlyph }

// Gutter returns the 2-cell left gutter string prepended to every chat item.
func (t Theme) Gutter() string { return Gutter }

// fillsEnabled reports whether opaque background fills should be drawn: only
// when the canvas is owned (not transparent) AND the terminal is truecolor.
// When false, surfaces degrade to foreground-only (left-bars / separators) per
// ADR-0002. Panel() and the markdown code-block repaint both gate on this so
// the degrade contract is enforced in exactly one place.
func (t Theme) fillsEnabled() bool { return !t.transparent && t.truecolor }

// Transparent reports whether canvas painting / bg fills are disabled.
func (t Theme) Transparent() bool { return t.transparent }

// Truecolor reports whether the terminal can render 24-bit color.
func (t Theme) Truecolor() bool { return t.truecolor }

// WithTransparent returns a copy of the theme with the transparent flag set.
// Used to thread the ui.transparent_background config opt-out into the theme.
func (t Theme) WithTransparent(v bool) Theme {
	t.transparent = v
	return t
}

// WithTruecolor returns a copy of the theme with the truecolor flag set.
// Used to thread color-profile detection into the theme.
func (t Theme) WithTruecolor(v bool) Theme {
	t.truecolor = v
	return t
}

// buildTheme expands a raw palette (+ its diff bands) into a full Theme.
// truecolor defaults to true so freshly-built themes paint; callers lower
// it via WithTruecolor after profile detection.
func buildTheme(name string, p palette, d diffBands) Theme {
	return Theme{
		Name:      name,
		truecolor: true,

		BrandDeep:   p.brandDeep,
		BrandLight:  p.brandLight,
		AccentFlash: p.accentFlash,
		AccentPro:   p.accentPro,

		SelBg: p.selBg,
		SelFg: p.selFg,

		BgBase:    p.bgBase,
		BgWell:    p.bgWell,
		BgSurface: p.bgSurface,
		BgRaised:  p.bgRaised,

		FgBase:   p.fgBase,
		FgMuted:  p.fgMuted,
		FgSubtle: p.fgSubtle,
		FgFaint:  p.fgFaint,

		BorderColor: p.border,
		OkColor:     p.ok,
		ErrColor:    p.errc,
		WarnColor:   p.warn,
		OnAccent:    p.onAccent,

		DiffAddFg: d.addFg,
		DiffAddBg: d.addBg,
		DiffDelFg: d.delFg,
		DiffDelBg: d.delBg,

		// Legacy semantic styles, re-derived from tokens.
		UserPrompt:     lipgloss.NewStyle().Foreground(p.fgBase).Bold(true),
		AssistantText:  lipgloss.NewStyle().Foreground(p.fgBase),
		Reasoning:      lipgloss.NewStyle().Foreground(p.fgSubtle).Italic(true),
		ReasoningFold:  lipgloss.NewStyle().Foreground(p.fgSubtle),
		ToolCall:       lipgloss.NewStyle().Foreground(p.accentFlash),
		ToolOk:         lipgloss.NewStyle().Foreground(p.ok),
		ToolErr:        lipgloss.NewStyle().Foreground(p.errc),
		ToolBody:       lipgloss.NewStyle().Foreground(p.fgSubtle),
		HookInfo:       lipgloss.NewStyle().Foreground(p.warn),
		HookDeny:       lipgloss.NewStyle().Foreground(p.errc).Bold(true),
		Repair:         lipgloss.NewStyle().Foreground(p.fgSubtle),
		Status:         lipgloss.NewStyle().Foreground(p.fgSubtle),
		StatusModel:    lipgloss.NewStyle().Foreground(p.accentFlash).Bold(true),
		StatusGood:     lipgloss.NewStyle().Foreground(p.ok),
		StatusBad:      lipgloss.NewStyle().Foreground(p.errc),
		Info:           lipgloss.NewStyle().Foreground(p.warn),
		Error:          lipgloss.NewStyle().Foreground(p.errc).Bold(true),
		Hint:           lipgloss.NewStyle().Foreground(p.fgFaint),
		InputBorder:    lipgloss.NewStyle().Foreground(p.accentFlash),
		InputBorderDim: lipgloss.NewStyle().Foreground(p.fgSubtle),
		PermPrompt:     lipgloss.NewStyle().Foreground(p.warn).Bold(true),
		PermFocus: lipgloss.NewStyle().
			Background(p.warn).
			Foreground(p.onAccent).
			Bold(true),
		PermButton: lipgloss.NewStyle().Foreground(p.fgBase),

		// Ocean identity.
		CardBar:    lipgloss.NewStyle().Foreground(p.brandDeep),
		CardHeader: lipgloss.NewStyle().Foreground(p.fgBase).Bold(true),
		CardBody:   lipgloss.NewStyle().Foreground(p.fgSubtle),

		pal:  p,
		diff: d,
	}
}

// DarkTheme returns the default dark theme (DeepSeek Ocean).
func DarkTheme() Theme { return buildTheme("dark", oceanPalette, oceanDiff) }

// LightTheme returns the light alt. Same semantics, inverted contrast.
func LightTheme() Theme { return buildTheme("light", lightPalette, lightDiff) }

// PickTheme returns the configured theme by name; defaults to dark on
// unknown names. truecolor is detected from the environment so themes built
// here degrade fills automatically on limited terminals; callers may still
// override via WithTransparent (config opt-out).
func PickTheme(name string) Theme {
	var t Theme
	switch name {
	case "light":
		t = LightTheme()
	default:
		t = DarkTheme()
	}
	return t.WithTruecolor(detectTruecolor())
}

// detectTruecolor reports whether the terminal advertises 24-bit color.
// It uses a minimal COLORTERM env check (truecolor / 24bit) to avoid
// promoting colorprofile from an indirect to a direct dependency. Absent or
// unrecognized COLORTERM is treated as truecolor-capable: modern terminals
// overwhelmingly support it, and over-painting on a rare 256-color terminal
// is far less jarring than refusing to paint on a capable one. The opt-out
// for users who hit artifacts is ui.transparent_background.
func detectTruecolor() bool {
	ct := strings.ToLower(os.Getenv("COLORTERM"))
	if strings.Contains(ct, "truecolor") || strings.Contains(ct, "24bit") {
		return true
	}
	// Explicit downgrade signals: a non-empty COLORTERM that is NOT a
	// truecolor marker (e.g. "256color") means the terminal told us it is
	// limited — honor that and degrade fills.
	if ct != "" {
		return false
	}
	// No signal at all → assume capable (see doc comment).
	return true
}
