package tokenizer

// bpeEncode greedily merges the lowest-rank adjacent pair until no merge
// applies. `piece` is already byte-level encoded. Returns the final pieces.
// A piece of length 0 returns nil; length 1 returns []string{piece}.
func bpeEncode(piece string, mergeRank map[string]int) []string {
	if len(piece) == 0 {
		return nil
	}
	// Split into individual runes.
	word := make([]string, 0, len(piece)/2)
	for _, r := range piece {
		word = append(word, string(r))
	}
	if len(word) <= 1 {
		if len(word) == 0 {
			return nil
		}
		return word
	}
	for {
		bestIdx := -1
		bestRank := 1<<31 - 1 // MaxInt
		for i := 0; i < len(word)-1; i++ {
			pair := word[i] + " " + word[i+1]
			rank, ok := mergeRank[pair]
			if ok && rank < bestRank {
				bestRank = rank
				bestIdx = i
				if rank == 0 {
					break // 0 is already the best possible
				}
			}
		}
		if bestIdx < 0 {
			break
		}
		// Merge the pair.
		merged := word[bestIdx] + word[bestIdx+1]
		newWord := make([]string, 0, len(word)-1)
		newWord = append(newWord, word[:bestIdx]...)
		newWord = append(newWord, merged)
		newWord = append(newWord, word[bestIdx+2:]...)
		word = newWord
		if len(word) == 1 {
			break
		}
	}
	return word
}
