package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeReq(t *testing.T, cwd, body string) {
	t.Helper()
	dir := filepath.Join(cwd, ".deepseek")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "requirements.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRequirementsAbsentIsNil(t *testing.T) {
	req, err := LoadRequirements(t.TempDir())
	if err != nil {
		t.Fatalf("absent requirements should not error: %v", err)
	}
	if req != nil {
		t.Errorf("absent requirements should be nil, got %+v", req)
	}
}

func TestLoadRequirementsParsed(t *testing.T) {
	cwd := t.TempDir()
	writeReq(t, cwd, "max_mode = \"default\"\nrequire_sandbox = true\n")
	req, err := LoadRequirements(cwd)
	if err != nil {
		t.Fatalf("LoadRequirements: %v", err)
	}
	if req == nil {
		t.Fatal("requirements should be non-nil")
	}
	if req.MaxMode != "default" || !req.RequireSandbox {
		t.Errorf("parsed = %+v, want max_mode=default require_sandbox=true", req)
	}
	if req.IsZero() {
		t.Error("a populated floor must not report IsZero")
	}
}

func TestLoadRequirementsMalformedErrors(t *testing.T) {
	cwd := t.TempDir()
	writeReq(t, cwd, "max_mode = = not valid [[[")
	if _, err := LoadRequirements(cwd); err == nil {
		t.Error("malformed requirements.toml must error, not silently fail open")
	}
}

func TestRequirementsIsZero(t *testing.T) {
	if !(Requirements{}).IsZero() {
		t.Error("empty Requirements must be zero")
	}
	if (Requirements{MaxMode: "yolo"}).IsZero() {
		t.Error("a max_mode floor is not zero")
	}
}
