package memory

import (
	"crypto/sha256"
	"encoding/hex"
)

// ContentSHA returns the hex-encoded SHA-256 of content.
func ContentSHA(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}
