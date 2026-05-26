package config

// ModelFlash is the v0.1 default main-loop model.
const ModelFlash = "deepseek-v4-flash"

// ModelPro is the escalation/validator model.
const ModelPro = "deepseek-v4-pro"

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
			Model:          ModelFlash,
			Thinking:       true,
			Theme:          "dark",
			VimKeybindings: true,
			AutoReasoning:  false,
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
