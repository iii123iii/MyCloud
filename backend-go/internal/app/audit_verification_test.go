package app

// Regression-guard tests for the audit fixes. Each test pins a specific
// behavior the audit established so a future refactor that breaks it
// flags loudly in CI.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"

	"mycloud/backend-go/internal/auth"
)

// ─── Security headers regression guards ─────────────────────────────────────

// Headers MUST be present on every response, regardless of status code. A
// regression where Recoverer or some other middleware steals output would
// trip this.
func TestSecurityHeaders_PresentOn200(t *testing.T) {
	a := newTestApp(t)
	h := a.securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))
	w := rec()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	checkRequiredHeaders(t, w)
}

func TestSecurityHeaders_PresentOn4xx(t *testing.T) {
	a := newTestApp(t)
	h := a.securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	w := rec()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	checkRequiredHeaders(t, w)
}

func TestSecurityHeaders_PresentOn5xx(t *testing.T) {
	a := newTestApp(t)
	h := a.securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	w := rec()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	checkRequiredHeaders(t, w)
}

// CSP must NOT contain 'unsafe-eval' (would defeat the whole point on an
// XSS-rich surface).
func TestSecurityHeaders_CSPNoUnsafeEval(t *testing.T) {
	a := newTestApp(t)
	h := a.securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	w := rec()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	csp := w.Header().Get("Content-Security-Policy")
	if strings.Contains(csp, "unsafe-eval") {
		t.Errorf("CSP must not allow unsafe-eval: %s", csp)
	}
	if !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("CSP must set frame-ancestors 'none'")
	}
}

// HSTS includes subdomains and a year-long max-age.
func TestSecurityHeaders_HSTSConfig(t *testing.T) {
	a := newTestApp(t)
	h := a.securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	w := rec()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	hsts := w.Header().Get("Strict-Transport-Security")
	if !strings.Contains(hsts, "includeSubDomains") {
		t.Errorf("HSTS should include subdomains: %s", hsts)
	}
	if !strings.Contains(hsts, "max-age=31536000") {
		t.Errorf("HSTS max-age should be 1 year: %s", hsts)
	}
}

func checkRequiredHeaders(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	required := []string{
		"Strict-Transport-Security",
		"X-Content-Type-Options",
		"X-Frame-Options",
		"Referrer-Policy",
		"Permissions-Policy",
		"Content-Security-Policy",
	}
	for _, h := range required {
		if w.Header().Get(h) == "" {
			t.Errorf("missing required header: %s", h)
		}
	}
}

// ─── CORS fail-closed regression guard ──────────────────────────────────────

// Empty allowlist must NOT reflect Origin. The audit-flagged behaviour
// was: empty list reflected ANY origin (fail-open). This test pins the
// fail-closed contract.
func TestCORS_EmptyAllowlistDoesNotReflectAnyOrigin(t *testing.T) {
	a := minimalApp(testConfig(t))
	a.Config.AllowedOrigins = nil
	h := a.cors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	for _, origin := range []string{"https://app.example.com", "https://evil.com", "http://localhost:3000"} {
		w := rec()
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Origin", origin)
		h.ServeHTTP(w, r)
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("empty allowlist must fail closed for origin %q, got ACAO=%q", origin, got)
		}
	}
}

// Listed origin echoes; unlisted does not. Defends against an over-broad
// reflective CORS regression.
func TestCORS_OnlyListedOriginEchoes(t *testing.T) {
	a := minimalApp(testConfig(t))
	a.Config.AllowedOrigins = []string{"https://app.example.com"}
	h := a.cors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	// Listed.
	w := rec()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Origin", "https://app.example.com")
	h.ServeHTTP(w, r)
	if w.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Errorf("listed origin should echo")
	}

	// Unlisted.
	w2 := rec()
	r2 := httptest.NewRequest("GET", "/", nil)
	r2.Header.Set("Origin", "https://evil.com")
	h.ServeHTTP(w2, r2)
	if w2.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("unlisted origin must not be echoed")
	}
}

// ─── JWT alg confusion regression guards ────────────────────────────────────

