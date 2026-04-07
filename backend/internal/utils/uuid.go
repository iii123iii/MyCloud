package utils

import "github.com/google/uuid"

// GenerateUUID returns a new random UUID string.
func GenerateUUID() string {
	return uuid.New().String()
}
