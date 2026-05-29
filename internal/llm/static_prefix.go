package llm

import (
	"encoding/json"
	"fmt"
	"sort"
)

// canonicalizeTools returns a copy of tools sorted by function name with each
// tool's JSON-Schema Parameters canonicalized (recursive key-sort via
// canonicalJSON). It is the SINGLE source of truth for cache-stable tool
// serialization: both MarshalCacheStable (the wire bytes / DeepSeek cache key)
// and the Prefix Fingerprint build on it, so the wire payload and the
// fingerprint can no longer diverge on tool order or JSON-Schema key order —
// the invariant that used to be held only by a "same as MarshalCacheStable"
// comment.
//
// On a malformed schema it records the first error but leaves that tool's
// parameters raw and keeps going, so a strict caller (the wire marshaler) can
// fail closed on the returned error while a best-effort caller (the fingerprint)
// can ignore it and still hash a stable, fully-sorted slice. The input slice is
// never mutated.
func canonicalizeTools(tools []Tool) ([]Tool, error) {
	out := make([]Tool, len(tools))
	copy(out, tools)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Function.Name < out[j].Function.Name
	})
	var firstErr error
	for i := range out {
		if len(out[i].Function.Parameters) == 0 {
			continue
		}
		canon, err := canonicalJSON(out[i].Function.Parameters)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("canonicalizing tool %q parameters: %w", out[i].Function.Name, err)
			}
			continue
		}
		out[i].Function.Parameters = canon
	}
	return out, firstErr
}

// StaticPrefix is the model-visible, cache-stable head of a request: the system
// prompt (which already contains the rendered skill directory), the
// actually-sent tool specs, and any leading few-shot turns. It is the single
// source of truth for "what the model sees" — see /CONTEXT.md.
//
// FewShots is carried for forward-compatibility but is NOT yet folded into the
// fingerprint: few-shots are currently always empty, and folding them in is the
// deliberate fingerprint redefinition done in M3 of
// docs/refactor-prefix-fingerprint.md (the visible/latent split). Keeping M1 a
// pure refactor means StaticPrefix.Fingerprint() reproduces the existing
// ComputeFingerprint value exactly.
type StaticPrefix struct {
	System   string
	Tools    []Tool
	FewShots []Message
}

// Fingerprint returns the canonical hash of the static prefix. Equal across any
// cache-irrelevant reordering (tool order, JSON-Schema key order). The Combined
// hash is the DeepSeek cache key.
func (p StaticPrefix) Fingerprint() PrefixFingerprint {
	sysH := sha256hex(p.System)
	toolsH := hashToolsCanonical(p.Tools)
	return PrefixFingerprint{sysH, toolsH, sha256hex(sysH + ":" + toolsH)}
}

// ToolDiff breaks down how a tool set changed between two static prefixes. The
// name lists are sorted. SchemaChanged compares the full canonical tool spec
// (type, description, key-sorted schema), so it matches exactly what moves
// ToolsSHA256 — tool order and JSON-Schema key order never register.
type ToolDiff struct {
	Added         []string
	Removed       []string
	SchemaChanged []string
}

// Changed reports whether the tool set differs.
func (d ToolDiff) Changed() bool {
	return len(d.Added) > 0 || len(d.Removed) > 0 || len(d.SchemaChanged) > 0
}

// PrefixDiff is the typed verdict of comparing two StaticPrefixes: which
// model-visible components moved. It is the single "did the prefix drift?"
// answer the per-turn monitor (and, in M3, the epoch manager) build on.
//
// FewShots is intentionally absent: it is not yet part of the fingerprint
// (see the StaticPrefix doc / M3 of docs/refactor-prefix-fingerprint.md).
type PrefixDiff struct {
	SystemChanged bool
	Tools         ToolDiff
}

// Changed reports whether any model-visible component moved.
func (d PrefixDiff) Changed() bool { return d.SystemChanged || d.Tools.Changed() }

// Which returns the legacy short drift label ("sys" | "tools" | "sys+tools" |
// "") used in receipts and drift events.
func (d PrefixDiff) Which() string {
	switch {
	case d.SystemChanged && d.Tools.Changed():
		return "sys+tools"
	case d.SystemChanged:
		return "sys"
	case d.Tools.Changed():
		return "tools"
	default:
		return ""
	}
}

// Diff returns the typed component diff between two static prefixes. System is
// compared by value; tools by name (added/removed) and by full canonical spec
// (schema-changed), so the result moves exactly when the fingerprint does.
func Diff(oldPfx, newPfx StaticPrefix) PrefixDiff {
	return PrefixDiff{
		SystemChanged: oldPfx.System != newPfx.System,
		Tools:         diffTools(oldPfx.Tools, newPfx.Tools),
	}
}

func diffTools(oldTools, newTools []Tool) ToolDiff {
	oc, _ := canonicalizeTools(oldTools)
	nc, _ := canonicalizeTools(newTools)
	oldByName := make(map[string]Tool, len(oc))
	for _, t := range oc {
		oldByName[t.Function.Name] = t
	}
	newByName := make(map[string]Tool, len(nc))
	for _, t := range nc {
		newByName[t.Function.Name] = t
	}
	var d ToolDiff
	for name := range newByName {
		if _, ok := oldByName[name]; !ok {
			d.Added = append(d.Added, name)
		}
	}
	for name := range oldByName {
		if _, ok := newByName[name]; !ok {
			d.Removed = append(d.Removed, name)
		}
	}
	for name, nt := range newByName {
		ot, ok := oldByName[name]
		if !ok {
			continue
		}
		ob, _ := json.Marshal(ot)
		nb, _ := json.Marshal(nt)
		if string(ob) != string(nb) {
			d.SchemaChanged = append(d.SchemaChanged, name)
		}
	}
	sort.Strings(d.Added)
	sort.Strings(d.Removed)
	sort.Strings(d.SchemaChanged)
	return d
}
