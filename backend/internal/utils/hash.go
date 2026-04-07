package utils

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/pbkdf2"
	"crypto/sha256"
)

const (
	pbkdf2Iter   = 310000
	pbkdf2KeyLen = 32
	saltLen      = 16
)

// HashPassword hashes a password using PBKDF2-HMAC-SHA256.
// Stored format: "pbkdf2$<iterations>$<salt_hex>$<hash_hex>"
// This is byte-for-byte compatible with the C++ backend's PKCS5_PBKDF2_HMAC format.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("rand salt: %w", err)
	}
	key := pbkdf2.Key([]byte(password), salt, pbkdf2Iter, pbkdf2KeyLen, sha256.New)
	return "pbkdf2$" + strconv.Itoa(pbkdf2Iter) +
		"$" + hex.EncodeToString(salt) +
		"$" + hex.EncodeToString(key), nil
}

// VerifyPassword checks a password against a stored PBKDF2 hash.
// Supports the format produced by both the C++ backend and this Go backend.
func VerifyPassword(password, stored string) bool {
	parts := strings.SplitN(stored, "$", 4)
	if len(parts) != 4 || parts[0] != "pbkdf2" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter <= 0 {
		return false
	}
	saltBytes, err := hex.DecodeString(parts[2])
	if err != nil {
		return false
	}
	storedKey, err := hex.DecodeString(parts[3])
	if err != nil {
		return false
	}
	derived := pbkdf2.Key([]byte(password), saltBytes, iter, len(storedKey), sha256.New)
	// Constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare(derived, storedKey) == 1
}

// HexToBytes decodes a hex string into bytes.
func HexToBytes(s string) ([]byte, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, errors.New("invalid hex string: " + err.Error())
	}
	return b, nil
}

// BytesToHex encodes bytes as a lowercase hex string.
func BytesToHex(b []byte) string {
	return hex.EncodeToString(b)
}
