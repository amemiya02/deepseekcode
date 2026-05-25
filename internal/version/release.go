package version

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Method string

const (
	MethodBrew      Method = "brew"
	MethodCurl      Method = "curl"
	MethodGoInstall Method = "go-install"
	MethodManual    Method = "manual"
)

// DetectInstallMethod infers the install method from the resolved
// executable path. Callers must filepath.EvalSymlinks before passing.
func DetectInstallMethod(execPath string) Method {
	if execPath == "" {
		return MethodManual
	}
	lp := strings.ToLower(execPath)
	if strings.Contains(lp, "/cellar/") || strings.Contains(lp, "homebrew") {
		return MethodBrew
	}
	if strings.Contains(execPath, "/go/bin/") {
		return MethodGoInstall
	}
	home, herr := os.UserHomeDir()
	if herr == nil {
		gp := os.Getenv("GOPATH")
		if gp == "" {
			gp = filepath.Join(home, "go")
		}
		if strings.HasPrefix(execPath, filepath.Join(gp, "bin")+string(filepath.Separator)) {
			return MethodGoInstall
		}
		curlBin := filepath.Join(home, ".local", "bin") + string(filepath.Separator)
		if strings.HasPrefix(execPath, curlBin) {
			return MethodCurl
		}
	}
	return MethodManual
}

var githubAPIBase = "https://api.github.com"

// LatestRelease queries GitHub for the newest release of amemiya02/deepseekcode.
func LatestRelease(ctx context.Context) (tag, htmlURL string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		githubAPIBase+"/repos/amemiya02/deepseekcode/releases/latest", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "dsc/"+Version)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 403 {
		return "", "", fmt.Errorf("GitHub API rate-limited (HTTP 403)")
	}
	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}
	var b struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&b); err != nil {
		return "", "", err
	}
	return b.TagName, b.HTMLURL, nil
}

// CompareVersions compares two "vX.Y.Z" semver strings.
// Returns -1 (current<latest), 0 (equal), 1 (current>latest).
// "dev"/"none"/empty current → -1.
func CompareVersions(current, latest string) int {
	if current == "" || current == "dev" || current == "none" {
		return -1
	}
	c, l := parseVer(current), parseVer(latest)
	for i := 0; i < 3; i++ {
		if c[i] < l[i] {
			return -1
		}
		if c[i] > l[i] {
			return 1
		}
	}
	return 0
}

func parseVer(v string) [3]int {
	var out [3]int
	for i, p := range strings.SplitN(strings.TrimPrefix(v, "v"), ".", 3) {
		if i >= 3 {
			break
		}
		out[i], _ = strconv.Atoi(p)
	}
	return out
}
