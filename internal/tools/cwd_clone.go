package tools

// CloneForCWD returns a copy of t with its working directory rebound to cwd.
// Tools that are not cwd-aware are returned unchanged.
// This is used when a sub-agent runs in a git worktree with a different
// filesystem root, so that relative-path tools resolve against the
// worktree instead of the parent cwd.
func CloneForCWD(t Tool, cwd string) Tool {
	if cwd == "" {
		return t
	}
	switch v := t.(type) {
	case *ReadFile:
		cp := *v
		cp.CWD = cwd
		return &cp
	case *WriteFile:
		cp := *v
		cp.CWD = cwd
		return &cp
	case *EditFile:
		cp := *v
		cp.CWD = cwd
		return &cp
	case *ApplyPatchTool:
		cp := NewApplyPatchTool(cwd, v.maxWriteBytes)
		cp.tracker = v.tracker // preserve the freshness guard across the clone (T3.2)
		return cp
	default:
		return t
	}
}
