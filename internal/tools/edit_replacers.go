package tools

import (
	"regexp"
	"strings"
)

// This file ports opencode's multi-strategy edit replacer cascade
// (packages/opencode/src/tool/edit.ts, the Replacer generators + replace())
// into Go. The goal: tolerate the realistic whitespace / indentation drift a
// model introduces when it copies an old_string snippet, WITHOUT ever editing
// the wrong region. The cascade tries strategies from strictest to loosest;
// the first strategy that yields a UNIQUE match wins. A non-unique candidate
// never fails immediately — a later, looser strategy may still resolve to a
// unique match (ambiguity carry). Only after the whole cascade is exhausted do
// we distinguish "not found" from "matched multiple locations".
//
// BYTE FIDELITY (the #1 porting trap): every strategy yields ORIGINAL content
// substrings — bytes that already exist verbatim in `content`, computed by
// byte-offset arithmetic over the original lines (never the normalized form).
// applyReplace then splices via strings.Index, so the bytes outside the
// matched region are preserved exactly. A strategy must NEVER yield a
// reconstructed/normalized string, or the splice would corrupt the file.

// replacer yields candidate substrings that exist verbatim in content. The
// cascade re-verifies each candidate via strings.Index before splicing.
type replacer func(content, find string) []string

// wsRun collapses any run of whitespace to a single space (mirrors opencode's
// /\s+/g). Precompiled package-level and read-only, so it is race-free.
var wsRun = regexp.MustCompile(`\s+`)

// exactReplacer mirrors SimpleReplacer: the find string verbatim.
func exactReplacer(_, find string) []string {
	return []string{find}
}

// lineTrimmedReplacer mirrors LineTrimmedReplacer. It splits content and find
// on "\n", drops a trailing empty find line, then slides a window of
// len(searchLines); a window matches when every line's TrimSpace is equal. For
// each match it yields the EXACT original substring spanning that window,
// computed by cumulative byte offsets (sum of line lengths + 1 per newline) so
// the yielded bytes are original, not the trimmed form.
func lineTrimmedReplacer(content, find string) []string {
	originalLines := strings.Split(content, "\n")
	searchLines := strings.Split(find, "\n")
	if n := len(searchLines); n > 0 && searchLines[n-1] == "" {
		searchLines = searchLines[:n-1]
	}
	if len(searchLines) == 0 {
		return nil
	}

	var out []string
	for i := 0; i <= len(originalLines)-len(searchLines); i++ {
		matches := true
		for j := 0; j < len(searchLines); j++ {
			if strings.TrimSpace(originalLines[i+j]) != strings.TrimSpace(searchLines[j]) {
				matches = false
				break
			}
		}
		if !matches {
			continue
		}
		matchStartIndex := 0
		for k := 0; k < i; k++ {
			matchStartIndex += len(originalLines[k]) + 1
		}
		matchEndIndex := matchStartIndex
		for k := 0; k < len(searchLines); k++ {
			matchEndIndex += len(originalLines[i+k])
			if k < len(searchLines)-1 {
				matchEndIndex++ // newline between lines, but not after the last
			}
		}
		out = append(out, content[matchStartIndex:matchEndIndex])
	}
	return out
}

// whitespaceNormalizedReplacer mirrors WhitespaceNormalizedReplacer minus the
// risky regex-word substring sub-branch (the rarest, most error-prone path in
// opencode — intentionally omitted per spec). Single-line: yield the original
// line when its normalized form equals the normalized find. Multi-line (find
// spans >1 line): slide a window of len(findLines) and yield the original
// block (Join of the original line slice) when normalized(block)==normalized(find).
func whitespaceNormalizedReplacer(content, find string) []string {
	normalizedFind := normalizeWS(find)

	var out []string
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if normalizeWS(line) == normalizedFind {
			out = append(out, line)
		}
	}

	findLines := strings.Split(find, "\n")
	if len(findLines) > 1 {
		for i := 0; i <= len(lines)-len(findLines); i++ {
			block := strings.Join(lines[i:i+len(findLines)], "\n")
			if normalizeWS(block) == normalizedFind {
				out = append(out, block)
			}
		}
	}
	return out
}

func normalizeWS(text string) string {
	return strings.TrimSpace(wsRun.ReplaceAllString(text, " "))
}

