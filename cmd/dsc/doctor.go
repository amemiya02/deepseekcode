package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/amemiya02/deepseekcode/internal/agents"
	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/lsp"
	"github.com/amemiya02/deepseekcode/internal/mcp"
	promptpkg "github.com/amemiya02/deepseekcode/internal/prompt"
	sandboxpkg "github.com/amemiya02/deepseekcode/internal/sandbox"
	"github.com/amemiya02/deepseekcode/internal/session"
	"github.com/amemiya02/deepseekcode/internal/version"
)

type checkResult struct {
	Name   string
	Status string // "ok", "warn", "fail"
	Detail string
}

// runDoctor inspects the environment and reports issues. loadErr is the
// (possibly non-nil) result of config.Load — surfaced as its own check
// so a malformed config doesn't silently degrade the other diagnostics.
func runDoctor(cfg config.Config, loadErr error) error {
	results := []checkResult{
		checkConfig(loadErr),
		checkActiveProvider(cfg),
		checkProviderSecret(cfg),
		checkProviderCapabilities(cfg),
		checkSandbox(cfg),
		checkAPIReachable(cfg),
		checkDeepSeekAccount(cfg),
		checkPricingFreshness(time.Now().UTC()),
		checkSQLite(),
		checkSnapshots(),
		checkTerminal(),
		checkGit(),
		checkInstructions(),
		checkHooks(cfg),
		checkAgentDefs(),
		checkRules(cfg),
		checkMCP(cfg),
		checkLSP(),
		checkCompaction(),
		checkConfigValidation(cfg),
		checkUpdate(),
		{
			Name:   "platform",
			Status: "ok",
			Detail: fmt.Sprintf("%s/%s %s", runtime.GOOS, runtime.GOARCH, runtime.Version()),
		},
	}

	fmt.Println("deepseekcode doctor")
	fmt.Println(strings.Repeat("─", 50))
	hasFail := false
	for _, r := range results {
		icon := "✓"
		switch r.Status {
		case "warn":
			icon = "⚠"
		case "fail":
			icon = "✗"
			hasFail = true
		}
		fmt.Printf("  %s %-20s %s\n", icon, r.Name, r.Detail)
	}
	fmt.Println(strings.Repeat("─", 50))
	if hasFail {
		fmt.Println("Some checks failed. Fix the issues above before using deepseekcode.")
		return fmt.Errorf("doctor failed")
	}
	fmt.Println("All checks passed.")
	return nil
}

func checkConfig(err error) checkResult {
	if err != nil {
		return checkResult{"config", "fail", err.Error()}
	}
	return checkResult{"config", "ok", "loaded"}
}

func checkActiveProvider(cfg config.Config) checkResult {
	name, pcfg, ok := activeProvider(cfg)
	if !ok {
		return checkResult{"active provider", "fail", "not configured: " + cfg.Active.Provider}
	}
	return checkResult{"active provider", "ok", fmt.Sprintf("%s (type=%s, base_url=%s)", name, pcfg.Type, pcfg.BaseURL)}
}

func checkProviderSecret(cfg config.Config) checkResult {
	_, pcfg, ok := activeProvider(cfg)
	if !ok {
		return checkResult{"secret", "fail", "active provider not configured"}
	}
	_, source, err := config.ResolveSecretWithSource(pcfg)
	if err != nil {
		return checkResult{"secret", "fail", "NOT FOUND: " + err.Error()}
	}
	switch source {
	case config.SecretSourceEnv:
		return checkResult{"secret", "ok", "present (from env " + pcfg.EnvVar + ")"}
	case config.SecretSourceFile:
		return checkResult{"secret", "ok", "present (from file " + config.SecretsPath() + ")"}
	default:
		return checkResult{"secret", "ok", "present (from explicit config)"}
	}
}

func checkProviderCapabilities(cfg config.Config) checkResult {
	_, pcfg, ok := activeProvider(cfg)
	if !ok {
		return checkResult{"capabilities", "fail", "active provider not configured"}
	}
	model := cfg.Defaults.Model
	if !cfg.DefaultsModelExplicit && pcfg.DefaultModel != "" {
		model = pcfg.DefaultModel
	}
	prov, err := llm.NewProvider(pcfg.Type, llm.ProviderConfig{
		BaseURL:         pcfg.BaseURL,
		APIKey:          "doctor-redacted",
		DefaultModel:    model,
		ValidationModel: model,
	})
	if err != nil {
		return checkResult{"capabilities", "fail", err.Error()}
	}
	caps := prov.Capabilities()
	efforts := ""
	for i, e := range caps.ReasoningEfforts {
		if i > 0 {
			efforts += ","
		}
		efforts += string(e)
	}
	detail := fmt.Sprintf("thinking=%v prefix_cache=%v json_mode=%v max_ctx=%d",
		caps.Thinking, caps.PrefixCache, caps.JSONMode, caps.MaxContextTokens)
	if caps.MaxOutputTokens > 0 {
		detail += fmt.Sprintf(" max_output=%d", caps.MaxOutputTokens)
	}
	if efforts != "" {
		detail += " effort=" + efforts
	}
	return checkResult{"capabilities", "ok", detail}
}

