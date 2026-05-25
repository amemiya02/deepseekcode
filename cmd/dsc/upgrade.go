package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/version"
)

func runUpgrade(args []string) error {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	check := fs.Bool("check", false, "only check for updates, don't suggest commands")
	apply := fs.Bool("apply", false, "execute the upgrade command (default: print only)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15e9)
	defer cancel()

	tag, url, err := version.LatestRelease(ctx)
	if err != nil {
		fmt.Printf("unable to check for updates: %v\n", err)
		return nil
	}

	cmp := version.CompareVersions(version.Version, tag)
	if cmp >= 0 {
		fmt.Printf("already up to date (current=%s, latest=%s)\n", version.Version, tag)
		return nil
	}

	exe, _ := os.Executable()
	exe, _ = filepath.EvalSymlinks(exe)
	m := version.DetectInstallMethod(exe)
	fmt.Printf("current=%s  latest=%s  method=%s\n", version.Version, tag, m)
	fmt.Printf("update available: %s\n", url)

	if *check {
		return nil
	}

	cmd, human := upgradeCommand(m, tag)
	if *apply && cmd != nil {
		fmt.Printf("running: %s\n", human)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("upgrade command failed: %v\n", err)
		}
		return nil
	}

	fmt.Printf("run: %s    (or: dsc upgrade --apply)\n", human)
	return nil
}

// upgradeCommand returns the exec.Command and a human-readable string
// for the upgrade method. Returns (nil, text) for manual installs.
func upgradeCommand(m version.Method, tag string) (*exec.Cmd, string) {
	switch m {
	case version.MethodBrew:
		c := exec.Command("brew", "upgrade", "deepseekcode")
		return c, "brew upgrade deepseekcode"
	case version.MethodCurl:
		c := exec.Command("sh", "-c", "curl -fsSL https://deepseekcode.dev/install.sh | sh")
		return c, "curl -fsSL https://deepseekcode.dev/install.sh | sh"
	case version.MethodGoInstall:
		mod := "github.com/amemiya02/deepseekcode/cmd/dsc@latest"
		c := exec.Command("go", "install", mod)
		return c, "go install " + mod
	default:
		rel := "https://github.com/amemiya02/deepseekcode/releases/tag/" + tag
		return nil, fmt.Sprintf("download from %s and place on $PATH", rel)
	}
}

// decideUpgrade is the pure-function core used by both runUpgrade and
// doctor. Returns a summary message and the recommended command string.
func decideUpgrade(current, latest string, m version.Method) (msg, command string) {
	cmp := version.CompareVersions(current, latest)
	if cmp >= 0 {
		return fmt.Sprintf("up to date (%s)", current), ""
	}
	msg = fmt.Sprintf("update available %s → %s", current, latest)
	switch m {
	case version.MethodBrew:
		command = "brew upgrade deepseekcode"
	case version.MethodCurl:
		command = "curl -fsSL https://deepseekcode.dev/install.sh | sh"
	case version.MethodGoInstall:
		command = "go install github.com/amemiya02/deepseekcode/cmd/dsc@latest"
	default:
		command = fmt.Sprintf("download from https://github.com/amemiya02/deepseekcode/releases/tag/%s", latest)
	}
	return msg, command
}

// runUpgradeWithDecider exists for testability: tests inject a custom
// LatestRelease function via the real one (httptest override of githubAPIBase).
var stringsJoin = strings.Join
