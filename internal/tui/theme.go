package tui

import "github.com/charmbracelet/lipgloss"

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

	DuetApprove lipgloss.Style
	DuetBlock   lipgloss.Style

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
}

// DarkTheme returns the default dark theme. Flash voice = cyan,
// Pro voice = magenta, per design doc §10.5.
func DarkTheme() Theme {
	cyan := lipgloss.Color("#5fd7d7")    // flash accent
	magenta := lipgloss.Color("#d75fd7") // pro accent
	dim := lipgloss.Color("#6c6c6c")
	red := lipgloss.Color("#ff5f5f")
	green := lipgloss.Color("#5fd75f")
	yellow := lipgloss.Color("#d7d75f")
	white := lipgloss.Color("#e4e4e4")

	return Theme{
		Name:           "dark",
		UserPrompt:     lipgloss.NewStyle().Foreground(white).Bold(true),
		AssistantText:  lipgloss.NewStyle().Foreground(white),
		Reasoning:      lipgloss.NewStyle().Foreground(dim).Italic(true),
		ReasoningFold:  lipgloss.NewStyle().Foreground(dim),
		ToolCall:       lipgloss.NewStyle().Foreground(cyan),
		ToolOk:         lipgloss.NewStyle().Foreground(green),
		ToolErr:        lipgloss.NewStyle().Foreground(red),
		ToolBody:       lipgloss.NewStyle().Foreground(dim),
		DuetApprove:    lipgloss.NewStyle().Foreground(magenta),
		DuetBlock:      lipgloss.NewStyle().Foreground(magenta).Bold(true),
		HookInfo:       lipgloss.NewStyle().Foreground(yellow),
		HookDeny:       lipgloss.NewStyle().Foreground(red).Bold(true),
		Status:         lipgloss.NewStyle().Foreground(dim),
		StatusModel:    lipgloss.NewStyle().Foreground(cyan).Bold(true),
		StatusGood:     lipgloss.NewStyle().Foreground(green),
		StatusBad:      lipgloss.NewStyle().Foreground(red),
		Info:           lipgloss.NewStyle().Foreground(yellow),
		Error:          lipgloss.NewStyle().Foreground(red).Bold(true),
		Hint:           lipgloss.NewStyle().Foreground(dim),
		InputBorder:    lipgloss.NewStyle().Foreground(cyan),
		InputBorderDim: lipgloss.NewStyle().Foreground(dim),
		PermPrompt:     lipgloss.NewStyle().Foreground(yellow).Bold(true),
		PermFocus: lipgloss.NewStyle().
			Background(yellow).
			Foreground(lipgloss.Color("#1c1c1c")).
			Bold(true),
		PermButton: lipgloss.NewStyle().Foreground(white),
	}
}

// LightTheme returns the light alt. Same semantics, inverted contrast.
func LightTheme() Theme {
	cyan := lipgloss.Color("#0087af")
	magenta := lipgloss.Color("#af00af")
	dim := lipgloss.Color("#878787")
	red := lipgloss.Color("#af0000")
	green := lipgloss.Color("#005f00")
	yellow := lipgloss.Color("#875f00")
	black := lipgloss.Color("#1c1c1c")

	return Theme{
		Name:           "light",
		UserPrompt:     lipgloss.NewStyle().Foreground(black).Bold(true),
		AssistantText:  lipgloss.NewStyle().Foreground(black),
		Reasoning:      lipgloss.NewStyle().Foreground(dim).Italic(true),
		ReasoningFold:  lipgloss.NewStyle().Foreground(dim),
		ToolCall:       lipgloss.NewStyle().Foreground(cyan),
		ToolOk:         lipgloss.NewStyle().Foreground(green),
		ToolErr:        lipgloss.NewStyle().Foreground(red),
		ToolBody:       lipgloss.NewStyle().Foreground(dim),
		DuetApprove:    lipgloss.NewStyle().Foreground(magenta),
		DuetBlock:      lipgloss.NewStyle().Foreground(magenta).Bold(true),
		HookInfo:       lipgloss.NewStyle().Foreground(yellow),
		HookDeny:       lipgloss.NewStyle().Foreground(red).Bold(true),
		Status:         lipgloss.NewStyle().Foreground(dim),
		StatusModel:    lipgloss.NewStyle().Foreground(cyan).Bold(true),
		StatusGood:     lipgloss.NewStyle().Foreground(green),
		StatusBad:      lipgloss.NewStyle().Foreground(red),
		Info:           lipgloss.NewStyle().Foreground(yellow),
		Error:          lipgloss.NewStyle().Foreground(red).Bold(true),
		Hint:           lipgloss.NewStyle().Foreground(dim),
		InputBorder:    lipgloss.NewStyle().Foreground(cyan),
		InputBorderDim: lipgloss.NewStyle().Foreground(dim),
		PermPrompt:     lipgloss.NewStyle().Foreground(yellow).Bold(true),
		PermFocus: lipgloss.NewStyle().
			Background(yellow).
			Foreground(lipgloss.Color("#ffffff")).
			Bold(true),
		PermButton: lipgloss.NewStyle().Foreground(black),
	}
}

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
