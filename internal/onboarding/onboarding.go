// Package onboarding provides helpers that detect whether the user needs
// first-run configuration before the agent can make API calls.
package onboarding

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/amemiya02/deepseekcode/internal/config"
)

// NeedsOnboarding reports whether the active provider has no resolvable API
// key or the base URL is empty — both are required to make any call.
//
// Resolution order for the key:
//  1. cfg.API.Key (legacy / direct field populated by Load or CLI flag)
//  2. cfg.Providers[cfg.Active.Provider] via config.ResolveSecret
//
// The second return value is non-nil only for unexpected I/O errors (e.g. bad
// secrets-file permissions, TOML parse failure). A missing key is not an
// error: it is expressed as (true, nil). Callers should surface a non-nil
// error as a diagnostic rather than treating it as "needs onboarding".
func NeedsOnboarding(cfg config.Config) (bool, error) {
	if cfg.API.BaseURL == "" {
		return true, nil
	}
	// Fast path: legacy API key field is already populated.
	if cfg.API.Key != "" {
		return false, nil
	}
	// Providers-map path (used when config was loaded via Load() with
	// applyLegacyAPICompat, or when explicit [providers] table is set).
	p, ok := cfg.Providers[cfg.Active.Provider]
	if !ok {
		return true, nil
	}
	_, err := config.ResolveSecret(p)
	if err != nil {
		if errors.Is(err, config.ErrNoAPIKey) {
			// Key is simply absent — that is the onboarding condition.
			return true, nil
		}
		// Real I/O error (bad permissions, TOML parse failure, etc.).
		// Do not silently report "needs onboarding"; propagate so the
		// caller can show a diagnostic.
		return false, err
	}
	// ResolveSecret always returns ErrNoAPIKey (wrapped) for a missing key,
	// so a non-error result here guarantees key != "". Returning false
	// documents that invariant; using key == "" would silently pass if the
	// invariant were ever violated.
	return false, nil
}

// OnboardingResult holds the values the user confirmed during init.
type OnboardingResult struct {
	APIKey  string
	BaseURL string
	Model   string
}

// PersistConfig writes:
//   - model + base_url to ~/.deepseek/config.toml  (user config layer)
//   - DEEPSEEK_API_KEY = apiKey to config.SecretsPath() at mode 0600
func PersistConfig(r OnboardingResult) error {
	// 1. User config dir
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	cfgDir := filepath.Join(home, ".deepseek")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	cfgPath := filepath.Join(cfgDir, "config.toml")

	// Read existing file so we don't clobber unrelated settings.
	existing := map[string]any{}
	if data, err := os.ReadFile(cfgPath); err == nil {
		if err := toml.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf("parse existing config: %w", err)
		}
	}

	// Overlay our values.
	api, _ := existing["api"].(map[string]any)
	if api == nil {
		api = map[string]any{}
	}
	api["base_url"] = r.BaseURL
	existing["api"] = api

	defaults, _ := existing["defaults"].(map[string]any)
	if defaults == nil {
		defaults = map[string]any{}
	}
	defaults["model"] = r.Model
	existing["defaults"] = defaults

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(existing); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.WriteFile(cfgPath, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	// 2. Secrets file
	secretsPath := config.SecretsPath()
	if err := os.MkdirAll(filepath.Dir(secretsPath), 0o700); err != nil {
		return fmt.Errorf("create secrets dir: %w", err)
	}
	secretsContent := fmt.Sprintf("DEEPSEEK_API_KEY = %q\n", r.APIKey)
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	f, err := os.OpenFile(secretsPath, flags, 0o600)
	if err != nil {
		return fmt.Errorf("open secrets file: %w", err)
	}
	defer f.Close()
	// Lock down permissions before writing any secret bytes, so that even
	// a non-default umask cannot leave group- or world-write bits set.
	if runtime.GOOS != "windows" {
		if err := f.Chmod(0o600); err != nil {
			return fmt.Errorf("chmod secrets: %w", err)
		}
	}
	if _, err := f.WriteString(secretsContent); err != nil {
		return fmt.Errorf("write secrets: %w", err)
	}
	return nil
}

