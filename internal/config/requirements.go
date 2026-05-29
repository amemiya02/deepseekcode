package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// RequirementsPath is the project-relative location of the admin policy floor.
const RequirementsPath = ".deepseek/requirements.toml"

// Requirements is an optional, admin-controlled policy floor committed to a repo
// at .deepseek/requirements.toml. Unlike config.toml (user preferences, often
// per-machine and gitignored), it constrains the launch from ABOVE the CLI
// flags: a launch more permissive than the floor is REFUSED outright, never
// silently clamped, so a repo can forbid `dsc --yolo` regardless of the user's
// flags or personal config.
//
// Absent file → no floor (the mechanism is opt-in). A malformed file is an
// error: an admin who wrote a floor should learn it is broken rather than have
// it silently ignored and fail open.
type Requirements struct {
	// MaxMode is the most permissive permission mode the session may run in.
	// One of "plan", "read-only", "ask-all", "default", "yolo" (the same names
	// as the --yolo/--read-only/--ask-all flags and permission rulesets). Empty
	// means no ceiling on the mode.
	//
	// Danger ordering (least → most permissive): plan = read-only < ask-all <
	// default < yolo. The floor refuses any launch strictly more permissive than
	// MaxMode. So max_mode="default" forbids only yolo; max_mode="read-only"
	// forbids default/ask-all/yolo. Note max_mode="ask-all" ALSO forbids the
	// normal default tiered policy — ask-all gates every call through a human, so
	// it ranks as LESS permissive than default. The common admin floors are
	// "default" (ban yolo) and "read-only" (ban all writes).
	MaxMode string `toml:"max_mode"`

	// RequireSandbox refuses the launch when the OS sandbox is disabled.
	RequireSandbox bool `toml:"require_sandbox"`
}

// IsZero reports whether the floor imposes no constraints (so callers can treat
// an all-default file the same as an absent one).
func (r Requirements) IsZero() bool {
	return r.MaxMode == "" && !r.RequireSandbox
}

// LoadRequirements reads .deepseek/requirements.toml under cwd. It returns
// (nil, nil) when the file is absent (the floor is opt-in) and an error when it
// exists but cannot be read or parsed.
func LoadRequirements(cwd string) (*Requirements, error) {
	path := filepath.Join(cwd, RequirementsPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("requirements: read %s: %w", RequirementsPath, err)
	}
	var req Requirements
	if err := toml.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("requirements: parse %s: %w", RequirementsPath, err)
	}
	return &req, nil
}
