package storage

import (
	"crypto/rand"
	"crypto/sha1" //nolint:gosec
	"fmt"
	"regexp"
)

const (
	// SHA1Short is the short display length used in CLI output.
	SHA1Short = 7
	// SHA1MinLen is the minimum prefix length considered for ID matching.
	SHA1MinLen = 4
	// SHA1ReadBlockSize is the size of random bytes mixed into ID generation.
	SHA1ReadBlockSize = 4096
)

// SHA1Regexp matches a full 40-char SHA-1 hex string.
var SHA1Regexp = regexp.MustCompile(`\b[0-9a-f]{40}\b`)

// NewConversationID generates a stable-looking ID for conversation records.
//
// This is not used for cryptographic security; it is used as an identifier.
func NewConversationID() string {
	b := make([]byte, SHA1ReadBlockSize)
	_, _ = rand.Read(b)
	//nolint:gosec // identifier generation; not used for cryptographic security.
	return fmt.Sprintf("%x", sha1.Sum(b))
}
