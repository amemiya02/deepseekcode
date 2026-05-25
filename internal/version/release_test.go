package version

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectInstallMethod(t *testing.T) {
	home, _ := os.UserHomeDir()
	tests := []struct {
		path string
		want Method
	}{
		{"/opt/homebrew/Cellar/deepseekcode/0.1/bin/dsc", MethodBrew},
		{"/home/linuxbrew/.linuxbrew/Cellar/dsc/0.1/bin/dsc", MethodBrew},
		{"/usr/local/Homebrew/bin/dsc", MethodBrew},
		{"/Users/x/go/bin/dsc", MethodGoInstall},
		{"/home/user/go/bin/dsc", MethodGoInstall},
		{filepath.Join(home, ".local", "bin", "dsc"), MethodCurl},
		{"/usr/local/bin/dsc", MethodManual},
		{"/tmp/dsc", MethodManual},
		{"", MethodManual},
	}
	for _, tt := range tests {
		got := DetectInstallMethod(tt.path)
		if got != tt.want {
			t.Errorf("DetectInstallMethod(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		current, latest string
		want            int
	}{
		{"v0.1.0", "v0.2.0", -1},
		{"v0.2.0", "v0.1.0", 1},
		{"v0.1.0", "v0.1.0", 0},
		{"dev", "v0.1.0", -1},
		{"", "v0.1.0", -1},
		{"none", "v0.1.0", -1},
		{"v0.1", "v0.1.0", 0},
		{"v0.1.0", "v0.1", 0},
		{"v1.0.0", "v0.9.9", 1},
	}
	for _, tt := range tests {
		got := CompareVersions(tt.current, tt.latest)
		if got != tt.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.current, tt.latest, got, tt.want)
		}
	}
}

func TestLatestRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/amemiya02/deepseekcode/releases/latest" {
			http.Error(w, "not found", 404)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{
			"tag_name": "v0.2.0",
			"html_url": "https://github.com/amemiya02/deepseekcode/releases/tag/v0.2.0",
		})
	}))
	defer srv.Close()

	orig := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = orig }()

	tag, url, err := LatestRelease(context.Background())
	if err != nil {
		t.Fatalf("LatestRelease() error: %v", err)
	}
	if tag != "v0.2.0" {
		t.Errorf("tag = %q, want %q", tag, "v0.2.0")
	}
	if url != "https://github.com/amemiya02/deepseekcode/releases/tag/v0.2.0" {
		t.Errorf("url = %q, want %q", url, "https://github.com/amemiya02/deepseekcode/releases/tag/v0.2.0")
	}
}

func TestLatestReleaseHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", 403)
	}))
	defer srv.Close()

	orig := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = orig }()

	_, _, err := LatestRelease(context.Background())
	if err == nil {
		t.Fatal("expected error for 403, got nil")
	}
}
