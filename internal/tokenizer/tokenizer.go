// Package tokenizer provides a pure-Go DeepSeek V4 BPE tokenizer for exact
// token counting. It is counting-only — never used on the wire or for
// cache-fingerprint computation.
//
// The embedded vocabulary is the official DeepSeek V4 tokenizer.json, gzipped.
// It is decompressed and parsed lazily on first use via sync.Once.
package tokenizer

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"io"
	"sort"
	"sync"

	"github.com/dlclark/regexp2"
)

//go:embed data/deepseek-tokenizer.json.gz
var gzVocab []byte

// tokenizerData mirrors the relevant subset of HF tokenizer.json.
type tokenizerData struct {
	AddedTokens []struct {
		ID      int    `json:"id"`
		Content string `json:"content"`
		Special bool   `json:"special"`
	} `json:"added_tokens"`
	PreTokenizer struct {
		Pretokenizers []struct {
			Type    string `json:"type"` // "Split" | "ByteLevel"
			Pattern struct {
				Regex string `json:"Regex"`
			} `json:"pattern"`
			Behavior string `json:"behavior"`
		} `json:"pretokenizers"`
	} `json:"pre_tokenizer"`
	Model struct {
		Type   string         `json:"type"` // "BPE"
		Vocab  map[string]int `json:"vocab"`
		Merges []string       `json:"merges"`
	} `json:"model"`
}

// loaded is the cached, ready-to-use tokenizer state.
type loaded struct {
	vocab        map[string]int
	mergeRank    map[string]int    // "a b" -> rank (index in merges)
	data         *tokenizerData    // retained for pretokenizer/added-token setup in later tasks
	byteToChar   [256]rune         // GPT-2 byte→rune map
	splitRegexes []*regexp2.Regexp // the 3 "Split" rules, in order
	addedMap     map[string]int    // non-special added token -> id
	addedPattern *regexp2.Regexp   // alternation of non-special added tokens, longest-first (nil if none)
}

var (
	loadOnce sync.Once
	loadErr  error
	loadedTz *loaded
)

// load lazily decompresses+parses the embedded vocab exactly once.
// Returns the same (*loaded, error) on every call (sync.Once).
func load() (*loaded, error) {
	loadOnce.Do(func() {
		gz, err := gzip.NewReader(bytes.NewReader(gzVocab))
		if err != nil {
			loadErr = err
			return
		}
		raw, err := io.ReadAll(gz)
		gz.Close()
		if err != nil {
			loadErr = err
			return
		}
		var data tokenizerData
		if err := json.Unmarshal(raw, &data); err != nil {
			loadErr = err
			return
		}
		if len(data.Model.Vocab) == 0 {
			loadErr = errEmptyVocab
			return
		}
		if len(data.Model.Merges) == 0 {
			loadErr = errEmptyMerges
			return
		}
		mergeRank := make(map[string]int, len(data.Model.Merges))
		for i, m := range data.Model.Merges {
			mergeRank[m] = i
		}

		// Build split regexes from pre_tokenizer Split rules.
		var splitRegexes []*regexp2.Regexp
		for _, p := range data.PreTokenizer.Pretokenizers {
			if p.Type == "Split" {
				re, err := regexp2.Compile(p.Pattern.Regex, regexp2.Unicode)
				if err != nil {
					loadErr = err
					return
				}
				splitRegexes = append(splitRegexes, re)
			}
		}

		// Build added-token map and pattern (non-special only).
		addedMap := make(map[string]int)
		var addedContents []string
		for _, t := range data.AddedTokens {
			if !t.Special {
				addedMap[t.Content] = t.ID
				addedContents = append(addedContents, t.Content)
			}
		}
		// Longest-first ensures greedy matching doesn't lose a longer token.
		sort.Slice(addedContents, func(i, j int) bool {
			return len(addedContents[i]) > len(addedContents[j])
		})
		var addedPattern *regexp2.Regexp
		if len(addedContents) > 0 {
			var b bytes.Buffer
			for i, c := range addedContents {
				if i > 0 {
					b.WriteByte('|')
				}
				b.WriteString(escapeRegexString(c))
			}
			addedPattern = regexp2.MustCompile(b.String(), regexp2.Unicode)
		}

		loadedTz = &loaded{
			vocab:        data.Model.Vocab,
			mergeRank:    mergeRank,
			data:         &data,
			byteToChar:   buildByteToChar(),
			splitRegexes: splitRegexes,
			addedMap:     addedMap,
			addedPattern: addedPattern,
		}
	})
	return loadedTz, loadErr
}

// Available reports whether the embedded tokenizer loaded successfully.
// Never panics. False means callers should fall back to the heuristic estimator.
func Available() bool {
	_, err := load()
	return err == nil && loadedTz != nil && len(loadedTz.vocab) > 0
}

var (
	errEmptyVocab  = &loadError{"tokenizer: empty vocab in embedded data"}
	errEmptyMerges = &loadError{"tokenizer: empty merges in embedded data"}
)

type loadError struct{ msg string }

func (e *loadError) Error() string { return e.msg }
