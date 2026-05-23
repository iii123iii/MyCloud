package app

// Shared test helpers for handler-level tests. Each handler typically needs:
//   - sqlmock DB so we can assert SQL shape + return canned rows
//   - miniredis so rate-limit / blacklist code paths exercise real Redis
//   - a real auth.Manager wired to that miniredis so JWT validation works
//   - the App struct populated enough that the handler doesn't NPE
//
// Tests that don't need every dep can construct App fields directly; this
// helper is for the common case.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"mycloud/backend-go/internal/auth"
	"mycloud/backend-go/internal/config"
	"mycloud/backend-go/internal/db/stmtcache"
)

type testApp struct {
	*App
	mock     sqlmock.Sqlmock
	miniRds  *miniredis.Miniredis
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close(); db.Close() })

	cfg := config.Config{
		AllowedOrigins:           []string{"https://localhost"},
		JWTSecret:                strings.Repeat("a", 32),
		AccessTokenTTL:           900,
		RefreshTokenTTL:          604800,
		MaxVersionsPerFile:       10,
		RateLimitLoginPerMin:     10,
		RateLimitSharePwdPerMin:  10,
	}
	a := &App{
		Config: cfg,
		DB:     db,
		Stmts:  stmtcache.New(db),
		Redis:  rc,
		Auth:   auth.NewManager(cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL, rc),
	}
	return &testApp{App: a, mock: mock, miniRds: mr}
}

// authedRequest builds an httptest request with the user-context already
// populated (skipping requireAuth). Used by handlers that read userIDFrom(r).
func authedRequest(method, path, body string) *http.Request {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	r.Header.Set("Content-Type", "application/json")
	ctx := withUser(r.Context(), "u-test", "user")
	ctx = context.WithValue(ctx, userJTIKey, "jti-test")
	return r.WithContext(ctx)
}

func adminRequest(method, path, body string) *http.Request {
	r := authedRequest(method, path, body)
	ctx := withUser(r.Context(), "u-admin", "admin")
	ctx = context.WithValue(ctx, userJTIKey, "jti-admin")
	return r.WithContext(ctx)
}

// withChiParams wraps a request so chi.URLParam returns the supplied values
// without going through the actual router. Handlers tested in isolation
// otherwise see empty strings for all path params.
func withChiParams(r *http.Request, kv ...string) *http.Request {
	if len(kv)%2 != 0 {
		panic("withChiParams: odd args")
	}
	rctx := chi.NewRouteContext()
	for i := 0; i < len(kv); i += 2 {
		rctx.URLParams.Add(kv[i], kv[i+1])
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// rec is a one-liner ResponseRecorder.
func rec() *httptest.ResponseRecorder { return httptest.NewRecorder() }
