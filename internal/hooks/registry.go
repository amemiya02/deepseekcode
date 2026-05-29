package hooks

// KnownBuiltinNames is the authoritative set of builtin hook names this binary
// can register (the constructors live in this package; the wiring lives in
// cmd/dsc). Config validation cross-checks user hook configs against it so a
// typo'd builtin name (e.g. "duett") is rejected at validation time instead of
// silently fail-opening at runtime.
//
// Keep this in sync with the builtin registrations in cmd/dsc — currently the
// single "duet" hook (see builtin_duet.go).
var KnownBuiltinNames = []string{"duet"}

// IsKnownBuiltin reports whether name is a registered builtin hook.
func IsKnownBuiltin(name string) bool {
	for _, n := range KnownBuiltinNames {
		if n == name {
			return true
		}
	}
	return false
}
