package commands

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	placeholderRe = regexp.MustCompile(`\$(\d+)`)
	bashRe        = regexp.MustCompile("!`([^`]+)`")
	fileRe        = regexp.MustCompile(`(?:^|[^\w` + "`" + `])@([^\s` + "`" + `]+)`)
)

// ShellRunner executes !`cmd` injections, returning stdout (trimmed).
type ShellRunner func(ctx context.Context, cmd string) (string, error)

// Expand interpolates a command template with the given arguments.
// Order: $N → $ARGUMENTS → append args if no placeholders → !`cmd` → @file.
func Expand(ctx context.Context, tmpl string, args []string,
	runner ShellRunner, readFile func(string) (string, error)) (string, error) {

	result := tmpl

	// 1. $N positional arguments.
	matches := placeholderRe.FindAllStringSubmatch(tmpl, -1)
	if len(matches) > 0 {
		last := 0
		for _, m := range matches {
			n, _ := strconv.Atoi(m[1])
			if n > last {
				last = n
			}
		}
		result = placeholderRe.ReplaceAllStringFunc(result, func(match string) string {
			n, _ := strconv.Atoi(match[1:])
			if n < 1 || n > len(args) {
				return ""
			}
			if n == last && n < len(args) {
				return strings.Join(args[n-1:], " ")
			}
			return args[n-1]
		})
	}

	// 2. $ARGUMENTS.
	hasArgPlaceholder := len(matches) > 0 || strings.Contains(tmpl, "$ARGUMENTS")
	result = strings.ReplaceAll(result, "$ARGUMENTS", strings.Join(args, " "))

	// 3. If no placeholders and args non-empty, append to new line.
	if !hasArgPlaceholder && len(args) > 0 {
		result = result + "\n" + strings.Join(args, " ")
	}

	// 4. !`cmd` shell injection.
	if runner != nil {
		result = bashRe.ReplaceAllStringFunc(result, func(match string) string {
			cmd := bashRe.FindStringSubmatch(match)[1]
			out, err := runner(ctx, cmd)
			if err != nil {
				return fmt.Sprintf("[command failed: %v]", err)
			}
			return out
		})
	}

	// 5. @file expansion.
	if readFile != nil {
		idxs := fileRe.FindAllStringSubmatchIndex(result, -1)
		if len(idxs) > 0 {
			var sb strings.Builder
			last := 0
			for _, loc := range idxs {
				atIdx := loc[2] - 1 // position of "@"
				sb.WriteString(result[last:atIdx])
				path := result[loc[2]:loc[3]]
				content, err := readFile(path)
				if err != nil {
					sb.WriteString(result[atIdx:loc[1]]) // preserve "@path"
				} else {
					sb.WriteString(content)
				}
				last = loc[1]
			}
			sb.WriteString(result[last:])
			result = sb.String()
		}
	}

	return result, nil
}
