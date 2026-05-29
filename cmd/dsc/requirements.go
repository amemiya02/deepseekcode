package main

import (
	"fmt"

	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/sandbox"
)

// resolveLaunchMode maps the CLI mode flags to a permission Mode and enforces
// the optional admin policy floor (.deepseek/requirements.toml). It refuses —
// does not clamp — a launch that exceeds the repo's ceiling, so the floor holds
// even against --yolo. Shared by runTUI and runOneShot so both entry points
// honor the same floor. sandboxEffective is whether a REAL OS sandbox is in
// force (not merely requested in config) — see sandboxEffectiveFor.
func resolveLaunchMode(mf modeFlags, cwd string, sandboxEffective bool) (permissions.Mode, error) {
	mode := permissions.ModeDefault
	switch {
	case mf.yolo:
		mode = permissions.ModeYolo
	case mf.readOnly:
		mode = permissions.ModeReadOnly
	case mf.askAll:
		mode = permissions.ModeAskAll
	}
	req, err := config.LoadRequirements(cwd)
	if err != nil {
		return mode, err
	}
	if err := enforceRequirements(req, mode, sandboxEffective); err != nil {
		return mode, err
	}
	return mode, nil
}

// enforceRequirements applies the admin policy floor. A nil floor (no
// requirements.toml) is always satisfied. A launch mode more permissive than
// MaxMode, or a require_sandbox floor with no real sandbox in force, is refused
// with a message an operator can act on.
func enforceRequirements(req *config.Requirements, mode permissions.Mode, sandboxEffective bool) error {
	if req == nil {
		return nil
	}
	if req.MaxMode != "" {
		ceiling, ok := permissions.ModeFromRuleset(req.MaxMode)
		if !ok {
			return fmt.Errorf("%s: unrecognized max_mode %q (use plan|read-only|ask-all|default|yolo)", config.RequirementsPath, req.MaxMode)
		}
		if permissions.ModeMoreDangerousThan(mode, ceiling) {
			return fmt.Errorf("%s forbids this launch: requested mode %q exceeds max_mode %q", config.RequirementsPath, modeName(mode), req.MaxMode)
		}
	}
	if req.RequireSandbox && !sandboxEffective {
		return fmt.Errorf("%s requires the OS sandbox, but no real sandbox is in force (it is disabled, or unavailable on this platform)", config.RequirementsPath)
	}
	return nil
}

// sandboxEffectiveFor reports whether a REAL OS sandbox will wrap commands, as
// opposed to the config merely requesting one. sandboxFromConfig returns nil
// when [sandbox] is disabled and a noop backend (Name=="noop") on platforms
// without a real implementation; the tool layer then runs unsandboxed. The
// require_sandbox floor must check this, not config intent, or it honest-washes
// an unprotected launch (e.g. --yolo-equivalent risk on Windows).
func sandboxEffectiveFor(sb sandbox.Sandbox) bool {
	return sb != nil && sb.Available() && sb.Name() != "noop"
}

// modeName renders a permission Mode using the same names the flags and
// rulesets use, for operator-facing messages.
func modeName(m permissions.Mode) string {
	switch m {
	case permissions.ModeYolo:
		return "yolo"
	case permissions.ModeReadOnly:
		return "read-only"
	case permissions.ModeAskAll:
		return "ask-all"
	case permissions.ModePlan:
		return "plan"
	default:
		return "default"
	}
}
