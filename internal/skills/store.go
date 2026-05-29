// Package skills implements a cache-stable skill metadata index.
// Skill bodies are loaded lazily and excluded from the prefix-stable
// index text to avoid invalidating the prompt cache when skill
// content changes.
package skills

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Skill is the metadata for a single skill.
type Skill struct {
	Name         string
	Description  string
	Scope        string
	RunAs        string
	AllowedTools []string
	Path         string

	// BodyHash is the SHA-256 of the skill body (everything after the
	// frontmatter). It is the source of VersionHash: a body-only edit
	// changes BodyHash even when the description is untouched, so the
	// stable skill directory in the prefix records a new version_hash
	// and the epoch sees a pending change. Computed at Load time.
	BodyHash string
}

// shortVersionHash is the body-derived version_hash placed in the stable
// skill directory. Truncated so the directory stays compact (the prefix
// is cache-stable and every byte counts) while still flipping on any body
// edit.
func (sk Skill) shortVersionHash() string {
	if len(sk.BodyHash) >= 12 {
		return sk.BodyHash[:12]
	}
	return sk.BodyHash
}

// shortDescription returns a single-line, length-capped description for
// the stable directory. The full description never enters the prefix.
func (sk Skill) shortDescription() string {
	d := sk.Description
	if i := strings.IndexByte(d, '\n'); i >= 0 {
		d = d[:i]
	}
	d = strings.TrimSpace(d)
	if r := []rune(d); len(r) > 80 {
		d = string(r[:80])
	}
	return d
}

// runMode returns the skill's run mode, defaulting to "direct".
func (sk Skill) runMode() string {
	if sk.RunAs == "" {
		return "direct"
	}
	return sk.RunAs
}

// Store holds the indexed skill metadata. Construct with Load.
type Store struct {
	skills []Skill
	byName map[string]int // index into skills for O(1) body lookup
}

// Load discovers SKILL.md files one directory below each root,
// parses their frontmatter, and returns a deterministic index.
// Missing roots are silently ignored.
func Load(roots []string) (*Store, error) {
	var all []Skill
	seen := make(map[string]bool)

	for _, root := range roots {
		skills, err := discoverSkills(root)
		if err != nil {
			continue // ignore missing roots
		}
		for _, s := range skills {
			if s.Description == "" {
				continue // skip skills with empty description
			}
			if seen[s.Name] {
				continue // duplicate: keep first root's entry
			}
			seen[s.Name] = true
			all = append(all, s)
		}
	}

	// Sort by name, then scope, for deterministic output.
	sort.Slice(all, func(i, j int) bool {
		if all[i].Name != all[j].Name {
			return all[i].Name < all[j].Name
		}
		return all[i].Scope < all[j].Scope
	})

	byName := make(map[string]int, len(all))
	for i, s := range all {
		byName[s.Name] = i
	}

	return &Store{skills: all, byName: byName}, nil
}

// scanDirs are the cross-tool skill directories scanned relative to each
// root, in priority order. Mirrors the set the prompt builder used before
// skill discovery was unified into this package, so model-visible skills do
// not shrink when the dual loader is removed.
var scanDirs = []string{
	".deepseek/skills",
	"skills",
	".opencode/skills",
	".claude/skills",
	".agents/skills",
}

const maxScanDepth = 8

// LoadScan is the single canonical skill loader. It walks the cross-tool
// skill directories under cwd (then home, if different) recursively,
// stopping descent at the first SKILL.md in any subtree, and returns a
// deterministic Store. The same Store feeds the prompt's stable skill
// directory (PromptIndex), the epoch hash (VersionHash), and skill_read
// (Body) — so the model-visible capability list and the cache-epoch hash
// are computed from one source and can never diverge.
//
// Unlike Load, LoadScan keeps skills whose description is empty: the prompt
// previously listed name-only skills, and dropping them here would silently
// shrink the model-visible directory.
//
// Session-start only (by design): callers load the Store once at session start
// and reuse it for the whole session. The Store is NOT re-read from disk per
// turn — editing a SKILL.md mid-session does not change the prefix the model is
// receiving, because the cache-epoch prefix is deliberately frozen for cache
// stability (the same way the system prompt and tool set are). A mid-session
// edit takes effect on the next session; VersionHash()/Diff() exist to compare
// two stores across that boundary, not to hot-reload within one. Reloading
// every turn would re-detect the same drift against the frozen baseline on
// every step, so it is intentionally not done. The one sanctioned mid-session
// reload is the explicit /reload-skills command (Agent.ReloadSkills): it
// re-scans via this loader, swaps the store in place (ReplaceFrom), and mints a
// new prefix epoch so the single resulting cache miss is deliberate, not silent
// per-turn drift.
func LoadScan(cwd, home string) (*Store, error) {
	if cwd == "" {
		return nil, fmt.Errorf("LoadScan: empty cwd")
	}
	roots := []string{cwd}
	if home != "" && home != cwd {
		roots = append(roots, home)
	}

	seen := make(map[string]bool)
	var all []Skill
	for _, root := range roots {
		for _, rel := range scanDirs {
			walkScan(filepath.Join(root, rel), 0, seen, &all)
		}
	}

	sort.Slice(all, func(i, j int) bool {
		if all[i].Name != all[j].Name {
			return all[i].Name < all[j].Name
		}
		return all[i].Scope < all[j].Scope
	})
	byName := make(map[string]int, len(all))
	for i, s := range all {
		byName[s.Name] = i
	}
	return &Store{skills: all, byName: byName}, nil
}

