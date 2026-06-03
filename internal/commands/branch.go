package commands

import (
	"errors"
	"strings"
)

// parseCheckpointArgs extracts the checkpoint name from "/checkpoint <name>".
func parseCheckpointArgs(input string) (string, error) {
	parts := strings.Fields(input)
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		return "", errors.New("/checkpoint requires a name: /checkpoint <name>")
	}
	return parts[1], nil
}

// parseBranchArgs extracts the turn/checkpoint identifier from "/branch <name|turn>".
func parseBranchArgs(input string) (string, error) {
	parts := strings.Fields(input)
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		return "", errors.New("/branch requires a name or turn number: /branch <name|turn>")
	}
	return parts[1], nil
}
