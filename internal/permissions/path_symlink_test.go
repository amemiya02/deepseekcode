package permissions

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/tools"
)

// pathAwareTool is a fake write/edit tool that declares a fixed set of
// affected paths. It lets Decide exercise the PathAware classification block,
// which the plain fakeTool (no AffectedPaths) never reaches.
type pathAwareTool struct {
	name  string
	paths []string
}

func (t *pathAwareTool) Name() string                { return t.name }
func (t *pathAwareTool) Description() string         { return "path-aware fake" }
func (t *pathAwareTool) Parameters() json.RawMessage { return nil }
func (t *pathAwareTool) Execute(_ context.Context, _ json.RawMessage) (tools.Result, error) {
	return tools.Result{}, nil
}
func (t *pathAwareTool) IsReadOnly() bool                         { return false }
func (t *pathAwareTool) AffectedPaths(_ json.RawMessage) []string { return t.paths }

// mustSymlink creates a symlink, skipping the test gracefully if the platform
// or filesystem does not support symlinks.
func mustSymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlinks unsupported on this platform/filesystem: %v", err)
	}
}

// realCwd resolves the symlinks on a TempDir so the Policy's cwd anchor matches
// what tools.ResolveAndCheck resolves to. On macOS, t.TempDir() lives under
// /var -> /private/var, so the test must compare against the resolved form.
func realCwd(t *testing.T, dir string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", dir, err)
	}
	return resolved
}

// TestDecideSymlinkInCwdToOutsideDenied is the core acceptance test: a symlink
// that lives inside cwd but points outside it must NOT be auto-allowed as a
// "safe write inside cwd". The gate must agree with tools.ResolveAndCheck,
// which rejects the same path.
func TestDecideSymlinkInCwdToOutsideDenied(t *testing.T) {
	cwd := realCwd(t, t.TempDir())
	outside := realCwd(t, t.TempDir())

	// <cwd>/escape -> <outside>
	link := filepath.Join(cwd, "escape")
	mustSymlink(t, outside, link)

	// The write targets a file *through* the symlink, so it lands outside cwd.
	target := filepath.Join(link, "loot.txt")

	pol := New(ModeDefault, cwd, nil, nil, nil)
	dec, reason := pol.Decide(Check{
		Tool: &pathAwareTool{name: "write_file", paths: []string{target}},
		Args: json.RawMessage(`{}`),
	})
	if dec == Allow {
		t.Fatalf("symlink-in-cwd -> outside must not auto-allow; got Allow (%q)", reason)
	}
	if dec != Ask {
		t.Fatalf("symlink-in-cwd -> outside: got %v (%q), want Ask", dec, reason)
	}

	// Cross-check: the tool layer rejects the identical path. The gate must
	// not be more permissive than the layer that actually touches the file.
	if _, err := tools.ResolveAndCheck(target, cwd); err == nil {
		t.Fatal("expected tools.ResolveAndCheck to reject the escaping symlink; got nil error")
	}
}

// TestDecideLexicalEscapeStillAsks guards the pre-existing lexical behavior:
// an absolute path plainly outside cwd (no symlink) still asks.
func TestDecideLexicalEscapeStillAsks(t *testing.T) {
	cwd := realCwd(t, t.TempDir())
	outside := realCwd(t, t.TempDir())
	target := filepath.Join(outside, "loot.txt")

	pol := New(ModeDefault, cwd, nil, nil, nil)
	dec, reason := pol.Decide(Check{
		Tool: &pathAwareTool{name: "write_file", paths: []string{target}},
		Args: json.RawMessage(`{}`),
	})
	if dec != Ask {
		t.Fatalf("plain outside-cwd write: got %v (%q), want Ask", dec, reason)
	}
}

