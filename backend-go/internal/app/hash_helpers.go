package app

import (
	"crypto/sha256"
	"hash"
)

// newSHA256Hasher returns a fresh SHA-256 hasher. Wrapped so handlers don't
// repeatedly import crypto/sha256 just to construct one.
func newSHA256Hasher() hash.Hash {
	return sha256.New()
}
