package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseUsageReasonix_EmptyOutputReturnsZero(t *testing.T) {
	h, m, o, c := parseUsageReasonix("nothing relevant here")
	if h != 0 || m != 0 || o != 0 || c != 0 {
		t.Fatalf("expected zeros for empty output, got h=%d m=%d o=%d c=%f", h, m, o, c)
	}
}

func TestParseUsageReasonix_PromptCompletionCached(t *testing.T) {
	out := `model: deepseek-v4-flash
prompt_tokens: 10000
completion_tokens: 800
cached_tokens: 6000
cost_usd: $0.0123
`
	h, m, o, c := parseUsageReasonix(out)
	if h != 6000 {
		t.Errorf("hits: want 6000 got %d", h)
	}
	if m != 4000 {
		t.Errorf("misses: want 4000 got %d", m)
	}
	if o != 800 {
		t.Errorf("output: want 800 got %d", o)
	}
	// USD → CNY conversion uses 7.2x; tolerate float noise.
	wantCNY := 0.0123 * 7.2
	if c < wantCNY*0.99 || c > wantCNY*1.01 {
		t.Errorf("cost: want ~%f got %f", wantCNY, c)
	}
}

func TestParseUsageReasonix_RateBasedCached(t *testing.T) {
	out := `input tokens: 1000
output tokens: 200
cache_hit_rate: 80%`
	h, m, o, c := parseUsageReasonix(out)
	if h != 800 || m != 200 || o != 200 {
		t.Fatalf("want h=800 m=200 o=200, got h=%d m=%d o=%d", h, m, o)
	}
	// No cost reported in this fixture → must stay 0. Estimating via the
	// DeepSeek price table would inflate the Reasonix comparison.
	if c != 0 {
		t.Fatalf("cost must be 0 when Reasonix reports no cost field; got %f", c)
	}
}

func TestParseUsageReasonix_NoCostNoFabrication(t *testing.T) {
	// Tokens reported but no cost field of any kind → cost must stay 0.
	out := `prompt_tokens: 5000
completion_tokens: 400
cached_tokens: 2500`
	_, _, _, c := parseUsageReasonix(out)
	if c != 0 {
		t.Fatalf("cost must be 0 when no cost field reported; got %f", c)
	}
}

func TestCheckCacheGate_PerAgentPerTask(t *testing.T) {
	tasks := []TaskSpec{
		{ID: "t-cache-a", Metrics: MetricsSpec{RequireCacheGate: true}},
		{ID: "t-cache-b", Metrics: MetricsSpec{RequireCacheGate: true}},
		{ID: "t-no-cache", Metrics: MetricsSpec{RequireCacheGate: false}},
	}
	results := []RunResult{
		// agent X: A passes (high hit), B fails (low hit). Aggregating would mask B.
		{Agent: "X", Task: "t-cache-a", UsageParsed: true, CacheHits: 9700, CacheMisses: 300},
		{Agent: "X", Task: "t-cache-b", UsageParsed: true, CacheHits: 100, CacheMisses: 9900},
		{Agent: "X", Task: "t-no-cache", UsageParsed: true, CacheHits: 50, CacheMisses: 50},
	}
	gates := checkCacheGate(results, tasks)
	if _, ok := gates["X"]["t-no-cache"]; ok {
		t.Fatal("t-no-cache must not appear in gate results (require_cache_gate=false)")
	}
	gA := gates["X"]["t-cache-a"]
	if !gA.Passed {
		t.Fatalf("t-cache-a should pass at 97%% hit; gate=%+v", gA)
	}
	gB := gates["X"]["t-cache-b"]
	if gB.Passed {
		t.Fatalf("t-cache-b should fail at 1%% hit; gate=%+v", gB)
	}
}

func TestCheckCacheGate_NoUsageParsedFails(t *testing.T) {
	tasks := []TaskSpec{{ID: "t1", Metrics: MetricsSpec{RequireCacheGate: true}}}
	results := []RunResult{{Agent: "Y", Task: "t1", UsageParsed: false}}
	gates := checkCacheGate(results, tasks)
	g := gates["Y"]["t1"]
	if g.Passed || g.UsageParsedOK {
		t.Fatalf("missing usage must fail the gate; gate=%+v", g)
	}
}

func TestRunTestCommandHonorsShellQuoting(t *testing.T) {
	dir := t.TempDir()
	skill := `# gozer

## Subcommands

### ` + "`analyze`" + `

Analyze a file path input and output a JSON analysis report.

Flags:

- ` + "`--verbose`" + ` Include detailed output.
`
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skill), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []string{
		"grep -Eq '^### `analyze`$' SKILL.md",
		"grep -Eiq 'file.?path|path.*input|input.*path' SKILL.md",
		"grep -Eiq 'JSON.*analysis|analysis.*JSON|analysis report' SKILL.md",
		"grep -q -- '--verbose' SKILL.md",
	}
	for _, cmd := range tests {
		t.Run(cmd, func(t *testing.T) {
			result := runTestCommand(cmd, dir)
			if !result.Passed {
				t.Fatalf("command should pass: exit=%d output=%q", result.ExitCode, result.Output)
			}
		})
	}
}

// initGitRepo turns dir into a git repo with one empty commit so
// `git diff HEAD` is well-defined. Skips the test if git is unavailable.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	commands := [][]string{
		{"git", "init", "-q", "-b", "main"},
		{"git", "config", "user.email", "bench@example.com"},
		{"git", "config", "user.name", "bench"},
		{"git", "commit", "-q", "--allow-empty", "-m", "init"},
	}
	for _, args := range commands {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v: %s", args, err, out)
		}
	}
}

