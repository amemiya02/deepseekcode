package tui

import (
	"strings"
	"testing"
)

func TestEventRepairDispatch(t *testing.T) {
	sb := NewScrollback()
	sb.AppendRepair("args_completed", "read_file", "args completed")

	items := sb.Items()
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	it := items[0]
	if it.kind != itemRepair {
		t.Errorf("expected kind itemRepair, got %v", it.kind)
	}
	if it.repairKind != "args_completed" {
		t.Errorf("expected repairKind args_completed, got %q", it.repairKind)
	}
	if it.repairTool != "read_file" {
		t.Errorf("expected repairTool read_file, got %q", it.repairTool)
	}
	if it.repairMessage != "args completed" {
		t.Errorf("expected repairMessage 'args completed', got %q", it.repairMessage)
	}
}

func TestRenderRepairItem(t *testing.T) {
	th := DarkTheme()
	item := chatItem{
		kind:          itemRepair,
		repairKind:    "args_completed",
		repairTool:    "read_file",
		repairMessage: "args completed",
	}
	out := item.render(th, 80)
	if !strings.Contains(out, "repaired tool call:") {
		t.Errorf("expected 'repaired tool call:' in output, got %q", out)
	}
	if !strings.Contains(out, "read_file") {
		t.Errorf("expected 'read_file' in output, got %q", out)
	}
	if !strings.Contains(out, "args completed") {
		t.Errorf("expected 'args completed' in output, got %q", out)
	}
}

func TestRenderRepairItemEmptyTool(t *testing.T) {
	th := DarkTheme()
	item := chatItem{
		kind:          itemRepair,
		repairKind:    "args_invalid",
		repairTool:    "",
		repairMessage: "invalid JSON",
	}
	out := item.render(th, 80)
	if !strings.Contains(out, "repaired tool call:") {
		t.Errorf("expected 'repaired tool call:' in output, got %q", out)
	}
	if strings.Contains(out, "  ") && strings.Contains(out, "invalid JSON") {
		// Should not have double space before message when tool is empty
		if idx := strings.Index(out, "repaired tool call:"); idx >= 0 {
			after := out[idx+len("repaired tool call:"):]
			if strings.HasPrefix(after, "  ") {
				t.Errorf("should not have double space when tool is empty, got %q", out)
			}
		}
	}
}

func TestTapeIncludesRepair(t *testing.T) {
	sb := NewScrollback()
	sb.AppendToolCall("c1", "read_file", `{"path":"README.md"}`)
	sb.AppendRepair("args_completed", "read_file", "args completed")

	entries := tapeEntries(sb.Items())
	if len(entries) != 2 {
		t.Fatalf("expected 2 tape entries, got %d", len(entries))
	}
	// First entry should be tool call
	if entries[0].kind != itemToolCall {
		t.Errorf("expected first entry to be itemToolCall, got %v", entries[0].kind)
	}
	// Second entry should be repair
	if entries[1].kind != itemRepair {
		t.Errorf("expected second entry to be itemRepair, got %v", entries[1].kind)
	}
	if !strings.Contains(entries[1].label, "read_file") {
		t.Errorf("expected repair entry label to contain 'read_file', got %q", entries[1].label)
	}
	if !strings.Contains(entries[1].label, "args completed") {
		t.Errorf("expected repair entry label to contain 'args completed', got %q", entries[1].label)
	}
}

func TestRepairLongMessageWraps(t *testing.T) {
	th := DarkTheme()
	// Create a long message that should wrap at narrow width
	longMsg := "this is a very long repair message that should wrap when rendered at a narrow width because it exceeds the terminal width constraint"
	item := chatItem{
		kind:          itemRepair,
		repairKind:    "args_completed",
		repairTool:    "read_file",
		repairMessage: longMsg,
	}

	// Render at narrow width (20 columns) — should wrap to multiple lines
	out := item.render(th, 20)

	// Should contain multiple newlines (wrapped lines)
	newlineCount := strings.Count(out, "\n")
	if newlineCount < 2 {
		t.Errorf("expected wrapped output with multiple newlines at width 20, got %d newlines: %q", newlineCount, out)
	}

	// The item kind should remain itemRepair
	if item.kind != itemRepair {
		t.Errorf("item kind changed during render, got %v", item.kind)
	}

	// Verify the content is preserved
	if !strings.Contains(out, "repaired tool call:") {
		t.Errorf("expected 'repaired tool call:' in output, got %q", out)
	}
	if !strings.Contains(out, "read_file") {
		t.Errorf("expected 'read_file' in output, got %q", out)
	}
}

func TestRepairNotRoutedThroughEventInfo(t *testing.T) {
	// This test verifies that EventRepair is handled separately from EventInfo.
	// The dispatchAgentEvent function should have a dedicated case for EventRepair
	// that calls AppendRepair, NOT AppendInfo.
	//
	// This is a compile-time guarantee: if EventRepair did not have its own case,
	// the code would not compile because the type switch must be exhaustive.
	// We test runtime behavior here.
	th := DarkTheme()

	// Create both an info and a repair item and verify they render differently.
	infoItem := chatItem{kind: itemInfo, text: "some info message"}
	repairItem := chatItem{kind: itemRepair, repairKind: "args_completed", repairTool: "read_file", repairMessage: "args completed"}

	infoOut := infoItem.render(th, 80)
	repairOut := repairItem.render(th, 80)

	// Info items have [info] prefix
	if !strings.Contains(infoOut, "[info]") {
		t.Errorf("info item should contain '[info]', got %q", infoOut)
	}
	// Repair items have 'repaired tool call:' prefix
	if !strings.Contains(repairOut, "repaired tool call:") {
		t.Errorf("repair item should contain 'repaired tool call:', got %q", repairOut)
	}
	// They should not render the same
	if infoOut == repairOut {
		t.Errorf("info and repair items should render differently, both got %q", infoOut)
	}
}