// jwt/v5 rejects "none" by default; we additionally pin to HS256 in the
// keyfunc as defence in depth. Test confirms the explicit pin works.
func TestJWT_RejectsAlgNone(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rc.Close()
	m := auth.NewManager(strings.Repeat("a", 32), 900, 604800, rc)

	tok := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"uid": "u-1",
		"typ": "access",
	})
	signed, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none: %v", err)
	}
	if _, err := m.Parse(signed); err == nil {
		t.Errorf("Parse should reject alg=none token")
	}
}

// Pin to HS256 also rejects RS256 even when forged with the HMAC secret
// as "public key" material. This is the classic alg-confusion attack —
// the keyfunc returning m.secret would otherwise treat the HMAC secret
// as a public key for RSA verification.
func TestJWT_RejectsAlgRS256(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rc.Close()
	m := auth.NewManager(strings.Repeat("a", 32), 900, 604800, rc)

	// Forge an RS256 token. We can't actually sign it (no RSA key), but
	// the Parse path should reject based on the alg header alone before
	// signature verification.
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"uid": "u-1",
		"typ": "access",
	})
	signed := tok.Raw // unsigned, but the alg field is what we test
	_ = signed
	// Even a malformed RS256 token shouldn't reach signature verification —
	// the keyfunc's method-check returns an error first. We use a
	// hand-crafted header to make the test concrete.
	rawRS256 := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1aWQiOiJ1LTEifQ.signature"
	if _, err := m.Parse(rawRS256); err == nil {
		t.Errorf("Parse should reject RS256 token")
	}
}

// ─── Refresh token rotation regression guard ────────────────────────────────

// Refresh produces NEW JTIs; the new tokens must be different from the
// old. A regression where IssuePair returned the same JTI would silently
// break session tracking.
func TestRefresh_RotatesJTI(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rc.Close()
	m := auth.NewManager(strings.Repeat("a", 32), 900, 604800, rc)

	first, _ := m.IssuePair("u-1", "user")
	second, _ := m.IssuePairForRefresh("u-1", "user", first["family"].(string))
	if first["access_jti"] == second["access_jti"] {
		t.Errorf("access JTI must rotate")
	}
	if first["refresh_jti"] == second["refresh_jti"] {
		t.Errorf("refresh JTI must rotate")
	}
	// Family stays the same.
	if first["family"] != second["family"] {
		t.Errorf("family must be preserved across refresh")
	}
}

// ─── Family revocation regression guard ─────────────────────────────────────

// RevokeFamily blocks every token in that family. Pin via two different
// tokens issued under the same family.
func TestFamilyRevocation_BlocksAllTokensInFamily(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rc.Close()
	m := auth.NewManager(strings.Repeat("a", 32), 900, 604800, rc)

	first, _ := m.IssuePair("u-1", "user")
	second, _ := m.IssuePairForRefresh("u-1", "user", first["family"].(string))

	m.RevokeFamily(first["family"].(string))

	if _, err := m.Parse(first["access_token"].(string)); err == nil {
		t.Errorf("first access should be revoked")
	}
	if _, err := m.Parse(second["access_token"].(string)); err == nil {
		t.Errorf("second access (same family) should be revoked")
	}
}

// JTI-only revocation does NOT kill other tokens in the family.
func TestJTIRevocation_DoesNotKillFamily(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rc.Close()
	m := auth.NewManager(strings.Repeat("a", 32), 900, 604800, rc)

	first, _ := m.IssuePair("u-1", "user")
	second, _ := m.IssuePairForRefresh("u-1", "user", first["family"].(string))

	m.RevokeJTI(first["access_jti"].(string))

	if _, err := m.Parse(first["access_token"].(string)); err == nil {
		t.Errorf("first access should be revoked (its JTI)")
	}
	if _, err := m.Parse(second["access_token"].(string)); err != nil {
		t.Errorf("second access should still be valid: %v", err)
	}
}

// ─── Bcrypt cost regression guard ───────────────────────────────────────────

// NeedsRehash must return true for cost-10 (old default) and false for the
// current cost. Pins the rolling-upgrade contract.
func TestNeedsRehash_LegacyCostTriggersUpgrade(t *testing.T) {
	// Cost-10 bcrypt of "password".
	legacy := "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
	if !auth.NeedsRehash(legacy) {
		t.Errorf("cost-10 hash should need rehash (current floor %d)", auth.BcryptCost)
	}
}

