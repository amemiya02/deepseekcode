package memory_test

import (
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/memory"
)

func TestStripSecretsAPIKey(t *testing.T) {
	cases := []struct {
		name  string
		input string
		must  string // substring that MUST remain
		must0 string // substring that MUST be gone
	}{
		{
			name:  "openai key",
			input: "token=sk-proj-abcdefghijklmnopqrstuvwxyz012345",
			must:  "token=",
			must0: "sk-proj-abcdefghijklmnopqrstuvwxyz012345",
		},
		{
			name:  "anthropic key",
			input: "key: sk-ant-api03-ABCDEFGHIJKLMNOPQRSTUVWXYZ",
			must:  "key:",
			must0: "sk-ant-api03-ABCDEFGHIJKLMNOPQRSTUVWXYZ",
		},
		{
			name:  "deepseek key",
			input: "DEEPSEEK_API_KEY=dsk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx rest of line",
			must:  "rest of line",
			must0: "dsk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		},
		{
			name:  "generic bearer",
			input: `Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9`,
			must:  "Authorization:",
			must0: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
		},
		{
			name:  "private tag",
			input: "public info <private>my secret plan</private> more public",
			must:  "public info",
			must0: "my secret plan",
		},
		{
			name:  "private tag multiline",
			input: "before\n<private>\nline1\nline2\n</private>\nafter",
			must:  "after",
			must0: "line1",
		},
		{
			name:  "env var assignment with long value",
			input: "export AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			must0: "wJalrXUtnFEMI",
		},
		{
			name:  "clean text unchanged",
			input: "The agent prefers terse replies in Go.",
			must:  "The agent prefers terse replies in Go.",
			must0: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := memory.StripSecrets(tc.input)
			if tc.must != "" && !strings.Contains(got, tc.must) {
				t.Errorf("expected %q to contain %q\noriginal: %q\ngot: %q",
					tc.name, tc.must, tc.input, got)
			}
			if tc.must0 != "" && strings.Contains(got, tc.must0) {
				t.Errorf("expected %q to NOT contain %q\noriginal: %q\ngot: %q",
					tc.name, tc.must0, tc.input, got)
			}
		})
	}
}

func TestStripSecretsIdempotent(t *testing.T) {
	in := "sk-proj-abcdefghijklmnopqrstuvwxyz012345 normal text"
	once := memory.StripSecrets(in)
	twice := memory.StripSecrets(once)
	if once != twice {
		t.Errorf("StripSecrets not idempotent:\nonce:  %q\ntwice: %q", once, twice)
	}
}
