// benchrunner is a black-box benchmark harness that compares coding agents
// by running them against a set of tasks and collecting structured traces.
//
// Usage:
//
//	go run ./bench/cmd/benchrunner/ [flags]
//	go build ./bench/cmd/benchrunner/ && ./benchrunner [flags]
//
// Flags:
//
//	--agent string    Filter to a single agent ID (e.g., "deepseekcode-current")
//	--task string     Filter to a single task ID (e.g., "ctx-long-readonly")
//	--dry-run         Show what would run without executing
//	--bench-dir string Root bench directory (default "bench")
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// AgentConfig represents an agent adapter configuration loaded from YAML.
type AgentConfig struct {
	ID          string                 `yaml:"id"`
	Command     string                 `yaml:"command"`
	Args        []string               `yaml:"args"`
	InputMode   string                 `yaml:"input_mode"` // "prompt_arg" or "stdin"
	Env         map[string]string      `yaml:"env"`
	TracePath   string                 `yaml:"trace_path"`
	UsageParse  string                 `yaml:"usage_parser"`
	WarmupRuns  int                    `yaml:"warmup_runs"`
	Features    map[string]interface{} `yaml:"features"`
	EnforceGate bool                   `yaml:"enforce_cache_gate"` // if false, gate is reported but not used for CI exit code
}

// TaskSpec represents a task specification loaded from YAML.
type TaskSpec struct {
	ID          string      `yaml:"id"`
	FixtureRepo string      `yaml:"fixture_repo"`
	Commit      string      `yaml:"commit"`
	PromptFile  string      `yaml:"prompt_file"`
	TimeoutSec  int         `yaml:"timeout_seconds"`
	ReadOnly    bool        `yaml:"read_only"`
	Success     SuccessSpec `yaml:"success"`
	Metrics     MetricsSpec `yaml:"metrics"`
}

// SuccessSpec defines what constitutes a successful run.
type SuccessSpec struct {
	Tests          []string        `yaml:"tests"`
	DiffInvariants []DiffInvariant `yaml:"diff_invariants"`
}

// DiffInvariant represents a constraint on file changes.
type DiffInvariant struct {
	NoChangesOutside []string `yaml:"no_changes_outside"`
	// NoChanges: when true, the workdir must be byte-identical to the
	// fixture commit. Any modified tracked file, untracked file, or
	// .gitignore-ignored file (e.g. build/, dist/) counts as a violation.
	// Used by read-only tasks to catch agents that "secretly" wrote files.
	NoChanges bool `yaml:"no_changes"`
}

// MetricsSpec defines metrics requirements.
type MetricsSpec struct {
	RequireCacheGate bool `yaml:"require_cache_gate"`

	// MinPostWarmTurns is the minimum number of warm (non-cold-start) turns a
	// cache-gated task must produce before its post-warm hit rate is trusted.
	// Defaults to 1 for cache-gated tasks (see loadTaskSpecs): a task that
	// requires the gate but never measured a cache hit must not pass on N/A.
	MinPostWarmTurns int `yaml:"min_post_warm_turns"`

	// RequireSubagentIsolation marks a task whose gate must prove parent/
	// subagent cache isolation. When set, the gate fails closed unless the
	// trace contains at least one child (subagent) epoch to evaluate.
	RequireSubagentIsolation bool `yaml:"require_subagent_isolation"`

	// RequireCompactionRecord marks a task whose gate must SEE a compaction
	// happen. When set, the gate fails closed unless the trace contains at
	// least one compaction record with before/after prefix hashes — so a
	// compaction task that never actually triggered compaction cannot pass the
	// "Compaction stable" dimension on the absence of evidence.
	RequireCompactionRecord bool `yaml:"require_compaction_record"`
}

// TraceRecord is a single JSONL trace line.
type TraceRecord struct {
	Type            string   `json:"type"`
	Agent           string   `json:"agent,omitempty"`
	Task            string   `json:"task,omitempty"`
	Timestamp       string   `json:"timestamp,omitempty"`
	Turn            *int     `json:"turn,omitempty"`
	CacheHitTokens  *int     `json:"cache_hit_tokens"`
	CacheMissTokens *int     `json:"cache_miss_tokens"`
	OutputTokens    *int     `json:"output_tokens"`
	CostCNY         *float64 `json:"cost_cny"`
	Success         *bool    `json:"success,omitempty"`
	DurationMs      *int64   `json:"duration_ms,omitempty"`
	Error           string   `json:"error,omitempty"`
	ExitCode        *int     `json:"exit_code,omitempty"`
	StdoutLines     *int     `json:"stdout_lines,omitempty"`
	StderrLines     *int     `json:"stderr_lines,omitempty"`

	// prefix.snapshot fields
	EpochID          *string `json:"epoch_id"`
	StaticPrefixHash *string `json:"static_prefix_hash"`
	ToolsHash        *string `json:"tools_hash"`

	// compaction fields
	Kind                    *string `json:"kind"`
	BeforeTokens            *int    `json:"before_tokens"`
	AfterTokens             *int    `json:"after_tokens"`
	StaticPrefixHashChanged *bool   `json:"static_prefix_hash_changed"`

	// turn.started fields
	Model *string `json:"model"`

	// run.finished fields
	PrefixHashChanged *bool `json:"prefix_hash_changed"`
}

// RunResult holds the outcome of a single agent+task execution.
type RunResult struct {
	Agent          string
	Task           string
	Success        bool
	TestsExpected  bool
	DurationMs     int64
	Error          string
	ExitCode       int
	StdoutLines    int
	StderrLines    int
	CacheHits      int
	CacheMisses    int
	OutputTokens   int
	CostCNY        float64
	UsageParsed    bool
	Turns          int
	TestResults    []TestResult
	DiffViolations []string

	// Trace is the parsed agent JSONL trace (--trace-jsonl) when the agent
	// emitted one. nil means no trace was requested or it was unparseable.
	Trace     *AgentTrace
	TracePath string
}

// AgentTrace is the parsed epoch/usage/compaction/drift evidence emitted by
// a deepseekcode agent via --trace-jsonl. The Cache Reliability gate is
// computed entirely from this — no fabricated placeholders.
type AgentTrace struct {
	Found              bool
	UsageTurns         int
	DriftBlocked       int // count of drift.blocked records (unauthorized drift)
	PendingChanges     int
	CompactionRecords  int // total compaction records seen
	CompactionUnstable int // compaction records where before != after prefix hash

	// Integrity counters. Any non-zero value fails an enforced gate closed:
	// a malformed or incomplete trace cannot prove cache reliability.
	ParseErrors           int // malformed JSONL lines
	UsageMissingEpoch     int // usage records with an empty epoch_id
	SnapshotMissingHash   int // prefix.snapshot records missing epoch_id or hash
	UsageWithoutSnapshot  int // usage records whose epoch had no valid snapshot
	CompactionMissingHash int // compaction records missing a before/after hash

	// ChildEpochs counts epochs attributed to a subagent (an epoch_id seen
	// under a non-root agent_role). Parent/subagent cache isolation can only be
	// judged when at least one child epoch is present.
	ChildEpochs int
	// ParentChildPollution counts isolation violations: an epoch_id seen under
	// both a root and a subagent role (a child reused the parent's epoch), or a
	// child whose parent_epoch_id equals its own epoch_id (self-parented).
	ParentChildPollution int
	// ChildMissingParent counts child (subagent) epochs that never carried a
	// parent_epoch_id — the child cannot be tied back to its parent at all.
	ChildMissingParent int
	// ChildUnknownParent counts child epochs whose parent_epoch_id is not a
	// known root epoch — the parent link points at nothing the root emitted.
	// Both are isolation-evidence failures, not free passes.
	ChildUnknownParent int

	// epochHashes maps an epoch_id to the set of static_prefix_hash values
	// seen in its prefix.snapshot records. A stable epoch has exactly one.
	epochHashes map[string]map[string]bool
	// epochUsage maps epoch_id to its per-turn usage, in arrival order.
	epochUsage map[string][]usageTurn
}

