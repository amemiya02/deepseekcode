package tools

// RegisterBuiltins installs the v0.1 core built-in tools into a fresh
// Registry. Tools are added in alphabetical order out of habit; the
// registry sorts on AsLLMTools regardless.
//
// Git tools are registered separately by internal/tools/git so callers
// can opt out (e.g. the validator pro-side tool list, which is empty).
func RegisterBuiltins(r *Registry, maxReadBytes, maxWriteBytes int64, cwd string) {
	r.Register(&ReadFile{MaxBytes: maxReadBytes, CWD: cwd})
	r.Register(&WriteFile{MaxBytes: maxWriteBytes, CWD: cwd})
	r.Register(NewApplyPatchTool(cwd, maxWriteBytes))
	r.Register(&EditFile{CWD: cwd})
	r.Register(Bash{})
	r.Register(BashPTY{})
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
