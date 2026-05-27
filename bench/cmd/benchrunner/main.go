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
	ID         string            `yaml:"id"`
	Command    string            `yaml:"command"`
	Args       []string          `yaml:"args"`
	InputMode  string            `yaml:"input_mode"` // "prompt_arg" or "stdin"
	Env        map[string]string `yaml:"env"`
	TracePath  string            `yaml:"trace_path"`
	UsageParse string            `yaml:"usage_parser"`
}

// TaskSpec represents a task specification loaded from YAML.
type TaskSpec struct {
	ID            string       `yaml:"id"`
	FixtureRepo   string       `yaml:"fixture_repo"`
	Commit        string       `yaml:"commit"`
	PromptFile    string       `yaml:"prompt_file"`
	TimeoutSec    int          `yaml:"timeout_seconds"`
	Success       SuccessSpec  `yaml:"success"`
	Metrics       MetricsSpec  `yaml:"metrics"`
}

// SuccessSpec defines what constitutes a successful run.
type SuccessSpec struct {
	Tests          []string         `yaml:"tests"`
	DiffInvariants []DiffInvariant  `yaml:"diff_invariants"`
}

// DiffInvariant represents a constraint on file changes.
type DiffInvariant struct {
	NoChangesOutside []string `yaml:"no_changes_outside"`
}

// MetricsSpec defines metrics requirements.
type MetricsSpec struct {
	RequireCacheGate bool `yaml:"require_cache_gate"`
}

// TraceRecord is a single JSONL trace line.
type TraceRecord struct {
	Type             string   `json:"type"`
	Agent            string   `json:"agent,omitempty"`
	Task             string   `json:"task,omitempty"`
	Timestamp        string   `json:"timestamp,omitempty"`
	Turn             *int     `json:"turn,omitempty"`
	CacheHitTokens   *int     `json:"cache_hit_tokens"`
	CacheMissTokens  *int     `json:"cache_miss_tokens"`
	OutputTokens     *int     `json:"output_tokens"`
	CostCNY          *float64 `json:"cost_cny"`
	Success          *bool    `json:"success,omitempty"`
	DurationMs       *int64   `json:"duration_ms,omitempty"`
	Error            string   `json:"error,omitempty"`
	ExitCode         *int     `json:"exit_code,omitempty"`
	StdoutLines      *int     `json:"stdout_lines,omitempty"`
	StderrLines      *int     `json:"stderr_lines,omitempty"`

	// prefix.snapshot fields
	EpochID         *string `json:"epoch_id"`
	StaticPrefixHash *string `json:"static_prefix_hash"`
	ToolsHash       *string `json:"tools_hash"`

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
	Turns          int
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
// Example: "usage: prompt=12345 (cache_hit=0 cache_miss=12345) completion=800"
var usageLineRE = regexp.MustCompile(
	`usage:\s+prompt=(\d+)\s+\(cache_hit=(\d+)\s+cache_miss=(\d+)\)\s+completion=(\d+)`)

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
		tasks = append(tasks, ts)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	return tasks, nil
}

// ---------------------------------------------------------------------------
// Fixture management
// ---------------------------------------------------------------------------

// prepareFixture copies the fixture repo to a temp dir and resets to the
// frozen commit. Returns the temp dir path and a cleanup function.
func prepareFixture(fixtureRepo, commit, benchDir string) (string, func(), error) {
	absFixture := fixtureRepo
	if !filepath.IsAbs(absFixture) {
		absFixture = filepath.Join(benchDir, absFixture)
	}

	// Check fixture exists
	if _, err := os.Stat(absFixture); os.IsNotExist(err) {
		return "", nil, fmt.Errorf("fixture repo not found: %s", absFixture)
	}

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "bench-fixture-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}

	cleanup := func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			log.Printf("WARNING: cleanup temp dir %s: %v", tmpDir, err)
		}
	}

	// Copy fixture repo to temp dir using cp -a
	cpCmd := exec.Command("cp", "-a", absFixture+"/.", tmpDir)
	if out, err := cpCmd.CombinedOutput(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("copy fixture: %s: %w", string(out), err)
	}

	// Reset to frozen commit if it's a git repo
	commitArg := commit
	if commitArg == "" || commitArg == "HEAD" {
		commitArg = "HEAD"
	}

	gitDir := filepath.Join(tmpDir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		// It's a git repo — reset to the specified commit
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
	}

	return tmpDir, cleanup, nil
}

// ---------------------------------------------------------------------------
// Agent execution
// ---------------------------------------------------------------------------