type usageTurn struct {
	turn, hit, miss int
}

type traceLine struct {
	Type                   string `json:"type"`
	EpochID                string `json:"epoch_id"`
	StaticPrefixHash       string `json:"static_prefix_hash"`
	Turn                   *int   `json:"turn"`
	CacheHitTokens         *int   `json:"cache_hit_tokens"`
	CacheMissTokens        *int   `json:"cache_miss_tokens"`
	BeforeStaticPrefixHash string `json:"before_static_prefix_hash"`
	AfterStaticPrefixHash  string `json:"after_static_prefix_hash"`
	AgentRole              string `json:"agent_role"`
	ParentEpochID          string `json:"parent_epoch_id"`
}

// parseAgentTrace reads a JSONL trace emitted by `dsc -p --trace-jsonl`.
// Returns (nil, err) when the file cannot be read; an empty-but-Found trace
// when the file exists but had no records.
func parseAgentTrace(path string) (*AgentTrace, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	tr := &AgentTrace{
		Found:       true,
		epochHashes: map[string]map[string]bool{},
		epochUsage:  map[string][]usageTurn{},
	}
	rootEpochs := map[string]bool{}            // epoch_id seen under role "root"
	childEpochs := map[string]bool{}           // epoch_id seen under a non-root role
	childParent := map[string]string{}         // child epoch_id -> first parent_epoch_id seen
	epochRoles := map[string]map[string]bool{} // epoch_id -> set of roles seen
	selfParented := 0
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var tl traceLine
		if err := json.Unmarshal([]byte(line), &tl); err != nil {
			// A malformed line in an enforced trace is an integrity failure,
			// not something to silently tolerate.
			tr.ParseErrors++
			continue
		}
		role := tl.AgentRole
		if role == "" {
			role = "root"
		}
		if tl.EpochID != "" {
			if epochRoles[tl.EpochID] == nil {
				epochRoles[tl.EpochID] = map[string]bool{}
			}
			epochRoles[tl.EpochID][role] = true
			if role == "root" {
				rootEpochs[tl.EpochID] = true
			} else {
				// A record attributed to a subagent marks its epoch as a child
				// epoch, which enables real parent/child pollution evaluation.
				childEpochs[tl.EpochID] = true
				if tl.ParentEpochID != "" && childParent[tl.EpochID] == "" {
					childParent[tl.EpochID] = tl.ParentEpochID
				}
			}
			// A child whose parent epoch is itself is an isolation violation.
			if tl.ParentEpochID == tl.EpochID && tl.ParentEpochID != "" {
				selfParented++
			}
		}
		switch tl.Type {
		case "prefix.snapshot":
			if tl.EpochID == "" || tl.StaticPrefixHash == "" {
				tr.SnapshotMissingHash++
				continue
			}
			if tr.epochHashes[tl.EpochID] == nil {
				tr.epochHashes[tl.EpochID] = map[string]bool{}
			}
			tr.epochHashes[tl.EpochID][tl.StaticPrefixHash] = true
		case "usage":
			tr.UsageTurns++
			if tl.EpochID == "" {
				tr.UsageMissingEpoch++
			}
			hit, miss, turn := 0, 0, 0
			if tl.CacheHitTokens != nil {
				hit = *tl.CacheHitTokens
			}
			if tl.CacheMissTokens != nil {
				miss = *tl.CacheMissTokens
			}
			if tl.Turn != nil {
				turn = *tl.Turn
			}
			tr.epochUsage[tl.EpochID] = append(tr.epochUsage[tl.EpochID], usageTurn{turn, hit, miss})
		case "drift.blocked":
			tr.DriftBlocked++
		case "pending_change":
			tr.PendingChanges++
		case "compaction":
			tr.CompactionRecords++
			if tl.BeforeStaticPrefixHash == "" || tl.AfterStaticPrefixHash == "" {
				tr.CompactionMissingHash++
				continue
			}
			if tl.BeforeStaticPrefixHash != tl.AfterStaticPrefixHash {
				tr.CompactionUnstable++
			}
		}
	}

	// A usage record whose epoch never produced a valid prefix.snapshot
	// cannot be tied to a stable prefix — count those for the integrity gate.
	for epoch, turns := range tr.epochUsage {
		if len(tr.epochHashes[epoch]) == 0 {
			tr.UsageWithoutSnapshot += len(turns)
		}
	}
	tr.ChildEpochs = len(childEpochs)
	// Pollution = self-parented children + epochs shared across root and a
	// subagent role (a child that mutated the parent's epoch namespace).
	pollution := selfParented
	for _, roles := range epochRoles {
		if roles["root"] && len(roles) > 1 {
			pollution++
		}
	}
	tr.ParentChildPollution = pollution
	// Validate every child epoch's parent link: it must carry a parent_epoch_id
	// AND that id must be a real root epoch. A subagent record with no parent,
	// or one pointing at an epoch the root never emitted, is not isolation
	// evidence — it must not pass the dimension just by existing.
	for ce := range childEpochs {
		p := childParent[ce]
		if p == "" {
			tr.ChildMissingParent++
			continue
		}
		if !rootEpochs[p] {
			tr.ChildUnknownParent++
		}
	}
	return tr, nil
}

// unstableEpochs returns how many epochs showed more than one distinct
// static_prefix_hash across their snapshots (i.e. mid-epoch prefix drift).
func (t *AgentTrace) unstableEpochs() int {
	n := 0
	for _, hashes := range t.epochHashes {
		if len(hashes) > 1 {
			n++
		}
	}
	return n
}

// postWarmHitRate computes the cache hit ratio across all turns EXCEPT the
// first (cold-start) turn of each epoch, which is expected to miss. Returns
// the ratio and the number of eligible (warm) turns counted.
func (t *AgentTrace) postWarmHitRate() (rate float64, eligibleTurns int) {
	var hit, total int
	for _, turns := range t.epochUsage {
		ordered := append([]usageTurn(nil), turns...)
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].turn < ordered[j].turn })
		for i, u := range ordered {
			if i == 0 {
				continue // cold start of this epoch
			}
			eligibleTurns++
			hit += u.hit
			total += u.hit + u.miss
		}
	}
	if total == 0 {
		return 0, eligibleTurns
	}
	return float64(hit) / float64(total), eligibleTurns
}

// TestResult holds the outcome of a single test command execution.
type TestResult struct {
	Command  string
	ExitCode int
	Output   string
	Passed   bool
}

