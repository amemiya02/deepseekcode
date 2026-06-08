package tui

import "testing"

func TestOverlay_FilePicker(t *testing.T) {
	var o Overlay
	o.OpenFilePicker([]string{"cmd/dsc/main.go", "internal/tui/app.go", "README.md"})
	if !o.IsOpen() {
		t.Fatal("file picker should be open")
	}
	if o.Mode() != modeFilePicker {
		t.Fatalf("mode should be modeFilePicker, got %d", o.Mode())
	}
	if !o.Filterable() {
		t.Fatal("file picker should be filterable")
	}
	// Apply filter
	o.FilterType('a')
	o.FilterType('p')
	o.FilterType('p')
	if got := o.SelectedFile(); got != "internal/tui/app.go" {
		t.Fatalf(`filtering "app" should select app.go, got %q`, got)
	}
	// Clear and check all paths accessible
	o.FilterClear()
	if n := len(o.FilePickerPaths()); n != 3 {
		t.Fatalf("expected 3 paths, got %d", n)
	}
}

func TestOverlay_FilePicker_Empty(t *testing.T) {
	var o Overlay
	o.OpenFilePicker(nil)
	if !o.IsOpen() {
		t.Fatal("file picker should be open even with empty list")
	}
	if got := o.SelectedFile(); got != "" {
		t.Fatalf("empty picker should return empty string, got %q", got)
	}
}

func TestOverlay_FilePicker_Selected(t *testing.T) {
	var o Overlay
	o.OpenFilePicker([]string{"a.go", "b.go", "c.go"})
	if got := o.SelectedFile(); got != "a.go" {
		t.Fatalf("cursor at 0 should select first path, got %q", got)
	}
	o.MoveDown()
	if got := o.SelectedFile(); got != "b.go" {
		t.Fatalf("after MoveDown should select b.go, got %q", got)
	}
}
