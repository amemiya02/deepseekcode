package modelreg

import "testing"

func TestSourceString(t *testing.T) {
	cases := map[Source]string{
		SourceDeclared: "declared", SourceBuiltin: "builtin",
		SourceFetched: "fetched", SourceDefault: "default",
	}
	for s, want := range cases {
		if s.String() != want {
			t.Errorf("Source(%d).String() = %q, want %q", s, s.String(), want)
		}
	}
}
