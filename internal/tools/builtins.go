package tools

import (
	"github.com/amemiya02/deepseekcode/internal/memory"
	"github.com/amemiya02/deepseekcode/internal/sandbox"
)

// RegisterBuiltins installs the v0.1 core built-in tools into a fresh
// Registry. Tools are added in alphabetical order out of habit; the
// registry sorts on AsLLMTools regardless.
//
// Git tools are registered separately by internal/tools/git so callers
// can opt out (e.g. the validator pro-side tool list, which is empty).
func RegisterBuiltins(r *Registry, maxReadBytes, maxWriteBytes int64, cwd string) {
	RegisterBuiltinsWithSandbox(r, maxReadBytes, maxWriteBytes, cwd, nil, sandbox.Profile{})
}

func RegisterBuiltinsWithSandbox(r *Registry, maxReadBytes, maxWriteBytes int64, cwd string, sb sandbox.Sandbox, profile sandbox.Profile) {
	// One read-before-write tracker shared by every file tool in this registry,
	// so a read by read_file is visible to a later edit_file/write_file/
	// apply_patch (T3.2). Stored on the registry so the agent can invalidate it
	// when the conversation is compacted.
	ft := NewFileTracker()
	r.fileTracker = ft
	ap := NewApplyPatchTool(cwd, maxWriteBytes)
	ap.tracker = ft
	r.Register(&ReadFile{MaxBytes: maxReadBytes, CWD: cwd, Tracker: ft})
	r.Register(&WriteFile{MaxBytes: maxWriteBytes, CWD: cwd, Tracker: ft})
	r.Register(ap)
	r.Register(&EditFile{CWD: cwd, Tracker: ft})
	r.Register(&Bash{Sandbox: sb, SandboxProfile: profile, CWD: cwd})
	r.Register(&BashPTY{Sandbox: sb, SandboxProfile: profile, CWD: cwd})
	r.Register(Glob{})
	r.Register(Grep{})
	r.Register(Ls{})
	r.Register(&TodoWrite{})

	// Structured git tools (v0.1 differentiator).
	r.Register(GitDiff{})
	r.Register(GitShow{})
	r.Register(GitBlame{})
	r.Register(GitLog{})
}

// RegisterMemoryTools installs the remember/recall/forget tools backed by
// the given Store. This is a separate helper so callers that manage their own
// memory store (e.g. the agent session) have a single, discoverable
// registration point.
func RegisterMemoryTools(r *Registry, store memory.Store) {
	r.Register(NewRememberTool(store))
	r.Register(NewRecallTool(store))
	r.Register(NewForgetTool(store))
}
