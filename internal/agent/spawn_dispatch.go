package agent

import (
	"context"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/agents"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
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
	// Remove "task" to prevent recursive spawning, unless def explicitly lists it.
	if !contains(def.Tools, "task") {
		names = remove(names, "task")
	}

	// Plan mode: narrow whitelist to read-only tools and use ModePlan.
	childMode := s.Parent.Permissions.Mode
	if def.Mode == "plan" {
		childMode = permissions.ModePlan
		// Re-derive names from the subset registry using read-only filter.
		sub := s.Parent.Tools.Subset(names)
		names = readOnlyToolNames(sub)
	}

	subReg := s.Parent.Tools.Subset(names)
	childPol := s.Parent.Permissions.DeriveChild(childMode)

	model := def.Model
	if model == "" {
		model = s.Parent.Model
	}

	child := New(s.Client, subReg, childPol, model)
	child.IsSubagent = true
	if def.Prompt != "" {
		child.System = def.Prompt
	} else {
		child.System = s.Parent.System
	}
	child.MaxToolCalls = 50

	// If the child registry has the "task" tool, bind it with depth+1.
	if _, ok := subReg.Get("task"); ok {
		childSpawner := &LoopSpawner{
			Client:   s.Client,
			Parent:   s.Parent,
			Defs:     s.Defs,
			MaxDepth: maxD,
			depth:    s.depth + 1,
		}
		subReg.Register(tools.NewSubagentTool(childSpawner))
	}

	// Drain child events to prevent goroutine deadlock when the
	// 256-capacity buffer fills up.
	go func() {
		for range child.Events() {
		}
	}()

	s.Parent.events <- EventSubagentStart{Agent: req.Agent, Description: req.Description}

	_, err := child.Run(ctx, req.Description)

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

	s.Parent.events <- EventSubagentFinish{
		Agent: req.Agent,
		Result: SubResult{
			Summary:    result.Summary,
			StepCount:  result.StepCount,
			TokenCount: result.TokenCount,
		},
	}

	return result, nil
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
