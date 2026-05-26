// Package skills implements a cache-stable skill metadata index.
// Skill bodies are loaded lazily and excluded from the prefix-stable
// index text to avoid invalidating the prompt cache when skill
// content changes.
package skills

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Skill is the metadata for a single skill.
type Skill struct {
	Name        string
	Description string
	Scope       string
	RunAs       string
	Path        string
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

// IndexText returns a deterministic, prefix-stable text representation
// of the skill index. One line per skill with name, description, scope,
// and run mode. Skill bodies are NOT included.
func (s *Store) IndexText() string {
	var b strings.Builder
	for _, sk := range s.skills {
		runAs := sk.RunAs
		if runAs == "" {
			runAs = "direct"
		}
		fmt.Fprintf(&b, "%s | %s | %s | %s\n", sk.Name, sk.Description, sk.Scope, runAs)
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

// parseSkillFile parses a SKILL.md file's frontmatter and returns metadata.
func parseSkillFile(path, dirName string) (Skill, error) {
	f, err := os.Open(path)
	if err != nil {
		return Skill{}, err
	}
	defer f.Close()

	sk := Skill{
		Name: dirName, // default to directory name
		Path: path,
	}

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	frontmatterDone := false

	for scanner.Scan() {
		line := scanner.Text()

		if line == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			// Second --- ends frontmatter
			frontmatterDone = true
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
			case "runAs":
				sk.RunAs = value
			}
		}
	}

	if !frontmatterDone {
		return Skill{}, fmt.Errorf("no frontmatter found")
	}

	return sk, nil
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
