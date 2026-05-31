package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// withRoutePattern sets the chi RoutePattern on a request so requireScope can
// resolve the per-route scope. withChiParams only sets URL params, not the
// pattern, so scope-middleware tests need this.
func withRoutePattern(r *http.Request, pattern string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.RoutePatterns = []string{pattern}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestCanonicalizeScopes(t *testing.T) {
	got, err := canonicalizeScopes([]string{"files:write", "files:read", "files:read"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "files:read,files:write" {
		t.Errorf("expected sorted+deduped CSV, got %q", got)
	}

	// Explicit "*" collapses to "*".
	if got, _ := canonicalizeScopes([]string{"files:read", "*"}); got != "*" {
		t.Errorf("explicit * should collapse to *, got %q", got)
	}

	// A fully-checked set is NOT auto-collapsed to "*" (least privilege).
	all := []string{"files:read", "files:write", "shares:read", "shares:write", "tags:read", "tags:write", "account:read"}
	got, err = canonicalizeScopes(all)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == "*" {
		t.Error("a fully-checked scope set was wrongly collapsed to *")
	}
	if !strings.Contains(got, "files:write") {
		t.Errorf("expected literal scopes, got %q", got)
	}

	if _, err := canonicalizeScopes([]string{"files:nuke"}); err == nil {
		t.Error("expected error for unknown scope")
	}
	if _, err := canonicalizeScopes(nil); err == nil {
		t.Error("expected error for empty scope set")
	}
}

func TestHasScope(t *testing.T) {
	if !hasScope([]string{"files:read"}, "files:read") {
		t.Error("exact scope should match")
	}
	if hasScope([]string{"files:read"}, "files:write") {
		t.Error("unrelated scope should not match")
	}
	if !hasScope([]string{"*"}, "anything:goes") {
		t.Error("* should match any scope")
	}
}

func TestRequiredScopeFor(t *testing.T) {
	cases := []struct{ method, pattern, want string }{
		{"GET", "/api/v2/files", "files:read"},
		{"POST", "/api/v2/files:upload", "files:write"},
		{"GET", "/api/v2/tags", "tags:read"},
		{"POST", "/api/v2/tags", "tags:write"},
		{"GET", "/api/v2/auth/me", "account:read"},
		{"POST", "/api/v2/files/tus/", "files:write"}, // tus mount, prefix-matched
		{"PATCH", "/api/v2/files/tus/abc", "files:write"},
		{"GET", "/api/v2/something/unmapped", "*"}, // fail closed
	}
	for _, c := range cases {
		if got := requiredScopeFor(c.method, c.pattern); got != c.want {
			t.Errorf("%s %s: got %q, want %q", c.method, c.pattern, got, c.want)
		}
	}
}

func TestRequireScope(t *testing.T) {
	a := &App{}
	// run returns the HTTP status produced by requireScope for the given
	// (scopes, isPAT, method, pattern). A 200 means the request passed through.
	run := func(scopes []string, isPAT bool, method, pattern string) int {
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		r := httptest.NewRequest(method, "http://example.com/", nil)
		ctx := r.Context()
		if isPAT {
			ctx = withPAT(ctx, "pat-1", scopes)
		}
		r = withRoutePattern(r.WithContext(ctx), pattern)
		w := rec()
		a.requireScope(next).ServeHTTP(w, r)
		return w.Code
	}

	// Interactive JWT session: no restriction at all.
	if code := run(nil, false, "POST", "/api/v2/files:upload"); code != http.StatusOK {
		t.Errorf("JWT session should pass, got %d", code)
	}
	// PAT with the matching scope.
	if code := run([]string{"files:read"}, true, "GET", "/api/v2/files"); code != http.StatusOK {
		t.Errorf("files:read on GET /files should pass, got %d", code)
	}
	// PAT lacking the write scope.
	if code := run([]string{"files:read"}, true, "POST", "/api/v2/files:upload"); code != http.StatusForbidden {
		t.Errorf("files:read on upload should be 403, got %d", code)
	}
	// tags:read allows reads but not writes.
	if code := run([]string{"tags:read"}, true, "GET", "/api/v2/tags"); code != http.StatusOK {
		t.Errorf("tags:read on GET /tags should pass, got %d", code)
	}
	if code := run([]string{"tags:read"}, true, "POST", "/api/v2/tags"); code != http.StatusForbidden {
		t.Errorf("tags:read on POST /tags should be 403, got %d", code)
	}
	// "*" passes any non-blocked route.
	if code := run([]string{"*"}, true, "POST", "/api/v2/files:upload"); code != http.StatusOK {
		t.Errorf("* should pass upload, got %d", code)
	}
	// Unmapped route is fail-closed: requires "*".
	if code := run([]string{"files:read"}, true, "GET", "/api/v2/brand-new"); code != http.StatusForbidden {
		t.Errorf("unmapped route should 403 without *, got %d", code)
	}
	if code := run([]string{"*"}, true, "GET", "/api/v2/brand-new"); code != http.StatusOK {
		t.Errorf("unmapped route should pass with *, got %d", code)
	}
	// Blocked route is forbidden even with "*".
	if code := run([]string{"*"}, true, "DELETE", "/api/v2/me/sessions/{jti}"); code != http.StatusForbidden {
		t.Errorf("blocked route should be 403 even with *, got %d", code)
	}
	// Admin prefix is forbidden even with "*".
	if code := run([]string{"*"}, true, "GET", "/api/v2/admin/users"); code != http.StatusForbidden {
		t.Errorf("admin route should be 403 for any PAT, got %d", code)
	}
	// Scope-exempt route (whoami) is reachable by any token, even a narrow one.
	if code := run([]string{"files:read"}, true, "GET", "/api/v2/auth/whoami"); code != http.StatusOK {
		t.Errorf("whoami should be reachable by any PAT regardless of scope, got %d", code)
	}
}

func TestMergePATID(t *testing.T) {
	// nil details → just pat_id.
	m, ok := mergePATID(nil, "pat-9").(map[string]any)
	if !ok || m["pat_id"] != "pat-9" {
		t.Errorf("nil details: expected pat_id, got %v", m)
	}
	// map details preserved + pat_id added.
	m, ok = mergePATID(map[string]any{"name": "x"}, "pat-9").(map[string]any)
	if !ok || m["name"] != "x" || m["pat_id"] != "pat-9" {
		t.Errorf("map merge failed: %v", m)
	}
}
