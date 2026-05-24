package tools

import "testing"

func TestLevenshteinLines(t *testing.T) {
	cases := []struct {
		name string
		a, b []string
		want int
	}{
		{"both empty", nil, nil, 0},
		{"a empty", nil, []string{"x"}, 1},
		{"b empty", []string{"x"}, nil, 1},
		{"identical", []string{"a", "b", "c"}, []string{"a", "b", "c"}, 0},
		{"single substitution", []string{"a", "b", "c"}, []string{"a", "X", "c"}, 1},
		{"single insertion", []string{"a", "c"}, []string{"a", "b", "c"}, 1},
		{"single deletion", []string{"a", "b", "c"}, []string{"a", "c"}, 1},
		{"complete replacement", []string{"x", "y"}, []string{"a", "b"}, 2},
		{"multi-line diff", []string{"foo", "bar", "baz"}, []string{"foo", "qux"}, 2},
		{"prefix match", []string{"a", "b", "c"}, []string{"a", "b", "c", "d"}, 1},
		{"suffix match", []string{"a", "b", "c"}, []string{"z", "a", "b", "c"}, 1},
		{"reorder", []string{"a", "b", "c"}, []string{"c", "b", "a"}, 2},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := LevenshteinLines(c.a, c.b)
			if got != c.want {
				t.Errorf("LevenshteinLines(%v, %v) = %d; want %d", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestClosestMatch(t *testing.T) {
	haystack := []string{
		"line1\nline2\nline3",
		"alpha\nbeta\ngamma",
		"line1\nline2\nline4",
		"completely different",
	}

	cases := []struct {
		name    string
		needle  string
		maxDist int
		wantIdx int
		wantD   int
	}{
		{"exact match", "line1\nline2\nline3", 0, 0, 0},
		{"one line off", "line1\nline2\nline4", 1, 2, 0},
		{"within maxDist", "line1\nline2\nlineX", 1, 0, 1},
		{"too far", "zzz\nzzz", 1, -1, 2},
		{"larger maxDist", "alpha\nbeta\ndelta", 2, 1, 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			idx, dist := ClosestMatch(c.needle, haystack, c.maxDist)
			if idx != c.wantIdx || dist != c.wantD {
				t.Errorf("ClosestMatch(%q, ..., %d) = (%d, %d); want (%d, %d)",
					c.needle, c.maxDist, idx, dist, c.wantIdx, c.wantD)
			}
		})
	}
}