// walkScan implements the "stop at SKILL.md, dedupe by name, skip hidden,
// bounded depth" discovery contract shared with the old prompt loader.
func walkScan(dir string, depth int, seen map[string]bool, out *[]Skill) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // missing or unreadable → skip
	}
	for _, e := range entries {
		if e.Name() == "SKILL.md" && !e.IsDir() {
			sk, err := parseSkillFile(filepath.Join(dir, "SKILL.md"), filepath.Base(dir))
			if err != nil {
				return // malformed skill dir — don't drill further
			}
			sk.Scope = filepath.Base(dir)
			if !seen[sk.Name] {
				seen[sk.Name] = true
				*out = append(*out, sk)
			}
			return // SKILL.md found → stop descending this subtree
		}
	}
	if depth >= maxScanDepth {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name()[0] == '.' {
			continue // skip files and hidden dirs at every level
		}
		walkScan(filepath.Join(dir, e.Name()), depth+1, seen, out)
	}
}

// PromptIndex returns the canonical stable skill directory rendered into the
// model-visible static prefix. It is exactly IndexText(): the same bytes that
// feed VersionHash(), so a body edit moves the prefix the model sees and the
// epoch hash in lock-step.
func (s *Store) PromptIndex() string { return s.IndexText() }

// List returns all indexed skills.
func (s *Store) List() []Skill {
	return s.skills
}

// Names returns the indexed skill names, sorted. Used by the skill_read
// tool to report available skills on a miss.
func (s *Store) Names() []string {
	out := make([]string, 0, len(s.skills))
	for _, sk := range s.skills {
		out = append(out, sk.Name)
	}
	sort.Strings(out)
	return out
}

// IndexText returns a deterministic, prefix-stable text representation
// of the skill index: one line per skill in the canonical stable-skill-
// directory format
//
//	name | short_description | run_mode | version_hash | allowed_tools
//
// where version_hash is derived from the skill body (see Skill.BodyHash).
// Full skill bodies and local absolute paths are NOT included, so the
// directory stays both compact and machine-independent. A body-only edit
// flips version_hash and therefore the whole IndexText, which is what
// drives a pending epoch change.
func (s *Store) IndexText() string {
	var b strings.Builder
	for _, sk := range s.skills {
		fmt.Fprintf(&b, "%s | %s | %s | %s | %s\n",
			sk.Name, sk.shortDescription(), sk.runMode(),
			sk.shortVersionHash(), strings.Join(sk.AllowedTools, ","))
	}
	return b.String()
}

// Body reads the skill body lazily by name. Returns ("", false) if
// the skill is not found.
func (s *Store) Body(name string) (string, bool) {
	idx, ok := s.byName[name]
	if !ok {
		return "", false
	}
	sk := s.skills[idx]
	body, err := readSkillBody(sk.Path)
	if err != nil {
		return "", false
	}
	return body, true
}

// discoverSkills finds SKILL.md files one directory below root.
func discoverSkills(root string) ([]Skill, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var skills []Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillPath := filepath.Join(root, entry.Name(), "SKILL.md")
		if _, err := os.Stat(skillPath); os.IsNotExist(err) {
			continue
		}
		sk, err := parseSkillFile(skillPath, entry.Name())
		if err != nil {
			continue // skip malformed files
		}
		sk.Scope = entry.Name()
		skills = append(skills, sk)
	}
	return skills, nil
}

var headingRe = regexp.MustCompile(`(?m)^#\s+(.+)$`)

// parseSkillFile parses a SKILL.md file's frontmatter and body. It reads the
// file once, extracts metadata, and computes the body hash so a body-only edit
// is detectable without keeping the body in memory.
//
// Two layouts are accepted, mirroring the prompt loader this package replaced
// so the model-visible directory never silently shrinks:
//
//   - With "---" frontmatter: name/description/run_mode/allowed-tools are read
//     from the fences (quoted values are unquoted; a colon in a value is kept).
//     The body is everything after the closing fence.
//   - Without frontmatter: the first "# Heading" becomes the name (falling back
//     to the directory name when there is no heading). The whole file is the
//     body. Such a heading-only SKILL.md is NOT rejected — it must keep showing
//     up in the directory and stay readable via skill_read.
func parseSkillFile(path, dirName string) (Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}
	content := strings.ReplaceAll(string(data), "\r\n", "\n")

	sk := Skill{
		Name: dirName, // default to directory name
		Path: path,
	}

	fm, body, hasFrontmatter := splitFrontmatter(content)
	if !hasFrontmatter {
		if h := firstHeading(content); h != "" {
			sk.Name = h
		}
		sk.BodyHash = hashString(strings.TrimSpace(content))
		return sk, nil
	}

	for _, line := range strings.Split(fm, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue // blank lines and "# comment" lines without a colon
		}
		// Keys are matched case-insensitively, mirroring the prompt loader this
		// package replaced — `Name:`/`Description:`/`Allowed-Tools:` must parse,
		// not silently fall back to the directory name with an empty description.
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := unquoteSkill(strings.TrimSpace(parts[1]))

		switch key {
		case "name":
			if value != "" {
				sk.Name = value
			}
		case "description":
			sk.Description = value
		case "runas", "run_mode", "run-mode":
			sk.RunAs = value
		case "allowed-tools", "allowed_tools", "allowedtools":
			sk.AllowedTools = parseToolList(value)
		}
	}
	sk.BodyHash = hashString(strings.TrimSpace(body))
	return sk, nil
}

