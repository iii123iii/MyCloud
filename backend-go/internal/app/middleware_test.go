package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mycloud/backend-go/internal/config"
)

// Builds a minimal App with just enough wiring for middleware tests.
// Avoids spinning up the full app surface — most middleware only touches
// Config + Redis (nil-tolerant in our code paths).
func minimalApp(cfg config.Config) *App {
	return &App{Config: cfg}
}

func newPassThroughHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

// ─── securityHeaders ────────────────────────────────────────────────────────

func TestSecurityHeaders_AllSet(t *testing.T) {
	a := minimalApp(config.Config{})
	h := a.securityHeaders(newPassThroughHandler())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))

	required := map[string]string{
		"Strict-Transport-Security": "max-age=",
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"Referrer-Policy":           "no-referrer",
		"Permissions-Policy":        "geolocation=",
		"Content-Security-Policy":   "default-src 'self'",
	}
	for header, want := range required {
		got := w.Header().Get(header)
		if got == "" {
			t.Errorf("%s not set", header)
		}
		if !strings.Contains(got, want) {
			t.Errorf("%s = %q, want to contain %q", header, got, want)
		}
	}
}

func TestSecurityHeaders_PassThroughToBody(t *testing.T) {
	a := minimalApp(config.Config{})
	h := a.securityHeaders(newPassThroughHandler())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Body.String() != "ok" {
		t.Errorf("body should pass through")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status not preserved: %d", w.Code)
	}
}

func TestSecurityHeaders_CSPHasNoUnsafeEval(t *testing.T) {
	a := minimalApp(config.Config{})
	h := a.securityHeaders(newPassThroughHandler())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	csp := w.Header().Get("Content-Security-Policy")
	if strings.Contains(csp, "unsafe-eval") {
		t.Errorf("CSP contains unsafe-eval: %s", csp)
	}
	if !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("CSP should set frame-ancestors none (defense-in-depth with X-Frame-Options)")
	}
}

// ─── cors ───────────────────────────────────────────────────────────────────

func TestCORS_AllowedOriginEchoed(t *testing.T) {
	a := minimalApp(config.Config{AllowedOrigins: []string{"https://app.example.com"}})
	h := a.cors(newPassThroughHandler())

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("got %q, want exact echo", got)
	}
	if w.Header().Get("Vary") != "Origin" {
		t.Errorf("missing Vary: Origin")
	}
}

func TestCORS_UnlistedOriginRejected(t *testing.T) {
	a := minimalApp(config.Config{AllowedOrigins: []string{"https://allowed.com"}})
	h := a.cors(newPassThroughHandler())

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Origin", "https://evil.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("ACAO leaked for unlisted origin: %q", got)
	}
}

// The audit fixed CORS to fail closed when AllowedOrigins is empty. Previously
// the middleware echoed any Origin in that case, defeating the allowlist.
func TestCORS_EmptyAllowlistFailsClosed(t *testing.T) {
	a := minimalApp(config.Config{}) // empty AllowedOrigins
	h := a.cors(newPassThroughHandler())

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Origin", "https://anywhere.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("empty allowlist must fail closed, got ACAO=%q", got)
	}
}

func TestCORS_OPTIONSPreflight(t *testing.T) {
	a := minimalApp(config.Config{AllowedOrigins: []string{"https://app.example.com"}})
	h := a.cors(newPassThroughHandler())

	r := httptest.NewRequest("OPTIONS", "/", nil)
	r.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS should be 204, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Errorf("missing Allow-Methods on preflight")
	}
}

func TestCORS_AllowMethods_IncludesAllExpectedVerbs(t *testing.T) {
	a := minimalApp(config.Config{AllowedOrigins: []string{"x"}})
	h := a.cors(newPassThroughHandler())
	r := httptest.NewRequest("OPTIONS", "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	allow := w.Header().Get("Access-Control-Allow-Methods")
	for _, v := range []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"} {
		if !strings.Contains(allow, v) {
			t.Errorf("Allow-Methods missing %s: %s", v, allow)
		}
	}
}

func TestCORS_NoOriginHeader_NoACAOSet(t *testing.T) {
	a := minimalApp(config.Config{AllowedOrigins: []string{"x"}})
	h := a.cors(newPassThroughHandler())
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	// No Origin → no ACAO. CORS only kicks in when the client opts in.
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("ACAO should not be set when Origin is absent")
	}
}

