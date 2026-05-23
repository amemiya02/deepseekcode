// Package version exposes build-time identifiers stamped via -ldflags.
package version

// Populated by the build via -ldflags. Defaults make `go run` informative
// even without -ldflags.
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

// String returns a "vX.Y.Z (commit, date)" rendering.
func String() string {
	return Version + " (" + Commit + ", " + BuildDate + ")"
}
