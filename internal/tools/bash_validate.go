package tools

import "strings"

// BashIntent classifies a bash command's risk level.
type BashIntent int

const (
	BashRead        BashIntent = iota // read-only: ls, cat, grep, git status, etc.
	BashSafe                          // safe-mutating: mkdir, touch, git add/commit, etc.
	BashDestructive                   // destructive: rm, git push --force, sed -i, etc.
	BashUnknown                       // unclear or complex: conservatively treated as ask
)

func (i BashIntent) String() string {
	switch i {
	case BashRead:
		return "read"
	case BashSafe:
		return "safe"
	case BashDestructive:
		return "destructive"
	default:
		return "unknown"
	}
}

// ClassifyBash categorizes a bash command by intent. It uses a simple
// first-token heuristic with special handling for subcommands and pipes.
// For pipe/&&/|| chains, the strictest intent wins.
func ClassifyBash(command string) BashIntent {
	command = strings.TrimSpace(command)
	if command == "" {
		return BashUnknown
	}

	segments := splitChain(command)
	worst := BashRead
	for _, seg := range segments {
		intent := classifySingle(seg)
		if intent > worst {
			worst = intent
		}
	}
	return worst
}

// splitChain splits on ;, |, &&, || respecting quotes and paren depth.
func splitChain(s string) []string {
	var segs []string
	var cur strings.Builder
	var quote byte
	depth := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			cur.WriteByte(c)
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			cur.WriteByte(c)
			continue
		}
		if c == '(' {
			depth++
			cur.WriteByte(c)
			continue
		}
		if c == ')' {
			depth--
			cur.WriteByte(c)
			continue
		}
		if depth > 0 {
			cur.WriteByte(c)
			continue
		}
		// ; splits sequential commands.
		if c == ';' {
			segs = append(segs, strings.TrimSpace(cur.String()))
			cur.Reset()
			continue
		}
		// Check for |, &&, ||
		if c == '|' && i+1 < len(s) && s[i+1] == '|' {
			// ||
			segs = append(segs, strings.TrimSpace(cur.String()))
			cur.Reset()
			i++ // skip second |
			continue
		}
		if c == '&' && i+1 < len(s) && s[i+1] == '&' {
			// &&
			segs = append(segs, strings.TrimSpace(cur.String()))
			cur.Reset()
			i++ // skip second &
			continue
		}
		if c == '|' {
			segs = append(segs, strings.TrimSpace(cur.String()))
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	if cur.Len() > 0 {
		segs = append(segs, strings.TrimSpace(cur.String()))
	}
	return segs
}

func classifySingle(cmd string) BashIntent {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return BashUnknown
	}

	// Redirect check: > or >> outside quotes makes it at least safe-mutating.
	if hasRedirect(cmd) {
		return BashSafe
	}

	tokens := tokenize(cmd)
	if len(tokens) == 0 {
		return BashUnknown
	}

	verb := tokens[0]

	// Subshell or eval: conservative.
	if verb == "eval" || verb == "bash" || verb == "sh" || verb == "zsh" || verb == "source" || verb == "." {
		return BashUnknown
	}

	// Git: unified handler that checks read-only first.
	if verb == "git" && len(tokens) > 1 {
		return classifyGit(tokens)
	}

	// Destructive verbs.
	destructive := map[string]bool{
		"rm": true, "rmdir": true, "mv": true,
	}
	if destructive[verb] {
		return BashDestructive
	}
	// cp with -f flag.
	if verb == "cp" && hasFlag(tokens, "-f", "--force") {
		return BashDestructive
	}
	// sed -i.
	if verb == "sed" && hasFlag(tokens, "-i", "--in-place") {
		return BashDestructive
	}
	// chmod/chown on system paths.
	if verb == "chmod" || verb == "chown" || verb == "chgrp" {
		return BashDestructive
	}
	// kill, pkill.
	if verb == "kill" || verb == "pkill" || verb == "killall" {
		return BashDestructive
	}
	// dd.
	if verb == "dd" {
		return BashDestructive
	}
	// mkfs, mount, umount.
	if verb == "mkfs" || verb == "mount" || verb == "umount" {
		return BashDestructive
	}
	// Shutdown/reboot.
	if verb == "shutdown" || verb == "reboot" || verb == "halt" || verb == "poweroff" {
		return BashDestructive
	}

	// Docker destructive.
	if verb == "docker" && len(tokens) > 1 {
		switch tokens[1] {
		case "rm", "rmi":
			return BashDestructive
		case "push":
			return BashDestructive
		}
	}

	// kubectl destructive.
	if verb == "kubectl" && len(tokens) > 1 {
		switch tokens[1] {
		case "delete":
			return BashDestructive
		case "apply":
			return BashSafe
		}
	}

	// npm/pnpm/yarn publish.
	if (verb == "npm" || verb == "pnpm" || verb == "yarn") && len(tokens) > 1 && tokens[1] == "publish" {
		return BashDestructive
	}

	// terraform apply/destroy.
	if verb == "terraform" && len(tokens) > 1 {
		switch tokens[1] {
		case "apply", "destroy":
			return BashDestructive
		case "plan", "show", "validate":
			return BashRead
		}
	}

	// curl/wget with write flags.
	if (verb == "curl" || verb == "wget") && hasFlag(tokens, "-X", "--request") {
		return BashUnknown
	}
	if verb == "curl" && hasFlag(tokens, "--upload-file", "-T") {
		return BashDestructive
	}

	// Read-only verbs.
	readOnly := map[string]bool{
		"ls": true, "cat": true, "head": true, "tail": true,
		"grep": true, "egrep": true, "fgrep": true, "rg": true,
		"find": true, "which": true, "where": true, "whereis": true,
		"wc": true, "sort": true, "uniq": true, "diff": true,
		"echo": true, "printf": true, "true": true, "false": true,
		"pwd": true, "whoami": true, "hostname": true, "uname": true,
		"date": true, "env": true, "printenv": true, "type": true,
		"file": true, "stat": true, "du": true, "df": true,
		"ps": true, "top": true, "htop": true,
		"curl": true, "wget": true,
		"tree": true, "less": true, "more": true,
		"jq": true, "yq": true,
		"xargs": true,
	}
	if readOnly[verb] {
		return BashRead
	}

	// Go read-only subcommands.
	if verb == "go" && len(tokens) > 1 {
		goRead := map[string]bool{
			"list": true, "vet": true, "doc": true, "version": true,
			"env": true, "test": true, "build": true,
		}
		if goRead[tokens[1]] {
			return BashRead
		}
		if tokens[1] == "mod" && len(tokens) > 2 && goRead["mod "+tokens[2]] {
			return BashRead
		}
	}

	// Safe-mutating verbs.
	safeMutating := map[string]bool{
		"mkdir": true, "touch": true, "cp": true, "ln": true,
		"tar": true, "zip": true, "unzip": true, "gzip": true,
		"pip": true, "pip3": true, "uv": true,
		"cargo": true, "npm": true, "pnpm": true, "yarn": true,
		"make": true, "cmake": true,
		"brew": true,
	}
	if safeMutating[verb] {
		return BashSafe
	}

	// Go safe subcommands.
	if verb == "go" && len(tokens) > 1 {
		goSafe := map[string]bool{
			"install": true, "get": true, "mod download": true,
			"mod tidy": true, "mod verify": true,
			"generate": true, "run": true,
		}
		if goSafe[tokens[1]] {
			return BashSafe
		}
		if tokens[1] == "mod" && len(tokens) > 2 && goSafe["mod "+tokens[2]] {
			return BashSafe
		}
	}

	return BashUnknown
}

