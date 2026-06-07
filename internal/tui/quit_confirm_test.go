package tui

import (
	"strings"
	"testing"
)

func TestOverlay_QuitConfirm(t *testing.T) {
	var o Overlay
	o.OpenQuitConfirm()
	if !o.IsOpen() {
		t.Fatal("quit confirm should be open")
	}
	if got := o.QuitConfirmResolve("n"); got != quitCancel {
		t.Fatalf(`"n" should cancel quit, got %v`, got)
	}
	o.OpenQuitConfirm()
	if got := o.QuitConfirmResolve("y"); got != quitConfirmed {
		t.Fatalf(`"y" should confirm quit, got %v`, got)
	}
}

func TestOverlay_QuitConfirmEscCancels(t *testing.T) {
	var o Overlay
	o.OpenQuitConfirm()
	if got := o.QuitConfirmResolve("esc"); got != quitCancel {
		t.Fatalf(`"esc" should cancel quit, got %v`, got)
	}
	if o.IsOpen() {
		t.Fatal("overlay should be closed after esc")
	}
}

func TestOverlay_QuitConfirmIgnoresOtherKeys(t *testing.T) {
	var o Overlay
	o.OpenQuitConfirm()
	if got := o.QuitConfirmResolve("x"); got != quitNone {
		t.Fatalf(`"x" should be ignored, got %v`, got)
	}
	if !o.IsOpen() {
		t.Fatal("overlay should still be open for unrecognized key")
	}
}

func TestRenderQuitConfirmShowsPrompt(t *testing.T) {
	th := DarkTheme()
	out := renderQuitConfirm(th, 100, 20)
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "quit") {
		t.Fatalf("renderQuitConfirm missing 'quit':\n%s", stripped)
	}
}
