package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/amemiya02/deepseekcode/internal/agents"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
	"github.com/amemiya02/deepseekcode/internal/worktree"
)

// LoopSpawner implements tools.Spawner by running a child Agent loop
// in the same process. It derives a child Registry and Policy from the
// parent Agent, respecting agent-def tool whitelists and depth limits.
type LoopSpawner struct {
	Client   *llm.Client
	Parent   *Agent
	Defs     map[string]agents.AgentDef
	MaxDepth int // 0 → default 2
	depth    int // current recursion depth (internal)

	// WT is the worktree manager for git-worktree isolation.
	// nil = worktree path disabled; def.Worktree==true degrades to normal spawn.
	WT *worktree.Manager

	// Locks provides branch-level mutual exclusion for worktree operations.
	Locks worktree.BranchLocker
}

var _ tools.Spawner = (*LoopSpawner)(nil)

func (s *LoopSpawner) Spawn(ctx context.Context, req tools.SpawnRequest) (tools.SpawnResult, error) {
	maxD := s.MaxDepth
	if maxD == 0 {
		maxD = 2
	}
	if s.depth+1 > maxD {
		return tools.SpawnResult{Summary: "subagent depth limit reached"}, nil
	}

	def := s.Defs[req.Agent] // zero value = generic sub-agent

	// Build tool whitelist.
	names := def.Tools
	if len(names) == 0 {
		// Inherit all parent tools.
		for _, t := range s.Parent.Tools.All() {
			names = append(names, t.Name())
		}
	}
	if len(req.Tools) > 0 {
		names = intersect(names, req.Tools)
	}
	// Remove "task" and "spawn_batch" to prevent unbounded recursive spawning,
	// unless the def explicitly lists them. Both are re-registered below with a
	// depth+1 LoopSpawner so the child gets depth-guarded versions.
	if !contains(def.Tools, "task") {
		names = remove(names, "task")
	}
	if !contains(def.Tools, "spawn_batch") {
		names = remove(names, "spawn_batch")
	}

	// Plan mode: narrow whitelist to read-only tools and use ModePlan.
	childMode := s.Parent.Permissions.Mode
	if def.Mode == "plan" {
		childMode = permissions.ModePlan
		// Re-derive names from the subset registry using read-only filter.
		sub := s.Parent.Tools.Subset(names)
		names = readOnlyToolNames(sub)
	}

	// permission_ruleset frontmatter (T7.1): map a recognized ruleset name to a
	// Mode for the child. DeriveChild clamps it against the parent, so this can
	// only ever restrict, never escalate. Plan mode (above) is a stronger,
	// tool-narrowing directive and takes precedence.
	if def.Mode != "plan" && def.PermissionRuleset != "" {
		if m, ok := permissions.ModeFromRuleset(def.PermissionRuleset); ok {
			childMode = m
		} else {
			s.Parent.EmitInfo("unknown permission_ruleset " + def.PermissionRuleset + "; using parent mode")
		}
	}

	// Worktree isolation: create a git worktree for the child agent
	// when the agent def has worktree:true and we have a manager.
	useWT := def.Worktree && def.Mode != "plan" && s.WT != nil && s.Locks != nil
	var wtCwd string
	var wtRelease func()
	if useWT {
		branch := fmt.Sprintf("subagent/%s-%d", coalesce(req.Agent, "generic"), time.Now().UnixNano())
		rel, err := s.Locks.Acquire(ctx, branch)
		if err != nil {
			return tools.SpawnResult{Summary: "worktree branch busy: " + err.Error()}, nil
		}
		wt, err := s.WT.Create(ctx, branch, "HEAD")
		if err != nil {
			rel()
			return tools.SpawnResult{Summary: "worktree create failed: " + err.Error()}, nil
		}
		wtCwd = wt.Path
		wtRelease = rel
	} else if def.Worktree && s.WT == nil {
		s.Parent.EmitInfo("worktree disabled: no manager configured")
	}

	subReg := s.Parent.Tools.Subset(names)
	if wtCwd != "" {
		subReg = subReg.CloneForCWD(wtCwd)
	}
	var childPol *permissions.Policy
	if wtCwd != "" {
		childPol = s.Parent.Permissions.DeriveChildWithCwd(childMode, wtCwd)
	} else {
		childPol = s.Parent.Permissions.DeriveChild(childMode)
	}

	model := def.Model
	if model == "" {
		model = s.Parent.Model
	}

	child := New(s.Client, subReg, childPol, model)
	child.IsSubagent = true
	child.System = childSystem(def, s.Parent.System)
	child.MaxToolCalls = 50

	// Apply the remaining agent-def frontmatter (T7.1). MaxSteps drives the
	// step-cap StopCondition only (NOT MaxToolCalls — they are orthogonal
	// mechanisms). Temperature/TopP feed the llm.Request via the new Agent
	// fields. DefaultAgent has no defined spawn semantics yet, so it is
	// surfaced rather than silently ignored.
	if def.MaxSteps > 0 {
		child.StopWhen = []StopCondition{MaxSteps(def.MaxSteps), child.loopDetection(5, 3)}
	}
	child.Temperature = def.Temperature
	child.TopP = def.TopP
	if def.DefaultAgent != "" {
		s.Parent.EmitInfo("agent def default_agent=" + def.DefaultAgent + " is parsed but not yet applied at spawn")
	}

	// Re-register "task" and "spawn_batch" with a depth+1 LoopSpawner so that
	// recursive calls by the child agent respect MaxDepth. Both tools were
	// stripped from the whitelist above; re-registering them here (when the
	// parent had them) gives the child depth-guarded versions rather than
	// leaving the tools absent entirely.
	if _, ok := s.Parent.Tools.Get("task"); ok {
		childSpawner := &LoopSpawner{
			Client:   s.Client,
			Parent:   s.Parent,
			Defs:     s.Defs,
			MaxDepth: maxD,
			depth:    s.depth + 1,
		}
		subReg.Register(tools.NewSubagentTool(childSpawner))
	}
	if _, ok := s.Parent.Tools.Get("spawn_batch"); ok {
		batchSpawner := &LoopSpawner{
			Client:   s.Client,
			Parent:   s.Parent,
			Defs:     s.Defs,
			MaxDepth: maxD,
			depth:    s.depth + 1,
		}
		subReg.Register(tools.NewSpawnBatchTool(batchSpawner, 4))
	}

	// When the parent is emitting a JSONL trace (benchmark harness), tee this
	// subagent's epoch/usage/compaction events into the same trace, stamped as
	// a child epoch (agent_role="subagent", parent_epoch_id=<parent epoch>).
	// No-op for normal runs. The child's own EpochManager mints a distinct
	// epoch_id, so the benchmark can prove the child did not reuse the parent's.
	childTrace := s.Parent.AttachChildTraceSink(child)

	// Drain child events to prevent goroutine deadlock when the
	// 256-capacity buffer fills up.
	go func() {
		for range child.Events() {
		}
	}()

	// Async path: run in goroutine and return job ID immediately
	if req.Async {
		job, jobCtx := s.Parent.Jobs.Start(context.Background(), JobSubagent, req.Description)

		s.Parent.bus.Publish(EventBackgroundJobStart{
			ID:   job.ID,
			Kind: JobSubagent,
		})

		go func() {
			if wtRelease != nil {
				defer wtRelease()
			}
			// The child trace handle (childTrace) is owned by the parent's
			// WaitChildTraces, which the one-shot CLI calls before closing the
			// root trace. Closing it here on a short timeout would drop a slow
			// async child's later epoch/usage records — exactly the loss we are
			// preventing — so the goroutine does not touch it.
			_, err := child.Run(jobCtx, req.Description)

			state := JobSucceeded
			summary := extractFinalText(child)
			if err != nil {
				if jobCtx.Err() == context.Canceled {
					state = JobCanceled
					summary = "canceled by agent shutdown"
				} else {
					state = JobFailed
					summary = "subagent failed: " + err.Error()
				}
			}

			s.Parent.Jobs.Finish(job.ID, state, summary)

			// Read final state/summary under lock so user-initiated
			// cancel (which sets "canceled by user") is not overwritten
			// by the goroutine's local "canceled by agent shutdown".
			job.mu.Lock()
			eventState := job.State
			eventSummary := job.Summary
			job.mu.Unlock()

			s.Parent.bus.Publish(EventBackgroundJobFinish{
				ID:      job.ID,
				State:   eventState,
				Summary: eventSummary,
			})
		}()

		return tools.SpawnResult{JobID: job.ID, Summary: "started"}, nil
	}

	// Sync path (original behavior)
	if wtRelease != nil {
		defer wtRelease()
	}
	s.Parent.bus.Publish(EventSubagentStart{Agent: req.Agent, Description: req.Description})

	_, err := child.Run(ctx, req.Description)
	if childTrace != nil {
		childTrace.WaitTimeout(2 * time.Second)
		childTrace.Close()
	}

	result := tools.SpawnResult{
		Summary:   extractFinalText(child),
		StepCount: len(child.steps),
	}
	for _, step := range child.steps {
		result.TokenCount += step.Usage.TotalTokens
	}

	if err != nil {
		result.Summary = "subagent failed: " + err.Error()
	}

	s.Parent.bus.Publish(EventSubagentFinish{
		Agent: req.Agent,
		Result: SubResult{
			Summary:    result.Summary,
			StepCount:  result.StepCount,
			TokenCount: result.TokenCount,
		},
	})

	return result, nil
}