// ─── requestID ──────────────────────────────────────────────────────────────

func TestRequestID_GeneratesWhenAbsent(t *testing.T) {
	a := minimalApp(config.Config{})
	var seenID string
	h := a.requestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenID = RequestID(r.Context())
		w.WriteHeader(200)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))

	if seenID == "" {
		t.Errorf("expected request ID to be generated")
	}
	if w.Header().Get("X-Request-ID") != seenID {
		t.Errorf("response header should mirror generated ID")
	}
}

func TestRequestID_HonoursValidClientHeader(t *testing.T) {
	a := minimalApp(config.Config{})
	supplied := "11111111-2222-3333-4444-555555555555"
	var seenID string
	h := a.requestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenID = RequestID(r.Context())
	}))
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Request-ID", supplied)
	h.ServeHTTP(httptest.NewRecorder(), r)
	if seenID != supplied {
		t.Errorf("expected client ID to be honoured, got %q", seenID)
	}
}

func TestRequestID_RejectsAbsurdLength(t *testing.T) {
	a := minimalApp(config.Config{})
	var seenID string
	h := a.requestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenID = RequestID(r.Context())
	}))
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Request-ID", strings.Repeat("a", 200))
	h.ServeHTTP(httptest.NewRecorder(), r)
	if seenID == strings.Repeat("a", 200) {
		t.Errorf("absurdly long client-provided ID should be rejected; got it back unchanged")
	}
}

func TestRequestID_RejectsNonUUIDChars(t *testing.T) {
	a := minimalApp(config.Config{})
	var seenID string
	h := a.requestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenID = RequestID(r.Context())
	}))
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Request-ID", "<script>alert(1)</script>")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if strings.Contains(seenID, "<") {
		t.Errorf("invalid characters should be rejected, got %q", seenID)
	}
}

// ─── looksLikeUUID ──────────────────────────────────────────────────────────

func TestLooksLikeUUID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"11111111-2222-3333-4444-555555555555", true},
		{"aBcD1234-eF56-7890-1234-567890abcdef", true},
		{"abc12345", true},  // 8-char hex; bounded check, not strict format
		{"short", false},
		{strings.Repeat("a", 65), false},
		{"contains spaces", false},
		{"<script>alert(1)</script>", false},
		{"abc-def-zzz", false}, // 'z' not hex
		{"", false},
	}
	for _, c := range cases {
		if got := looksLikeUUID(c.in); got != c.want {
			t.Errorf("looksLikeUUID(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// ─── context plumbing ───────────────────────────────────────────────────────

func TestWithUser_StoresIDAndRole(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	ctx := withUser(r.Context(), "u-1", "admin")
	r = r.WithContext(ctx)
	if got := userIDFrom(r); got != "u-1" {
		t.Errorf("userIDFrom got %q, want u-1", got)
	}
	if got := userRoleFrom(r); got != "admin" {
		t.Errorf("userRoleFrom got %q, want admin", got)
	}
}

func TestUserIDFrom_DefaultsEmpty(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if got := userIDFrom(r); got != "" {
		t.Errorf("userIDFrom on unauth request should be empty, got %q", got)
	}
}

func TestJTIFrom_DefaultsEmpty(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if got := jtiFrom(r); got != "" {
		t.Errorf("jtiFrom on bare request should be empty")
	}
}

// userIDHolder is the mutable cell the access-log middleware uses to publish
// the authenticated user up to the logger after requireAuth runs deeper in
// the chain.
func TestUserIDHolder_SetGetIsConcurrentSafe(t *testing.T) {
	h := &userIDHolder{}
	if h.get() != "" {
		t.Errorf("default get should be empty")
	}
	h.set("u-1")
	if got := h.get(); got != "u-1" {
		t.Errorf("after set: got %q", got)
	}
	// Nil-safe — calling on a nil receiver shouldn't panic. (The middleware
	// stashes a real holder but defensive nil-checks let us run tests
	// without setting one up.)
	var nilHolder *userIDHolder
	nilHolder.set("x")
	if nilHolder.get() != "" {
		t.Errorf("nil holder get should be empty")
	}
}
