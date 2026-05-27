package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Hash returns the SHA-256 hex of a string.
func Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// Normalize lowercases and trims a string.
func Normalize(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
}

// Chunk splits a string into fixed-size pieces.
func Chunk(s string, size int) []string {
	if size <= 0 {
		return []string{s}
	}
	var chunks []string
	for len(s) > size {
		chunks = append(chunks, s[:size])
		s = s[size:]
	}
	if s != "" {
		chunks = append(chunks, s)
	}
	return chunks
}
