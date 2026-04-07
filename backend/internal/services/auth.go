package services

import (
	"context"
	"time"

	jwtpkg "github.com/iii123iii/mycloud/backend/internal/auth"
	"github.com/iii123iii/mycloud/backend/internal/redisclient"
)

// AuthService provides JWT issuance, token blacklisting, and rate limiting.
type AuthService struct {
	redis  *redisclient.Client
	secret string
}

func NewAuthService(rdb *redisclient.Client, jwtSecret string) *AuthService {
	return &AuthService{redis: rdb, secret: jwtSecret}
}

// IssueTokenPair mints both an access and a refresh token.
func (s *AuthService) IssueTokenPair(userID, role string) (accessToken, refreshToken string, err error) {
	accessToken, err = jwtpkg.CreateAccessToken(userID, role, s.secret)
	if err != nil {
		return
	}
	refreshToken, err = jwtpkg.CreateRefreshToken(userID, s.secret)
	return
}

// BlacklistToken stores a refresh token in Redis for 7 days.
func (s *AuthService) BlacklistToken(ctx context.Context, token string) {
	_ = s.redis.Setex(ctx, "bl:"+token, 7*24*time.Hour)
}

// IsBlacklisted returns true if the token has been revoked.
func (s *AuthService) IsBlacklisted(ctx context.Context, token string) bool {
	return s.redis.Exists(ctx, "bl:"+token)
}

// CheckRateLimit returns true if the IP is under the login rate limit
// (max 10 attempts per 15 minutes).
func (s *AuthService) CheckRateLimit(ctx context.Context, ip string) bool {
	count, err := s.redis.Incr(ctx, "rl:login:"+ip, 15*60)
	if err != nil {
		// Redis down — fail open
		return true
	}
	return count <= 10
}

// VerifyToken delegates to the JWT package.
func (s *AuthService) VerifyToken(tokenStr string) (*jwtpkg.Claims, error) {
	return jwtpkg.VerifyToken(tokenStr, s.secret)
}
