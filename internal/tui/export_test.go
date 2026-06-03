package tui

// ExportedRenderWelcome exposes renderWelcome for package-external tests.
func ExportedRenderWelcome(t Theme, width int) string {
	return renderWelcome(t, width)
}
