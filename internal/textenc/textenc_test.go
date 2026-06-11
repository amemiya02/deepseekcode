package textenc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		file string
		want Kind
	}{
		{"utf8.txt", UTF8},
		{"utf16le.txt", UTF16LE},
		{"utf16be.txt", UTF16BE},
		// GBK content also decodes as GB18030 (strict superset), so Detect
		// returns GB18030 for it — this is correct and expected.
		{"gbk.txt", GB18030},
		{"gb18030.txt", GB18030},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatal(err)
			}
			got := Detect(b)
			if got != tt.want {
				t.Errorf("Detect(%s) = %v, want %v", tt.file, got, tt.want)
			}
		})
	}
}

func TestDetectEmpty(t *testing.T) {
	if got := Detect(nil); got != UTF8 {
		t.Errorf("Detect(nil) = %v, want UTF8", got)
	}
	if got := Detect([]byte{}); got != UTF8 {
		t.Errorf("Detect(empty) = %v, want UTF8", got)
	}
}

func TestDetectUTF8BOM(t *testing.T) {
	b := []byte{0xEF, 0xBB, 0xBF, 'h', 'e', 'l', 'l', 'o'}
	if got := Detect(b); got != UTF8 {
		t.Errorf("Detect(UTF-8 BOM) = %v, want UTF8", got)
	}
}

func TestRoundTrip(t *testing.T) {
	// All fixtures contain the same logical content.
	const want = "你好，世界\nhello\n"
	tests := []struct {
		file string
		kind Kind
	}{
		{"utf8.txt", UTF8},
		{"utf16le.txt", UTF16LE},
		{"utf16be.txt", UTF16BE},
		{"gbk.txt", GBK},
		{"gb18030.txt", GB18030},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatal(err)
			}
			decoded := Decode(raw, tt.kind)
			if string(decoded) != want {
				t.Errorf("Decode(%s) = %q, want %q", tt.file, decoded, want)
			}
			// Re-encode and decode again to verify round-trip.
			reencoded := Encode(string(decoded), tt.kind)
			red := Decode(reencoded, tt.kind)
			if string(red) != want {
				t.Errorf("round-trip %s: got %q, want %q", tt.file, red, want)
			}
		})
	}
}

func TestRoundTripUTF8(t *testing.T) {
	// UTF-8 Encode/Decode should be identity.
	const s = "hello world\n"
	encoded := Encode(s, UTF8)
	if string(encoded) != s {
		t.Errorf("Encode UTF-8: got %q, want %q", encoded, s)
	}
	decoded := Decode(encoded, UTF8)
	if string(decoded) != s {
		t.Errorf("Decode UTF-8: got %q, want %q", decoded, s)
	}
}

func TestMatchLineEndings(t *testing.T) {
	tests := []struct {
		name        string
		fileContent string
		old         string
		new         string
		wantOld     string
		wantNew     string
	}{
		{
			name:        "LF file stays LF",
			fileContent: "hello\nworld\n",
			old:         "hello\n",
			new:         "goodbye\n",
			wantOld:     "hello\n",
			wantNew:     "goodbye\n",
		},
		{
			name:        "CRLF file converts LF input",
			fileContent: "hello\r\nworld\r\n",
			old:         "hello\n",
			new:         "goodbye\n",
			wantOld:     "hello\r\n",
			wantNew:     "goodbye\r\n",
		},
		{
			name:        "CRLF input stays CRLF",
			fileContent: "hello\r\nworld\r\n",
			old:         "hello\r\n",
			new:         "goodbye\r\n",
			wantOld:     "hello\r\n",
			wantNew:     "goodbye\r\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOld, gotNew := MatchLineEndings(tt.fileContent, tt.old, tt.new)
			if gotOld != tt.wantOld {
				t.Errorf("old = %q, want %q", gotOld, tt.wantOld)
			}
			if gotNew != tt.wantNew {
				t.Errorf("new = %q, want %q", gotNew, tt.wantNew)
			}
		})
	}
}
