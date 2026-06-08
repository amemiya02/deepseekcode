package tui

import (
	"strings"
	"testing"
)

func TestRenderHUD_FourCauseWhenPresent(t *testing.T) {
	data := HUDData{
		Model: "deepseek-v4-flash", InputHitTokens: 900, InputMissTokens: 100, SavedCNY: 0.5,
		MissCauses: &MissCauses{ColdTokens: 60, MutTokens: 10, ResidualTokens: 20, ResetTokens: 10},
	}
	out := RenderHUD(data, 200)
	if !strings.Contains(out, "cold 60") {
		t.Fatalf("HUD should show four-cause breakdown when MissCauses present:\n%s", out)
	}
}

func TestRenderHUD_DegradesWithoutCauses(t *testing.T) {
	data := HUDData{Model: "deepseek-v4-flash", InputHitTokens: 900, InputMissTokens: 100}
	out := RenderHUD(data, 200)
	if strings.Contains(out, "cold") {
		t.Fatalf("without MissCauses the HUD must not show four-cause:\n%s", out)
	}
	if !strings.Contains(out, "cache") {
		t.Fatalf("baseline cache%% chip must still render:\n%s", out)
	}
}
