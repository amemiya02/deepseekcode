package agent

import (
	"fmt"

	"github.com/amemiya02/deepseekcode/internal/skills"
)

// ReloadResult summarizes an Agent.ReloadSkills (the /reload-skills command).
type ReloadResult struct {
	// Changes is the skill-level diff (added / removed / body_changed) between
	// the previously loaded skills and the freshly re-scanned set.
	Changes []skills.SkillChange

	// FingerprintMoved reports whether the rebuilt model-visible system prompt
	// differs from the pre-reload one. True means the bytes DeepSeek caches
	// actually changed, so a new epoch was minted and the next turn will miss
	// cache exactly once. False means nothing the model can see changed and no
	// epoch was switched — so the reload costs no cache miss.
	FingerprintMoved bool

	// OldEpochID / NewEpochID are populated only when an epoch switch occurred
	// (FingerprintMoved && an epoch already existed).
	OldEpochID string
	NewEpochID string
}

// ReloadSkills re-scans the skill directories under cwd (then home), refreshes
// the in-place skill store, rebuilds the model-visible system prompt, and — only
// when the rebuilt prefix actually moves — mints a new prefix epoch so the edit
// takes effect mid-session.
//
// This is the deliberate, user-triggered exception to skills.LoadScan's
// session-start-only rule: the skill directory is normally frozen for the whole
// session to protect DeepSeek's 50x prompt cache. /reload-skills trades exactly
// one cache miss (the new epoch's first turn) for the model seeing edited skills
// now instead of next session.
//
// The store is mutated in place (skills.Store.ReplaceFrom). a.Skills and the
// skill_read dispatcher share one *Store pointer, so the capability set used for
// drift detection and on-demand skill-body lookups both pick up the reloaded
// skills atomically.
//
// Concurrency: the caller MUST ensure no turn is in flight — neither the main
// loop (a.running) NOR any background job (HasActiveBackgroundWork). ReloadSkills
// mutates a.System, the shared skill store (which runStep's capability set and
// the skill_read tool both read — including from a still-live async subagent),
// and the epoch state, none of which is safe to touch while a reader runs. The
// TUI gates this behind both checks; /undo's guard covers only the main loop, so
// reload's guard is deliberately stronger.
func (a *Agent) ReloadSkills(cwd, home string) (ReloadResult, error) {
	if a.Skills == nil {
		return ReloadResult{}, fmt.Errorf("reload-skills: no skill store configured for this session")
	}
	if a.PromptBuilder == nil {
		// Without a PromptBuilder the skill directory is not part of the
		// model-visible prompt, so a reload could only change skill_read bodies
		// invisibly — no prefix move, no epoch, no receipt. Refuse rather than
		// mutate the shared store with no trace. (Both production entry points
		// always wire a PromptBuilder, so this only guards misuse.)
		return ReloadResult{}, fmt.Errorf("reload-skills: no prompt builder; skills are not model-visible in this session")
	}

	fresh, err := skills.LoadScan(cwd, home)
	if err != nil {
		return ReloadResult{}, fmt.Errorf("reload-skills: %w", err)
	}

	// Diff new-relative-to-old BEFORE mutating the store.
	changes := fresh.Diff(a.Skills)

	// Capture the pre-reload model-visible prefix so we only mint a new epoch
	// (and eat one cache miss) when the bytes DeepSeek caches actually move.
	oldStatic := a.staticSystem()

	// Swap the store contents in place — a.Skills and the skill_read dispatcher
	// share this pointer, so both update — then rebuild the system prompt from
	// the refreshed skill directory.
	a.Skills.ReplaceFrom(fresh)
	a.PromptBuilder.SkillDirectory = a.Skills.IndexText()
	a.System = a.PromptBuilder.Build()

	res := ReloadResult{Changes: changes}
	if a.staticSystem() == oldStatic {
		// Nothing model-visible changed. Don't churn the epoch for nothing.
		return res, nil
	}
	res.FingerprintMoved = true

	// Mint a new epoch so the new prefix is cached fresh. Mirror SwitchProfile:
	// before the first turn there is no epoch yet, and the initial runStep folds
	// the reloaded skills into epoch #1, so there is nothing to switch.
	if cur := a.epochMgr.CurrentEpoch(); cur != nil {
		res.OldEpochID = cur.EpochID
		epoch := a.epochMgr.SwitchEpoch("skills_reload", a.buildEpochComponents())
		res.NewEpochID = epoch.EpochID
	}
	return res, nil
}

// HasActiveBackgroundWork reports whether any background job (async subagent or
// background_bash) is still running. ReloadSkills mutates state those detached
// goroutines read — notably the shared skill store via skill_read — so a caller
// must refuse a reload while this is true. a.running alone covers only the main
// loop, not work that outlives the parent turn.
func (a *Agent) HasActiveBackgroundWork() bool {
	return a.Jobs != nil && a.Jobs.HasActive()
}
