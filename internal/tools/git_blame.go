package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// GitBlame returns per-line authorship for a file or range. We pass
// `--line-porcelain` and reformat to one line per source line:
//
//	pkg/auth/jwt.go:42  a3f1c2d Alice Bob   2025-09-01  return token, nil
//
// This is much easier for the model to consume than git's default
// blame output, which interleaves metadata blocks between source lines.
type GitBlame struct{}

func (GitBlame) Name() string { return "git_blame" }

func (GitBlame) Description() string {
	return "Show per-line authorship for a file or range, one line per source line. " +
		"Format: 'path:lineno SHA8 Author YYYY-MM-DD content'. " +
		"Optional range like '10,40' restricts to lines 10 through 40. " +
		"Use this to find who introduced a bug, when an invariant was added, " +
		"or to understand the history of a particular block."
}

func (GitBlame) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "File to blame.",
			},
			"range": map[string]any{
				"type":        "string",
				"description": "Optional line range, e.g. '10,40' or '1,+50'.",
			},
		},
		"required": []string{"path"},
	})
}

func (GitBlame) IsReadOnly() bool { return true }

func (GitBlame) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Path  string `json:"path"`
		Range string `json:"range"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("invalid args: %v", err), nil
	}
	if p.Path == "" {
		return Errf("path is required"), nil
	}
	cmdArgs := []string{"blame", "--line-porcelain"}
	if p.Range != "" {
		cmdArgs = append(cmdArgs, "-L", p.Range)
	}
	cmdArgs = append(cmdArgs, "--", p.Path)

	out, err := runGit(ctx, cmdArgs...)
	if err != nil {
		return Errf("git blame: %v", err), nil
	}
	return Result{Content: reformatBlame(out, p.Path)}, nil
}

// reformatBlame collapses --line-porcelain output into one line per
// source line. Porcelain emits an SHA header, then metadata lines
// (author, author-time, etc.), then a `\t` + source line.
func reformatBlame(porcelain, path string) string {
	var b strings.Builder
	lines := strings.Split(porcelain, "\n")
	var (
		sha    string
		author string
		date   string
		lineNo int
	)
	for _, ln := range lines {
		if ln == "" {
			continue
		}
		// SHA header lines look like: "abc1234... orig-lineno final-lineno [group-size]"
		if len(ln) >= 40 && ln[40] == ' ' {
			parts := strings.Fields(ln)
			if len(parts) >= 3 {
				sha = parts[0]
				if n, err := strconv.Atoi(parts[2]); err == nil {
					lineNo = n
				}
				continue
			}
		}
		if strings.HasPrefix(ln, "author ") {
			author = strings.TrimPrefix(ln, "author ")
			continue
		}
		if strings.HasPrefix(ln, "author-time ") {
			if t, err := strconv.ParseInt(strings.TrimPrefix(ln, "author-time "), 10, 64); err == nil {
				date = formatUnixDate(t)
			}
			continue
		}
		if strings.HasPrefix(ln, "\t") {
			content := strings.TrimPrefix(ln, "\t")
			short := sha
			if len(short) > 8 {
				short = short[:8]
			}
			fmt.Fprintf(&b, "%s:%d  %s  %-20s  %s  %s\n",
				path, lineNo, short, author, date, content)
		}
	}
	return b.String()
}

func formatUnixDate(t int64) string {
	// Minimal YYYY-MM-DD formatter to avoid pulling time in heavyweight ways.
	const day = 86400
	d := t / day
	// Days from Unix epoch (1970-01-01 = Thu).
	// Algorithm from Howard Hinnant's date library, simplified.
	y, m, dd := civilFromDays(int(d))
	return fmt.Sprintf("%04d-%02d-%02d", y, m, dd)
}

func civilFromDays(z int) (year, month, day int) {
	z += 719468
	era := z / 146097
	if z < 0 && z%146097 != 0 {
		era--
	}
	doe := z - era*146097
	yoe := (doe - doe/1460 + doe/36524 - doe/146096) / 365
	y := yoe + era*400
	doy := doe - (365*yoe + yoe/4 - yoe/100)
	mp := (5*doy + 2) / 153
	d := doy - (153*mp+2)/5 + 1
	mo := mp + 3
	if mo > 12 {
		mo -= 12
		y++
	}
	return y, mo, d
}
