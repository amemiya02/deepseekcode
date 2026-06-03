package commands

import "testing"

func TestParseCheckpointCommand(t *testing.T) {
	cases := []struct {
		input    string
		wantErr  bool
		wantName string
	}{
		{"/checkpoint before-refactor", false, "before-refactor"},
		{`/checkpoint "my feature"`, false, "my feature"},
		{"/checkpoint before big refactor", true, ""},
		{"/checkpoint", true, ""},
		{"/checkpoint   ", true, ""},
	}
	for _, tc := range cases {
		name, err := parseCheckpointArgs(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("parseCheckpointArgs(%q) err=%v, wantErr=%v", tc.input, err, tc.wantErr)
		}
		if err == nil && name != tc.wantName {
			t.Errorf("parseCheckpointArgs(%q) name=%q, want %q", tc.input, name, tc.wantName)
		}
	}
}

func TestParseBranchCommand(t *testing.T) {
	cases := []struct {
		input    string
		wantErr  bool
		wantName string
	}{
		{"/branch before-refactor", false, "before-refactor"},
		{"/branch 7", false, "7"},
		{`/branch "my feature"`, false, "my feature"},
		{"/branch before big refactor", true, ""},
		{"/branch", true, ""},
		{"/branch   ", true, ""},
	}
	for _, tc := range cases {
		name, err := parseBranchArgs(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("parseBranchArgs(%q) err=%v wantErr=%v", tc.input, err, tc.wantErr)
		}
		if err == nil && name != tc.wantName {
			t.Errorf("parseBranchArgs(%q) = %q, want %q", tc.input, name, tc.wantName)
		}
	}
}