func checkDeepSeekAccount(cfg config.Config) checkResult {
	_, pcfg, ok := activeProvider(cfg)
	if !ok {
		return checkResult{"account", "fail", "active provider not configured"}
	}
	if pcfg.Type != "deepseek" {
		return checkResult{"account", "ok", "skipped (provider type " + pcfg.Type + ")"}
	}
	apiKey, err := config.ResolveSecret(pcfg)
	if err != nil {
		return checkResult{"account", "warn", "cannot resolve secret: " + err.Error()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := llm.NewClient(apiKey, pcfg.BaseURL)
	ub, err := client.GetUserBalance(ctx)
	if err != nil {
		return checkResult{"account", "warn", "balance endpoint error: " + err.Error()}
	}
	if !ub.IsAvailable {
		return checkResult{"account", "warn", "unavailable " + formatUserBalanceForDoctor(ub)}
	}
	return checkResult{"account", "ok", "available " + formatUserBalanceForDoctor(ub)}
}

// formatUserBalanceForDoctor renders balance infos as "CNY=110.00 USD=2.50"
// in deterministic sorted-currency order.
func formatUserBalanceForDoctor(balance llm.UserBalance) string {
	infos := make([]llm.BalanceInfo, len(balance.BalanceInfos))
	copy(infos, balance.BalanceInfos)
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Currency < infos[j].Currency
	})
	var parts []string
	for _, bi := range infos {
		parts = append(parts, bi.Currency+"="+bi.TotalBalance)
	}
	return strings.Join(parts, " ")
}

func checkPricingFreshness(now time.Time) checkResult {
	f, err := llm.PricingFreshnessAt(now)
	if err != nil {
		return checkResult{"pricing", "warn", "freshness check failed: " + err.Error()}
	}
	detail := fmt.Sprintf("checked %s (%d days old) source=%s",
		llm.PricingCheckedDate, f.AgeDays, f.SourceURL)
	if f.Stale {
		return checkResult{"pricing", "warn", detail}
	}
	return checkResult{"pricing", "ok", detail}
}

func checkSandbox(cfg config.Config) checkResult {
	sb := sandboxpkg.Detect()
	status := "ok"
	if cfg.Sandbox.Enabled && !sb.Available() {
		status = "warn"
	}
	return checkResult{"sandbox", status,
		fmt.Sprintf("%s available=%v enabled=%v", sb.Name(), sb.Available(), cfg.Sandbox.Enabled)}
}

func checkAPIReachable(cfg config.Config) checkResult {
	_, pcfg, ok := activeProvider(cfg)
	if !ok {
		return checkResult{"api_reachable", "fail", "skipped (active provider not configured)"}
	}
	apiKey, err := config.ResolveSecret(pcfg)
	if err != nil || pcfg.BaseURL == "" {
		return checkResult{"api_reachable", "fail", "skipped (missing key or base URL)"}
	}
	// 15s — reasoner cold starts and slow networks need headroom.
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodGet, pcfg.BaseURL+"/v1/models", nil)
	if err != nil {
		return checkResult{"api_reachable", "fail", err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return checkResult{"api_reachable", "fail", err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return checkResult{"api_reachable", "fail", fmt.Sprintf("auth failed (HTTP %d)", resp.StatusCode)}
	}
	if resp.StatusCode/100 != 2 {
		return checkResult{"api_reachable", "warn", fmt.Sprintf("HTTP %d from %s", resp.StatusCode, pcfg.BaseURL)}
	}
	return checkResult{"api_reachable", "ok", pcfg.BaseURL + " responded"}
}

func activeProvider(cfg config.Config) (string, config.ProviderConfigTOML, bool) {
	name := cfg.Active.Provider
	if name == "" {
		name = "deepseek"
	}
	pcfg, ok := cfg.Providers[name]
	return name, pcfg, ok
}

func checkSQLite() checkResult {
	store, err := session.Open("")
	if err != nil {
		return checkResult{"sqlite", "fail", err.Error()}
	}
	defer store.Close()
	return checkResult{"sqlite", "ok", "writable at " + store.Path()}
}

// checkSnapshots verifies we can write under the system temp dir; we
// deliberately do NOT create .deepseek/ in the user's cwd just to test.
func checkSnapshots() checkResult {
	dir, err := os.MkdirTemp("", "dsc-doctor-")
	if err != nil {
		return checkResult{"snapshots", "fail", "tmpdir: " + err.Error()}
	}
	defer os.RemoveAll(dir)

	testFile := filepath.Join(dir, "test")
	if err := os.WriteFile(testFile, []byte("ok"), 0o644); err != nil {
		return checkResult{"snapshots", "fail", "not writable: " + err.Error()}
	}
	return checkResult{"snapshots", "ok", "tmpdir writable"}
}

func checkTerminal() checkResult {
	var details []string
	if fi, err := os.Stdout.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) != 0 {
		details = append(details, "tty")
	} else {
		details = append(details, "pipe")
	}
	if term := os.Getenv("TERM"); term != "" {
		details = append(details, "TERM="+term)
	}
	if lang := os.Getenv("LANG"); lang != "" && strings.Contains(strings.ToUpper(lang), "UTF") {
		details = append(details, "UTF-8")
	}
	return checkResult{"terminal", "ok", strings.Join(details, ", ")}
}

