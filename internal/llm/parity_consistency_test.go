package llm

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestParityConsistency asserts the four sources of truth for the parity
// harness stay in sync:
//
//	A = scenario names from ParityScenarios()
//	B = scenario names recorded in testdata/parity/manifest.json
//	C = first-column scenario names listed in docs/dev/parity.md
//	D = filenames of testdata/parity/*.golden.json (sans .golden.json)
//
// A ≠ B ≠ C ≠ D means somebody added/removed/renamed a scenario in one
// place without updating the others — the test reports each missing-side
// pair so the fix is mechanical.
func TestParityConsistency(t *testing.T) {
	a := namesFromScenarios()
	b, err := namesFromManifest(filepath.Join("testdata", "parity", "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	paritDocPath := filepath.Join("..", "..", "docs", "dev", "parity.md")
	c, err := parsePARITYNames(paritDocPath)
	if err != nil {
		t.Fatalf("parse docs/dev/parity.md (%s): %v\nCreate it with a table whose first column is the scenario name.", paritDocPath, err)
	}
	d, err := namesFromGoldens(filepath.Join("testdata", "parity"))
	if err != nil {
		t.Fatalf("scan golden files: %v", err)
	}

	checkEqualSets(t, "ParityScenarios()", a, "manifest.json", b)
	checkEqualSets(t, "ParityScenarios()", a, "docs/dev/parity.md", c)
	checkEqualSets(t, "ParityScenarios()", a, "*.golden.json", d)
}

// parsePARITYNames extracts the first column from every markdown table
// row in path, skipping the header row and the `|---|` separator row,
// tolerating leading whitespace, blank lines, and non-table text.
func parsePARITYNames(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var names []string
	sc := bufio.NewScanner(f)
	sawHeader := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "|") {
			continue
		}
		// Strip leading/trailing pipes so an empty leading cell does not
		// shift the first-column index.
		inner := strings.Trim(line, "|")
		cols := strings.Split(inner, "|")
		if len(cols) == 0 {
			continue
		}
		first := strings.TrimSpace(cols[0])
		// Header separator row: cells made only of '-' and ':'.
		if isSeparatorRow(cols) {
			continue
		}
		if !sawHeader {
			// First table row encountered is the header — skip it.
			sawHeader = true
			continue
		}
		if first == "" {
			continue
		}
		names = append(names, first)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return names, nil
}

func isSeparatorRow(cols []string) bool {
	for _, c := range cols {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		for _, r := range c {
			if r != '-' && r != ':' {
				return false
			}
		}
	}
	return true
}

func namesFromScenarios() []string {
	scenarios := ParityScenarios()
	names := make([]string, 0, len(scenarios))
	for _, s := range scenarios {
		names = append(names, s.Name)
	}
	return names
}

func namesFromManifest(path string) ([]string, error) {
	m, err := loadParityManifest(path)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(m.Scenarios))
	for _, s := range m.Scenarios {
		names = append(names, s.Name)
	}
	return names, nil
}

// namesFromGoldens lists *.golden.json under dir, excluding the manifest
// file (which is not a golden) and stripping the .golden.json suffix.
func namesFromGoldens(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".golden.json") {
			continue
		}
		names = append(names, strings.TrimSuffix(name, ".golden.json"))
	}
	return names, nil
}

// checkEqualSets compares two name slices as sets and reports each
// asymmetric difference. Order does not matter; duplicates are flagged
// because they imply a defect upstream.
func checkEqualSets(t *testing.T, leftLabel string, left []string, rightLabel string, right []string) {
	t.Helper()
	ls := toSet(t, leftLabel, left)
	rs := toSet(t, rightLabel, right)

	var onlyLeft, onlyRight []string
	for k := range ls {
		if !rs[k] {
			onlyLeft = append(onlyLeft, k)
		}
	}
	for k := range rs {
		if !ls[k] {
			onlyRight = append(onlyRight, k)
		}
	}
	if len(onlyLeft) == 0 && len(onlyRight) == 0 {
		return
	}
	sort.Strings(onlyLeft)
	sort.Strings(onlyRight)
	msg := fmt.Sprintf("scenario sets differ between %s and %s:", leftLabel, rightLabel)
	if len(onlyLeft) > 0 {
		msg += fmt.Sprintf("\n  only in %s: %v", leftLabel, onlyLeft)
	}
	if len(onlyRight) > 0 {
		msg += fmt.Sprintf("\n  only in %s: %v", rightLabel, onlyRight)
	}
	t.Errorf("%s", msg)
}

func toSet(t *testing.T, label string, names []string) map[string]bool {
	t.Helper()
	out := make(map[string]bool, len(names))
	for _, n := range names {
		if out[n] {
			t.Errorf("%s: duplicate scenario name %q", label, n)
		}
		out[n] = true
	}
	return out
}
