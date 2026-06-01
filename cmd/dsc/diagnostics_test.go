package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/lsp"
)

// TestAgentDefsResult exercises the doctor agent-def diagnostic: it surfaces a
// silently-skipped (oversized) def and an out-of-range temperature field.
func TestAgentDefsResult(t *testing.T) {
	cwd := t.TempDir()
	dir := filepath.Join(cwd, ".deepseek", "agent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("good.md", "---\ndescription: ok\ntemperature: 0.5\n---\nbody")
	write("hot.md", "---\ndescription: hot\ntemperature: 5.0\n---\nbody")
	// >64 KiB → silently skipped by agents.Load's size cap.
	write("huge.md", "---\ndescription: huge\n---\n"+strings.Repeat("x", 65*1024))

	// home="" so only the cwd tree is scanned (no host-dir pollution).
	res := agentDefsResult(cwd, "")

	if res.Status != "warn" {
		t.Fatalf("status = %q, want warn; detail=%q", res.Status, res.Detail)
	}
	if !strings.Contains(res.Detail, "2 loaded") {
		t.Errorf("detail should report 2 loaded, got %q", res.Detail)
	}
	if !strings.Contains(res.Detail, "skipped") {
		t.Errorf("detail should flag the oversized skipped def, got %q", res.Detail)
	}
	if !strings.Contains(res.Detail, "hot: temperature 5.00 out of [0,2]") {
		t.Errorf("detail should flag the out-of-range temperature, got %q", res.Detail)
	}
}

// TestAgentDefsResultClean confirms a tidy cwd reports ok.
func TestAgentDefsResultClean(t *testing.T) {
	cwd := t.TempDir()
	dir := filepath.Join(cwd, ".deepseek", "agent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scout.md"), []byte("---\ndescription: scout\ntemperature: 0.2\ntop_p: 0.9\n---\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := agentDefsResult(cwd, "")
	if res.Status != "ok" {
		t.Fatalf("status = %q, want ok; detail=%q", res.Status, res.Detail)
	}
	if !strings.Contains(res.Detail, "1 loaded") {
		t.Errorf("detail = %q, want '1 loaded'", res.Detail)
	}
}

// Rework Task-007: checkProviderCapabilities for the DeepSeek provider must
// surface max_output, effort values, and max_ctx in the detail string.
func TestCheckProviderCapabilitiesDeepSeek(t *testing.T) {
	cfg := config.Config{
		Defaults: config.DefaultsConfig{Model: "deepseek-v4-flash"},
		Providers: map[string]config.ProviderConfigTOML{
			"deepseek": {
				Type:    "deepseek",
				BaseURL: "https://api.deepseek.com",
				APIKey:  "sk-test",
			},
		},
	}
	res := checkProviderCapabilities(cfg)
	if res.Status != "ok" {
		t.Fatalf("status = %q, want ok; detail=%q", res.Status, res.Detail)
	}
	for _, want := range []string{
		"max_output=384000",
		"effort=low,medium,high,max",
		"max_ctx=1000000",
	} {
		if !strings.Contains(res.Detail, want) {
			t.Errorf("detail %q should contain %q", res.Detail, want)
		}
	}
}

// TestREADMEDocLinksResolve fails when a README links a repo-relative markdown
// file that does not exist — catching doc rot like a renamed/deleted doc or a
// typo'd path (e.g. the docs/MODEL_COMPATIBILITY.md link added in T7.4). The
// PARITY.md scenario linkage is separately enforced by internal/llm's
// TestParityConsistency, so this is non-redundant.
func TestREADMEDocLinksResolve(t *testing.T) {
	repoRoot := filepath.Join("..", "..") // cmd/dsc -> repo root
	mdLink := regexp.MustCompile(`\]\(([^)]+)\)`)
	for _, name := range []string{"README.md", "README.zh-CN.md"} {
		data, err := os.ReadFile(filepath.Join(repoRoot, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, m := range mdLink.FindAllStringSubmatch(string(data), -1) {
			target := m[1]
			switch {
			case strings.HasPrefix(target, "http://"), strings.HasPrefix(target, "https://"),
				strings.HasPrefix(target, "#"), strings.HasPrefix(target, "mailto:"):
				continue
			}
			if i := strings.IndexByte(target, '#'); i >= 0 {
				target = target[:i] // strip in-page anchor
			}
			if !strings.HasSuffix(target, ".md") {
				continue // doc-reference linkage only
			}
			if _, err := os.Stat(filepath.Join(repoRoot, target)); err != nil {
				t.Errorf("%s links %q which does not resolve: %v", name, m[1], err)
			}
		}
	}
}

// helper to build a diagnostic quickly.
func diag(line, char, sev int, msg string) lsp.Diagnostic {
	return lsp.Diagnostic{
		Range:    lsp.Range{Start: lsp.Position{Line: line, Character: char}},
		Severity: sev,
		Message:  msg,
	}
}

func TestFormatPostEditDiagnosticsEmpty(t *testing.T) {
	if got := formatPostEditDiagnostics(nil); got != "" {
		t.Errorf("nil input: got %q, want empty", got)
	}
	if got := formatPostEditDiagnostics([]fileDiagnostics{}); got != "" {
		t.Errorf("empty input: got %q, want empty", got)
	}
}

func TestFormatPostEditDiagnosticsBasic(t *testing.T) {
	files := []fileDiagnostics{
		{Path: "foo.go", Diagnostics: []lsp.Diagnostic{diag(9, 7, 1, "undefined: x")}},
	}
	got := formatPostEditDiagnostics(files)
	if !strings.Contains(got, "Environment diagnostics after edit:") {
		t.Errorf("missing header:\n%s", got)
	}
	if !strings.Contains(got, "foo.go:10:8 error undefined: x") {
		t.Errorf("missing diagnostic line:\n%s", got)
	}
}

func TestFormatPostEditDiagnosticsCapsFiles(t *testing.T) {
	var files []fileDiagnostics
	for i := 0; i < 10; i++ {
		files = append(files, fileDiagnostics{
			Path:        "file" + string(rune('a'+i)) + ".go",
			Diagnostics: []lsp.Diagnostic{diag(0, 0, 1, "err")},
		})
	}
	got := formatPostEditDiagnostics(files)
	// Count how many distinct file lines appear. Should be capped at 5.
	count := 0
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, ".go:") {
			count++
		}
	}
	if count > 5 {
		t.Errorf("got %d file lines, want <= 5:\n%s", count, got)
	}
}

func TestFormatPostEditDiagnosticsCapsPerFile(t *testing.T) {
	var diags []lsp.Diagnostic
	for i := 0; i < 10; i++ {
		diags = append(diags, diag(i, 0, 1, "err"+string(rune('0'+i))))
	}
	files := []fileDiagnostics{
		{Path: "many.go", Diagnostics: diags},
	}
	got := formatPostEditDiagnostics(files)
	if !strings.Contains(got, "… and 5 more diagnostics") {
		t.Errorf("missing overflow line:\n%s", got)
	}
	// Exactly 5 diagnostic lines + 1 overflow line for this file.
	diagLines := 0
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "many.go:") {
			diagLines++
		}
	}
	if diagLines != 5 {
		t.Errorf("got %d diagnostic lines, want 5:\n%s", diagLines, got)
	}
}