func checkGit() checkResult {
	// Git is optional — the agent runs fine without it. Surface a warn
	// so the user knows git-aware features (git_diff / commit log in
	// the system prompt) will be inactive, but don't fail Doctor.
	path, err := exec.LookPath("git")
	if err != nil {
		return checkResult{"git", "warn", "git not found in PATH (git-aware prompt context will be empty)"}
	}
	out, err := exec.Command("git", "version").Output()
	if err != nil {
		return checkResult{"git", "warn", "found at " + path + " but version check failed"}
	}
	return checkResult{"git", "ok", strings.TrimSpace(string(out))}
}

func checkCompaction() checkResult {
	data, err := os.ReadFile(filepath.Join(".deepseek", "last_session"))
	if err != nil {
		return checkResult{"compaction", "ok", "no recent session (compaction history n/a)"}
	}
	id := strings.TrimSpace(string(data))
	if id == "" {
		return checkResult{"compaction", "ok", "no recent session (compaction history n/a)"}
	}
	store, err := session.Open("")
	if err != nil {
		return checkResult{"compaction", "warn", "store open failed: " + err.Error()}
	}
	defer store.Close()
	sess, err := store.GetSession(context.Background(), id)
	if err != nil {
		return checkResult{"compaction", "warn", "session lookup failed: " + err.Error()}
	}
	return checkResult{"compaction", "ok",
		fmt.Sprintf("session compacted %d times; last summary: %d chars",
			sess.CompactionCount, len(sess.CompactionSummary))}
}

func checkHooks(cfg config.Config) checkResult {
	var parts []string
	for _, h := range cfg.Hooks {
		label := h.Name
		if label == "" {
			label = "<inline>"
		}
		parts = append(parts, fmt.Sprintf("%s:%s(%s)", h.Event, label, h.Type))
	}
	if cfg.Duet.Enabled {
		parts = append(parts, "PreToolUse:duet(builtin) [default]")
	}
	if len(parts) == 0 {
		return checkResult{"hooks", "ok", "none configured"}
	}
	return checkResult{"hooks", "ok", strings.Join(parts, ", ")}
}

func checkAgentDefs() checkResult {
	cwd, err := os.Getwd()
	if err != nil {
		return checkResult{"agent defs", "warn", "cannot resolve cwd: " + err.Error()}
	}
	home, _ := os.UserHomeDir()
	return agentDefsResult(cwd, home)
}

// agentDefsResult is the testable core of checkAgentDefs. It loads agent
// definitions, flags cwd .md files that were silently skipped (malformed or
// over the 64 KiB cap), and warns on out-of-range numeric frontmatter. Skip
// detection compares on-disk cwd files against loaded defs sourced from cwd
// (via AgentDef.Path), so home-dir defs and name-shadowing don't false-positive.
func agentDefsResult(cwd, home string) checkResult {
	defs, err := agents.Load(cwd, home)
	if err != nil {
		return checkResult{"agent defs", "warn", "load error: " + err.Error()}
	}

	cwdAgentDir := filepath.Join(cwd, ".deepseek", "agent")
	cwdMd := 0
	_ = filepath.WalkDir(cwdAgentDir, func(p string, d os.DirEntry, werr error) error {
		if werr != nil || d == nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(p, ".md") {
			cwdMd++
		}
		return nil
	})
	cwdLoaded := 0
	for _, d := range defs {
		if strings.HasPrefix(d.Path, cwdAgentDir) {
			cwdLoaded++
		}
	}

	if len(defs) == 0 && cwdMd == 0 {
		return checkResult{"agent defs", "ok", "none configured"}
	}

	var warns []string
	if cwdMd > cwdLoaded {
		warns = append(warns, fmt.Sprintf("%d of %d cwd .md file(s) skipped (malformed or >64KiB) — run `dsc agent validate`", cwdMd-cwdLoaded, cwdMd))
	}
	for _, d := range defs {
		if d.Temperature != nil && (*d.Temperature < 0 || *d.Temperature > 2) {
			warns = append(warns, fmt.Sprintf("%s: temperature %.2f out of [0,2]", d.Name, *d.Temperature))
		}
		if d.TopP != nil && (*d.TopP < 0 || *d.TopP > 1) {
			warns = append(warns, fmt.Sprintf("%s: top_p %.2f out of [0,1]", d.Name, *d.TopP))
		}
	}
	if len(warns) > 0 {
		sort.Strings(warns) // deterministic order (defs is a map)
		return checkResult{"agent defs", "warn", fmt.Sprintf("%d loaded; %s", len(defs), strings.Join(warns, "; "))}
	}
	return checkResult{"agent defs", "ok", fmt.Sprintf("%d loaded, fields valid", len(defs))}
}

