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

// traceWith builds an AgentTrace for one epoch with the given per-turn
// usage and a single (stable) prefix hash.
func traceWith(epoch, prefixHash string, turns []usageTurn) *AgentTrace {
	tr := &AgentTrace{
		Found:       true,
		UsageTurns:  len(turns),
		epochHashes: map[string]map[string]bool{epoch: {prefixHash: true}},
		epochUsage:  map[string][]usageTurn{epoch: turns},
	}
	return tr
}

// TestParseUsageReasonix_RunSummaryLine parses the real `reasonix run`
// stdout footer: "— turns:N cache:NN.N% cost:$X save-vs-claude:NN.N%".
// Cost (USD→CNY) must be captured; no token split is fabricated from a
// bare percentage.
func TestParseUsageReasonix_RunSummaryLine(t *testing.T) {
	out := "— turns:5 cache:99.8% cost:$0.001234 save-vs-claude:78.5%\n"
	h, m, o, c := parseUsageReasonix(out)
	if h != 0 || m != 0 || o != 0 {
		t.Errorf("no token split should be fabricated from a percentage; got h=%d m=%d o=%d", h, m, o)
	}
	wantCNY := 0.001234 * 7.2
	if c < wantCNY*0.99 || c > wantCNY*1.01 {
		t.Errorf("cost: want ~%f got %f", wantCNY, c)
	}
}

func TestCheckCacheGate_PerAgentPerTask(t *testing.T) {
	tasks := []TaskSpec{
		{ID: "t-cache-a", Metrics: MetricsSpec{RequireCacheGate: true}},
		{ID: "t-cache-b", Metrics: MetricsSpec{RequireCacheGate: true}},
		{ID: "t-no-cache", Metrics: MetricsSpec{RequireCacheGate: false}},
	}
	results := []RunResult{
		// agent X: A passes (warm turn 97% hit), B fails (warm turn 1% hit).
		// Turn 1 is the cold start (excluded from post-warm). Aggregating
		// across tasks would mask B.
		{Agent: "X", Task: "t-cache-a", UsageParsed: true,
			Trace: traceWith("e1", "h1", []usageTurn{{1, 0, 1000}, {2, 9700, 300}})},
		{Agent: "X", Task: "t-cache-b", UsageParsed: true,
			Trace: traceWith("e1", "h1", []usageTurn{{1, 0, 1000}, {2, 100, 9900}})},
		{Agent: "X", Task: "t-no-cache", UsageParsed: true,
			Trace: traceWith("e1", "h1", []usageTurn{{1, 0, 1000}, {2, 50, 50}})},
	}
	gates := checkCacheGate(results, tasks)
	if _, ok := gates["X"]["t-no-cache"]; ok {
		t.Fatal("t-no-cache must not appear in gate results (require_cache_gate=false)")
	}
	gA := gates["X"]["t-cache-a"]
	if !gA.Passed {
		t.Fatalf("t-cache-a should pass at 97%% post-warm hit; gate=%+v", gA)
	}
	gB := gates["X"]["t-cache-b"]
	if gB.Passed {
		t.Fatalf("t-cache-b should fail at 1%% post-warm hit; gate=%+v", gB)
	}
}

// TestCheckCacheGate_NoTraceFailsClosed is the core review requirement:
// a task that requires the gate but produced no trace must FAIL, not be
// reported as N/A and pass.
func TestCheckCacheGate_NoTraceFailsClosed(t *testing.T) {
	tasks := []TaskSpec{{ID: "t1", Metrics: MetricsSpec{RequireCacheGate: true}}}
	results := []RunResult{{Agent: "Y", Task: "t1", UsageParsed: true, Trace: nil}}
	gates := checkCacheGate(results, tasks)
	g := gates["Y"]["t1"]
	if g.Passed || g.TraceFoundOK {
		t.Fatalf("missing trace must fail the gate closed; gate=%+v", g)
	}
}

// TestCheckCacheGate_DriftFails verifies a blocked drift event fails the gate
// even when the post-warm hit rate is perfect.
func TestCheckCacheGate_DriftFails(t *testing.T) {
	tasks := []TaskSpec{{ID: "t1", Metrics: MetricsSpec{RequireCacheGate: true}}}
	tr := traceWith("e1", "h1", []usageTurn{{1, 0, 1000}, {2, 1000, 0}})
	tr.DriftBlocked = 1
	results := []RunResult{{Agent: "Z", Task: "t1", UsageParsed: true, Trace: tr}}
	g := checkCacheGate(results, tasks)["Z"]["t1"]
	if g.Passed || g.UnauthorizedDriftOK {
		t.Fatalf("unauthorized drift must fail the gate; gate=%+v", g)
	}
}

