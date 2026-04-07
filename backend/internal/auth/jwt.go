package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims holds the data extracted from a validated JWT.
type Claims struct {
	UserID string
	Role   string
	Type   string // "access" or "refresh"
}

// CreateAccessToken creates a 15-minute access JWT.
func CreateAccessToken(userID, role, secret string) (string, error) {
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  userID,
		"role": role,
		"type": "access",
		"iat":  now.Unix(),
		"exp":  now.Add(15 * time.Minute).Unix(),
	})
	return tok.SignedString([]byte(secret))
}

// CreateRefreshToken creates a 7-day refresh JWT.
func CreateRefreshToken(userID, secret string) (string, error) {
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  userID,
		"type": "refresh",
		"iat":  now.Unix(),
		"exp":  now.Add(7 * 24 * time.Hour).Unix(),
	})
	return tok.SignedString([]byte(secret))
}

// VerifyToken parses and validates a JWT, returning its Claims.
func VerifyToken(tokenString, secret string) (*Claims, error) {
	tok, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	mc, ok := tok.Claims.(jwt.MapClaims)
	if !ok || !tok.Valid {
		return nil, errors.New("invalid token claims")
	}
	sub, _ := mc["sub"].(string)
	role, _ := mc["role"].(string)
	typ, _ := mc["type"].(string)
	return &Claims{UserID: sub, Role: role, Type: typ}, nil
}
