// Package textenc detects and converts the legacy CJK and UTF-16 encodings
// Windows tools still emit, so dsc's file tools read/write them as UTF-8
// without mojibake. Concepts ported from DeepSeek-Reasonix
// internal/fileutil/encoding (MIT) — see NOTICE.
package textenc

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// Kind identifies a detected text encoding.
type Kind int

const (
	UTF8    Kind = iota
	UTF16LE      // UTF-16 Little-Endian (BOM or heuristic)
	UTF16BE      // UTF-16 Big-Endian (BOM or heuristic)
	GBK          // GBK fallback (superset of GB2312)
	GB18030      // GB18030 (strict superset of GBK)
)

// Detect returns the most likely encoding of b. The order of checks is:
// BOM markers first, then UTF-8 validity, then UTF-16 heuristic (NUL-byte
// distribution), then GB18030/GBK trial decode.
func Detect(b []byte) Kind {
	switch {
	case len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE:
		return UTF16LE
	case len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF:
		return UTF16BE
	case len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF:
		return UTF8
	}
	if utf8.Valid(b) {
		return UTF8
	}
	if le, be, ok := utf16Heuristic(b); ok {
		if le {
			return UTF16LE
		}
		if be {
			return UTF16BE
		}
	}
	if _, _, err := transform.Bytes(simplifiedchinese.GB18030.NewDecoder(), b); err == nil {
		return GB18030
	}
	return GBK
}

// utf16Heuristic checks the NUL-byte distribution in the first 4096 bytes to
// guess UTF-16 LE vs BE. Returns (le, be, ok).
func utf16Heuristic(b []byte) (le, be, ok bool) {
	n := len(b)
	if n > 4096 {
		n = 4096
	}
	if n < 2 {
		return false, false, false
	}
	var evenNUL, oddNUL int
	for i := 0; i < n; i++ {
		if b[i] == 0 {
			if i%2 == 0 {
				evenNUL++
			} else {
				oddNUL++
			}
		}
	}
	half := n / 2
	if oddNUL*10 > half*3 && oddNUL > evenNUL {
		return true, false, true
	}
	if evenNUL*10 > half*3 && evenNUL > oddNUL {
		return false, true, true
	}
	return false, false, false
}

// Decode converts b from encoding k to UTF-8, stripping any BOM.
func Decode(b []byte, k Kind) []byte {
	// Strip BOM.
	if len(b) >= 2 {
		if (b[0] == 0xFF && b[1] == 0xFE) || (b[0] == 0xFE && b[1] == 0xFF) {
			b = b[2:]
		}
	}
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		b = b[3:]
	}
	switch k {
	case UTF16LE:
		dec := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder()
		out, _, _ := transform.Bytes(dec, b)
		return out
	case UTF16BE:
		dec := unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewDecoder()
		out, _, _ := transform.Bytes(dec, b)
		return out
	case GBK:
		out, _, _ := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), b)
		return out
	case GB18030:
		out, _, _ := transform.Bytes(simplifiedchinese.GB18030.NewDecoder(), b)
		return out
	default:
		return b
	}
}

// Encode converts a UTF-8 string to encoding k, prepending the appropriate BOM
// for UTF-16 variants.
func Encode(s string, k Kind) []byte {
	switch k {
	case UTF16LE:
		enc := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewEncoder()
		out, _, _ := transform.Bytes(enc, []byte(s))
		return append([]byte{0xFF, 0xFE}, out...)
	case UTF16BE:
		enc := unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewEncoder()
		out, _, _ := transform.Bytes(enc, []byte(s))
		return append([]byte{0xFE, 0xFF}, out...)
	case GBK:
		out, _, _ := transform.Bytes(simplifiedchinese.GBK.NewEncoder(), []byte(s))
		return out
	case GB18030:
		out, _, _ := transform.Bytes(simplifiedchinese.GB18030.NewEncoder(), []byte(s))
		return out
	default:
		return []byte(s)
	}
}

// MatchLineEndings normalises old and new to the same line-ending convention as
// fileContent. If the file uses CRLF and the caller passed LF, both are
// converted to CRLF so a replacement preserves the file's convention.
//
// Note: edit_file.go uses its own normalizeLineEndings/convertToLineEnding
// pair which normalises everything to LF for matching and converts back on
// write. MatchLineEndings is a lighter-weight helper for callers that only
// need to align line endings without full round-trip editing.
func MatchLineEndings(fileContent, old, new string) (normalizedOld, normalizedNew string) {
	if strings.Contains(fileContent, "\r\n") && !strings.Contains(old, "\r\n") {
		old = strings.ReplaceAll(old, "\n", "\r\n")
		new = strings.ReplaceAll(new, "\n", "\r\n")
	}
	return old, new
}
