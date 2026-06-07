package config

// ModelFlash is the v0.1 default main-loop model.
const ModelFlash = "deepseek-v4-flash"

// ModelPro is the escalation/validator model.
const ModelPro = "deepseek-v4-pro"

// legacyAliases are DeepSeek model IDs scheduled for retirement on 2026-07-24.
// They are still accepted but not shown as primary picker rows.
var legacyAliases = map[string]bool{
	"deepseek-chat":     true,
	"deepseek-reasoner": true,
}

// IsOfficialDeepSeekV4Model reports whether model is one of the two
// official DeepSeek V4 models (flash or pro).
func IsOfficialDeepSeekV4Model(model string) bool {
	return model == ModelFlash || model == ModelPro
}

// IsLegacyDeepSeekAlias reports whether model is a legacy alias
// scheduled for retirement on 2026-07-24.
func IsLegacyDeepSeekAlias(model string) bool {
	return legacyAliases[model]
}

// Default returns a Config populated with v0.1 defaults.
// Callers then layer user/project config + CLI flags on top.
func Default() Config {
	return Config{
		API: APIConfig{
			BaseURL:             "https://api.deepseek.com",
			FirstTokenTimeoutMs: 45000,
			ChunkStallTimeoutMs: 20000,
		},
		Defaults: DefaultsConfig{
			Model:           ModelFlash,
			Thinking:        true,
			ReasoningEffort: "max",
			Theme:           "dark",
			VimKeybindings:  true,
			AutoReasoning:   false,
		},
		Tools: ToolsConfig{
			MaxReadBytes:  5 * 1024 * 1024, // 5 MiB
			MaxWriteBytes: 5 * 1024 * 1024, // 5 MiB
		},
		Duet: DuetConfig{
			Enabled:            true,
			RetryOnFailure:     true,
			ValidatorTimeoutMs: 10000,
			ExtraDestructive:   nil,
		},
		Permissions: PermissionsConfig{
			Default: "ask",
			AllowBash: []string{
				"git status *",
				"git log *",
				"git diff *",
				"ls *",
				"pwd",
				"cat *",
			},
			SecretPathPatterns: []string{
				"*.pem",
				"*.key",
				"id_rsa*",
				".env*",
			},
		},
		Sessions: SessionsConfig{
			TTLDays:       90,
			SnapshotKeep:  30,
			AutoResumeAge: 24,
		},
		MCPServers: map[string]MCPServerConfig{},
		Web: WebConfig{
			Enabled:        true,
			SearchProvider: "duckduckgo",
		},
		Sandbox: SandboxConfig{
			Enabled:         false,
			AllowReadPaths:  []string{"/usr", "/System", "/Library"},
			AllowWritePaths: nil,
			AllowNetwork:    false,
		},
		UI: UIConfig{
			Accent:   "indigo",
			Density:  "comfortable",
			Language: "auto",
		},
		Network: NetworkConfig{
			ProxyMode:   "auto",
			ProxyScheme: "http",
		},
		Cache: CacheConfig{
			AlignUnit: 128, // empirically measured via bench/cmd/cacheprobe
		},
		Active: ActiveConfig{Provider: "deepseek"},
		Providers: map[string]ProviderConfigTOML{
			"deepseek": {
				Type:                "deepseek",
				BaseURL:             "https://api.deepseek.com",
				EnvVar:              "DEEPSEEK_API_KEY",
				FirstTokenTimeoutMs: 45000,
				ChunkStallTimeoutMs: 20000,
			},
		},
	}
}
