package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/amemiya02/deepseekcode/internal/sandbox"
)

func sandboxProfileWithCWD(profile sandbox.Profile, cwd string) sandbox.Profile {
	out := profile
	out.AllowReadPaths = append([]string(nil), profile.AllowReadPaths...)
	out.AllowWritePaths = append([]string(nil), profile.AllowWritePaths...)
	out.AllowExecPaths = append([]string(nil), profile.AllowExecPaths...)

	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	if cwd != "" {
		if abs, err := filepath.Abs(cwd); err == nil {
			cwd = abs
		}
		out.AllowReadPaths = appendUniquePath(out.AllowReadPaths, cwd)
		out.AllowWritePaths = appendUniquePath(out.AllowWritePaths, cwd)
	}
	return out
}

func wrapWithSandbox(ctx context.Context, sb sandbox.Sandbox, profile sandbox.Profile, cmd *exec.Cmd) string {
	if sb == nil {
		return ""
	}
	if !sb.Available() {
		return "(sandbox unavailable; running unsandboxed)\n"
	}
	if err := sb.Wrap(ctx, profile, cmd); err != nil {
		return "(sandbox unavailable; running unsandboxed: " + err.Error() + ")\n"
	}
	return ""
}

func classifyBlocked(sb sandbox.Sandbox, output string) (blocked bool, body string) {
	if sb == nil || !sb.WasDenied(output) {
		return false, output
	}
	return true, "sandboxed: blocked by " + sb.Name() + "\n" + output
}

// ClassifySandboxBlocked is the exported form used by the agent package for
// background jobs, whose process runner lives outside internal/tools.
func ClassifySandboxBlocked(sb sandbox.Sandbox, output string) (blocked bool, body string) {
	return classifyBlocked(sb, output)
}

func appendUniquePath(paths []string, path string) []string {
	for _, existing := range paths {
		if existing == path {
			return paths
		}
	}
	return append(paths, path)
}
