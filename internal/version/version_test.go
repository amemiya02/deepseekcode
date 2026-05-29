package version

import (
	"strings"
	"testing"
)

// TestDisplay pins the prefix-normalization contract: exactly one "v" for a
// semver-shaped Version, and no fake "v" for an untagged (bare-hash) build.
// Regression guard for the old banner that rendered a commit hash as
// "v0612286" and would have double-prefixed a real tag as "vv0.2.0".
func TestDisplay(t *testing.T) {
	orig := Version
	defer func() { Version = orig }()

	cases := []struct {
		in, want string
	}{
		{"v0.2.0", "v0.2.0"},                       // semver tag at HEAD
		{"v0.2.0-3-gf1fd170", "v0.2.0-3-gf1fd170"}, // tag + distance + hash
		{"0.2.0", "v0.2.0"},                        // tag created without the "v"
		{"1.0", "v1.0"},                            // short semver
		{"f1fd170", "build f1fd170"},               // bare commit hash, no reachable tag
		{"0612286", "build 0612286"},               // all-digit hash must NOT look like a version
		{"dev", "dev"},                             // go run, no ldflags
		{"none", "dev"},
		{"", "dev"},
		{"  v0.2.0  ", "v0.2.0"}, // stamping whitespace is trimmed
	}
	for _, c := range cases {
		Version = c.in
		if got := Display(); got != c.want {
			t.Errorf("Display() with Version=%q = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestStringUsesDisplay confirms `dsc version` shows the normalized label, so
// the banner and the CLI can never disagree on the version.
func TestStringUsesDisplay(t *testing.T) {
	orig := Version
	defer func() { Version = orig }()

	Version = "v0.2.0"
	if got := String(); !strings.HasPrefix(got, "v0.2.0 (") {
		t.Errorf("String() = %q, want it to lead with the display version %q", got, Display())
	}
}
