// internal/cacheunit/align_test.go
package cacheunit

import "testing"

func TestAlignPadding(t *testing.T) {
	cases := []struct{ prefix, unit, want int }{
		{1000, 64, 24}, // next multiple of 64 is 1024 -> pad 24
		{1024, 64, 0},  // already aligned
		{0, 64, 0},
		{100, 0, 0}, // unknown unit -> no padding
	}
	for _, c := range cases {
		if got := AlignPadding(c.prefix, c.unit); got != c.want {
			t.Fatalf("AlignPadding(%d,%d)=%d want %d", c.prefix, c.unit, got, c.want)
		}
	}
}