// TestDecideSymlinkIntoSecretAsks: a symlink that resolves into a .git
// directory which itself stays inside cwd must trip the secret classification
// (.git) rather than auto-allowing. This isolates matchesSecret from the
// outside-cwd check: the resolved path is still within cwd, so only the secret
// rule can catch it.
func TestDecideSymlinkIntoSecretAsks(t *testing.T) {
	cwd := realCwd(t, t.TempDir())

	// Real <cwd>/.git, and a benign-looking symlink <cwd>/gitlink -> <cwd>/.git.
	gitDir := filepath.Join(cwd, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	link := filepath.Join(cwd, "gitlink")
	mustSymlink(t, gitDir, link)

	// Lexically "<cwd>/gitlink/config" (in-cwd, no ".git" component);
	// resolved it is "<cwd>/.git/config" (secret).
	target := filepath.Join(link, "config")

	pol := New(ModeDefault, cwd, nil, nil, nil)
	dec, reason := pol.Decide(Check{
		Tool: &pathAwareTool{name: "write_file", paths: []string{target}},
		Args: json.RawMessage(`{}`),
	})
	if dec == Allow {
		t.Fatalf("symlink-into-.git must not auto-allow; got Allow (%q)", reason)
	}
	if dec != Ask {
		t.Fatalf("symlink-into-.git: got %v (%q), want Ask", dec, reason)
	}
	// Sanity: the path stays inside cwd, so it is the secret rule (not the
	// outside-cwd rule) that catches it.
	cwdReal, real, err := pol.resolveAffected(target)
	if err != nil {
		t.Fatalf("resolveAffected: %v", err)
	}
	if !withinCwd(cwdReal, real) {
		t.Fatalf("expected resolved %q to stay within cwd %q", real, cwdReal)
	}
}

// TestDecideSymlinkIntoSecretPatternAsks: a symlink resolving to a file whose
// basename matches a configured secret pattern asks, even though the lexical
// (symlink) name does not match the pattern.
func TestDecideSymlinkIntoSecretPatternAsks(t *testing.T) {
	cwd := realCwd(t, t.TempDir())
	secretHome := realCwd(t, t.TempDir())

	secretFile := filepath.Join(secretHome, ".env")
	if err := os.WriteFile(secretFile, []byte("KEY=1"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	// Innocuous-looking name inside cwd that actually points at the secret.
	link := filepath.Join(cwd, "settings.txt")
	mustSymlink(t, secretFile, link)

	pol := New(ModeDefault, cwd, []string{".env"}, nil, nil)
	dec, reason := pol.Decide(Check{
		Tool: &pathAwareTool{name: "write_file", paths: []string{link}},
		Args: json.RawMessage(`{}`),
	})
	if dec != Ask {
		t.Fatalf("symlink -> secret pattern file: got %v (%q), want Ask", dec, reason)
	}
}

// TestDecideOrdinaryInCwdWriteAllows confirms the common case still
// auto-allows: a plain new file inside cwd, no symlinks involved.
func TestDecideOrdinaryInCwdWriteAllows(t *testing.T) {
	cwd := realCwd(t, t.TempDir())
	target := filepath.Join(cwd, "src", "main.go") // parent need not exist yet

	pol := New(ModeDefault, cwd, nil, nil, nil)
	dec, reason := pol.Decide(Check{
		Tool: &pathAwareTool{name: "write_file", paths: []string{target}},
		Args: json.RawMessage(`{}`),
	})
	if dec != Allow {
		t.Fatalf("ordinary in-cwd write: got %v (%q), want Allow", dec, reason)
	}

	// And the tool layer agrees it is in-bounds.
	if _, err := tools.ResolveAndCheck(target, cwd); err != nil {
		t.Fatalf("tools.ResolveAndCheck rejected an in-cwd write: %v", err)
	}
}

// TestDecideInCwdViaSymlinkedDirAllows confirms that a symlink which stays
// inside cwd (a sibling dir under the same resolved cwd) still auto-allows,
// matching ResolveAndCheck. This guards against an over-eager deny.
func TestDecideInCwdViaSymlinkedDirAllows(t *testing.T) {
	cwd := realCwd(t, t.TempDir())

	realDir := filepath.Join(cwd, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatalf("mkdir real: %v", err)
	}
	link := filepath.Join(cwd, "alias") // <cwd>/alias -> <cwd>/real (stays inside)
	mustSymlink(t, realDir, link)

	target := filepath.Join(link, "file.txt")

	pol := New(ModeDefault, cwd, nil, nil, nil)
	dec, reason := pol.Decide(Check{
		Tool: &pathAwareTool{name: "write_file", paths: []string{target}},
		Args: json.RawMessage(`{}`),
	})
	if dec != Allow {
		t.Fatalf("in-cwd symlinked dir write: got %v (%q), want Allow", dec, reason)
	}
	if _, err := tools.ResolveAndCheck(target, cwd); err != nil {
		t.Fatalf("tools.ResolveAndCheck rejected an in-cwd symlinked write: %v", err)
	}
}