// indentationFlexibleReplacer mirrors IndentationFlexibleReplacer. It strips
// the common minimum leading whitespace off every non-empty line, then slides
// a window of len(findLines); a block matches when its de-indented form equals
// the de-indented find. Yields the ORIGINAL block (Join of original lines).
func indentationFlexibleReplacer(content, find string) []string {
	normalizedFind := removeCommonIndent(find)
	contentLines := strings.Split(content, "\n")
	findLines := strings.Split(find, "\n")

	var out []string
	for i := 0; i <= len(contentLines)-len(findLines); i++ {
		block := strings.Join(contentLines[i:i+len(findLines)], "\n")
		if removeCommonIndent(block) == normalizedFind {
			out = append(out, block)
		}
	}
	return out
}

// removeCommonIndent removes the minimum leading-whitespace prefix (over
// non-empty lines) from every non-empty line. Leading whitespace here is ASCII
// space/tab, so the byte length of the prefix equals its character count; we
// compute the slice boundary from the matched prefix length to be safe with
// any byte/rune ambiguity.
func removeCommonIndent(text string) string {
	lines := strings.Split(text, "\n")
	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := leadingWhitespaceLen(line)
		if minIndent < 0 || indent < minIndent {
			minIndent = indent
		}
	}
	if minIndent <= 0 {
		// No non-empty lines, or no common indent to strip.
		return text
	}
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Each non-empty line has at least minIndent leading-ws bytes by
		// construction, and that prefix is ASCII, so slicing by byte index is
		// safe and equals slicing by the leading-ws character count.
		lines[i] = line[minIndent:]
	}
	return strings.Join(lines, "\n")
}

// leadingWhitespaceLen returns the byte length of the leading run of ASCII
// whitespace (the same set the indentation strip treats as indent).
func leadingWhitespaceLen(s string) int {
	n := 0
	for n < len(s) {
		c := s[n]
		if c == ' ' || c == '\t' || c == '\r' || c == '\f' || c == '\v' {
			n++
			continue
		}
		break
	}
	return n
}

// Similarity thresholds for block-anchor matching, mirroring opencode.
const (
	singleCandidateSimilarityThreshold   = 0.0
	multipleCandidateSimilarityThreshold = 0.3
)