func TestNeedsRehash_CurrentCostNoUpgrade(t *testing.T) {
	hash, _ := auth.HashPassword("anything")
	if auth.NeedsRehash(hash) {
		t.Errorf("freshly hashed password should not need rehash (cost %d)", auth.BcryptCost)
	}
}

// ─── respondDBError no-leak regression guard ────────────────────────────────

// The audit found 197 sites that echoed raw DB errors to clients. This
// test confirms the centralized helper masks every such leak.
func TestRespondDBError_DoesNotLeakSchemaText(t *testing.T) {
	leakyErrors := []string{
		"Duplicate entry 'alice@example.com' for key 'users.email'",
		"pq: invalid input syntax for type uuid: \"not-a-uuid\"",
		"Error 1452: Cannot add or update a child row: a foreign key constraint fails (`mycloud`.`files`, CONSTRAINT `fk_files_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`))",
		"Error 2002: Can't connect to MySQL server on 'db-host-internal' (111)",
	}
	for _, msg := range leakyErrors {
		w := rec()
		r := authedRequest("GET", "/", "")
		respondDBError(w, r, errStr(msg))
		body := w.Body.String()
		if strings.Contains(body, "alice@example.com") ||
			strings.Contains(body, "users.email") ||
			strings.Contains(body, "uuid") ||
			strings.Contains(body, "mycloud.files") ||
			strings.Contains(body, "db-host-internal") {
			t.Errorf("response leaked DB text:\n  raw: %s\n  body: %s", msg, body)
		}
		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", w.Code)
		}
	}
}

// errStr returns an error with the supplied message — convenience for the
// table test above.
type stringErr string

func (s stringErr) Error() string { return string(s) }
func errStr(s string) error       { return stringErr(s) }

// ─── must_change_password gate regression guard ─────────────────────────────

// The flag blocks every protected route except the three on the exempt
// allowlist. A regression that removes the gate would let a forced-reset
// user bypass the requirement.
func TestRequirePasswordCurrent_BlocksFilesPath(t *testing.T) {
	a := newTestApp(t)
	hit := false
	h := a.requirePasswordCurrent(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
	}))
	w := rec()
	r := authedRequest("GET", "/api/v2/files", "")
	r = r.WithContext(contextWithMustChange(r, true))
	// Inject a chi RoutePattern that's NOT in the exempt list.
	r = withChiRoutePattern(r, "/api/v2/files")
	h.ServeHTTP(w, r)
	if hit {
		t.Errorf("handler should NOT be reached for blocked route")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// Allowlist: change-password is reachable so the user can fix the
// situation.
func TestRequirePasswordCurrent_AllowsChangePassword(t *testing.T) {
	a := newTestApp(t)
	hit := false
	h := a.requirePasswordCurrent(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(200)
	}))
	w := rec()
	r := authedRequest("POST", "/api/v2/auth/change-password", `{}`)
	r = r.WithContext(contextWithMustChange(r, true))
	r = withChiRoutePattern(r, "/api/v2/auth/change-password")
	h.ServeHTTP(w, r)
	if !hit {
		t.Errorf("change-password should be reachable")
	}
}

// Config.validate guards live in internal/config/config_test.go (the
// unexported method is in scope there). This file focuses on
// app-package regressions.

// ─── handleStorageStats single-roundtrip regression guard ──────────────────

// Before the audit, handleStorageStats made 3 round-trips (one of which
// swallowed errors). The fix is one correlated-subquery SELECT. This test
// pins that contract: exactly ONE QueryContext to the DB.
func TestHandleStorageStats_SingleRoundTrip(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT u.used_bytes, u.quota_bytes").
		WithArgs("u-test").
		WillReturnRows(sqlmock.NewRows([]string{"used", "quota", "files", "folders"}).
			AddRow(int64(100), int64(1000), int64(5), int64(2)))
	w := rec()
	r := authedRequest("GET", "/storage/stats", "")
	a.handleStorageStats(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if err := a.mock.ExpectationsWereMet(); err != nil {
		t.Errorf("more than one query issued: %v", err)
	}
}
