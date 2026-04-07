package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/pbkdf2"
)

type Claims struct {
	UserID string `json:"uid"`
	Role   string `json:"role"`
	Type   string `json:"typ"`
	jwt.RegisteredClaims
}

type Manager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	redis      *redis.Client
}

func NewManager(secret string, accessTTLSeconds, refreshTTLSeconds int, redis *redis.Client) *Manager {
	return &Manager{
		secret:     []byte(secret),
		accessTTL:  time.Duration(accessTTLSeconds) * time.Second,
		refreshTTL: time.Duration(refreshTTLSeconds) * time.Second,
		redis:      redis,
	}
}

func (m *Manager) IssuePair(userID, role string) (map[string]interface{}, error) {
	access, err := m.sign(userID, role, "access", m.accessTTL)
	if err != nil {
		return nil, err
	}
	refresh, err := m.sign(userID, role, "refresh", m.refreshTTL)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"access_token":  access,
		"refresh_token": refresh,
		"token_type":    "Bearer",
		"user_id":       userID,
		"role":          role,
	}, nil
}

func (m *Manager) sign(userID, role, tokenType string, ttl time.Duration) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		UserID: userID,
		Role:   role,
		Type:   tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Subject:   userID,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

func (m *Manager) Parse(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	if m.redis != nil {
		exists, err := m.redis.Exists(context.Background(), "bl:"+tokenString).Result()
		if err == nil && exists > 0 {
			return nil, fmt.Errorf("token revoked")
		}
	}
	return claims, nil
}

func (m *Manager) Revoke(token string) {
	if m.redis == nil || token == "" {
		return
	}
	_ = m.redis.SetEx(context.Background(), "bl:"+token, "1", m.refreshTTL).Err()
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func ComparePassword(hash, password string) error {
	if strings.HasPrefix(hash, "pbkdf2$") {
		return comparePBKDF2(hash, password)
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func NeedsRehash(hash string) bool {
	return strings.HasPrefix(hash, "pbkdf2$")
}

func comparePBKDF2(stored, password string) error {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 {
		return fmt.Errorf("invalid pbkdf2 hash format")
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("invalid pbkdf2 iteration count")
	}
	salt, err := hex.DecodeString(parts[2])
	if err != nil {
		return fmt.Errorf("invalid pbkdf2 salt")
	}
	expected, err := hex.DecodeString(parts[3])
	if err != nil {
		return fmt.Errorf("invalid pbkdf2 digest")
	}
	actual := pbkdf2.Key([]byte(password), salt, iter, len(expected), sha256.New)
	if subtle.ConstantTimeCompare(actual, expected) != 1 {
		return bcrypt.ErrMismatchedHashAndPassword
	}
	return nil
}
