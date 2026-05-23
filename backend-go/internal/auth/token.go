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
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/pbkdf2"
)

type Claims struct {
	UserID string `json:"uid"`
	Role   string `json:"role"`
	Type   string `json:"typ"`
	// Family groups access + refresh tokens that share a single login
	// session. Refresh rotation reuses the family; reuse-detection blacklists
	// it. Empty for legacy tokens issued before this field existed —
	// blacklist behavior degrades gracefully (per-JTI only) in that case.
	Family string `json:"fam,omitempty"`
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

// IssuePair issues an access+refresh token pair for a NEW session. The pair
// shares a freshly-generated family ID so refresh rotation can chain new
// pairs to the same family, and reuse-detection can revoke them all at once.
// Both JTIs + the family are returned alongside the tokens so the caller can
// record the session row.
func (m *Manager) IssuePair(userID, role string) (map[string]interface{}, error) {
	return m.issuePair(userID, role, uuid.NewString())
}

// IssuePairForRefresh issues a new access+refresh pair inheriting the family
// of the refresh token being rotated. Called from handleRefresh after the
// presented refresh token has been validated and its old JTI blacklisted.
func (m *Manager) IssuePairForRefresh(userID, role, family string) (map[string]interface{}, error) {
	if family == "" {
		// Defensive: a legacy refresh token might not carry a family. Mint
		// a new one so the rotated session still gets reuse-detection.
		family = uuid.NewString()
	}
	return m.issuePair(userID, role, family)
}

func (m *Manager) issuePair(userID, role, family string) (map[string]interface{}, error) {
	accessJTI := uuid.NewString()
	refreshJTI := uuid.NewString()
	access, err := m.sign(userID, role, "access", m.accessTTL, accessJTI, family)
	if err != nil {
		return nil, err
	}
	refresh, err := m.sign(userID, role, "refresh", m.refreshTTL, refreshJTI, family)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"access_token":      access,
		"refresh_token":     refresh,
		"access_jti":        accessJTI,
		"refresh_jti":       refreshJTI,
		"family":            family,
		"access_expires_at": time.Now().UTC().Add(m.accessTTL),
		"token_type":        "Bearer",
		"user_id":           userID,
		"role":              role,
	}, nil
}

func (m *Manager) sign(userID, role, tokenType string, ttl time.Duration, jti, family string) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		UserID: userID,
		Role:   role,
		Type:   tokenType,
		Family: family,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Subject:   userID,
			ID:        jti, // serialises as the standard "jti" claim
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

func (m *Manager) Parse(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Pin to HS256 explicitly. jwt/v5 already rejects "none" but defence
		// in depth: rule out any caller-chosen algorithm switch (RS*/ES*
		// would interpret our HMAC secret as a public key).
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
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
		// Prefer JTI-keyed blacklist (smaller keys, set by
		// handleRevokeMySession). Fall back to the legacy full-token key
		// for tokens issued before this migration so existing sessions
		// stay revocable. Family-keyed blacklist kills every token in a
		// session at once (used by refresh-reuse detection + logout).
		ctx := context.Background()
		if claims.Family != "" {
			exists, err := m.redis.Exists(ctx, "bl:family:"+claims.Family).Result()
			if err == nil && exists > 0 {
				return nil, fmt.Errorf("token revoked")
			}
		}
		if claims.ID != "" {
			exists, err := m.redis.Exists(ctx, "bl:"+claims.ID).Result()
			if err == nil && exists > 0 {
				return nil, fmt.Errorf("token revoked")
			}
		}
		exists, err := m.redis.Exists(ctx, "bl:"+tokenString).Result()
		if err == nil && exists > 0 {
			return nil, fmt.Errorf("token revoked")
		}
	}
	return claims, nil
}

// RevokeFamily blacklists an entire session family. Used by refresh-reuse
// detection (a revoked refresh JTI being re-presented) and by full-logout to
// kill the access token, its refresh token, and any other pairs that share
// the same family in one shot.
func (m *Manager) RevokeFamily(family string) {
	if m.redis == nil || family == "" {
		return
	}
	_ = m.redis.SetEx(context.Background(), "bl:family:"+family, "1", m.refreshTTL).Err()
}

// parseUnverified extracts claims without checking the signature OR the
// revocation list. Used internally by Revoke so an already-revoked token can
// still be re-revoked (Parse would refuse to return claims for one).
// Never use for authentication — signature is unverified.
func (m *Manager) parseUnverified(tokenString string) (*Claims, error) {
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	token, _, err := parser.ParseUnverified(tokenString, &Claims{})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

// ParseUnverifiedForReuse exposes parseUnverified for the refresh-reuse
// detection path in the handlers. Returns claims even when the token has
// been blacklisted (Parse rejects those, but for reuse-detection we
// specifically need to read the JTI/Family of an already-revoked token).
// CALLER MUST NOT TRUST CLAIMS FOR AUTHENTICATION — they are unsigned.
func (m *Manager) ParseUnverifiedForReuse(tokenString string) (*Claims, error) {
	return m.parseUnverified(tokenString)
}

// Revoke blacklists a token by its JTI claim when present, falling back to
// the full-token key. Logout calls this with the access token string —
// either path makes the token immediately invalid for the remainder of its
// natural TTL. Uses parseUnverified internally so the call still works for
// tokens that were already revoked (Parse would reject those).
func (m *Manager) Revoke(token string) {
	if m.redis == nil || token == "" {
		return
	}
	ctx := context.Background()
	claims, err := m.parseUnverified(token)
	if err == nil && claims != nil && claims.ID != "" {
		_ = m.redis.SetEx(ctx, "bl:"+claims.ID, "1", m.refreshTTL).Err()
		return
	}
	_ = m.redis.SetEx(ctx, "bl:"+token, "1", m.refreshTTL).Err()
}

// RevokeJTI blacklists a JTI directly, useful when the caller already has
// the claims (e.g. refresh-token rotation) and doesn't want to re-parse.
func (m *Manager) RevokeJTI(jti string) {
	if m.redis == nil || jti == "" {
		return
	}
	_ = m.redis.SetEx(context.Background(), "bl:"+jti, "1", m.refreshTTL).Err()
}

// IsJTIRevoked reports whether a JTI has been blacklisted. Used by refresh-
// reuse detection: if a refresh token presents a JTI that's already in the
// blacklist, that's strong evidence the token was stolen and replayed.
func (m *Manager) IsJTIRevoked(jti string) bool {
	if m.redis == nil || jti == "" {
		return false
	}
	exists, err := m.redis.Exists(context.Background(), "bl:"+jti).Result()
	return err == nil && exists > 0
}

// BcryptCost is the work factor used for new password hashes. Raised from
// the bcrypt default of 10 to 12 — at 2026 hardware, cost 10 falls to a single
// consumer GPU in hours; cost 12 buys roughly 4x the work per attempt. Tuned
// to keep login latency under ~250ms on commodity server CPUs.
const BcryptCost = 12

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
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

// NeedsRehash returns true when the stored hash should be upgraded on the
// next successful login. Triggers for: legacy PBKDF2 hashes, AND bcrypt
// hashes whose cost is below our current target. Cost extraction can fail
// (malformed hash) — we treat that as "leave it alone" since a rehash
// against a hash we can't parse would not be safe.
func NeedsRehash(hash string) bool {
	if strings.HasPrefix(hash, "pbkdf2$") {
		return true
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		return false
	}
	return cost < BcryptCost
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
