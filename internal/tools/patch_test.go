package tools

import (
	"strings"
	"testing"
)

func TestParsePatch(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []Hunk
		wantErr bool
	}{
		{
			name:  "add file",
			input: "*** Begin Patch\n*** Add File: a.txt\n+hello\n*** End Patch",
			want:  []Hunk{{Op: "add", Path: "a.txt", Contents: "hello"}},
		},
		{
			name:  "delete file",
			input: "*** Begin Patch\n*** Delete File: old.go\n*** End Patch",
			want:  []Hunk{{Op: "delete", Path: "old.go"}},
		},
		{
			name:  "update single chunk",
			input: "*** Begin Patch\n*** Update File: f.go\n@@\n line1\n-old\n+new\n*** End Patch",
			want: []Hunk{{Op: "update", Path: "f.go", Chunks: []UpdateChunk{{
				OldLines: []string{"line1", "old"}, NewLines: []string{"line1", "new"}, Context: "",
			}}}},
		},
		{
			name:  "update multi chunk",
			input: "*** Begin Patch\n*** Update File: f.go\n@@ ctx1\n a\n-x\n+y\n@@ ctx2\n b\n-p\n+q\n*** End Patch",
			want: []Hunk{{Op: "update", Path: "f.go", Chunks: []UpdateChunk{
				{OldLines: []string{"a", "x"}, NewLines: []string{"a", "y"}, Context: " ctx1"},
				{OldLines: []string{"b", "p"}, NewLines: []string{"b", "q"}, Context: " ctx2"},
			}}},
		},
		{
			name:  "move to",
			input: "*** Begin Patch\n*** Update File: old.go\n*** Move to: new.go\n@@\n line\n*** End Patch",
			want: []Hunk{{Op: "update", Path: "old.go", MovePath: "new.go", Chunks: []UpdateChunk{{
				OldLines: []string{"line"}, NewLines: []string{"line"},
			}}}},
		},
		{
			name:  "add empty file",
			input: "*** Begin Patch\n*** Add File: empty.txt\n*** End Patch",
			want:  []Hunk{{Op: "add", Path: "empty.txt", Contents: ""}},
		},
		{
			name:    "missing begin",
			input:   "*** Add File: a.txt\n+hi\n*** End Patch",
			wantErr: true,
		},
		{
			name:    "missing end",
			input:   "*** Begin Patch\n*** Add File: a.txt\n+hi",
			wantErr: true,
		},
		{
			name:  "multi file mixed",
			input: "*** Begin Patch\n*** Add File: a.txt\n+hello\n*** Update File: b.go\n@@\n-old\n+new\n*** Delete File: c.go\n*** End Patch",
			want: []Hunk{
				{Op: "add", Path: "a.txt", Contents: "hello"},
				{Op: "update", Path: "b.go", Chunks: []UpdateChunk{{
					OldLines: []string{"old"}, NewLines: []string{"new"},
				}}},
				{Op: "delete", Path: "c.go"},
			},
		},
		{
			name:    "malformed line in add",
			input:   "*** Begin Patch\n*** Add File: a.txt\nno-plus-prefix\n*** End Patch",
			wantErr: true,
		},
		{
			name:    "malformed line in update",
			input:   "*** Begin Patch\n*** Update File: f.go\n@@\n*bad\n*** End Patch",
			wantErr: true,
		},
		{
			name:  "update without @@ header",
			input: "*** Begin Patch\n*** Update File: f.go\n line1\n-old\n+new\n*** End Patch",
			want: []Hunk{{Op: "update", Path: "f.go", Chunks: []UpdateChunk{{
				OldLines: []string{"line1", "old"}, NewLines: []string{"line1", "new"},
			}}}},
		},
		{
			name:  "heredoc wrapper",
			input: "cat <<'EOF'\n*** Begin Patch\n*** Add File: a.txt\n+hi\n*** End Patch\nEOF",
			want:  []Hunk{{Op: "add", Path: "a.txt", Contents: "hi"}},
		},
		{
			name:  "CRLF lines",
			input: "*** Begin Patch\r\n*** Add File: a.txt\r\n+hi\r\n*** End Patch\r\n",
			want:  []Hunk{{Op: "add", Path: "a.txt", Contents: "hi"}},
		},
		{
			name:  "End of File anchor",
			input: "*** Begin Patch\n*** Update File: f.go\n@@\n+x\n*** End of File\n*** End Patch",
			want: []Hunk{{Op: "update", Path: "f.go", Chunks: []UpdateChunk{{
				NewLines: []string{"x"}, IsEndOfFile: true,
			}}}},
		},
		{
			name:  "End of File between chunks",
			input: "*** Begin Patch\n*** Update File: f.go\n@@\n a\n-x\n+y\n*** End of File\n@@\n b\n*** End Patch",
			want: []Hunk{{Op: "update", Path: "f.go", Chunks: []UpdateChunk{
				{OldLines: []string{"a", "x"}, NewLines: []string{"a", "y"}, IsEndOfFile: true},
				{OldLines: []string{"b"}, NewLines: []string{"b"}},
			}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePatch(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d hunks, want %d", len(got), len(tt.want))
			}
			for i, h := range got {
				w := tt.want[i]
				if h.Op != w.Op {
					t.Errorf("hunk[%d].Op = %q, want %q", i, h.Op, w.Op)
				}
				if h.Path != w.Path {
					t.Errorf("hunk[%d].Path = %q, want %q", i, h.Path, w.Path)
				}
				if h.MovePath != w.MovePath {
					t.Errorf("hunk[%d].MovePath = %q, want %q", i, h.MovePath, w.MovePath)
				}
				if h.Op == "add" && h.Contents != w.Contents {
					t.Errorf("hunk[%d].Contents = %q, want %q", i, h.Contents, w.Contents)
				}
				if h.Op == "update" {
					if len(h.Chunks) != len(w.Chunks) {
						t.Errorf("hunk[%d] got %d chunks, want %d", i, len(h.Chunks), len(w.Chunks))
						continue
					}
					for j, c := range h.Chunks {
						wc := w.Chunks[j]
						if !sliceEq(c.OldLines, wc.OldLines) {
							t.Errorf("hunk[%d].Chunks[%d].OldLines = %v, want %v", i, j, c.OldLines, wc.OldLines)
						}
						if !sliceEq(c.NewLines, wc.NewLines) {
							t.Errorf("hunk[%d].Chunks[%d].NewLines = %v, want %v", i, j, c.NewLines, wc.NewLines)
						}
						if c.Context != wc.Context {
							t.Errorf("hunk[%d].Chunks[%d].Context = %q, want %q", i, j, c.Context, wc.Context)
						}
					}
				}
			}
		})
	}
}

