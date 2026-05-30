package main

import (
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/llm"
)

func TestFormatUserBalanceForDoctor(t *testing.T) {
	ub := llm.UserBalance{
		IsAvailable: true,
		BalanceInfos: []llm.BalanceInfo{
			{Currency: "USD", TotalBalance: "2.50"},
			{Currency: "CNY", TotalBalance: "110.00"},
		},
	}
	got := formatUserBalanceForDoctor(ub)
	want := "CNY=110.00 USD=2.50"
	if got != want {
		t.Errorf("formatUserBalanceForDoctor = %q, want %q", got, want)
	}
}

func TestFormatUserBalanceForDoctorEmpty(t *testing.T) {
	got := formatUserBalanceForDoctor(llm.UserBalance{})
	if got != "" {
		t.Errorf("formatUserBalanceForDoctor(empty) = %q, want %q", got, "")
	}
}

func TestCheckDeepSeekAccountNonDeepSeek(t *testing.T) {
	cfg := configFromProvider("openai", "https://api.openai.com")
	res := checkDeepSeekAccount(cfg)
	if res.Status != "ok" {
		t.Errorf("status = %q, want ok", res.Status)
	}
	if !containsStr(res.Detail, "skipped") {
		t.Errorf("detail %q should contain 'skipped'", res.Detail)
	}
}

func TestCheckPricingFreshness(t *testing.T) {
	// Use a date 6 days after the checked date (2026-05-25).
	now := timeFromYMD(2026, 5, 31)
	res := checkPricingFreshness(now)
	if res.Status != "ok" {
		t.Errorf("status = %q, want ok; detail=%q", res.Status, res.Detail)
	}
	if !containsStr(res.Detail, "2026-05-25") {
		t.Errorf("detail %q should contain checked date", res.Detail)
	}
	if !containsStr(res.Detail, "6 days") {
		t.Errorf("detail %q should contain '6 days'", res.Detail)
	}
	if !containsStr(res.Detail, llm.PricingSourceURL) {
		t.Errorf("detail %q should contain source URL", res.Detail)
	}
}

func TestCheckPricingFreshnessStale(t *testing.T) {
	// Use a date 36 days after the checked date.
	now := timeFromYMD(2026, 6, 30)
	res := checkPricingFreshness(now)
	if res.Status != "warn" {
		t.Errorf("status = %q, want warn for stale pricing", res.Status)
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstring(s, sub))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func configFromProvider(ptype, baseURL string) config.Config {
	return config.Config{
		Defaults: config.DefaultsConfig{Model: "deepseek-v4-flash"},
		Providers: map[string]config.ProviderConfigTOML{
			"test": {Type: ptype, BaseURL: baseURL, APIKey: "sk-test"},
		},
		Active: config.ActiveConfig{Provider: "test"},
	}
}

func timeFromYMD(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}
