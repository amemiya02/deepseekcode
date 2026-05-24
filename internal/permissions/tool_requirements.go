package permissions

// MinModeFor returns the minimum Mode required to execute a given tool.
// Default: ModeDefault (any mode — yolo, default, read-only, ask-all all
// permit the tool). Future versions may restrict dangerous tools to
// higher privilege levels.
func MinModeFor(toolName string) Mode {
	switch toolName {
	case "bash":
		return ModeDefault
	case "write_file":
		return ModeDefault
	case "edit_file":
		return ModeDefault
	default:
		return ModeDefault
	}
}
