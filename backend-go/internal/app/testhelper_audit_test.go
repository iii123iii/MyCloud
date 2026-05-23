package app

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"mycloud/backend-go/internal/config"
)

// testConfig returns a baseline test config used by the audit tests.
// Centralised so each test isn't a wall of field assignments.
func testConfig(_ interface{ Helper() }) config.Config {
	return config.Config{
		AllowedOrigins:          []string{"https://localhost"},
		JWTSecret:               strings.Repeat("a", 32),
		AccessTokenTTL:          900,
		RefreshTokenTTL:         604800,
		MaxVersionsPerFile:      10,
		RateLimitLoginPerMin:    10,
		RateLimitSharePwdPerMin: 10,
	}
}

// withChiRoutePattern populates the chi RouteContext's RoutePattern field,
// which is what requirePasswordCurrent inspects to check the exempt list.
// Tests outside the full router need this shim because chi only populates
// RoutePattern when a real route match happens.
func withChiRoutePattern(r *http.Request, pattern string) *http.Request {
	rctx, _ := r.Context().Value(chi.RouteCtxKey).(*chi.Context)
	if rctx == nil {
		rctx = chi.NewRouteContext()
	}
	rctx.RoutePatterns = append(rctx.RoutePatterns, pattern)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

