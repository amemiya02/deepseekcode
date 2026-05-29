// Package version exposes build-time identifiers stamped via -ldflags.
package version

import "strings"

// Populated by the build via -ldflags. Defaults make `go run` informative
// even without -ldflags.
//
// Version is the output of `git describe --tags --always`: a semver tag
// (e.g. "v0.2.0"), a tag plus distance (e.g. "v0.2.0-3-gf1fd170"), or — when
// no tag is reachable from HEAD — a bare short commit hash (e.g. "f1fd170").
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

// Display returns the human-facing version label, used for the startup banner
// and `dsc version`. It guarantees exactly one "v" prefix for a semver-shaped
// Version and never fakes one for an untagged build:
//
//	"v0.2.0"            → "v0.2.0"          (already prefixed by git describe)
//	"v0.2.0-3-gf1fd170" → "v0.2.0-3-gf1fd170"
//	"0.2.0"             → "v0.2.0"          (tag created without the "v")
//	"f1fd170"           → "build f1fd170"   (bare commit hash — not a version)
//	"dev" / "none" / "" → "dev"
//
// The callers must NOT prepend their own "v"; doing so used to render a bare
// commit hash as a pseudo-version ("v0612286") and would double-prefix a real
// tag ("vv0.2.0").
func Display() string {
	v := strings.TrimSpace(Version)
	switch v {
	case "", "dev", "none":
		return "dev"
	}
	if v[0] == 'v' {
		return v
	}
	if looksLikeSemver(v) {
		return "v" + v
	}
	return "build " + v
}

// looksLikeSemver reports whether s begins with a numeric component followed
// by a dot (e.g. "0.2.0"), distinguishing a version from a hex commit hash
// (which has no dot).
func looksLikeSemver(s string) bool {
	dot := strings.IndexByte(s, '.')
	if dot <= 0 {
		return false
	}
	for _, r := range s[:dot] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// String returns a "<display> (commit, date)" rendering for `dsc version`.
func String() string {
	return Display() + " (" + Commit + ", " + BuildDate + ")"
}