// blockAnchorReplacer mirrors BlockAnchorReplacer. It requires >=3 search
// lines, anchors on the trimmed first and last lines, and collects candidate
// (start,end) pairs where the first anchor matches at i and the last anchor
// matches at the first j >= i+2. A single candidate is accepted at the relaxed
// 0.0 threshold; with multiple candidates it picks the one with the highest
// average middle-line char-level Levenshtein similarity and accepts only if
// that similarity >= 0.3. Yields the ORIGINAL substring for the chosen block.
func blockAnchorReplacer(content, find string) []string {
	originalLines := strings.Split(content, "\n")
	searchLines := strings.Split(find, "\n")
	if len(searchLines) < 3 {
		return nil
	}
	if n := len(searchLines); n > 0 && searchLines[n-1] == "" {
		searchLines = searchLines[:n-1]
	}
	if len(searchLines) < 3 {
		// After popping the trailing empty line we may drop below 3; opencode
		// only guards before the pop, but a sub-3 block has no middle line and
		// the anchors would overlap, so bail out to avoid bogus matches.
		return nil
	}

	firstLineSearch := strings.TrimSpace(searchLines[0])
	lastLineSearch := strings.TrimSpace(searchLines[len(searchLines)-1])
	searchBlockSize := len(searchLines)

	type candidate struct{ startLine, endLine int }
	var candidates []candidate
	for i := 0; i < len(originalLines); i++ {
		if strings.TrimSpace(originalLines[i]) != firstLineSearch {
			continue
		}
		for j := i + 2; j < len(originalLines); j++ {
			if strings.TrimSpace(originalLines[j]) == lastLineSearch {
				candidates = append(candidates, candidate{startLine: i, endLine: j})
				break // only the first occurrence of the last line
			}
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	yieldBlock := func(c candidate) string {
		matchStartIndex := 0
		for k := 0; k < c.startLine; k++ {
			matchStartIndex += len(originalLines[k]) + 1
		}
		matchEndIndex := matchStartIndex
		for k := c.startLine; k <= c.endLine; k++ {
			matchEndIndex += len(originalLines[k])
			if k < c.endLine {
				matchEndIndex++
			}
		}
		return content[matchStartIndex:matchEndIndex]
	}

	if len(candidates) == 1 {
		c := candidates[0]
		actualBlockSize := c.endLine - c.startLine + 1
		similarity := 0.0
		linesToCheck := min2(searchBlockSize-2, actualBlockSize-2)
		if linesToCheck > 0 {
			for j := 1; j < searchBlockSize-1 && j < actualBlockSize-1; j++ {
				originalLine := strings.TrimSpace(originalLines[c.startLine+j])
				searchLine := strings.TrimSpace(searchLines[j])
				maxLen := maxInt(len([]rune(originalLine)), len([]rune(searchLine)))
				if maxLen == 0 {
					continue
				}
				distance := levenshteinChars(originalLine, searchLine)
				similarity += (1 - float64(distance)/float64(maxLen)) / float64(linesToCheck)
				if similarity >= singleCandidateSimilarityThreshold {
					break
				}
			}
		} else {
			similarity = 1.0
		}
		if similarity >= singleCandidateSimilarityThreshold {
			return []string{yieldBlock(c)}
		}
		return nil
	}

	var best *candidate
	maxSimilarity := -1.0
	for idx := range candidates {
		c := candidates[idx]
		actualBlockSize := c.endLine - c.startLine + 1
		similarity := 0.0
		linesToCheck := min2(searchBlockSize-2, actualBlockSize-2)
		if linesToCheck > 0 {
			for j := 1; j < searchBlockSize-1 && j < actualBlockSize-1; j++ {
				originalLine := strings.TrimSpace(originalLines[c.startLine+j])
				searchLine := strings.TrimSpace(searchLines[j])
				maxLen := maxInt(len([]rune(originalLine)), len([]rune(searchLine)))
				if maxLen == 0 {
					continue
				}
				distance := levenshteinChars(originalLine, searchLine)
				similarity += 1 - float64(distance)/float64(maxLen)
			}
			similarity /= float64(linesToCheck)
		} else {
			similarity = 1.0
		}
		if similarity > maxSimilarity {
			maxSimilarity = similarity
			c := c
			best = &c
		}
	}
	if maxSimilarity >= multipleCandidateSimilarityThreshold && best != nil {
		return []string{yieldBlock(*best)}
	}
	return nil
}

// levenshteinChars is a char-level (rune-level) edit distance, mirroring
// opencode's per-middle-line levenshtein(). It is intentionally separate from
// LevenshteinLines (which is line-level / token-level) because block-anchor
// similarity compares individual lines character by character.
func levenshteinChars(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}
	// Two-row DP, O(min) memory; keep ra as the shorter dimension.
	if len(rb) < len(ra) {
		ra, rb = rb, ra
	}
	prev := make([]int, len(ra)+1)
	curr := make([]int, len(ra)+1)
	for i := 0; i <= len(ra); i++ {
		prev[i] = i
	}
	for j := 1; j <= len(rb); j++ {
		curr[0] = j
		for i := 1; i <= len(ra); i++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[i] = min3(prev[i]+1, curr[i-1]+1, prev[i-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(ra)]
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// cascade is the fixed strategy order. exact first means a unique exact match
// always wins; block-anchor last because it is the loosest. NOTE: this order
// (exact, line-trimmed, whitespace, indentation, block-anchor) follows the
// T2.2 spec, which differs slightly from opencode's runtime order — opencode
// runs BlockAnchor before Whitespace/Indentation; the spec deliberately puts
// the two normalizing strategies ahead of the anchor heuristic so a precise
// whitespace/indentation match is preferred over the fuzzier anchor match.
var cascade = []replacer{
	exactReplacer,
	lineTrimmedReplacer,
	whitespaceNormalizedReplacer,
	indentationFlexibleReplacer,
	blockAnchorReplacer,
}

// applyReplace runs the cascade and returns the spliced result.
//
//   - matched=true: a unique candidate (or, with replaceAll, any candidate) was
//     found and spliced; updated holds the new content.
//   - matched=false, ambiguous=true: at least one strategy yielded a candidate
//     present in content but it matched multiple locations, and no strategy
//     resolved to a unique match. The caller should ask for more context.
//   - matched=false, ambiguous=false: no strategy yielded any candidate that
//     exists in content (genuine not-found).
//
// Ambiguity carry: a non-unique candidate does NOT short-circuit; a later,
// looser strategy may still produce a unique match. This mirrors opencode's
// replace(): notFound tracks "any candidate existed", and only after the whole
// cascade do we decide not-found vs multiple-match.
func applyReplace(content, old, newStr string, replaceAll bool) (updated string, matched, ambiguous bool) {
	sawAmbiguous := false
	for _, r := range cascade {
		for _, s := range r(content, old) {
			idx := strings.Index(content, s)
			if idx < 0 {
				continue
			}
			if replaceAll {
				return strings.ReplaceAll(content, s, newStr), true, false
			}
			if idx == strings.LastIndex(content, s) {
				return content[:idx] + newStr + content[idx+len(s):], true, false
			}
			// Candidate exists but is not unique; remember and keep scanning.
			sawAmbiguous = true
		}
	}
	return "", false, sawAmbiguous
}
