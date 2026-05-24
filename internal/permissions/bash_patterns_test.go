package permissions

import "testing"

func TestBashPattern(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"git status -sb", "git status *"},
		{"git status", "git status *"},
		{"npm install foo bar", "npm install *"},
		{"rm -rf node_modules", "rm -rf *"},
		{"ls", "ls"},
		{"./build.sh --verbose", "./build.sh *"},
		{"git push origin main", "git push *"},
		{"cargo build --release", "cargo build *"},
		{"kubectl delete pod foo", "kubectl delete *"},
		{"terraform apply -auto-approve", "terraform apply *"},
	}
	for _, c := range cases {
		got := bashPattern(c.in)
		if got != c.want {
			t.Errorf("bashPattern(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestIsDestructiveBash(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"git status", false},
		{"git log --oneline", false},
		{"ls -la", false},
		{"rm foo.txt", true},
		{"rm -rf node_modules", true},
		{"git push", false},                // safe (not --force)
		{"git push origin main", false},    // safe
		{"git push --force", true},         // destructive
		{"git reset --hard HEAD~3", true},
		{"curl -X POST https://example.com", false}, // Unknown, not destructive
		{"curl https://example.com", false},
		{"kubectl delete pod foo", true},
		{"kubectl apply -f deploy.yaml", false}, // safe
		{"terraform apply", true},
		{"terraform plan", false},
		{"npm publish", true},
		{"docker push myimage", true},
		{"docker pull myimage", false},
	}
	for _, c := range cases {
		got := IsDestructiveBash(c.in, nil)
		if got != c.want {
			t.Errorf("IsDestructiveBash(%q) = %v; want %v", c.in, got, c.want)
		}
	}
}

func TestIsDestructiveBashExtra(t *testing.T) {
	extra := []string{`^\s*git\s+push(\s|$)`}
	// git push without --force: ClassifyBash says safe, but extra regex adds it back.
	if !IsDestructiveBash("git push origin main", extra) {
		t.Error("extra patterns should override ClassifyBash")
	}
	// git push --force: ClassifyBash already says destructive.
	if !IsDestructiveBash("git push --force", extra) {
		t.Error("force push should be destructive")
	}
	// Normal read command should not match extra pattern.
	if IsDestructiveBash("git status", extra) {
		t.Error("git status should not be destructive even with extra patterns")
	}
}
