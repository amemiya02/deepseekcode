package tokenizer

import (
	"reflect"
	"testing"
)

func TestBPEEncode_Merges(t *testing.T) {
	tests := []struct {
		name      string
		piece     string
		mergeRank map[string]int
		want      []string
	}{
		{
			name:      "empty",
			piece:     "",
			mergeRank: nil,
			want:      nil,
		},
		{
			name:      "single rune",
			piece:     "a",
			mergeRank: nil,
			want:      []string{"a"},
		},
		{
			name:      "no merges",
			piece:     "ab",
			mergeRank: nil,
			want:      []string{"a", "b"},
		},
		{
			name:      "merge a+b",
			piece:     "ab",
			mergeRank: map[string]int{"a b": 0},
			want:      []string{"ab"},
		},
		{
			name:      "chain abc",
			piece:     "abc",
			mergeRank: map[string]int{"a b": 0, "ab c": 1},
			want:      []string{"abc"},
		},
		{
			name:      "merge bc only",
			piece:     "abc",
			mergeRank: map[string]int{"b c": 0},
			want:      []string{"a", "bc"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bpeEncode(tt.piece, tt.mergeRank)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("bpeEncode(%q) = %v, want %v", tt.piece, got, tt.want)
			}
		})
	}
}
