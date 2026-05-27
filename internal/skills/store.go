// Package skills implements a cache-stable skill metadata index.
// Skill bodies are loaded lazily and excluded from the prefix-stable
// index text to avoid invalidating the prompt cache when skill
// content changes.
package skills

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
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

// parseSkillFile parses a SKILL.md file's frontmatter and body. It reads
// the file once, extracts metadata from the frontmatter, and computes the
// body hash (everything after the closing "---") so a body-only edit is
// detectable without keeping the body in memory.
func parseSkillFile(path, dirName string) (Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}

	sk := Skill{
		Name: dirName, // default to directory name
		Path: path,
	}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	inFrontmatter := false
	frontmatterDone := false
	bodyStart := -1

	for i, line := range lines {
		if line == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			frontmatterDone = true
			bodyStart = i + 1
			break
		}
		if inFrontmatter {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			switch key {
			case "name":
				if value != "" {
					sk.Name = value
				}
			case "description":
				sk.Description = value
			case "runAs", "run_mode", "run-mode":
				sk.RunAs = value
			case "allowed-tools", "allowed_tools", "allowedTools":
				sk.AllowedTools = parseToolList(value)
			}
		}
	}

	if !frontmatterDone {
		return Skill{}, fmt.Errorf("no frontmatter found")
	}

	body := ""
	if bodyStart >= 0 && bodyStart < len(lines) {
		body = strings.TrimSpace(strings.Join(lines[bodyStart:], "\n"))
	}
	sk.BodyHash = hashString(body)

	return sk, nil
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

func hashString(s string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(s)))
}

// readSkillBody reads the body of a SKILL.md file (after frontmatter).
func readSkillBody(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	frontmatterDone := false
	var body strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		if line == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			frontmatterDone = true
			continue
		}

		if frontmatterDone {
			if body.Len() > 0 {
				body.WriteString("\n")
			}
			body.WriteString(line)
		}
	}

	return body.String(), nil
}