// TestCheckDiffInvariants_IgnoredFileIsViolation ensures the read-only
// audit catches generated artifacts that .gitignore would hide. Pre-fix
// behavior used `git ls-files --others --exclude-standard`, which silently
// dropped these files.
func TestCheckDiffInvariants_IgnoredFileIsViolation(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("build/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Commit the .gitignore so it isn't itself a violation.
	for _, args := range [][]string{
		{"git", "add", ".gitignore"},
		{"git", "commit", "-q", "-m", "add gitignore"},
	} {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v: %s", args, err, out)
		}
	}

	if err := os.MkdirAll(filepath.Join(dir, "build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "build", "out.txt"), []byte("artifact"), 0o644); err != nil {
		t.Fatal(err)
	}

	violations := checkDiffInvariants([]DiffInvariant{{NoChanges: true}}, dir)
	found := false
	for _, v := range violations {
		if strings.Contains(v, "build/out.txt") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ignored file build/out.txt must be reported as violation; got %v", violations)
	}
}

// TestCheckDiffInvariants_NoChangesOutside_IgnoredFile exercises the
// sibling no_changes_outside branch that also used to be fail-open and
// only inspected tracked diff. After unification both branches go
// through changedFiles, so ignored files outside the allow-list must
// surface as violations.
func TestCheckDiffInvariants_NoChangesOutside_IgnoredFile(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Allowed scope: only files under "src/". Anything else — including
	// ignored generated artifacts — should be flagged.
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("dist/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", ".gitignore"},
		{"git", "commit", "-q", "-m", "add gitignore"},
	} {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v: %s", args, err, out)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dist", "bundle.js"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	inv := DiffInvariant{NoChangesOutside: []string{"src"}}
	violations := checkDiffInvariants([]DiffInvariant{inv}, dir)

	found := false
	for _, v := range violations {
		if strings.Contains(v, "dist/bundle.js") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ignored dist/bundle.js outside src/ must be reported; got %v", violations)
	}
}

// TestCheckDiffInvariants_NonGitDirFailsClosed verifies that a workdir
// without a .git directory produces an explicit violation rather than
// silently passing. This guards against corrupted fixtures or accidental
// non-git checkouts.
func TestCheckDiffInvariants_NonGitDirFailsClosed(t *testing.T) {
	dir := t.TempDir() // intentionally not a git repo

	violations := checkDiffInvariants([]DiffInvariant{{NoChanges: true}}, dir)
	if len(violations) == 0 {
		t.Fatalf("non-git workdir must fail the no_changes audit; got no violations")
	}
	// At least one violation message should call out the git failure mode.
	sawGitFailure := false
	for _, v := range violations {
		if strings.Contains(v, "git diff failed") || strings.Contains(v, "git ls-files failed") {
			sawGitFailure = true
			break
		}
	}
	if !sawGitFailure {
		t.Fatalf("expected a git-failure violation message; got %v", violations)
	}
}

func TestPrepareFixturePlainDir(t *testing.T) {
	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "fixtures", "test-fixture")
	if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureDir, "hello.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	workDir, cleanup, err := prepareFixture(fixtureDir, "HEAD", tmpDir)
	if err != nil {
		t.Fatalf("prepareFixture failed: %v", err)
	}
	defer cleanup()

	data, err := os.ReadFile(filepath.Join(workDir, "hello.txt"))
	if err != nil {
		t.Fatalf("read copied file: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("got %q, want %q", string(data), "hello")
	}

	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		t.Error("workDir should exist")
	}
}

func TestPrepareFixturePlainDirFakeSHA(t *testing.T) {
	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "fixtures", "test-fixture")
	if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureDir, "hello.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Plain dirs must reject non-HEAD commits — a fake SHA would silently
	// run different content than intended, breaking reproducibility.
	_, _, err := prepareFixture(fixtureDir, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", tmpDir)
	if err == nil {
		t.Fatal("expected error for non-HEAD commit on plain dir, got nil")
	}
	if !strings.Contains(err.Error(), "commit must be HEAD or empty") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPrepareFixtureGitRepo(t *testing.T) {
	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "fixtures", "test-git-fixture")
	if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = fixtureDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %s: %v", args[1], string(out), err)
		}
	}

	if err := os.WriteFile(filepath.Join(fixtureDir, "hello.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "initial"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = fixtureDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %s: %v", args[1], string(out), err)
		}
	}

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = fixtureDir
	shaBytes, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}
	sha := strings.TrimSpace(string(shaBytes))

	workDir, cleanup, err := prepareFixture(fixtureDir, sha, tmpDir)
	if err != nil {
		t.Fatalf("prepareFixture failed: %v", err)
	}
	defer cleanup()

	data, err := os.ReadFile(filepath.Join(workDir, "hello.txt"))
	if err != nil {
		t.Fatalf("read copied file: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("got %q, want %q", string(data), "hello")
	}

	if _, err := os.Stat(filepath.Join(workDir, ".git")); os.IsNotExist(err) {
		t.Error("workDir should be a git repo")
	}

	diffCmd := exec.Command("git", "diff", "--name-only", "HEAD")
	diffCmd.Dir = workDir
	diffOut, err := diffCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git diff: %v", err)
	}
	if strings.TrimSpace(string(diffOut)) != "" {
		t.Errorf("git diff should be clean, got: %s", string(diffOut))
	}
}