// CacheGateResult holds the evaluated Cache Reliability gate verdict. Every
// dimension is now derived from the agent's emitted JSONL trace
// (--trace-jsonl); when the trace is absent the gate fails closed for
// enforced agents instead of silently passing the N/A dimensions.
type CacheGateResult struct {
	Passed bool

	// TraceFound is true when the agent emitted a parseable epoch/usage
	// trace. Missing instrumentation fails an enforced gate.
	TraceFound   bool
	TraceFoundOK bool

	// TraceIntegrityOK is false when the trace was malformed or incomplete
	// (parse errors, usage with no epoch_id, snapshots missing a hash, usage
	// with no backing snapshot, or compaction missing a before/after hash).
	// A failing integrity check fails the gate closed.
	TraceIntegrityOK bool

	// PrefixStable: every prefix.snapshot within an epoch shares the same
	// static_prefix_hash.
	PrefixStable   bool
	PrefixStableOK bool

	PostWarmHitRate   float64
	PostWarmHitRateOK bool
	PostWarmEligible  bool // had >=1 warm (non-cold-start) turn
	PostWarmTurns     int  // number of warm turns observed
	MinPostWarmTurns  int  // required minimum (0 = not enforced)

	UnauthorizedDrift   int // count of drift.blocked records
	UnauthorizedDriftOK bool

	CompactionStable   bool // no compaction record moved the static prefix
	CompactionStableOK bool
	CompactionRecords  int  // number of compaction records seen
	CompactionRequired bool // task requires at least one compaction record

	// ParentChildEvaluated is true only when the trace contained a child
	// (subagent) epoch to judge. When false the dimension is N/A, not a pass:
	// it never contributes a ✅, and a task that requires subagent isolation
	// fails closed.
	ParentChildEvaluated bool
	ParentChildPollution int
	ChildMissingParent   int
	ChildUnknownParent   int
	ParentChildOK        bool

	UsageParsed   bool
	UsageParsedOK bool

	Notes []string
}

// Pricing mirrors internal/llm/cache_metrics.go pricing table.
// Reimplemented here to avoid importing internal packages from a separate binary
// that may evolve independently.
var pricing = map[string][3]float64{
	// {input_cache_hit, input_cache_miss, output} per 1M tokens
	"deepseek-v4-flash": {0.02, 1.0, 2.0},
	"deepseek-v4-pro":   {0.025, 3.0, 6.0},
	"deepseek-chat":     {0.02, 1.0, 2.0},
	"deepseek-reasoner": {0.02, 1.0, 2.0},
}

// usageLineRE matches dsc's one-line usage summary emitted at end of -p runs.
// Example: "[step done: stop · in=12345 out=800 cache=45% ¥0.0123 · 1.234s]"
var usageLineRE = regexp.MustCompile(
	`\[(?:step done|done):\s+\S+\s+·\s+in=(\d+)\s+out=(\d+)\s+cache=(\d+)%\s+¥([\d.]+)`)

// ---------------------------------------------------------------------------
// YAML loading
// ---------------------------------------------------------------------------

func loadAgentConfigs(dir string) ([]AgentConfig, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read agents dir: %w", err)
	}
	var agents []AgentConfig
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		var ac AgentConfig
		if err := yaml.Unmarshal(data, &ac); err != nil {
			return nil, fmt.Errorf("parse %s: %w", e.Name(), err)
		}
		if ac.ID == "" {
			return nil, fmt.Errorf("%s: missing required field 'id'", e.Name())
		}
		agents = append(agents, ac)
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].ID < agents[j].ID })
	return agents, nil
}

func loadTaskSpecs(dir string) ([]TaskSpec, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read tasks dir: %w", err)
	}
	var tasks []TaskSpec
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		var ts TaskSpec
		if err := yaml.Unmarshal(data, &ts); err != nil {
			return nil, fmt.Errorf("parse %s: %w", e.Name(), err)
		}
		if ts.ID == "" {
			return nil, fmt.Errorf("%s: missing required field 'id'", e.Name())
		}
		if ts.TimeoutSec <= 0 {
			ts.TimeoutSec = 300 // default 5 minutes
		}
		if ts.Metrics.RequireCacheGate && ts.Metrics.MinPostWarmTurns <= 0 {
			ts.Metrics.MinPostWarmTurns = 1 // cache-gated tasks need a warm turn
		}
		tasks = append(tasks, ts)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	return tasks, nil
}

// ---------------------------------------------------------------------------
// Fixture management
// ---------------------------------------------------------------------------

// prepareFixture copies the fixture to a temp dir and makes sure it is a
// git repo with a single committed snapshot of its frozen state. Returns
// the temp dir path and a cleanup function.
//
// Two fixture layouts are supported:
//
//  1. Plain directory (preferred for committed fixtures): the repository
//     does not contain any nested .git. After copying, we init a fresh
//     repo and commit everything as the frozen baseline. The yaml `commit`
//     field is ignored in this mode — there is only one commit, HEAD.
//
//  2. Embedded git repo (legacy): the fixture itself ships with .git.
//     We checkout the requested `commit` and run `git clean -fdx`. This
//     mode is discouraged because nested .git inside the outer repo
//     either becomes an embedded repo/submodule or fails to commit at
//     all — but the path is kept for fixtures that genuinely need
//     multi-commit history.
//
// In both modes the post-condition is the same: `git diff HEAD` is
// well-defined and returns empty for a clean workdir, which is what the
// no_changes audit relies on.
func prepareFixture(fixtureRepo, commit, benchDir string) (string, func(), error) {
	absFixture := fixtureRepo
	if !filepath.IsAbs(absFixture) {
		absFixture = filepath.Join(benchDir, absFixture)
	}

	if _, err := os.Stat(absFixture); os.IsNotExist(err) {
		return "", nil, fmt.Errorf("fixture repo not found: %s", absFixture)
	}

	tmpDir, err := os.MkdirTemp("", "bench-fixture-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	cleanup := func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			log.Printf("WARNING: cleanup temp dir %s: %v", tmpDir, err)
		}
	}

	cpCmd := exec.Command("cp", "-a", absFixture+"/.", tmpDir)
	if out, err := cpCmd.CombinedOutput(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("copy fixture: %s: %w", string(out), err)
	}

	gitDir := filepath.Join(tmpDir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		// Layout 2: existing git repo — reset to the frozen commit.
		commitArg := commit
		if commitArg == "" {
			commitArg = "HEAD"
		}
		cmds := [][]string{
			{"git", "checkout", "--force", commitArg},
			{"git", "clean", "-fdx"},
		}
		for _, args := range cmds {
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Dir = tmpDir
			if out, err := cmd.CombinedOutput(); err != nil {
				cleanup()
				return "", nil, fmt.Errorf("git reset (%s): %s: %w", strings.Join(args, " "), string(out), err)
			}
		}
		return tmpDir, cleanup, nil
	}

	// Layout 1: plain directory — materialize a single-commit repo.
	// Plain dirs have no git history, so only "HEAD" or empty is allowed.
	// A specific SHA would silently run different content than intended.
	if commit != "" && commit != "HEAD" {
		cleanup()
		return "", nil, fmt.Errorf("plain directory fixture %q has no git history; commit must be HEAD or empty, got %q", fixtureRepo, commit)
	}
	initCmds := [][]string{
		{"git", "init", "-q", "-b", "main"},
		{"git", "config", "user.email", "bench@deepseekcode.local"},
		{"git", "config", "user.name", "bench"},
		{"git", "config", "commit.gpgsign", "false"},
		{"git", "add", "-A"},
		// --allow-empty keeps the audit valid for fixtures that ship no files.
		{"git", "commit", "-q", "--allow-empty", "-m", "bench fixture frozen"},
	}
	for _, args := range initCmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = tmpDir
		if out, err := cmd.CombinedOutput(); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("materialize fixture (%s): %s: %w",
				strings.Join(args, " "), string(out), err)
		}
	}
	return tmpDir, cleanup, nil
}