func checkRules(cfg config.Config) checkResult {
	r := cfg.Permissions.Rules
	nAllow := len(r.Allow)
	nDeny := len(r.Deny)
	nAsk := len(r.Ask)
	if nAllow+nDeny+nAsk == 0 {
		return checkResult{"permission rules:", "ok", "none configured"}
	}
	return checkResult{"permission rules:", "ok",
		fmt.Sprintf("%d allow, %d deny, %d ask", nAllow, nDeny, nAsk)}
}

func checkMCP(cfg config.Config) checkResult {
	if len(cfg.MCPServers) == 0 {
		return checkResult{"mcp", "ok", "none configured"}
	}
	var parts []string
	reg := mcp.NewRegistry()
	for name, srv := range cfg.ActiveMCPServers() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := reg.Connect(ctx, name, srv.Command, srv.Args, srv.Env)
		cancel()
		if err != nil {
			parts = append(parts, fmt.Sprintf("mcp[%s]: failed (%v)", name, err))
		} else {
			proxies := reg.Servers()
			for _, p := range proxies {
				if p.Name == name && p.State == mcp.StateConnected {
					parts = append(parts, fmt.Sprintf("mcp[%s]: connected (%d tools)", name, len(p.Tools)))
					break
				}
			}
		}
	}
	reg.Shutdown()
	if len(parts) == 0 {
		return checkResult{"mcp", "warn", "no servers started"}
	}
	return checkResult{"mcp", "ok", strings.Join(parts, ", ")}
}

func checkLSP() checkResult {
	cwd, err := os.Getwd()
	if err != nil {
		return checkResult{"lsp", "warn", "cwd unreadable: " + err.Error()}
	}
	servers := lsp.DetectServers(cwd)
	if len(servers) == 0 {
		return checkResult{"lsp", "ok", "no language servers detected for this project"}
	}
	var parts []string
	for _, s := range servers {
		parts = append(parts, s.Name)
	}
	return checkResult{"lsp", "ok", fmt.Sprintf("available: %s", strings.Join(parts, ", "))}
}

func checkInstructions() checkResult {
	cwd, err := os.Getwd()
	if err != nil {
		return checkResult{"instructions", "warn", "cwd unreadable: " + err.Error()}
	}
	home, _ := os.UserHomeDir()
	files, err := promptpkg.LoadInstructionFiles(cwd, home)
	if err != nil {
		return checkResult{"instructions", "warn", err.Error()}
	}
	if len(files) == 0 {
		return checkResult{"instructions", "warn", "none found — consider creating DEEPSEEK.md"}
	}
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, filepath.Base(f.Path))
	}
	return checkResult{"instructions", "ok", fmt.Sprintf("%d file(s): %s", len(files), strings.Join(names, ", "))}
}

func checkConfigValidation(cfg config.Config) checkResult {
	errs := config.ValidateStrict(&cfg)
	if len(errs) == 0 {
		return checkResult{"config validation", "ok", "PASS"}
	}
	details := make([]string, 0, len(errs))
	for _, e := range errs {
		details = append(details, e.Error())
	}
	return checkResult{"config validation", "fail", strings.Join(details, "; ")}
}

func checkUpdate() checkResult {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tag, _, err := version.LatestRelease(ctx)
	if err != nil {
		return checkResult{"update", "ok", "update check skipped (" + err.Error() + ")"}
	}
	if version.CompareVersions(version.Version, tag) >= 0 {
		return checkResult{"update", "ok", "up to date (" + version.Version + ")"}
	}
	return checkResult{"update", "ok", fmt.Sprintf("current %s, latest %s (run: dsc upgrade)", version.Version, tag)}
}