// splitFrontmatter splits SKILL.md content (already \n-normalized) into its
// frontmatter and body. hasFrontmatter is true only when an opening "---\n"
// fence has a matching "\n---\n" close; otherwise the whole content is the body
// (a heading-only or fence-less skill).
func splitFrontmatter(content string) (fm, body string, hasFrontmatter bool) {
	const open = "---\n"
	if !strings.HasPrefix(content, open) {
		return "", content, false
	}
	rest := content[len(open):]
	if idx := strings.Index(rest, "\n---\n"); idx >= 0 {
		return rest[:idx], rest[idx+len("\n---\n"):], true
	}
	return "", content, false
}

// firstHeading returns the text of the first markdown "# Heading", trimmed,
// or "" when the content has none.
func firstHeading(content string) string {
	if m := headingRe.FindStringSubmatch(content); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// unquoteSkill strips a single matching pair of surrounding single or double
// quotes from a frontmatter value.
func unquoteSkill(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// parseToolList splits a comma-separated tool list, trims each entry, and
// sorts for a deterministic stable directory.
func parseToolList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(value, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

// SkillChange describes a mutation between two skill stores.
type SkillChange struct {
	Kind    string // "added", "removed", "body_changed"
	Name    string
	OldHash string // for body_changed
	NewHash string // for body_changed
}

// VersionHash returns a SHA-256 hex digest of IndexText(). This is
// suitable for epoch hash computation — same skills produce same hash.
func (s *Store) VersionHash() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(s.IndexText())))
}

// Diff compares this store against an older store and returns the list
// of changes. Nil old means everything in s is "added".
func (s *Store) Diff(old *Store) []SkillChange {
	if old == nil {
		var changes []SkillChange
		for _, sk := range s.skills {
			changes = append(changes, SkillChange{Kind: "added", Name: sk.Name})
		}
		return changes
	}

	oldByName := make(map[string]Skill, len(old.skills))
	for _, sk := range old.skills {
		oldByName[sk.Name] = sk
	}
	newByName := make(map[string]Skill, len(s.skills))
	for _, sk := range s.skills {
		newByName[sk.Name] = sk
	}

	var changes []SkillChange

	for _, sk := range s.skills {
		oldSk, existed := oldByName[sk.Name]
		if !existed {
			changes = append(changes, SkillChange{Kind: "added", Name: sk.Name})
			continue
		}
		// Compare the body hash, not the description: a skill whose body
		// is edited while its frontmatter description stays the same must
		// still surface as a body change. This is the bug the cache-epoch
		// review called out — Diff used to read only the description.
		if oldSk.BodyHash != sk.BodyHash {
			changes = append(changes, SkillChange{
				Kind:    "body_changed",
				Name:    sk.Name,
				OldHash: oldSk.BodyHash,
				NewHash: sk.BodyHash,
			})
		}
	}

	for _, sk := range old.skills {
		if _, exists := newByName[sk.Name]; !exists {
			changes = append(changes, SkillChange{Kind: "removed", Name: sk.Name})
		}
	}

	return changes
}

// ReplaceFrom swaps this store's contents for other's, in place. Every holder
// of the *Store pointer sees the new skills at once — the agent's capability set
// and the skill_read dispatcher share one pointer, so a reload need not re-thread
// a new store through them. nil other empties the store.
//
// This is the mechanism behind Agent.ReloadSkills / the /reload-skills command,
// the sanctioned exception to the session-start-only rule documented on
// LoadScan. Not safe to call concurrently with readers: the caller (the agent
// loop must be idle) guarantees no in-flight turn is reading the store.
func (s *Store) ReplaceFrom(other *Store) {
	if other == nil {
		s.skills = nil
		s.byName = map[string]int{}
		return
	}
	s.skills = other.skills
	s.byName = other.byName
}

func hashString(s string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(s)))
}

// readSkillBody reads the body of a SKILL.md file. With frontmatter, that is
// everything after the closing fence; without frontmatter (a heading-only
// skill) the whole file is the body. The result is trimmed so it matches the
// body that BodyHash was computed over in parseSkillFile.
func readSkillBody(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	_, body, hasFrontmatter := splitFrontmatter(content)
	if !hasFrontmatter {
		return strings.TrimSpace(content), nil
	}
	return strings.TrimSpace(body), nil
}