// ---------------------------------------------------------------------------
// Agent execution
// ---------------------------------------------------------------------------

// runAgent executes an agent against a task and returns the result. When
// traceOut is non-empty the agent is asked (via DEEPSEEKCODE_TRACE_JSONL) to
// write a JSONL epoch/usage trace there, which is parsed back into
// result.Trace and feeds the Cache Reliability gate.
func runAgent(ctx context.Context, agent AgentConfig, task TaskSpec, workDir, benchDir, traceOut string) RunResult {
	result := RunResult{
		Agent:         agent.ID,
		Task:          task.ID,
		TestsExpected: len(task.Success.Tests) > 0,
		TracePath:     traceOut,
	}

	// Resolve agent command relative to project root (parent of benchDir).
	// cmd.Dir will be set to the fixture's temp copy, so a relative command
	// like "./bin/dsc" must be anchored to the repo root first.
	projectDir := filepath.Dir(benchDir)
	cmdPath := agent.Command
	if !filepath.IsAbs(cmdPath) {
		cmdPath = filepath.Join(projectDir, cmdPath)
	}

	// Read prompt (resolve relative to benchDir, not workDir)
	promptPath := task.PromptFile
	if !filepath.IsAbs(promptPath) {
		promptPath = filepath.Join(benchDir, promptPath)
	}
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		result.Error = fmt.Sprintf("read prompt: %v", err)
		return result
	}
	promptStr := strings.TrimSpace(string(prompt))

	// Build command args
	args := make([]string, len(agent.Args))
	copy(args, agent.Args)

	if task.ReadOnly {
		args = append([]string{"--read-only"}, args...)
	}

	var cmd *exec.Cmd
	switch agent.InputMode {
	case "prompt_arg":
		args = append(args, promptStr)
		cmd = exec.CommandContext(ctx, cmdPath, args...)
	case "stdin":
		cmd = exec.CommandContext(ctx, cmdPath, args...)
		cmd.Stdin = strings.NewReader(promptStr)
	default:
		args = append(args, promptStr)
		cmd = exec.CommandContext(ctx, cmdPath, args...)
	}

	cmd.Dir = workDir

	// Set environment variables
	cmd.Env = os.Environ()
	for k, v := range agent.Env {
		if v == "required" {
			if envVal := os.Getenv(k); envVal != "" {
				cmd.Env = append(cmd.Env, k+"="+envVal)
			} else {
				result.Error = fmt.Sprintf("required env var %s not set", k)
				return result
			}
		} else {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	// Pass feature flags as env vars
	for k, v := range agent.Features {
		envKey := "DEEPSEEKCODE_" + strings.ToUpper(k)
		cmd.Env = append(cmd.Env, envKey+"="+fmt.Sprintf("%v", v))
	}

	// Ask the agent to emit a real epoch/usage trace at traceOut. The agent
	// honors DEEPSEEKCODE_TRACE_JSONL regardless of positional arg ordering.
	if traceOut != "" {
		_ = os.Remove(traceOut) // start clean so a stale trace can't pass the gate
		cmd.Env = append(cmd.Env, "DEEPSEEKCODE_TRACE_JSONL="+traceOut)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err = cmd.Run()
	duration := time.Since(start)
	result.DurationMs = duration.Milliseconds()

	result.StdoutLines = countLines(stdout.String())
	result.StderrLines = countLines(stderr.String())

	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
	} else if err != nil {
		result.Error = fmt.Sprintf("exec: %v", err)
		return result
	}

	combined := stdout.String() + "\n" + stderr.String()
	result.CacheHits, result.CacheMisses, result.OutputTokens, result.CostCNY = parseUsageForAgent(combined, agent.UsageParse)
	result.UsageParsed = result.CacheHits+result.CacheMisses+result.OutputTokens > 0
	result.Turns = 1

	// Parse the JSONL trace the agent emitted (real epoch/usage evidence).
	if traceOut != "" {
		if tr, perr := parseAgentTrace(traceOut); perr == nil {
			result.Trace = tr
			result.Turns = tr.UsageTurns
		}
	}

	result.Success = result.ExitCode == 0

	// Cache gate: if required and no usage metrics were parsed, fail the run.
	if task.Metrics.RequireCacheGate && !result.UsageParsed {
		result.Success = false
		if result.Error == "" {
			result.Error = "cache gate failed: no usage metrics parsed from agent output"
		}
	}

	// Run tests if defined
	if len(task.Success.Tests) > 0 && result.Success {
		for _, testCmd := range task.Success.Tests {
			tr := runTestCommand(testCmd, workDir)
			result.TestResults = append(result.TestResults, tr)
			if !tr.Passed {
				result.Success = false
			}
		}
	}

	// Check diff invariants
	if result.Success {
		violations := checkDiffInvariants(task.Success.DiffInvariants, workDir)
		if len(violations) > 0 {
			result.DiffViolations = violations
			result.Success = false
		}
	}

	return result
}

func runTestCommand(testCmd, workDir string) TestResult {
	tr := TestResult{Command: testCmd}
	if strings.TrimSpace(testCmd) == "" {
		tr.Output = "empty command"
		return tr
	}
	// Bench task tests are trusted repo-local assertions. Run through the
	// shell so YAML can express quoted grep patterns and small pipelines.
	cmd := exec.Command("sh", "-c", testCmd)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	tr.Output = string(out)
	if exitErr, ok := err.(*exec.ExitError); ok {
		tr.ExitCode = exitErr.ExitCode()
	} else if err != nil {
		tr.ExitCode = -1
		tr.Output = fmt.Sprintf("exec error: %v\n%s", err, tr.Output)
	}
	tr.Passed = err == nil
	return tr
}

func checkDiffInvariants(invariants []DiffInvariant, workDir string) []string {
	var violations []string
	for _, inv := range invariants {
		switch {
		case inv.NoChanges:
			files, errs := changedFiles(workDir)
			violations = append(violations, errs...)
			violations = append(violations, files...)
		case len(inv.NoChangesOutside) > 0:
			files, errs := changedFiles(workDir)
			// Errors are violations in this branch too — silently skipping
			// turned the audit into a fail-open check whenever git was
			// broken (non-repo workdir, corrupted checkout).
			violations = append(violations, errs...)
			for _, f := range files {
				if !pathAllowedByPrefixes(f, inv.NoChangesOutside) {
					violations = append(violations, f)
				}
			}
		}
	}
	return violations
}

// pathAllowedByPrefixes reports whether path is under one of the given
// allow-list prefixes. The "." prefix matches everything (used to say
// "the entire workdir is allowed").
func pathAllowedByPrefixes(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if p == "." || path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

// changedFiles returns every file in workDir that differs from the
// committed baseline: tracked-but-modified, untracked, AND .gitignore-
// ignored generated artifacts (build/, dist/, __pycache__/, etc.).
//
// The returned errs slice is non-empty when git itself fails — caller
// must treat each entry as a violation, not silently skip. Pre-fix
// behavior dropped these errors and let non-git fixtures appear clean.
func changedFiles(workDir string) (files, errs []string) {
	// Tracked modifications vs HEAD.
	diffOut, err := runGit(workDir, "diff", "--name-only", "HEAD")
	if err != nil {
		errs = append(errs, fmt.Sprintf("no_changes audit: git diff failed: %v: %s",
			err, strings.TrimSpace(diffOut)))
	} else {
		files = append(files, splitNonEmptyLines(diffOut)...)
	}
	// Every untracked path. Crucially we do NOT pass --exclude-standard:
	// generated files matched by .gitignore must surface so a "read-only"
	// run that secretly wrote into dist/ still fails the audit.
	lsOut, err := runGit(workDir, "ls-files", "--others")
	if err != nil {
		errs = append(errs, fmt.Sprintf("no_changes audit: git ls-files failed: %v: %s",
			err, strings.TrimSpace(lsOut)))
	} else {
		files = append(files, splitNonEmptyLines(lsOut)...)
	}
	return files, errs
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func splitNonEmptyLines(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\n")
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// checkCacheGate returns Cache Reliability gate results keyed by agent ID
// and then task ID. Per-(agent, task) granularity prevents a high-hit task
// from masking another task's failure. Only tasks with require_cache_gate=true
// are included; agents whose RequireCacheGate tasks did not run produce an
// empty inner map.
// checkCacheGate evaluates the Cache Reliability gate per (agent, task)
// entirely from the agent's emitted trace. Per the cache-epoch review it
// FAILS CLOSED: a task that requires the gate but produced no trace (or a
// trace with no usage/epoch fields) fails rather than reporting N/A as a
// pass. Only tasks with require_cache_gate=true are included.
func checkCacheGate(results []RunResult, tasks []TaskSpec) map[string]map[string]CacheGateResult {
	taskLookup := make(map[string]TaskSpec)
	for _, t := range tasks {
		taskLookup[t.ID] = t
	}

	gates := make(map[string]map[string]CacheGateResult)
	for _, r := range results {
		ts, ok := taskLookup[r.Task]
		if !ok || !ts.Metrics.RequireCacheGate {
			continue
		}

		gate := evalCacheGate(r, ts)

		if gates[r.Agent] == nil {
			gates[r.Agent] = make(map[string]CacheGateResult)
		}
		gates[r.Agent][r.Task] = gate
	}
	return gates
}

// evalCacheGate derives a single CacheGateResult from one run's trace and the
// task's metrics requirements. Every dimension is derived from real trace
// evidence; missing or malformed evidence fails the gate closed rather than
// passing on N/A.
func evalCacheGate(r RunResult, ts TaskSpec) CacheGateResult {
	gate := CacheGateResult{}

	tr := r.Trace
	gate.TraceFound = tr != nil && tr.Found
	gate.TraceFoundOK = gate.TraceFound

	if !gate.TraceFound {
		// No instrumentation → cannot prove cache reliability. Fail closed.
		gate.Notes = append(gate.Notes,
			"no agent trace emitted (epoch/usage instrumentation missing) — fail closed")
		gate.Passed = false
		return gate
	}

	// Integrity: a malformed or incomplete trace cannot prove anything.
	gate.TraceIntegrityOK = tr.ParseErrors == 0 && tr.UsageMissingEpoch == 0 &&
		tr.SnapshotMissingHash == 0 && tr.UsageWithoutSnapshot == 0 &&
		tr.CompactionMissingHash == 0
	if !gate.TraceIntegrityOK {
		gate.Notes = append(gate.Notes, fmt.Sprintf(
			"trace integrity failed: parse_errors=%d usage_missing_epoch=%d snapshot_missing_hash=%d usage_without_snapshot=%d compaction_missing_hash=%d",
			tr.ParseErrors, tr.UsageMissingEpoch, tr.SnapshotMissingHash,
			tr.UsageWithoutSnapshot, tr.CompactionMissingHash))
	}

	gate.UsageParsed = tr.UsageTurns > 0
	gate.UsageParsedOK = gate.UsageParsed
	if !gate.UsageParsed {
		gate.Notes = append(gate.Notes, "trace had no usage turns")
	}

	gate.UnauthorizedDrift = tr.DriftBlocked
	gate.UnauthorizedDriftOK = tr.DriftBlocked == 0

	// Compaction stability is MEASURED: a compaction record is unstable when
	// the agent's before/after static-prefix hashes differ. A task that
	// REQUIRES compaction evidence additionally fails when the trace has no
	// compaction record at all — it cannot pass this dimension on the absence
	// of a record.
	gate.CompactionRecords = tr.CompactionRecords
	gate.CompactionRequired = ts.Metrics.RequireCompactionRecord
	gate.CompactionStable = tr.CompactionUnstable == 0
	gate.CompactionStableOK = gate.CompactionStable
	if gate.CompactionRequired && tr.CompactionRecords == 0 {
		gate.CompactionStableOK = false
		gate.Notes = append(gate.Notes,
			"no compaction record in trace — task requires compaction evidence (fail closed)")
	}

	unstable := tr.unstableEpochs()
	gate.PrefixStable = unstable == 0
	gate.PrefixStableOK = gate.PrefixStable
	if unstable > 0 {
		gate.Notes = append(gate.Notes,
			fmt.Sprintf("%d epoch(s) showed >1 distinct static_prefix_hash", unstable))
	}

	rate, eligible := tr.postWarmHitRate()
	gate.PostWarmHitRate = rate
	gate.PostWarmTurns = eligible
	gate.PostWarmEligible = eligible > 0
	gate.MinPostWarmTurns = ts.Metrics.MinPostWarmTurns
	if gate.MinPostWarmTurns <= 0 && ts.Metrics.RequireCacheGate {
		gate.MinPostWarmTurns = 1 // default for cache-gated tasks
	}
	switch {
	case eligible >= gate.MinPostWarmTurns && gate.PostWarmEligible:
		gate.PostWarmHitRateOK = rate >= 0.95
	case gate.MinPostWarmTurns > 0:
		// A cache-gated task that never measured the required number of warm
		// turns has NOT demonstrated cache reuse — fail, do not pass on N/A.
		gate.PostWarmHitRateOK = false
		gate.Notes = append(gate.Notes, fmt.Sprintf(
			"insufficient post-warm turns (%d/%d) — cache hit rate not demonstrated",
			eligible, gate.MinPostWarmTurns))
	default:
		// Not a cache-gated task: warm turns are not required.
		gate.PostWarmHitRateOK = true
	}

	// Parent/subagent pollution. Only evaluable when the trace contains a
	// child (subagent) epoch. A single-process root trace has none, so the
	// dimension is N/A — it never contributes a ✅. A task that requires
	// subagent isolation fails closed when there is no child trace to judge.
	gate.ParentChildEvaluated = tr.ChildEpochs > 0
	if gate.ParentChildEvaluated {
		gate.ParentChildPollution = tr.ParentChildPollution
		gate.ChildMissingParent = tr.ChildMissingParent
		gate.ChildUnknownParent = tr.ChildUnknownParent
		// A child epoch is only valid evidence when it is tied to a real root
		// epoch. Pollution, a missing parent link, or a parent that is not a
		// known root epoch all fail the dimension.
		gate.ParentChildOK = gate.ParentChildPollution == 0 &&
			tr.ChildMissingParent == 0 && tr.ChildUnknownParent == 0
		if tr.ChildMissingParent > 0 || tr.ChildUnknownParent > 0 {
			gate.Notes = append(gate.Notes, fmt.Sprintf(
				"subagent epoch parent link invalid: missing_parent=%d unknown_parent=%d",
				tr.ChildMissingParent, tr.ChildUnknownParent))
		}
	} else if ts.Metrics.RequireSubagentIsolation {
		gate.ParentChildOK = false
		gate.Notes = append(gate.Notes,
			"no subagent/child epoch in trace — cannot prove parent/subagent isolation (fail closed)")
	} else {
		gate.ParentChildOK = true // not applicable
	}

	gate.Passed = gate.TraceFoundOK && gate.TraceIntegrityOK && gate.UsageParsedOK &&
		gate.PrefixStableOK && gate.UnauthorizedDriftOK && gate.CompactionStableOK &&
		gate.PostWarmHitRateOK && gate.ParentChildOK

	return gate
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n")
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// parseUsage extracts usage metrics from agent output.
func parseUsage(output string) (cacheHits, cacheMisses, outputTokens int, costCNY float64) {
	matches := usageLineRE.FindStringSubmatch(output)
	if matches == nil {
		return 0, 0, 0, 0
	}

	totalInput, _ := strconv.Atoi(matches[1])
	outputTokens, _ = strconv.Atoi(matches[2])
	cacheHitPct, _ := strconv.ParseFloat(matches[3], 64)

	cacheHits = int(float64(totalInput) * cacheHitPct / 100)
	cacheMisses = totalInput - cacheHits

	p := pricing["deepseek-v4-flash"]
	const million = 1_000_000.0
	costCNY = (float64(cacheHits)*p[0] + float64(cacheMisses)*p[1] + float64(outputTokens)*p[2]) / million

	return cacheHits, cacheMisses, outputTokens, costCNY
}

// reasonix output patterns. Reasonix CLI is third-party and its exact format
// is not vendored; this parser is best-effort and falls back to zeros when
// nothing matches. The Reasonix adapter is *not* gated for CI exit (see
// enforce_cache_gate in agent config); it is included for side-by-side
// reporting only. When Reasonix output stabilizes, tighten these patterns.
var (
	reasonixInputRE     = regexp.MustCompile(`(?i)(?:prompt|input)[_\s-]*tokens?\s*[:=]\s*(\d+)`)
	reasonixOutputRE    = regexp.MustCompile(`(?i)(?:completion|output)[_\s-]*tokens?\s*[:=]\s*(\d+)`)
	reasonixCachedRE    = regexp.MustCompile(`(?i)(?:cached|cache[_\s-]*hit)[_\s-]*tokens?\s*[:=]\s*(\d+)`)
	reasonixCacheRateRE = regexp.MustCompile(`(?i)cache[_\s-]*(?:hit[_\s-]*)?(?:rate|pct|percent|%)\s*[:=]\s*([\d.]+)\s*%?`)
	reasonixCostUSDRE   = regexp.MustCompile(`(?i)(?:cost|usd)[_\s-]*[:=]\s*\$?([\d.]+)`)
	reasonixCostCNYRE   = regexp.MustCompile(`(?i)(?:cost|cny|rmb|¥)\s*[:=]?\s*¥?([\d.]+)`)
)

// parseUsageReasonix attempts to extract usage from Reasonix CLI stdout/stderr.
// Returns zero values when nothing matches; the report will show "no usage
// metrics parsed" rather than fabricating data.
func parseUsageReasonix(output string) (cacheHits, cacheMisses, outputTokens int, costCNY float64) {
	var totalInput int
	if m := reasonixInputRE.FindStringSubmatch(output); m != nil {
		totalInput, _ = strconv.Atoi(m[1])
	}
	if m := reasonixOutputRE.FindStringSubmatch(output); m != nil {
		outputTokens, _ = strconv.Atoi(m[1])
	}
	if m := reasonixCachedRE.FindStringSubmatch(output); m != nil {
		cacheHits, _ = strconv.Atoi(m[1])
	} else if m := reasonixCacheRateRE.FindStringSubmatch(output); m != nil && totalInput > 0 {
		rate, _ := strconv.ParseFloat(m[1], 64)
		cacheHits = int(float64(totalInput) * rate / 100)
	}
	if totalInput > 0 {
		cacheMisses = totalInput - cacheHits
		if cacheMisses < 0 {
			cacheMisses = 0
		}
	}

	// Cost is reported only when Reasonix explicitly emits one (CNY or USD).
	// Estimating with DeepSeek pricing would inflate the comparison and make
	// the gate evidence look stronger than it is, so we leave costCNY=0 and
	// let the report render it as N/A.
	if m := reasonixCostCNYRE.FindStringSubmatch(output); m != nil {
		costCNY, _ = strconv.ParseFloat(m[1], 64)
	} else if m := reasonixCostUSDRE.FindStringSubmatch(output); m != nil {
		usd, _ := strconv.ParseFloat(m[1], 64)
		const approxUSDtoCNY = 7.2
		costCNY = usd * approxUSDtoCNY
	}
	return cacheHits, cacheMisses, outputTokens, costCNY
}

func parseUsageForAgent(output string, parserName string) (cacheHits, cacheMisses, outputTokens int, costCNY float64) {
	switch parserName {
	case "reasonix":
		return parseUsageReasonix(output)
	default:
		return parseUsage(output)
	}
}

// ---------------------------------------------------------------------------
// Trace writing
// ---------------------------------------------------------------------------

func writeTrace(w io.Writer, records []TraceRecord) error {
	enc := json.NewEncoder(w)
	for _, r := range records {
		if err := enc.Encode(r); err != nil {
			return fmt.Errorf("encode trace record: %w", err)
		}
	}
	return nil
}

// buildTraceRecords builds the harness-level run summary trace. The
// authoritative per-turn epoch/usage/compaction evidence is the agent's own
// `<task>.agent.jsonl` (parsed into result.Trace); this file records only
// what the harness itself observed (start/finish, aggregate usage). It no
// longer fabricates empty prefix.snapshot/compaction placeholders.
func buildTraceRecords(result RunResult) []TraceRecord {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	turn := 1

	usage := TraceRecord{Type: "usage", Turn: &turn}
	if result.UsageParsed {
		usage.OutputTokens = &result.OutputTokens
		usage.CostCNY = &result.CostCNY
		if result.CacheHits > 0 || result.CacheMisses > 0 {
			usage.CacheHitTokens = &result.CacheHits
			usage.CacheMissTokens = &result.CacheMisses
		}
	}

	return []TraceRecord{
		{Type: "run.started", Agent: result.Agent, Task: result.Task, Timestamp: now},
		usage,
		{
			Type:        "run.finished",
			Success:     &result.Success,
			DurationMs:  &result.DurationMs,
			Error:       result.Error,
			ExitCode:    &result.ExitCode,
			StdoutLines: &result.StdoutLines,
			StderrLines: &result.StderrLines,
		},
	}
}

// ---------------------------------------------------------------------------
// Report generation
// ---------------------------------------------------------------------------

func generateReport(results []RunResult, agents []AgentConfig, tasks []TaskSpec, gates map[string]map[string]CacheGateResult) string {
	var b strings.Builder

	b.WriteString("# Benchmark Report\n\n")
	b.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().UTC().Format(time.RFC3339)))

	// Summary table
	b.WriteString("## Summary\n\n")
	b.WriteString("| Agent | Task | Success | Duration | Cache Hit% | Output Tokens | Cost (¥) |\n")
	b.WriteString("|-------|------|---------|----------|------------|---------------|----------|\n")

	for _, r := range results {
		cacheTotal := r.CacheHits + r.CacheMisses
		cachePct := "N/A"
		if cacheTotal > 0 {
			cachePct = fmt.Sprintf("%.1f%%", float64(r.CacheHits)/float64(cacheTotal)*100)
		}
		costStr := "N/A"
		if r.CostCNY > 0 {
			costStr = fmt.Sprintf("%.4f", r.CostCNY)
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %v | %dms | %s | %d | %s |\n",
			r.Agent, r.Task, r.Success, r.DurationMs, cachePct, r.OutputTokens, costStr))
	}

	b.WriteString("\n")

	// Per-agent summary
	b.WriteString("## Per-Agent Summary\n\n")
	agentResults := make(map[string][]RunResult)
	for _, r := range results {
		agentResults[r.Agent] = append(agentResults[r.Agent], r)
	}

	b.WriteString("| Agent | Tasks | Successes | Failures | Avg Duration | Total Cost (¥) |\n")
	b.WriteString("|-------|-------|-----------|----------|--------------|----------------|\n")

	for _, agent := range agents {
		rr := agentResults[agent.ID]
		if len(rr) == 0 {
			continue
		}
		successes := 0
		var totalDur int64
		var totalCost float64
		for _, r := range rr {
			if r.Success {
				successes++
			}
			totalDur += r.DurationMs
			totalCost += r.CostCNY
		}
		avgDur := totalDur / int64(len(rr))
		b.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %dms | %.4f |\n",
			agent.ID, len(rr), successes, len(rr)-successes, avgDur, totalCost))
	}

	b.WriteString("\n")

	// Error details
	b.WriteString("## Errors\n\n")
	hasErrors := false
	for _, r := range results {
		if r.Error != "" {
			b.WriteString(fmt.Sprintf("- **%s / %s**: %s\n", r.Agent, r.Task, r.Error))
			hasErrors = true
		}
		for _, tr := range r.TestResults {
			if !tr.Passed {
				b.WriteString(fmt.Sprintf("- **%s / %s** test `%s` FAILED (exit %d): %s\n",
					r.Agent, r.Task, tr.Command, tr.ExitCode, firstLine(tr.Output)))
				hasErrors = true
			}
		}
		for _, v := range r.DiffViolations {
			b.WriteString(fmt.Sprintf("- **%s / %s** diff violation: `%s`\n", r.Agent, r.Task, v))
			hasErrors = true
		}
	}
	if !hasErrors {
		b.WriteString("No errors.\n")
	}

	// Cache Reliability Gate — per (agent, task)
	for _, agent := range agents {
		taskGates, ok := gates[agent.ID]
		if !ok || len(taskGates) == 0 {
			continue
		}

		enforcedLabel := "report-only"
		if agent.EnforceGate {
			enforcedLabel = "ENFORCED (CI gate)"
		}
		b.WriteString(fmt.Sprintf("\n## Cache Reliability Gate — %s (%s)\n\n", agent.ID, enforcedLabel))

		b.WriteString("| Task | Trace | Prefix Stable | Post-warm Hit | Drift | Compaction | Pollution | Verdict |\n")
		b.WriteString("|------|-------|---------------|---------------|-------|------------|-----------|---------|\n")

		// Sort tasks for stable output
		taskIDs := make([]string, 0, len(taskGates))
		for tID := range taskGates {
			taskIDs = append(taskIDs, tID)
		}
		sort.Strings(taskIDs)

		agentAllPassed := true
		var notes []string
		for _, tID := range taskIDs {
			gate := taskGates[tID]

			traceCell := "❌ missing"
			switch {
			case !gate.TraceFound:
				traceCell = "❌ missing"
			case !gate.TraceIntegrityOK:
				traceCell = "❌ malformed"
			default:
				traceCell = "✅ yes"
			}
			prefixCell := "—"
			hitCell := "—"
			driftCell := "—"
			compCell := "—"
			pollCell := "—"
			if gate.TraceFound {
				prefixCell = boolMark(gate.PrefixStableOK)
				switch {
				case gate.PostWarmEligible:
					hitCell = fmt.Sprintf("%.1f%% %s", gate.PostWarmHitRate*100, boolMark(gate.PostWarmHitRateOK))
				case gate.MinPostWarmTurns > 0:
					hitCell = fmt.Sprintf("n/a %s", boolMark(gate.PostWarmHitRateOK))
				default:
					hitCell = "n/a"
				}
				driftCell = fmt.Sprintf("%d %s", gate.UnauthorizedDrift, boolMark(gate.UnauthorizedDriftOK))
				switch {
				case gate.CompactionRequired && gate.CompactionRecords == 0:
					compCell = "none ❌" // required but no compaction happened
				case gate.CompactionRequired:
					compCell = fmt.Sprintf("%d %s", gate.CompactionRecords, boolMark(gate.CompactionStableOK))
				default:
					compCell = boolMark(gate.CompactionStableOK)
				}
				switch {
				case gate.ParentChildEvaluated && (gate.ChildMissingParent > 0 || gate.ChildUnknownParent > 0):
					pollCell = fmt.Sprintf("badlink %s", boolMark(gate.ParentChildOK))
				case gate.ParentChildEvaluated:
					pollCell = fmt.Sprintf("%d %s", gate.ParentChildPollution, boolMark(gate.ParentChildOK))
				case !gate.ParentChildOK:
					pollCell = "n/a ❌" // required isolation, no child trace
				default:
					pollCell = "n/a"
				}
			}
			verdictCell := "✅ PASS"
			if !gate.Passed {
				verdictCell = "❌ FAIL"
				agentAllPassed = false
			}
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s | %s |\n",
				tID, traceCell, prefixCell, hitCell, driftCell, compCell, pollCell, verdictCell))
			for _, n := range gate.Notes {
				notes = append(notes, fmt.Sprintf("- `%s`: %s", tID, n))
			}
		}

		b.WriteString("\n_Gate dimensions are computed from the agent's `<task>.agent.jsonl` trace " +
			"(epoch/usage/compaction/drift). A missing trace fails an enforced gate (fail-closed)._\n")
		if len(notes) > 0 {
			b.WriteString("\nNotes:\n")
			for _, n := range notes {
				b.WriteString(n + "\n")
			}
		}
		switch {
		case agentAllPassed:
			b.WriteString("**Agent verdict: PASS**\n")
		case agent.EnforceGate:
			b.WriteString("**Agent verdict: FAIL (enforced — will fail CI)**\n")
		default:
			b.WriteString("**Agent verdict: FAIL (report-only — does not fail CI)**\n")
		}
	}

	return b.String()
}