// childSystem picks the system prompt a spawned child runs with. A def with its
// own prompt uses it verbatim. Otherwise the child inherits the parent prompt —
// minus the per-turn project context block (cwd/date/git status/diff) when the
// def sets omit_project_context, so a read-only/explore child need not carry a
// large git diff it cannot act on. The flag is a no-op for a def that supplies
// its own prompt, which never carried project context to begin with.
func childSystem(def agents.AgentDef, parentSystem string) string {
	switch {
	case def.Prompt != "":
		return def.Prompt
	case def.OmitProjectContext:
		return staticPrefixOf(parentSystem)
	default:
		return parentSystem
	}
}

// extractFinalText returns the concatenated text of the last assistant
// message, or a placeholder if none exists.
func extractFinalText(child *Agent) string {
	for i := len(child.Messages) - 1; i >= 0; i-- {
		if child.Messages[i].Role == "assistant" {
			var sb strings.Builder
			for _, b := range child.Messages[i].Blocks {
				if tb, ok := b.(llm.TextBlock); ok {
					sb.WriteString(tb.Text)
				}
			}
			if sb.Len() > 0 {
				return sb.String()
			}
		}
	}
	return "(subagent produced no text)"
}

// intersect returns elements present in both a and b.
func intersect(a, b []string) []string {
	set := make(map[string]bool, len(b))
	for _, s := range b {
		set[s] = true
	}
	var out []string
	for _, s := range a {
		if set[s] {
			out = append(out, s)
		}
	}
	return out
}

// remove returns s with the first occurrence of elem removed.
func remove(s []string, elem string) []string {
	for i, v := range s {
		if v == elem {
			return append(s[:i], s[i+1:]...)
		}
	}
	return s
}

// contains reports whether s is in slice.
func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// coalesce returns a if non-empty, otherwise b.
func coalesce(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
