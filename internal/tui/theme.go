package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Theme holds the lipgloss styles used across the TUI. We ship a dark
// theme as default and a light alt. Theme is selected at startup from
// config; mid-session switching is a v0.2 feature.
type Theme struct {
	Name string

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
	BrandDeep  color.Color    // #1D4ED8
	BrandLight color.Color    // #7DD3FC
	CardBar    lipgloss.Style // left sidebar bar (foreground BrandDeep)
	CardHeader lipgloss.Style
	CardBody   lipgloss.Style
}

// palette holds the semantic color tokens that buildTheme expands into
// a full Theme.
type palette struct {
	brandDeep, brandLight color.Color
	flash, pro            color.Color
	fg, bg                color.Color
	dim                   color.Color
	success, errc, warn   color.Color
}

var oceanPalette = palette{
	brandDeep:  lipgloss.Color("#1D4ED8"),
	brandLight: lipgloss.Color("#7DD3FC"),
	flash:      lipgloss.Color("#5fd7d7"),
	pro:        lipgloss.Color("#c77dff"),
	fg:         lipgloss.Color("#e4e4e4"),
	bg:         lipgloss.Color("#0d1b2a"),
	dim:        lipgloss.Color("#6c7a89"),
	success:    lipgloss.Color("#5fd75f"),
	errc:       lipgloss.Color("#ff5f5f"),
	warn:       lipgloss.Color("#d7d75f"),
}

var lightPalette = palette{
	brandDeep:  lipgloss.Color("#1D4ED8"),
	brandLight: lipgloss.Color("#7DD3FC"),
	flash:      lipgloss.Color("#0087af"),
	pro:        lipgloss.Color("#7c3aed"),
	fg:         lipgloss.Color("#1c1c1c"),
	bg:         lipgloss.Color("#ffffff"),
	dim:        lipgloss.Color("#878787"),
	success:    lipgloss.Color("#005f00"),
	errc:       lipgloss.Color("#af0000"),
	warn:       lipgloss.Color("#875f00"),
}

// buildTheme expands a semantic palette into a full Theme.
func buildTheme(name string, p palette) Theme {
	return Theme{
		Name:           name,
		UserPrompt:     lipgloss.NewStyle().Foreground(p.fg).Bold(true),
		AssistantText:  lipgloss.NewStyle().Foreground(p.fg),
		Reasoning:      lipgloss.NewStyle().Foreground(p.dim).Italic(true),
		ReasoningFold:  lipgloss.NewStyle().Foreground(p.dim),
		ToolCall:       lipgloss.NewStyle().Foreground(p.flash),
		ToolOk:         lipgloss.NewStyle().Foreground(p.success),
		ToolErr:        lipgloss.NewStyle().Foreground(p.errc),
		ToolBody:       lipgloss.NewStyle().Foreground(p.dim),
		HookInfo:       lipgloss.NewStyle().Foreground(p.warn),
		HookDeny:       lipgloss.NewStyle().Foreground(p.errc).Bold(true),
		Status:         lipgloss.NewStyle().Foreground(p.dim),
		StatusModel:    lipgloss.NewStyle().Foreground(p.flash).Bold(true),
		StatusGood:     lipgloss.NewStyle().Foreground(p.success),
		StatusBad:      lipgloss.NewStyle().Foreground(p.errc),
		Info:           lipgloss.NewStyle().Foreground(p.warn),
		Error:          lipgloss.NewStyle().Foreground(p.errc).Bold(true),
		Hint:           lipgloss.NewStyle().Foreground(p.dim),
		InputBorder:    lipgloss.NewStyle().Foreground(p.flash),
		InputBorderDim: lipgloss.NewStyle().Foreground(p.dim),
		PermPrompt:     lipgloss.NewStyle().Foreground(p.warn).Bold(true),
		PermFocus: lipgloss.NewStyle().
			Background(p.warn).
			Foreground(lipgloss.Color("#1c1c1c")).
			Bold(true),
		PermButton: lipgloss.NewStyle().Foreground(p.fg),

		// Ocean identity.
		BrandDeep:  p.brandDeep,
		BrandLight: p.brandLight,
		CardBar:    lipgloss.NewStyle().Foreground(p.brandDeep),
		CardHeader: lipgloss.NewStyle().Foreground(p.fg).Bold(true),
		CardBody:   lipgloss.NewStyle().Foreground(p.dim),
	}
}

// DarkTheme returns the default dark theme (DeepSeek Ocean).
func DarkTheme() Theme { return buildTheme("dark", oceanPalette) }

// LightTheme returns the light alt. Same semantics, inverted contrast.
func LightTheme() Theme { return buildTheme("light", lightPalette) }

// PickTheme returns the configured theme by name; defaults to dark on
// unknown names.
func PickTheme(name string) Theme {
	switch name {
	case "light":
		return LightTheme()
	default:
		return DarkTheme()
	}
}
