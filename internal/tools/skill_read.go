package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// SkillSource supplies skill bodies on demand. internal/skills.Store
// satisfies it. Kept as a tiny interface so the tools package does not
// depend on the skills package and the tool stays unit-testable with a
// fake source.
type SkillSource interface {
	// Body returns the full body of the named skill and true, or
	// ("", false) when the skill is unknown.
	Body(name string) (string, bool)
	// Names returns the available skill names (for helpful misses).
	Names() []string
}

// SkillRead is the lazy capability dispatcher that loads a skill's full
// body by name. It exists so the cache-stable static prefix can carry only
// a short skill directory (name + short description + version_hash) instead
// of either the full bodies or local absolute paths. The body becomes
// ordinary conversation state when read, never silently mutating the prefix.
//
// SkillRead is a stable dispatcher: its schema is small and fixed, so adding
// or editing skills never changes the tool list hash.
type SkillRead struct {
	src SkillSource
}

// NewSkillReadTool builds a skill_read tool backed by src.
func NewSkillReadTool(src SkillSource) *SkillRead {
	return &SkillRead{src: src}
}

func (*SkillRead) Name() string { return "skill_read" }

func (*SkillRead) Description() string {
	return "Load the full instructions of a skill by name. The ## Skills section " +
		"lists available skill names with short descriptions; call this to fetch " +
		"the complete body before following a skill. Read-only."
}

func (*SkillRead) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Exact skill name as shown in the ## Skills directory.",
			},
		},
		"required": []string{"name"},
	})
}

func (*SkillRead) IsReadOnly() bool { return true }

func (s *SkillRead) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("invalid args: %v", err), nil
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return Errf("skill_read: 'name' is required"), nil
	}
	if s.src == nil {
		return Errf("skill_read: no skills are loaded in this session"), nil
	}
	body, ok := s.src.Body(p.Name)
	if !ok {
		avail := s.src.Names()
		sort.Strings(avail)
		if len(avail) == 0 {
			return Errf("skill_read: unknown skill %q (no skills loaded)", p.Name), nil
		}
		return Errf("skill_read: unknown skill %q. Available: %s",
			p.Name, strings.Join(avail, ", ")), nil
	}
	if strings.TrimSpace(body) == "" {
		return Result{Content: fmt.Sprintf("(skill %q has an empty body)", p.Name)}, nil
	}
	return Result{Content: body}, nil
}
