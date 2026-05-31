package tokenizer

import (
	"math"
	"strings"

	"github.com/dlclark/regexp2"
)

// charToByteOffset converts a regexp2 character (rune) index to a byte offset
// in s. regexp2 returns character indices, but Go string slicing needs byte offsets.
func charToByteOffset(s string, charIdx int) int {
	byteIdx := 0
	for i := range s {
		if byteIdx == charIdx {
			return i
		}
		byteIdx++
	}
	return len(s)
}

// applySplit splits each chunk by re in Isolated behavior: matches become
// their own tokens, and so do the gaps between them.
func applySplit(chunks []string, re *regexp2.Regexp) []string {
	var out []string
	for _, chunk := range chunks {
		if chunk == "" {
			continue
		}
		lastByte := 0
		for m, err := re.FindStringMatch(chunk); m != nil; m, err = re.FindNextMatch(m) {
			if err != nil {
				break
			}
			matchByteStart := charToByteOffset(chunk, m.Index)
			match := m.String()
			matchByteEnd := matchByteStart + len(match)
			if matchByteStart > lastByte {
				out = append(out, chunk[lastByte:matchByteStart])
			}
			if len(match) > 0 {
				out = append(out, match)
			}
			lastByte = matchByteEnd
		}
		if lastByte < len(chunk) {
			out = append(out, chunk[lastByte:])
		}
	}
	return out
}

// Encode returns the token ids for text under the V4 BPE + pretokenization.
func (l *loaded) Encode(text string) []int {
	if text == "" {
		return nil
	}
	var ids []int

	// process encodes one segment (non-added-token text).
	process := func(segment string) {
		if segment == "" {
			return
		}
		chunks := []string{segment}
		for _, re := range l.splitRegexes {
			chunks = applySplit(chunks, re)
		}
		for _, chunk := range chunks {
			if chunk == "" {
				continue
			}
			byteLevel := byteLevelEncode(chunk, l.byteToChar)
			pieces := bpeEncode(byteLevel, l.mergeRank)
			for _, p := range pieces {
				if id, ok := l.vocab[p]; ok {
					ids = append(ids, id)
				}
				// Absent from vocab → skip (under-count rather than panic).
			}
		}
	}

	if l.addedPattern != nil {
		last := 0
		for m, err := l.addedPattern.FindStringMatch(text); m != nil; m, err = l.addedPattern.FindNextMatch(m) {
			if err != nil {
				break
			}
			idx := m.Index
			if idx > last {
				process(text[last:idx])
			}
			if id, ok := l.addedMap[m.String()]; ok {
				ids = append(ids, id)
			}
			last = idx + len(m.String())
		}
		if last < len(text) {
			process(text[last:])
		}
	} else {
		process(text)
	}
	return ids
}

// Encode is the exported wrapper around loaded.Encode for use by test helpers
// and generators. Returns nil if the tokenizer is unavailable.
func Encode(text string) []int {
	l, err := load()
	if err != nil {
		return nil
	}
	return l.Encode(text)
}

// Count returns len(Encode(text)). Returns 0 for "".
func Count(text string) int {
	l, err := load()
	if err != nil {
		return 0
	}
	return len(l.Encode(text))
}

// DefaultBoundedTokenizeChars is the default max-chars cap for CountBounded.
const DefaultBoundedTokenizeChars = 2 * 1024

// CountBounded counts exactly when len(text) <= maxChars, else estimates via
// head+tail sampling (ratio extrapolation). maxChars<=0 => 0.3*len fallback.
func CountBounded(text string, maxChars int) int {
	if len(text) == 0 {
		return 0
	}
	cap := maxChars
	if cap <= 0 {
		return max(1, int(math.Ceil(float64(len(text))*0.3)))
	}
	if len(text) <= cap {
		return Count(text)
	}
	headChars := (cap + 1) / 2 // ceil
	tailChars := cap / 2       // floor
	head := text[:headChars]
	tail := ""
	if tailChars > 0 {
		tail = text[len(text)-tailChars:]
	}
	sampleChars := len(head) + len(tail)
	sampleTokens := Count(head) + Count(tail)
	ratio := 0.3
	if sampleChars > 0 {
		ratio = float64(sampleTokens) / float64(sampleChars)
	}
	return max(1, int(math.Ceil(float64(len(text))*ratio)))
}

// escapeRegexString escapes special regex characters in s for use in regexp2.
func escapeRegexString(s string) string {
	var b strings.Builder
	b.Grow(len(s) * 2)
	for _, c := range s {
		switch c {
		case '.', '*', '+', '?', '^', '$', '{', '}', '(', ')', '|', '[', ']', '\\':
			b.WriteRune('\\')
			b.WriteRune(c)
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}
