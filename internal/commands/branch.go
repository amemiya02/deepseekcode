package commands

import "fmt"

// parseOneArgCommand parses a slash-command that takes exactly one argument,
// which may be a quoted multi-word string. Tokenize handles quote stripping.
// Returns an error when the argument is absent or when more than one argument
// is supplied without quotes.
func parseOneArgCommand(input, cmdName string) (string, error) {
	tokens := Tokenize(input)
	// tokens[0] is the command token (e.g. "/checkpoint"); args follow.
	if len(tokens) < 2 {
		return "", fmt.Errorf("%s requires a name: %s <name>", cmdName, cmdName)
	}
	if len(tokens) > 2 {
		return "", fmt.Errorf("%s takes exactly one argument; quote multi-word names: %s \"my name\"", cmdName, cmdName)
	}
	return tokens[1], nil
}

// parseCheckpointArgs extracts the checkpoint name from "/checkpoint <name>".
func parseCheckpointArgs(input string) (string, error) {
	return parseOneArgCommand(input, "/checkpoint")
}

// parseBranchArgs extracts the turn/checkpoint identifier from "/branch <name|turn>".
func parseBranchArgs(input string) (string, error) {
	return parseOneArgCommand(input, "/branch")
}