// TestCheckCacheGate_UnstablePrefixFails verifies that two distinct prefix
// hashes within one epoch fail the gate.
func TestCheckCacheGate_UnstablePrefixFails(t *testing.T) {
	tasks := []TaskSpec{{ID: "t1", Metrics: MetricsSpec{RequireCacheGate: true}}}
	tr := &AgentTrace{
		Found:       true,
		UsageTurns:  2,
		epochHashes: map[string]map[string]bool{"e1": {"h1": true, "h2": true}},
		epochUsage:  map[string][]usageTurn{"e1": {{1, 0, 1000}, {2, 1000, 0}}},
	}
	results := []RunResult{{Agent: "Q", Task: "t1", UsageParsed: true, Trace: tr}}
	g := checkCacheGate(results, tasks)["Q"]["t1"]
	if g.Passed || g.PrefixStableOK {
		t.Fatalf("mid-epoch prefix drift must fail the gate; gate=%+v", g)
	}
}

// TestParseAgentTrace round-trips the exact JSONL shape emitted by the
// agent's TraceSink (internal/agent/trace.go) and checks derived metrics.
func TestParseAgentTrace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.agent.jsonl")
	lines := []string{
		`{"type":"prefix.snapshot","epoch_id":"e1","static_prefix_hash":"H","tools_hash":"T","reason":"session_start"}`,
		`{"type":"epoch.frozen","epoch_id":"e1"}`,
		`{"type":"prefix.snapshot","turn":1,"epoch_id":"e1","static_prefix_hash":"H","tools_hash":"T"}`,
		`{"type":"usage","turn":1,"epoch_id":"e1","model":"deepseek-v4-flash","cache_hit_tokens":0,"cache_miss_tokens":1000,"output_tokens":50,"cost_cny":0.001}`,
		`{"type":"prefix.snapshot","turn":2,"epoch_id":"e1","static_prefix_hash":"H","tools_hash":"T"}`,
		`{"type":"usage","turn":2,"epoch_id":"e1","model":"deepseek-v4-flash","cache_hit_tokens":980,"cache_miss_tokens":20,"output_tokens":40,"cost_cny":0.0005}`,
		`{"type":"compaction","epoch_id":"e1","kind":"semantic","static_prefix_hash":"H","before_static_prefix_hash":"P","after_static_prefix_hash":"P","summary_cost_cny":0.002}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tr, err := parseAgentTrace(path)
	if err != nil {
		t.Fatalf("parseAgentTrace: %v", err)
	}
	if !tr.Found {
		t.Fatal("trace should be Found")
	}
	if tr.UsageTurns != 2 {
		t.Errorf("UsageTurns = %d, want 2", tr.UsageTurns)
	}
	if tr.DriftBlocked != 0 {
		t.Errorf("DriftBlocked = %d, want 0", tr.DriftBlocked)
	}
	if tr.CompactionUnstable != 0 {
		t.Errorf("CompactionUnstable = %d, want 0", tr.CompactionUnstable)
	}
	if got := tr.unstableEpochs(); got != 0 {
		t.Errorf("unstableEpochs = %d, want 0", got)
	}
	rate, eligible := tr.postWarmHitRate()
	if eligible != 1 {
		t.Errorf("eligible warm turns = %d, want 1", eligible)
	}
	if rate < 0.97 { // turn 2: 980/1000 = 0.98
		t.Errorf("post-warm hit rate = %.3f, want >= 0.97", rate)
	}

	// Compaction was measured stable (before == after) and the integrity
	// counters are clean.
	if tr.CompactionRecords != 1 || tr.CompactionUnstable != 0 {
		t.Errorf("compaction records=%d unstable=%d, want 1/0", tr.CompactionRecords, tr.CompactionUnstable)
	}
	if tr.ParseErrors != 0 || tr.UsageMissingEpoch != 0 || tr.SnapshotMissingHash != 0 ||
		tr.UsageWithoutSnapshot != 0 || tr.CompactionMissingHash != 0 {
		t.Errorf("expected clean integrity counters, got %+v", tr)
	}

	// And the gate built from it passes.
	g := evalCacheGate(RunResult{Agent: "a", Task: "t", Trace: tr}, TaskSpec{Metrics: MetricsSpec{RequireCacheGate: true}})
	if !g.Passed {
		t.Errorf("gate from healthy trace should pass; gate=%+v", g)
	}
}

// TestParseAgentTrace_MissingFile reports not-found so the gate fails closed.
func TestParseAgentTrace_MissingFile(t *testing.T) {
	_, err := parseAgentTrace(filepath.Join(t.TempDir(), "nope.jsonl"))
	if err == nil {
		t.Fatal("expected error for missing trace file")
	}
}

// writeTrace writes JSONL lines to a temp file and parses them.
func parseTraceLines(t *testing.T, lines ...string) *AgentTrace {
	t.Helper()
	path := filepath.Join(t.TempDir(), "t.agent.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tr, err := parseAgentTrace(path)
	if err != nil {
		t.Fatalf("parseAgentTrace: %v", err)
	}
	return tr
}

// TestCacheGate_MalformedJSONLFailsClosed: a stray non-JSON line in an
// otherwise-healthy trace fails the enforced gate instead of being tolerated.
func TestCacheGate_MalformedJSONLFailsClosed(t *testing.T) {
	tr := parseTraceLines(t,
		`{"type":"prefix.snapshot","turn":1,"epoch_id":"e1","static_prefix_hash":"H"}`,
		`this is not json`,
		`{"type":"usage","turn":1,"epoch_id":"e1","cache_hit_tokens":0,"cache_miss_tokens":1000}`,
		`{"type":"prefix.snapshot","turn":2,"epoch_id":"e1","static_prefix_hash":"H"}`,
		`{"type":"usage","turn":2,"epoch_id":"e1","cache_hit_tokens":1000,"cache_miss_tokens":0}`,
	)
	if tr.ParseErrors != 1 {
		t.Fatalf("ParseErrors = %d, want 1", tr.ParseErrors)
	}
	g := evalCacheGate(RunResult{Trace: tr}, TaskSpec{Metrics: MetricsSpec{RequireCacheGate: true}})
	if g.Passed || g.TraceIntegrityOK {
		t.Fatalf("malformed JSONL must fail the gate closed; gate=%+v", g)
	}
}

// TestCacheGate_UsageWithoutSnapshotFailsClosed: usage whose epoch never
// emitted a prefix.snapshot cannot be tied to a stable prefix.
func TestCacheGate_UsageWithoutSnapshotFailsClosed(t *testing.T) {
	tr := parseTraceLines(t,
		`{"type":"usage","turn":1,"epoch_id":"e1","cache_hit_tokens":0,"cache_miss_tokens":1000}`,
		`{"type":"usage","turn":2,"epoch_id":"e1","cache_hit_tokens":1000,"cache_miss_tokens":0}`,
	)
	if tr.UsageWithoutSnapshot != 2 {
		t.Fatalf("UsageWithoutSnapshot = %d, want 2", tr.UsageWithoutSnapshot)
	}
	g := evalCacheGate(RunResult{Trace: tr}, TaskSpec{Metrics: MetricsSpec{RequireCacheGate: true}})
	if g.Passed || g.TraceIntegrityOK {
		t.Fatalf("usage-without-snapshot must fail the gate closed; gate=%+v", g)
	}
}

// TestCacheGate_EmptyEpochUsageFailsClosed: a usage record with no epoch_id
// is an integrity failure.
func TestCacheGate_EmptyEpochUsageFailsClosed(t *testing.T) {
	tr := parseTraceLines(t,
		`{"type":"prefix.snapshot","turn":1,"epoch_id":"e1","static_prefix_hash":"H"}`,
		`{"type":"usage","turn":1,"cache_hit_tokens":0,"cache_miss_tokens":1000}`,
	)
	if tr.UsageMissingEpoch != 1 {
		t.Fatalf("UsageMissingEpoch = %d, want 1", tr.UsageMissingEpoch)
	}
	g := evalCacheGate(RunResult{Trace: tr}, TaskSpec{Metrics: MetricsSpec{RequireCacheGate: true}})
	if g.Passed || g.TraceIntegrityOK {
		t.Fatalf("empty-epoch usage must fail the gate closed; gate=%+v", g)
	}
}

// TestCacheGate_CompactionMovedPrefixFails: a compaction whose before/after
// hashes differ means the prefix moved — measured, not asserted.
func TestCacheGate_CompactionMovedPrefixFails(t *testing.T) {
	tr := parseTraceLines(t,
		`{"type":"prefix.snapshot","turn":1,"epoch_id":"e1","static_prefix_hash":"H"}`,
		`{"type":"usage","turn":1,"epoch_id":"e1","cache_hit_tokens":0,"cache_miss_tokens":1000}`,
		`{"type":"prefix.snapshot","turn":2,"epoch_id":"e1","static_prefix_hash":"H"}`,
		`{"type":"usage","turn":2,"epoch_id":"e1","cache_hit_tokens":1000,"cache_miss_tokens":0}`,
		`{"type":"compaction","epoch_id":"e1","kind":"semantic","before_static_prefix_hash":"P1","after_static_prefix_hash":"P2"}`,
	)
	if tr.CompactionUnstable != 1 {
		t.Fatalf("CompactionUnstable = %d, want 1", tr.CompactionUnstable)
	}
	g := evalCacheGate(RunResult{Trace: tr}, TaskSpec{Metrics: MetricsSpec{RequireCacheGate: true}})
	if g.Passed || g.CompactionStableOK {
		t.Fatalf("compaction that moved the prefix must fail the gate; gate=%+v", g)
	}
}

// TestCacheGate_CompactionMissingHashFails: a compaction record without a
// before/after hash is incomplete instrumentation and fails closed.
func TestCacheGate_CompactionMissingHashFails(t *testing.T) {
	tr := parseTraceLines(t,
		`{"type":"prefix.snapshot","turn":1,"epoch_id":"e1","static_prefix_hash":"H"}`,
		`{"type":"usage","turn":1,"epoch_id":"e1","cache_hit_tokens":0,"cache_miss_tokens":1000}`,
		`{"type":"prefix.snapshot","turn":2,"epoch_id":"e1","static_prefix_hash":"H"}`,
		`{"type":"usage","turn":2,"epoch_id":"e1","cache_hit_tokens":1000,"cache_miss_tokens":0}`,
		`{"type":"compaction","epoch_id":"e1","kind":"semantic"}`,
	)
	if tr.CompactionMissingHash != 1 {
		t.Fatalf("CompactionMissingHash = %d, want 1", tr.CompactionMissingHash)
	}
	g := evalCacheGate(RunResult{Trace: tr}, TaskSpec{Metrics: MetricsSpec{RequireCacheGate: true}})
	if g.Passed || g.TraceIntegrityOK {
		t.Fatalf("compaction missing before/after hash must fail closed; gate=%+v", g)
	}
}

// TestCacheGate_SubagentIsolationRequiresChildTrace: a task that requires
// subagent isolation must fail closed when the trace has no child epoch,
// rather than reporting a hardcoded ✅.
func TestCacheGate_SubagentIsolationRequiresChildTrace(t *testing.T) {
	tr := traceWith("e1", "h1", []usageTurn{{1, 0, 1000}, {2, 1000, 0}})
	g := evalCacheGate(RunResult{Trace: tr},
		TaskSpec{Metrics: MetricsSpec{RequireCacheGate: true, RequireSubagentIsolation: true}})
	if g.ParentChildEvaluated {
		t.Error("ParentChild must be reported as not evaluated (no child epoch)")
	}
	if g.ParentChildOK || g.Passed {
		t.Fatalf("subagent-isolation task with no child trace must fail closed; gate=%+v", g)
	}
}

// TestCacheGate_ChildEpochEvaluated: when a child (subagent) epoch is present,
// pollution is actually evaluated rather than N/A.
func TestCacheGate_ChildEpochEvaluated(t *testing.T) {
	tr := parseTraceLines(t,
		`{"type":"prefix.snapshot","turn":1,"epoch_id":"e1","static_prefix_hash":"H","agent_role":"root"}`,
		`{"type":"usage","turn":1,"epoch_id":"e1","cache_hit_tokens":0,"cache_miss_tokens":1000,"agent_role":"root"}`,
		`{"type":"prefix.snapshot","turn":2,"epoch_id":"e1","static_prefix_hash":"H","agent_role":"root"}`,
		`{"type":"usage","turn":2,"epoch_id":"e1","cache_hit_tokens":1000,"cache_miss_tokens":0,"agent_role":"root"}`,
		`{"type":"prefix.snapshot","turn":1,"epoch_id":"c1","static_prefix_hash":"CH","agent_role":"subagent","parent_epoch_id":"e1"}`,
	)
	if tr.ChildEpochs != 1 {
		t.Fatalf("ChildEpochs = %d, want 1", tr.ChildEpochs)
	}
	g := evalCacheGate(RunResult{Trace: tr},
		TaskSpec{Metrics: MetricsSpec{RequireCacheGate: true, RequireSubagentIsolation: true}})
	if !g.ParentChildEvaluated {
		t.Error("ParentChild must be evaluated when a child epoch is present")
	}
}

// TestCacheGate_InsufficientPostWarmFails: a cache-gated task whose run never
// produced a warm turn must fail, not pass on N/A.
func TestCacheGate_InsufficientPostWarmFails(t *testing.T) {
	// Single (cold-only) turn → 0 warm turns.
	tr := traceWith("e1", "h1", []usageTurn{{1, 0, 1000}})
	g := evalCacheGate(RunResult{Trace: tr}, TaskSpec{Metrics: MetricsSpec{RequireCacheGate: true}})
	if g.PostWarmHitRateOK || g.Passed {
		t.Fatalf("cache-gated task with no warm turn must fail; gate=%+v", g)
	}
	// A non-cache-gated task with the same trace is not penalized for it.
	g2 := evalCacheGate(RunResult{Trace: tr}, TaskSpec{Metrics: MetricsSpec{RequireCacheGate: false}})
	if !g2.PostWarmHitRateOK {
		t.Errorf("non-cache-gated task should not require warm turns; gate=%+v", g2)
	}
}

// TestCacheGate_RequireCompactionRecordMissingFails: a task that requires
// compaction evidence must fail when the run never emitted a compaction record,
// even if every other dimension is healthy — no passing on the absence of one.
func TestCacheGate_RequireCompactionRecordMissingFails(t *testing.T) {
	tr := traceWith("e1", "h1", []usageTurn{{1, 0, 1000}, {2, 1000, 0}})
	// Baseline: this otherwise-healthy trace passes without the requirement.
	base := evalCacheGate(RunResult{Trace: tr}, TaskSpec{Metrics: MetricsSpec{RequireCacheGate: true}})
	if !base.Passed {
		t.Fatalf("baseline trace should pass without the compaction requirement; gate=%+v", base)
	}
	// With require_compaction_record and no compaction record → fail closed.
	g := evalCacheGate(RunResult{Trace: tr},
		TaskSpec{Metrics: MetricsSpec{RequireCacheGate: true, RequireCompactionRecord: true}})
	if g.CompactionStableOK || g.Passed {
		t.Fatalf("require_compaction_record with no record must fail closed; gate=%+v", g)
	}
}

// TestCacheGate_RequireCompactionRecordPresentPasses: a stable compaction
// record (before == after, as the deterministic path emits) satisfies the
// requirement and the gate passes.
func TestCacheGate_RequireCompactionRecordPresentPasses(t *testing.T) {
	tr := parseTraceLines(t,
		`{"type":"prefix.snapshot","turn":1,"epoch_id":"e1","static_prefix_hash":"H"}`,
		`{"type":"usage","turn":1,"epoch_id":"e1","cache_hit_tokens":0,"cache_miss_tokens":1000}`,
		`{"type":"prefix.snapshot","turn":2,"epoch_id":"e1","static_prefix_hash":"H"}`,
		`{"type":"usage","turn":2,"epoch_id":"e1","cache_hit_tokens":1000,"cache_miss_tokens":0}`,
		`{"type":"compaction","epoch_id":"e1","kind":"deterministic","before_static_prefix_hash":"P","after_static_prefix_hash":"P"}`,
	)
	if tr.CompactionRecords != 1 {
		t.Fatalf("CompactionRecords = %d, want 1", tr.CompactionRecords)
	}
	g := evalCacheGate(RunResult{Trace: tr},
		TaskSpec{Metrics: MetricsSpec{RequireCacheGate: true, RequireCompactionRecord: true}})
	if !g.CompactionStableOK || !g.Passed {
		t.Fatalf("a stable compaction record must satisfy require_compaction_record; gate=%+v", g)
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
