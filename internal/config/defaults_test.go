package config

import "testing"

func TestDefault_FillsUINetworkPermissionBaselines(t *testing.T) {
	d := Default()
	cases := map[string]string{
		"UI.Accent":           d.UI.Accent,
		"UI.Density":          d.UI.Density,
		"UI.Language":         d.UI.Language,
		"Network.ProxyMode":   d.Network.ProxyMode,
		"Network.ProxyScheme": d.Network.ProxyScheme,
		"Permissions.Default": d.Permissions.Default,
	}
	want := map[string]string{
		"UI.Accent": "indigo", "UI.Density": "comfortable", "UI.Language": "auto",
		"Network.ProxyMode": "auto", "Network.ProxyScheme": "http", "Permissions.Default": "ask",
	}
	for k, got := range cases {
		if got != want[k] {
			t.Errorf("%s = %q, want %q", k, got, want[k])
		}
	}
}