// Run is the entry point for `dsc init`.
// In interactive mode it prompts on stdin/stdout; in --non-interactive it
// reads from env / existing config and errors loudly if anything is missing.
// hc is the HTTP client used for key validation (nil = http.DefaultClient).
func Run(ctx context.Context, cfg config.Config, args []string, in io.Reader, out io.Writer, hc *http.Client) error {
	nonInteractive := false
	for _, a := range args {
		if a == "--non-interactive" || a == "-n" {
			nonInteractive = true
		}
	}

	if nonInteractive {
		return runNonInteractive(ctx, cfg, out, hc)
	}
	return runInteractive(ctx, cfg, in, out, hc)
}

func runNonInteractive(ctx context.Context, cfg config.Config, out io.Writer, hc *http.Client) error {
	p, ok := cfg.Providers[cfg.Active.Provider]
	if !ok {
		p = config.ProviderConfigTOML{Type: "deepseek", EnvVar: "DEEPSEEK_API_KEY"}
	}
	key, err := config.ResolveSecret(p)
	if err != nil {
		if errors.Is(err, config.ErrNoAPIKey) {
			return fmt.Errorf("non-interactive init requires DEEPSEEK_API_KEY to be set (env var or secrets file)")
		}
		return fmt.Errorf("non-interactive init: resolving secret: %w", err)
	}
	if key == "" {
		return fmt.Errorf("non-interactive init requires DEEPSEEK_API_KEY to be set (env var or secrets file)")
	}
	baseURL := cfg.API.BaseURL
	if baseURL == "" {
		baseURL = "https://api.deepseek.com/v1"
	}
	model := cfg.Defaults.Model
	if model == "" {
		model = "deepseek-v4-flash"
	}
	if err := ValidateKey(ctx, baseURL, key, hc); err != nil {
		return fmt.Errorf("key validation failed: %w", err)
	}
	if err := PersistConfig(OnboardingResult{APIKey: key, BaseURL: baseURL, Model: model}); err != nil {
		return fmt.Errorf("saving configuration: %w", err)
	}
	fmt.Fprintln(out, "dsc init: OK — API key validated, configuration ready.")
	return nil
}

func runInteractive(ctx context.Context, cfg config.Config, in io.Reader, out io.Writer, hc *http.Client) error {
	scanner := bufio.NewScanner(in)

	fmt.Fprintln(out, "Welcome to deepseekcode. Let's get you set up.")
	fmt.Fprintln(out, "")

	// 1. API key
	fmt.Fprint(out, "Enter your DeepSeek API key (input hidden in real TTY): ")
	key := ""
	if scanner.Scan() {
		key = strings.TrimSpace(scanner.Text())
	}
	if key == "" {
		return fmt.Errorf("API key cannot be empty")
	}

	// 2. Base URL
	baseURL := cfg.API.BaseURL
	if baseURL == "" {
		baseURL = "https://api.deepseek.com/v1"
	}

	// 3. Model selection
	fmt.Fprintf(out, "Select model [deepseek-v4-flash / deepseek-v4-pro] (default: deepseek-v4-flash): ")
	model := "deepseek-v4-flash"
	if scanner.Scan() {
		if m := strings.TrimSpace(scanner.Text()); m != "" {
			switch m {
			case "deepseek-v4-flash", "deepseek-v4-pro":
				model = m
			default:
				fmt.Fprintf(out, "Unknown model %q, using default deepseek-v4-flash\n", m)
			}
		}
	}

	// 4. Validate key
	fmt.Fprint(out, "Validating API key... ")
	if err := ValidateKey(ctx, baseURL, key, hc); err != nil {
		fmt.Fprintln(out, "FAILED")
		return fmt.Errorf("key validation: %w", err)
	}
	fmt.Fprintln(out, "OK")

	// 5. Persist
	if err := PersistConfig(OnboardingResult{APIKey: key, BaseURL: baseURL, Model: model}); err != nil {
		return fmt.Errorf("saving configuration: %w", err)
	}

	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Setup complete. Run `dsc` to start a coding session.")
	return nil
}
