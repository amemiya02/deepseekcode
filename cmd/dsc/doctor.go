package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/amemiya02/deepseekcode/internal/config"
	promptpkg "github.com/amemiya02/deepseekcode/internal/prompt"
	"github.com/amemiya02/deepseekcode/internal/session"
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
		checkAPIKey(cfg),
		checkAPIReachable(cfg),
		checkSQLite(),
		checkSnapshots(),
		checkTerminal(),
		checkGit(),
		checkInstructions(),
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

func checkAPIKey(cfg config.Config) checkResult {
	if cfg.API.Key == "" {
		return checkResult{"api_key", "fail", "DEEPSEEK_API_KEY is not set"}
	}
	if len(cfg.API.Key) < 10 {
		return checkResult{"api_key", "warn", "API key looks too short"}
	}
	return checkResult{"api_key", "ok", "set (" + cfg.API.Key[:4] + "...)"}
}

func checkAPIReachable(cfg config.Config) checkResult {
	if cfg.API.Key == "" || cfg.API.BaseURL == "" {
		return checkResult{"api_reachable", "fail", "skipped (missing key or base URL)"}
	}
	// 15s — reasoner cold starts and slow networks need headroom.
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodGet, cfg.API.BaseURL+"/v1/models", nil)
	if err != nil {
		return checkResult{"api_reachable", "fail", err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+cfg.API.Key)

	resp, err := client.Do(req)
	if err != nil {
		return checkResult{"api_reachable", "fail", err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return checkResult{"api_reachable", "fail", fmt.Sprintf("auth failed (HTTP %d)", resp.StatusCode)}
	}
	if resp.StatusCode/100 != 2 {
		return checkResult{"api_reachable", "warn", fmt.Sprintf("HTTP %d from %s", resp.StatusCode, cfg.API.BaseURL)}
	}
	return checkResult{"api_reachable", "ok", cfg.API.BaseURL + " responded"}
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