// classifyGit classifies git subcommands: read-only first, then destructive, then safe.
func classifyGit(tokens []string) BashIntent {
	sub := tokens[1]

	// Read-only subcommands.
	gitRead := map[string]bool{
		"status": true, "log": true, "diff": true, "show": true,
		"blame": true, "describe": true, "reflog": true, "shortlog": true,
	}
	if gitRead[sub] {
		return BashRead
	}
	// branch/remote/tag are read-only only when listing (no extra args beyond flags).
	if sub == "branch" || sub == "remote" || sub == "tag" {
		// If there's a non-flag argument, it's a mutating subcommand.
		for _, t := range tokens[2:] {
			if !strings.HasPrefix(t, "-") {
				return BashSafe
			}
		}
		return BashRead
	}
	if sub == "stash" && len(tokens) > 2 && tokens[2] == "list" {
		return BashRead
	}

	// Destructive subcommands.
	switch sub {
	case "push":
		if hasFlag(tokens, "--force", "-f", "--force-with-lease") {
			return BashDestructive
		}
		return BashSafe
	case "reset":
		if hasFlag(tokens, "--hard") {
			return BashDestructive
		}
		return BashSafe
	case "checkout":
		if hasFlag(tokens, "--") && len(tokens) > 2 {
			if tokens[len(tokens)-1] == "." {
				return BashDestructive
			}
		}
		return BashSafe
	case "clean":
		if hasFlag(tokens, "-f", "--force", "-fd", "-df") {
			return BashDestructive
		}
		return BashSafe
	}

	// Safe subcommands.
	gitSafe := map[string]bool{
		"add": true, "commit": true, "mv": true,
		"rebase": true, "cherry-pick": true, "merge": true,
		"stash": true, "branch": true, "remote": true, "tag": true,
	}
	if gitSafe[sub] {
		return BashSafe
	}

	return BashUnknown
}

// hasFlag checks if any of the given flags appear in the token list.
// Also matches combined short flags: hasFlag(tokens, "-f") matches "-fd" or "-f".
func hasFlag(tokens []string, flags ...string) bool {
	flagSet := make(map[string]bool, len(flags))
	shortFlags := make(map[rune]bool)
	for _, f := range flags {
		flagSet[f] = true
		// Track short flags (single-char like -f) for combined matching.
		if len(f) == 2 && f[0] == '-' {
			shortFlags[rune(f[1])] = true
		}
	}
	for _, t := range tokens {
		if flagSet[t] {
			return true
		}
		// Combined short flags: -fd contains -f if we're looking for -f.
		if len(t) > 2 && t[0] == '-' && t[1] != '-' {
			for _, c := range t[1:] {
				if shortFlags[c] {
					return true
				}
			}
		}
	}
	return false
}

// hasRedirect returns true if the command contains > or >> outside of
// quotes. This avoids misclassifying e.g. echo "a > b" as a redirect.
func hasRedirect(s string) bool {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if c == '>' {
			return true
		}
	}
	return false
}

// tokenize is a simple shell-like splitter.
func tokenize(s string) []string {
	var out []string
	var cur strings.Builder
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
				continue
			}
			cur.WriteByte(c)
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if c == ' ' || c == '\t' {
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
			continue
		}
		cur.WriteByte(c)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
