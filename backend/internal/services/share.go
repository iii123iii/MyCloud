package services

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/iii123iii/mycloud/backend/internal/utils"
)

// GenerateShareToken returns a cryptographically random 32-byte hex token.
func GenerateShareToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashSharePassword hashes a share password using PBKDF2-HMAC-SHA256,
// identical to the regular user password hashing for consistency with C++ backend.
func HashSharePassword(password string) (string, error) {
	return utils.HashPassword(password)
}

// VerifySharePassword reports whether password matches the stored PBKDF2 hash.
func VerifySharePassword(password, hash string) bool {
	return utils.VerifyPassword(password, hash)
}