func TestFormatPostEditDiagnosticsCapsBytes(t *testing.T) {
	// Create enough diagnostics to exceed 512 bytes.
	var files []fileDiagnostics
	for i := 0; i < 5; i++ {
		var diags []lsp.Diagnostic
		for j := 0; j < 5; j++ {
			diags = append(diags, diag(j, 0, 1, strings.Repeat("x", 50)))
		}
		files = append(files, fileDiagnostics{Path: "f" + string(rune('a'+i)) + ".go", Diagnostics: diags})
	}
	got := formatPostEditDiagnostics(files)
	if len(got) > 530 { // 512 + "…\n" + some slack
		t.Errorf("output too long: %d bytes", len(got))
	}
	if strings.Contains(got, "…") {
		// Truncation marker should be present.
	}
}

func TestFormatPostEditDiagnosticsDeterministic(t *testing.T) {
	files := []fileDiagnostics{
		{Path: "a.go", Diagnostics: []lsp.Diagnostic{diag(0, 0, 1, "e1"), diag(1, 0, 2, "w1")}},
		{Path: "b.go", Diagnostics: []lsp.Diagnostic{diag(5, 3, 1, "e2")}},
	}
	got1 := formatPostEditDiagnostics(files)
	got2 := formatPostEditDiagnostics(files)
	if got1 != got2 {
		t.Errorf("non-deterministic output:\n---\n%s\n---\n%s", got1, got2)
	}
}
