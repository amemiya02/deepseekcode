package tokenizer

import (
	"hash/fnv"
	"sync"
)

const runCacheMax = 4096

var (
	cacheMu     sync.Mutex
	runCache    = make(map[uint64]int, runCacheMax)
	cacheOrder  = make([]uint64, 0, runCacheMax)
	cacheHits   int
	cacheMisses int
)

// hashRun returns a 64-bit FNV-1a hash of run.
func hashRun(run string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(run)) //nolint:errcheck
	return h.Sum64()
}

// countRun returns the token count of a NON-added run, identical to counting
// that run inside Encode (splitRegexes → byteLevelEncode → bpeEncode → vocab
// hits). Memoized by a 64-bit FNV-1a hash of run. Safe for concurrent use.
func (l *loaded) countRun(run string) int {
	h := hashRun(run)

	cacheMu.Lock()
	if cnt, ok := runCache[h]; ok {
		cacheHits++
		cacheMu.Unlock()
		return cnt
	}
	cacheMisses++
	cacheMu.Unlock()

	// Compute count using the same pipeline as Encode.
	cnt := 0
	chunks := []string{run}
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
			if _, ok := l.vocab[p]; ok {
				cnt++
			}
		}
	}

	cacheMu.Lock()
	// Store; if already present (rare race), keep existing.
	if _, ok := runCache[h]; !ok {
		runCache[h] = cnt
		cacheOrder = append(cacheOrder, h)
		for len(cacheOrder) > runCacheMax {
			evict := cacheOrder[0]
			cacheOrder = cacheOrder[1:]
			delete(runCache, evict)
		}
	}
	cacheMu.Unlock()
	return cnt
}

// CountExact returns the exact token count of text — byte-identical to
// len(Encode(text)). Returns 0 for "" or when the tokenizer is unavailable.
func CountExact(text string) int {
	if text == "" {
		return 0
	}
	l, err := load()
	if err != nil {
		return 0
	}
	segs, isAdded := l.splitAddedTokens(text)
	total := 0
	for i, s := range segs {
		if isAdded[i] {
			total++
		} else {
			total += l.countRun(s)
		}
	}
	return total
}

// cacheStatsForTest reports run-cache (hits, misses, entries). TEST-ONLY.
func cacheStatsForTest() (hits, misses, entries int) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	return cacheHits, cacheMisses, len(runCache)
}

// resetCacheForTest clears the run cache and zeroes its stats. TEST-ONLY.
func resetCacheForTest() {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	runCache = make(map[uint64]int, runCacheMax)
	cacheOrder = cacheOrder[:0]
	cacheHits = 0
	cacheMisses = 0
}
