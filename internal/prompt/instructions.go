package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxInstructionFileChars  = 4000
	maxTotalInstructionChars = 12000
)

// allowedInstructionFiles is the strict load whitelist. Order =
// priority. cwd entries always beat home entries with the same base
// name. CLAUDE.md is intentionally absent — see rejectClaudeNames.
var allowedInstructionFiles = []string{
	"DEEPSEEK.md",
	"AGENTS.md",
	".deepseek/instructions.md",
}

// LoadInstructionFiles scans cwd and home for whitelist files and
// returns them in priority order (cwd before home; within each base,
// declaration order). Same base name in both locations: cwd wins,
// home copy is skipped.
//
// Hard limits:
//   - Per file: maxInstructionFileChars (truncated with marker)
//   - Total: maxTotalInstructionChars (later files dropped when full)
//
// Safety:
//   - Any base name containing "claude" (case-insensitive) is
//     dropped even if a future edit erroneously adds it to the
//     whitelist. CLAUDE.md MUST NOT load.
//   - Missing files and read errors return no error — instructions
//     are advisory.
func LoadInstructionFiles(cwd, home string) ([]InstructionFile, error) {
	if cwd == "" {
		return nil, fmt.Errorf("LoadInstructionFiles: empty cwd")
	}
	roots := []string{cwd}
	// Skip duplicate root when the user runs dsc from their home dir.
	if home != "" && home != cwd {
		roots = append(roots, home)
	}

	seenBase := map[string]bool{}
	var out []InstructionFile
	total := 0
	for _, root := range roots {
		for _, rel := range allowedInstructionFiles {
			base := filepath.Base(rel)
			if rejectClaudeNames(base) || rejectClaudeNames(rel) {
				continue
			}
			if seenBase[base] {
				continue
			}
			full := filepath.Join(root, rel)
			content, err := os.ReadFile(full)
			if err != nil {
				continue
			}
			if len(content) == 0 {
				continue
			}
			text := string(content)
			if len(text) > maxInstructionFileChars {
				text = text[:maxInstructionFileChars] + "\n... [truncated]"
			}
			remaining := maxTotalInstructionChars - total
			if remaining <= 0 {
				return out, nil
			}
			if len(text) > remaining {
				text = text[:remaining] + "\n... [truncated]"
			}
			out = append(out, InstructionFile{Path: full, Content: text})
			total += len(text)
			seenBase[base] = true
		}
	}
	return out, nil
}

// rejectClaudeNames returns true for any path or base name that
// contains "claude" (case-insensitive). Hard guard so the user's
// Claude Code instructions cannot leak into a DeepSeek session.
func rejectClaudeNames(name string) bool {
	return strings.Contains(strings.ToLower(name), "claude")
}