func boolMark(ok bool) string {
	if ok {
		return "✅"
	}
	return "❌"
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	var (
		agentFilter string
		taskFilter  string
		dryRun      bool
		benchDir    string
	)

	flag.StringVar(&agentFilter, "agent", "", "Filter to a single agent ID")
	flag.StringVar(&taskFilter, "task", "", "Filter to a single task ID")
	flag.BoolVar(&dryRun, "dry-run", false, "Show what would run without executing")
	flag.StringVar(&benchDir, "bench-dir", "bench", "Root bench directory")
	flag.Parse()

	// Resolve bench dir to absolute
	absBenchDir, err := filepath.Abs(benchDir)
	if err != nil {
		log.Fatalf("resolve bench dir: %v", err)
	}

	// Load configurations
	agents, err := loadAgentConfigs(filepath.Join(absBenchDir, "agents"))
	if err != nil {
		log.Fatalf("load agents: %v", err)
	}

	tasks, err := loadTaskSpecs(filepath.Join(absBenchDir, "tasks"))
	if err != nil {
		log.Fatalf("load tasks: %v", err)
	}

	// Apply filters
	if agentFilter != "" {
		var filtered []AgentConfig
		for _, a := range agents {
			if a.ID == agentFilter {
				filtered = append(filtered, a)
			}
		}
		if len(filtered) == 0 {
			log.Fatalf("agent %q not found", agentFilter)
		}
		agents = filtered
	}

	if taskFilter != "" {
		var filtered []TaskSpec
		for _, t := range tasks {
			if t.ID == taskFilter {
				filtered = append(filtered, t)
			}
		}
		if len(filtered) == 0 {
			log.Fatalf("task %q not found", taskFilter)
		}
		tasks = filtered
	}

	// Dry run mode
	if dryRun {
		fmt.Println("=== DRY RUN ===")
		fmt.Printf("Agents: %d\n", len(agents))
		for _, a := range agents {
			fmt.Printf("  - %s (%s)\n", a.ID, a.Command)
		}
		fmt.Printf("Tasks: %d\n", len(tasks))
		for _, t := range tasks {
			fmt.Printf("  - %s (timeout: %ds)\n", t.ID, t.TimeoutSec)
		}
		fmt.Printf("Combinations: %d\n", len(agents)*len(tasks))
		return
	}

	// Ensure output directories exist
	tracesDir := filepath.Join(absBenchDir, "traces")
	reportsDir := filepath.Join(absBenchDir, "reports")
	if err := os.MkdirAll(tracesDir, 0o755); err != nil {
		log.Fatalf("create traces dir: %v", err)
	}
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		log.Fatalf("create reports dir: %v", err)
	}

	// Run benchmarks
	var results []RunResult
	totalRuns := len(agents) * len(tasks)
	runIdx := 0

	for _, agent := range agents {
		agentTraceDir := agent.TracePath
		if agentTraceDir == "" {
			agentTraceDir = filepath.Join(tracesDir, agent.ID)
		}
		if !filepath.IsAbs(agentTraceDir) {
			agentTraceDir = filepath.Join(filepath.Dir(absBenchDir), agentTraceDir)
		}
		if err := os.MkdirAll(agentTraceDir, 0o755); err != nil {
			log.Fatalf("create agent trace dir: %v", err)
		}

		for _, task := range tasks {
			runIdx++
			log.Printf("[%d/%d] agent=%s task=%s", runIdx, totalRuns, agent.ID, task.ID)

			// Warmup runs (not measured)
			if agent.WarmupRuns > 0 {
				log.Printf("  warmup: %d runs", agent.WarmupRuns)
				for w := 0; w < agent.WarmupRuns; w++ {
					warmupDir, warmupCleanup, err := prepareFixture(task.FixtureRepo, task.Commit, absBenchDir)
					if err != nil {
						log.Printf("  warmup %d: fixture error: %v", w+1, err)
						continue
					}
					warmupCtx, warmupCancel := context.WithTimeout(context.Background(),
						time.Duration(task.TimeoutSec)*time.Second)
					runAgent(warmupCtx, agent, task, warmupDir, absBenchDir, "")
					warmupCancel()
					warmupCleanup()
				}
			}

			// Prepare fixture
			fixtureDir, cleanup, err := prepareFixture(task.FixtureRepo, task.Commit, absBenchDir)
			if err != nil {
				log.Printf("  SKIP: fixture error: %v", err)
				results = append(results, RunResult{
					Agent:   agent.ID,
					Task:    task.ID,
					Error:   fmt.Sprintf("fixture: %v", err),
					Success: false,
				})
				continue
			}

			// Create context with timeout
			ctx, cancel := context.WithTimeout(context.Background(),
				time.Duration(task.TimeoutSec)*time.Second)

			// Only deepseekcode agents emit the JSONL epoch/usage trace.
			// External agents (e.g. reasonix) have no such instrumentation.
			traceOut := ""
			if agent.UsageParse == "deepseekcode" {
				traceOut = filepath.Join(agentTraceDir, task.ID+".agent.jsonl")
			}

			// Run agent
			result := runAgent(ctx, agent, task, fixtureDir, absBenchDir, traceOut)
			cancel()

			if ctx.Err() == context.DeadlineExceeded {
				result.Error = fmt.Sprintf("timeout after %ds", task.TimeoutSec)
				result.Success = false
			}

			// Write JSONL trace
			tracePath := filepath.Join(agentTraceDir, task.ID+".jsonl")
			traceFile, err := os.Create(tracePath)
			if err != nil {
				log.Printf("  WARNING: create trace file: %v", err)
			} else {
				records := buildTraceRecords(result)
				if err := writeTrace(traceFile, records); err != nil {
					log.Printf("  WARNING: write trace: %v", err)
				}
				traceFile.Close()
			}

			// Log result
			status := "PASS"
			if !result.Success {
				status = "FAIL"
			}
			log.Printf("  %s duration=%dms exit=%d", status, result.DurationMs, result.ExitCode)

			results = append(results, result)
			cleanup()
		}
	}

	// Generate and write report
	gates := checkCacheGate(results, tasks)
	report := generateReport(results, agents, tasks, gates)
	reportPath := filepath.Join(reportsDir, fmt.Sprintf("bench-%s.md",
		time.Now().UTC().Format("20060102-150405")))
	if err := os.WriteFile(reportPath, []byte(report), 0o644); err != nil {
		log.Fatalf("write report: %v", err)
	}
	log.Printf("Report written to %s", reportPath)

	// Print summary to stdout
	fmt.Println("\n" + report)

	// Exit non-zero if any *enforced* agent has any failing per-task gate.
	// Agents with enforce_cache_gate=false are report-only — useful for the
	// `current` baseline (intentionally below threshold) and external agents
	// like Reasonix (best-effort usage parsing).
	enforcedAgents := make(map[string]bool)
	for _, a := range agents {
		enforcedAgents[a.ID] = a.EnforceGate
	}
	anyEnforcedFailed := false
	for agentID, taskGates := range gates {
		if !enforcedAgents[agentID] {
			continue
		}
		for _, gate := range taskGates {
			if !gate.Passed {
				log.Printf("ENFORCED gate FAIL: agent=%s", agentID)
				anyEnforcedFailed = true
				break
			}
		}
	}
	if anyEnforcedFailed {
		os.Exit(1)
	}
}