// runAgent executes an agent against a task and returns the result.
func runAgent(ctx context.Context, agent AgentConfig, task TaskSpec, workDir string) RunResult {
	result := RunResult{
		Agent:         agent.ID,
		Task:          task.ID,
		TestsExpected: len(task.Success.Tests) > 0,
	}

	// Read prompt
	promptPath := task.PromptFile
	if !filepath.IsAbs(promptPath) {
		promptPath = filepath.Join(workDir, promptPath)
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

	var cmd *exec.Cmd
	switch agent.InputMode {
	case "prompt_arg":
		args = append(args, promptStr)
		cmd = exec.CommandContext(ctx, agent.Command, args...)
	case "stdin":
		cmd = exec.CommandContext(ctx, agent.Command, args...)
		cmd.Stdin = strings.NewReader(promptStr)
	default:
		args = append(args, promptStr)
		cmd = exec.CommandContext(ctx, agent.Command, args...)
	}

	// Set working directory to the fixture copy
	cmd.Dir = workDir

	// Set environment variables
	cmd.Env = os.Environ()
	for k, v := range agent.Env {
		if v == "required" {
			// Check if already set in environment
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

	// Capture stdout and stderr separately
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run with timeout
	start := time.Now()
	err = cmd.Run()
	duration := time.Since(start)
	result.DurationMs = duration.Milliseconds()

	// Count lines
	result.StdoutLines = countLines(stdout.String())
	result.StderrLines = countLines(stderr.String())

	// Get exit code
	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
	} else if err != nil {
		result.Error = fmt.Sprintf("exec: %v", err)
		return result
	}

	// Parse usage from combined output
	combined := stdout.String() + "\n" + stderr.String()
	result.CacheHits, result.CacheMisses, result.OutputTokens, result.CostCNY = parseUsage(combined)
	result.Turns = 1 // single-turn for -p mode

	// Determine success based on exit code.
	// TODO: When test execution is implemented, success should also require
	// tests to pass when success.tests is non-empty. For now, log a warning
	// if tests are defined but can't be run yet.
	result.Success = result.ExitCode == 0
	if result.TestsExpected && result.Success {
		log.Printf("  WARNING: task %s defines tests but test execution not yet implemented", task.ID)
	}

	return result
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n")
}

// parseUsage extracts usage metrics from agent output.
func parseUsage(output string) (cacheHits, cacheMisses, outputTokens int, costCNY float64) {
	matches := usageLineRE.FindStringSubmatch(output)
	if matches == nil {
		return 0, 0, 0, 0
	}

	cacheHits, _ = strconv.Atoi(matches[2])
	cacheMisses, _ = strconv.Atoi(matches[3])
	outputTokens, _ = strconv.Atoi(matches[4])

	// Calculate cost using flash pricing (default)
	p := pricing["deepseek-v4-flash"]
	const million = 1_000_000.0
	costCNY = (float64(cacheHits)*p[0] + float64(cacheMisses)*p[1] + float64(outputTokens)*p[2]) / million

	return cacheHits, cacheMisses, outputTokens, costCNY
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

func buildTraceRecords(result RunResult) []TraceRecord {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	turn := 1

	records := []TraceRecord{
		{
			Type:      "run.started",
			Agent:     result.Agent,
			Task:      result.Task,
			Timestamp: now,
		},
		{
			Type: "prefix.snapshot",
			// Fields are null when not available from agent output
		},
		{
			Type: "turn.started",
			Turn: &turn,
			// EpochID and Model are null when not available
		},
		{
			Type:            "usage",
			Turn:            &turn,
			CacheHitTokens:  &result.CacheHits,
			CacheMissTokens: &result.CacheMisses,
			OutputTokens:    &result.OutputTokens,
			CostCNY:         &result.CostCNY,
		},
		{
			Type: "compaction",
			// Fields are null when not available from agent output
		},
		{
			Type:        "run.finished",
			Success:     &result.Success,
			DurationMs:  &result.DurationMs,
			Error:       result.Error,
			ExitCode:    &result.ExitCode,
			StdoutLines: &result.StdoutLines,
			StderrLines: &result.StderrLines,
			// PrefixHashChanged is null when not available
		},
	}
	return records
}

// ---------------------------------------------------------------------------
// Report generation
// ---------------------------------------------------------------------------

func generateReport(results []RunResult, agents []AgentConfig, tasks []TaskSpec) string {
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
	}
	if !hasErrors {
		b.WriteString("No errors.\n")
	}

	return b.String()
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
		// Determine agent trace directory: use trace_path from config if specified,
		// otherwise fall back to tracesDir + agent.ID
		agentTraceDir := agent.TracePath
		if agentTraceDir == "" {
			agentTraceDir = filepath.Join(tracesDir, agent.ID)
		}
		if !filepath.IsAbs(agentTraceDir) {
			agentTraceDir = filepath.Join(absBenchDir, agentTraceDir)
		}
		if err := os.MkdirAll(agentTraceDir, 0o755); err != nil {
			log.Fatalf("create agent trace dir: %v", err)
		}

		for _, task := range tasks {
			runIdx++
			log.Printf("[%d/%d] agent=%s task=%s", runIdx, totalRuns, agent.ID, task.ID)

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

			// Run agent
			result := runAgent(ctx, agent, task, fixtureDir)
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
	report := generateReport(results, agents, tasks)
	reportPath := filepath.Join(reportsDir, fmt.Sprintf("bench-%s.md",
		time.Now().UTC().Format("20060102-150405")))
	if err := os.WriteFile(reportPath, []byte(report), 0o644); err != nil {
		log.Fatalf("write report: %v", err)
	}
	log.Printf("Report written to %s", reportPath)

	// Print summary to stdout
	fmt.Println("\n" + report)
}
