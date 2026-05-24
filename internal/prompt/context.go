package prompt

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// DiscoverProjectContext fills the static-per-session fields of a
// ProjectContext: cwd, current date (UTC), OS name and version,
// and shell. Git fields are deliberately left empty — they are
// per-turn and the agent refreshes them via gitctx.
func DiscoverProjectContext(cwd string) ProjectContext {
	return ProjectContext{
		CWD:         cwd,
		CurrentDate: time.Now().UTC().Format("2006-01-02"),
		OSName:      runtime.GOOS,
		OSVersion:   readOSVersion(),
		Shell:       readShell(),
	}
}

func readOSVersion() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	default:
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "uname", "-r").Output()
		if err != nil {
			return "unknown"
		}
		v := strings.TrimSpace(string(out))
		if v == "" {
			return "unknown"
		}
		return v
	}
}

func readShell() string {
	if s := strings.TrimSpace(os.Getenv("SHELL")); s != "" {
		return s
	}
	return "unknown"
}
