package config

import (
	"strconv"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/hooks"
)

// ValidationError describes a single config validation problem.
type ValidationError struct {
	Path    string // e.g. "mcp_servers[myserver].command"
	Message string
}

func (e ValidationError) Error() string {
	return e.Path + ": " + e.Message
}

var knownHookEvents = map[string]bool{
	"PreToolUse":         true,
	"PostToolUse":        true,
	"PostToolUseFailure": true,
	"SessionStart":       true,
	"SessionEnd":         true,
}

var knownHookTypes = map[string]bool{
	"subprocess": true,
	"builtin":    true,
}

var knownRuleDecisions = map[string]bool{
	"allow": true,
	"deny":  true,
	"ask":   true,
}

var knownProviderTypes = map[string]bool{
	"deepseek":      true,
	"openai-compat": true,
}

// ValidateStrict checks the full config tree for structural errors that
// Validate (the lightweight startup check) does not catch. Returns an
// empty slice when the config is sound.
func ValidateStrict(c *Config) []ValidationError {
	var errs []ValidationError

	// API timeouts must be positive.
	if c.API.FirstTokenTimeoutMs <= 0 {
		errs = append(errs, ValidationError{
			Path:    "api.first_token_timeout_ms",
			Message: "must be > 0",
		})
	}
	if c.API.ChunkStallTimeoutMs <= 0 {
		errs = append(errs, ValidationError{
			Path:    "api.chunk_stall_timeout_ms",
			Message: "must be > 0",
		})
	}

	// MCP servers: name and command required.
	for name, srv := range c.MCPServers {
		if name == "" {
			errs = append(errs, ValidationError{
				Path:    "mcp_servers",
				Message: "server name must not be empty",
			})
		}
		if srv.Command == "" {
			errs = append(errs, ValidationError{
				Path:    "mcp_servers[" + name + "].command",
				Message: "must not be empty",
			})
		}
	}

	// Hooks: event and type must be known values.
	for i, h := range c.Hooks {
		pfx := hookPath(i)
		if !knownHookEvents[h.Event] {
			errs = append(errs, ValidationError{
				Path:    pfx + ".event",
				Message: "unknown event " + quote(h.Event) + "; valid: PreToolUse, PostToolUse, PostToolUseFailure, SessionStart, SessionEnd",
			})
		}
		if h.Type != "" && !knownHookTypes[h.Type] {
			errs = append(errs, ValidationError{
				Path:    pfx + ".type",
				Message: "unknown type " + quote(h.Type) + "; valid: subprocess, builtin",
			})
		}
		if h.Type == "builtin" && h.Name == "" {
			errs = append(errs, ValidationError{
				Path:    pfx + ".name",
				Message: "builtin hooks must have a name",
			})
		} else if h.Type == "builtin" && !hooks.IsKnownBuiltin(h.Name) {
			errs = append(errs, ValidationError{
				Path:    pfx + ".name",
				Message: "unknown builtin " + quote(h.Name) + "; valid: " + strings.Join(hooks.KnownBuiltinNames, ", "),
			})
		}
		if h.Type == "subprocess" && h.Command == "" {
			errs = append(errs, ValidationError{
				Path:    pfx + ".command",
				Message: "subprocess hooks must have a command",
			})
		}
	}

	// Permission rules: decision must be allow/deny/ask.
	errs = append(errs, validateRules(c.Permissions.Rules.Allow, "allow")...)
	errs = append(errs, validateRules(c.Permissions.Rules.Deny, "deny")...)
	errs = append(errs, validateRules(c.Permissions.Rules.Ask, "ask")...)

	// Web config: search_provider must be known, searxng requires base_url
	if c.Web.SearchProvider != "" && c.Web.SearchProvider != "duckduckgo" && c.Web.SearchProvider != "searxng" {
		errs = append(errs, ValidationError{
			Path:    "web.search_provider",
			Message: "must be 'duckduckgo' or 'searxng', got " + quote(c.Web.SearchProvider),
		})
	}
	if c.Web.SearchProvider == "searxng" && c.Web.SearXNGBaseURL == "" {
		errs = append(errs, ValidationError{
			Path:    "web.searxng_base_url",
			Message: "required when search_provider is 'searxng'",
		})
	}

	for i, path := range c.Sandbox.AllowReadPaths {
		if path == "" {
			errs = append(errs, ValidationError{
				Path:    "sandbox.allow_read_paths[" + itoa(i) + "]",
				Message: "must not be empty",
			})
		}
	}
	for i, path := range c.Sandbox.AllowWritePaths {
		if path == "" {
			errs = append(errs, ValidationError{
				Path:    "sandbox.allow_write_paths[" + itoa(i) + "]",
				Message: "must not be empty",
			})
		}
	}

	if c.Active.Provider == "" {
		errs = append(errs, ValidationError{
			Path:    "active.provider",
			Message: "must not be empty",
		})
	} else if _, ok := c.Providers[c.Active.Provider]; !ok {
		errs = append(errs, ValidationError{
			Path:    "active.provider",
			Message: "unknown provider " + quote(c.Active.Provider),
		})
	}
	for name, p := range c.Providers {
		if name == "" {
			errs = append(errs, ValidationError{
				Path:    "providers",
				Message: "provider name must not be empty",
			})
		}
		if p.Type == "" {
			errs = append(errs, ValidationError{
				Path:    "providers[" + name + "].type",
				Message: "must not be empty",
			})
		} else if !knownProviderTypes[p.Type] {
			errs = append(errs, ValidationError{
				Path:    "providers[" + name + "].type",
				Message: "unknown provider type " + quote(p.Type),
			})
		}
		if p.BaseURL == "" {
			errs = append(errs, ValidationError{
				Path:    "providers[" + name + "].base_url",
				Message: "must not be empty",
			})
		}
		if p.EnvVar != "" && !envVarRe.MatchString("${"+p.EnvVar+"}") {
			errs = append(errs, ValidationError{
				Path:    "providers[" + name + "].env_var",
				Message: "must be an environment variable name",
			})
		}
	}

	return errs
}

func validateRules(rules []RuleItemConfig, bucket string) []ValidationError {
	var errs []ValidationError
	for i, r := range rules {
		if r.Tool == "" {
			errs = append(errs, ValidationError{
				Path:    "permissions.rules." + bucket + "[" + itoa(i) + "].tool",
				Message: "must not be empty",
			})
		}
	}
	return errs
}

func hookPath(i int) string {
	return "hooks[" + itoa(i) + "]"
}

func itoa(i int) string {
	return strconv.Itoa(i)
}

func quote(s string) string {
	if s == "" {
		return "(empty)"
	}
	return `"` + s + `"`
}
