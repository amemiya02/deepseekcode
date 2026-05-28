package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAgentListAndShow(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".deepseek", "agent")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "scout.md"), []byte("---\ndescription: Scout repo\ntools: read_file, grep\n---\nScout."), 0o644); err != nil {
		t.Fatalf("write agent: %v", err)
	}

	list, err := runAgentCommand(dir, []string{"list"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(list, "scout") || !strings.Contains(list, "Scout repo") {
		t.Fatalf("list output missing scout:\n%s", list)
	}

	show, err := runAgentCommand(dir, []string{"show", "scout"})
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if !strings.Contains(show, "tools: read_file, grep") {
		t.Fatalf("show output missing tools:\n%s", show)
	}
}

func TestRunAgentNew(t *testing.T) {
	dir := t.TempDir()
	out, err := runAgentCommand(dir, []string{"new", "reviewer"})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if !strings.Contains(out, ".deepseek/agent/reviewer.md") {
		t.Fatalf("new output missing path:\n%s", out)
	}
	body, err := os.ReadFile(filepath.Join(dir, ".deepseek", "agent", "reviewer.md"))
	if err != nil {
		t.Fatalf("read new agent: %v", err)
	}
	if !strings.Contains(string(body), "description: reviewer") {
		t.Fatalf("new agent template unexpected:\n%s", body)
	}
}
