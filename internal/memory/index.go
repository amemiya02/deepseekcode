package memory

import (
	"math"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// BM25Index is a simple in-memory BM25 inverted index.
// Parameters: k1=1.5, b=0.75 (standard defaults).
const (
	bm25K1 = 1.5
	bm25B  = 0.75
)

type docEntry struct {
	id     string
	tokens []string
}

// BM25Index holds the inverted index and document store for BM25 ranking.
type BM25Index struct {
	mu      sync.RWMutex
	docs    map[string]docEntry // id → entry
	posting map[string][]string // token → []id
	df      map[string]int      // token → doc freq
	avgLen  float64
}

// NewBM25Index returns an empty BM25Index.
func NewBM25Index() *BM25Index {
	return &BM25Index{
		docs:    make(map[string]docEntry),
		posting: make(map[string][]string),
		df:      make(map[string]int),
	}
}

func tokenize(text string) []string {
	text = strings.ToLower(text)
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return fields
}

// Add indexes a document. Calling Add with an existing ID replaces it.
func (idx *BM25Index) Add(id, content string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Remove old entry if present.
	if old, ok := idx.docs[id]; ok {
		for _, tok := range unique(old.tokens) {
			idx.df[tok]--
			if idx.df[tok] <= 0 {
				delete(idx.df, tok)
			}
			idx.posting[tok] = removeStr(idx.posting[tok], id)
		}
	}

	tokens := tokenize(content)
	idx.docs[id] = docEntry{id: id, tokens: tokens}
	for _, tok := range unique(tokens) {
		idx.df[tok]++
		idx.posting[tok] = append(idx.posting[tok], id)
	}
	idx.recomputeAvgLen()
}

// Remove removes a document from the index.
func (idx *BM25Index) Remove(id string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	old, ok := idx.docs[id]
	if !ok {
		return
	}
	for _, tok := range unique(old.tokens) {
		idx.df[tok]--
		if idx.df[tok] <= 0 {
			delete(idx.df, tok)
		}
		idx.posting[tok] = removeStr(idx.posting[tok], id)
	}
	delete(idx.docs, id)
	idx.recomputeAvgLen()
}

// Search returns up to topN document IDs ranked by BM25 score.
func (idx *BM25Index) Search(query string, topN int) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	qtoks := unique(tokenize(query))
	N := float64(len(idx.docs))
	if N == 0 {
		return nil
	}

	scores := make(map[string]float64)
	for _, tok := range qtoks {
		df := float64(idx.df[tok])
		if df == 0 {
			continue
		}
		idf := math.Log((N-df+0.5)/(df+0.5) + 1)
		for _, id := range idx.posting[tok] {
			doc := idx.docs[id]
			tf := termFreq(tok, doc.tokens)
			dl := float64(len(doc.tokens))
			score := idf * (tf * (bm25K1 + 1)) /
				(tf + bm25K1*(1-bm25B+bm25B*dl/idx.avgLen))
			scores[id] += score
		}
	}

	type kv struct {
		id    string
		score float64
	}
	ranked := make([]kv, 0, len(scores))
	for id, sc := range scores {
		ranked = append(ranked, kv{id, sc})
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })

	if topN > len(ranked) {
		topN = len(ranked)
	}
	out := make([]string, topN)
	for i := range out {
		out[i] = ranked[i].id
	}
	return out
}

func (idx *BM25Index) recomputeAvgLen() {
	if len(idx.docs) == 0 {
		idx.avgLen = 0
		return
	}
	total := 0
	for _, d := range idx.docs {
		total += len(d.tokens)
	}
	idx.avgLen = float64(total) / float64(len(idx.docs))
}

func unique(toks []string) []string {
	seen := make(map[string]bool, len(toks))
	out := toks[:0]
	for _, t := range toks {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

func termFreq(tok string, tokens []string) float64 {
	c := 0
	for _, t := range tokens {
		if t == tok {
			c++
		}
	}
	return float64(c)
}

func removeStr(sl []string, s string) []string {
	out := sl[:0]
	for _, v := range sl {
		if v != s {
			out = append(out, v)
		}
	}
	return out
}