// TestParsePatchMalformedLineNo verifies error message contains line number.
func TestParsePatchMalformedLineNo(t *testing.T) {
	input := "*** Begin Patch\n*** Update File: f.go\n@@\n line\n*bad\n*** End Patch"
	_, err := ParsePatch(input)
	if err == nil {
		t.Fatal("expected error for malformed line")
	}
	if !strings.Contains(err.Error(), "line 4") {
		t.Errorf("error should mention line number, got: %v", err)
	}
}

func sliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestDeriveNewContents(t *testing.T) {
	tests := []struct {
		name     string
		original string
		chunks   []UpdateChunk
		want     string
		wantErr  bool
	}{
		{
			name:     "exact match",
			original: "a\nb\nc\n",
			chunks:   []UpdateChunk{{OldLines: []string{"b"}, NewLines: []string{"B", "B2"}}},
			want:     "a\nB\nB2\nc\n",
		},
		{
			name:     "rstrip match",
			original: "a\nb  \nc\n",
			chunks:   []UpdateChunk{{OldLines: []string{"b"}, NewLines: []string{"X"}}},
			want:     "a\nX\nc\n",
		},
		{
			name:     "trim match",
			original: "a\n  b  \nc\n",
			chunks:   []UpdateChunk{{OldLines: []string{"b"}, NewLines: []string{"X"}}},
			want:     "a\nX\nc\n",
		},
		{
			name:     "pure add at EOF",
			original: "a\nb\n",
			chunks:   []UpdateChunk{{OldLines: nil, NewLines: []string{"c"}, IsEndOfFile: true}},
			want:     "a\nb\nc\n",
		},
		{
			name:     "pure add mid-file after prior chunk",
			original: "a\nb\nc\n",
			chunks: []UpdateChunk{
				{OldLines: []string{"a"}, NewLines: []string{"a"}},
				{OldLines: nil, NewLines: []string{"a2"}},
			},
			want: "a\na2\nb\nc\n",
		},
		{
			name:     "locate failure",
			original: "a\nb\nc\n",
			chunks:   []UpdateChunk{{OldLines: []string{"z"}, NewLines: []string{"Z"}}},
			wantErr:  true,
		},
		{
			name:     "multi chunk sequential",
			original: "a\nb\nc\nd\ne\n",
			chunks: []UpdateChunk{
				{OldLines: []string{"b"}, NewLines: []string{"B"}},
				{OldLines: []string{"d"}, NewLines: []string{"D"}},
			},
			want: "a\nB\nc\nD\ne\n",
		},
		{
			name:     "empty original pure add",
			original: "",
			chunks:   []UpdateChunk{{OldLines: nil, NewLines: []string{"hello"}, IsEndOfFile: true}},
			want:     "hello",
		},
		{
			name:     "context lines preserved",
			original: "a\nb\nc\n",
			chunks:   []UpdateChunk{{OldLines: []string{"a", "b"}, NewLines: []string{"a", "B"}}},
			want:     "a\nB\nc\n",
		},
		{
			name:     "CRLF original normalized",
			original: "a\r\nb\r\nc\r\n",
			chunks:   []UpdateChunk{{OldLines: []string{"b"}, NewLines: []string{"B"}}},
			want:     "a\r\nB\r\nc\r\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DeriveNewContents("test.go", tt.chunks, tt.original)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}
