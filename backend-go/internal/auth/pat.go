package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

// PATPrefix marks a token as a Personal Access Token so the auth middleware
// can route it to the PAT lookup path instead of JWT parsing. Chosen to be
// human-recognisable in logs/CI configs and unambiguous against base64url
// JWTs (which never start with "mc_pat_").
const PATPrefix = "mc_pat_"

// patRandomBytes is the entropy of a token's random segment. 32 bytes = 256
// bits — far beyond brute-force reach, which is what lets us store a plain
// SHA-256 (no per-hash salt / slow KDF needed for a high-entropy secret).
const patRandomBytes = 32

// GeneratePATToken mints a new token. It returns the full plaintext token
// (shown to the user exactly once), a short display prefix, and the last four
// characters — the prefix+last_four pair lets the UI render "mc_pat_ab12…wxyz"
// without ever persisting the secret. The token itself is never stored; only
// HashPATToken(full) is.
func GeneratePATToken() (full, prefix, lastFour string, err error) {
	buf := make([]byte, patRandomBytes)
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", err
	}
	// RawURLEncoding: compact, header-safe (no '+'/'/'/'=' that need escaping).
	full = PATPrefix + base64.RawURLEncoding.EncodeToString(buf)
	// Prefix is the marker plus the first 6 chars of the random segment —
	// enough to disambiguate tokens in a list without leaking the secret.
	prefix = full
	if len(full) > len(PATPrefix)+6 {
		prefix = full[:len(PATPrefix)+6]
	}
	lastFour = full[len(full)-4:]
	return full, prefix, lastFour, nil
}

// HashPATToken returns the hex SHA-256 of a token. Used both when persisting a
// freshly-minted token and when looking one up on each authenticated request:
// hash the presented bearer, then SELECT … WHERE token_hash=?. Comparing by a
// stored hash (rather than a constant-time compare of a fetched secret) is safe
// here precisely because the token is high-entropy random — an attacker cannot
// produce a value whose SHA-256 collides with a row they haven't already seen.
func HashPATToken(full string) string {
	sum := sha256.Sum256([]byte(full))
	return hex.EncodeToString(sum[:])
}

// IsPATToken reports whether a bearer value is a Personal Access Token.
func IsPATToken(token string) bool {
	return strings.HasPrefix(token, PATPrefix)
}
