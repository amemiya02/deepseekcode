package tools

import "testing"

func TestClassifyBash(t *testing.T) {
	tests := []struct {
		cmd  string
		want BashIntent
	}{
		// Read-only commands
		{"ls", BashRead},
		{"ls -la /tmp", BashRead},
		{"cat file.txt", BashRead},
		{"head -5 file.go", BashRead},
		{"tail -f log.txt", BashRead},
		{"grep pattern *.go", BashRead},
		{"find . -name '*.go'", BashRead},
		{"wc -l file.go", BashRead},
		{"pwd", BashRead},
		{"echo hello", BashRead},
		{"git status", BashRead},
		{"git log --oneline -5", BashRead},
		{"git diff", BashRead},
		{"git show HEAD", BashRead},
		{"git blame main.go", BashRead},
		{"env", BashRead},
		{"which go", BashRead},
		{"file binary", BashRead},
		{"du -sh .", BashRead},
		{"ps aux", BashRead},
		{"curl https://example.com", BashRead},
		{"jq '.name' file.json", BashRead},
		{"go test ./...", BashRead},
		{"go build ./...", BashRead},
		{"go vet ./...", BashRead},
		{"terraform plan", BashRead},

		// Safe-mutating commands
		{"mkdir newdir", BashSafe},
		{"touch file.txt", BashSafe},
		{"cp a.txt b.txt", BashSafe},
		{"tar czf out.tgz dir/", BashSafe},
		{"npm install", BashSafe},
		{"cargo build", BashSafe},
		{"make all", BashSafe},
		{"brew install gopls", BashSafe},
		{"git add .", BashSafe},
		{"git commit -m 'msg'", BashSafe},
		{"git push", BashSafe},
		{"git push origin main", BashSafe},
		{"git stash", BashSafe},
		{"git branch feature", BashSafe},
		{"git rebase main", BashSafe},
		{"git cherry-pick abc123", BashSafe},
		{"go install ./...", BashSafe},
		{"go mod tidy", BashRead},
		{"echo hello > file.txt", BashSafe},
		{"kubectl apply -f deploy.yaml", BashSafe},

		// Destructive commands
		{"rm file.txt", BashDestructive},
		{"rm -rf node_modules", BashDestructive},
		{"rmdir emptydir", BashDestructive},
		{"mv old new", BashDestructive},
		{"cp -f src dst", BashDestructive},
		{"sed -i 's/old/new/g' file", BashDestructive},
		{"chmod 777 file", BashDestructive},
		{"kill -9 1234", BashDestructive},
		{"pkill -f process", BashDestructive},
		{"dd if=/dev/zero of=disk.img", BashDestructive},
		{"git push --force", BashDestructive},
		{"git push -f", BashDestructive},
		{"git push --force-with-lease", BashDestructive},
		{"git reset --hard HEAD~1", BashDestructive},
		{"git clean -fd", BashDestructive},
		{"git checkout -- .", BashDestructive},
		{"docker rm container", BashDestructive},
		{"docker push image", BashDestructive},
		{"kubectl delete pod/my-pod", BashDestructive},
		{"npm publish", BashDestructive},
		{"terraform apply", BashDestructive},
		{"terraform destroy", BashDestructive},
		{"shutdown -h now", BashDestructive},
		{"reboot", BashDestructive},
		{"curl --upload-file data https://server", BashDestructive},

		// Unknown / complex
		{"eval $(ssh-agent)", BashUnknown},
		{"bash script.sh", BashUnknown},
		{"source env.sh", BashUnknown},
		{". env.sh", BashUnknown},
		{"curl -X POST https://api.example.com", BashUnknown},
		{"my-custom-script --flag", BashUnknown},
		{"", BashUnknown},

		// Pipe chains: strictest wins
		{"cat file | grep pattern", BashRead},
		{"git status && git log", BashRead},
		{"ls || echo missing", BashRead},
		{"cat file | rm -rf dir", BashDestructive},
		{"echo hi && rm -rf /", BashDestructive},
		{"mkdir dir && git status", BashSafe},
		{"grep pattern file | wc -l", BashRead},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			got := ClassifyBash(tt.cmd)
			if got != tt.want {
				t.Errorf("ClassifyBash(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestBashIntentString(t *testing.T) {
	tests := []struct {
		intent BashIntent
		want   string
	}{
		{BashRead, "read"},
		{BashSafe, "safe"},
		{BashDestructive, "destructive"},
		{BashUnknown, "unknown"},
	}
	for _, tt := range tests {
		if got := tt.intent.String(); got != tt.want {
			t.Errorf("BashIntent(%d).String() = %q, want %q", tt.intent, got, tt.want)
		}
	}
}
