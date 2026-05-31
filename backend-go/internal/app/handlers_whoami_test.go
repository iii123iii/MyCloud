package app

import (
	"net/http"
	"strings"
	"testing"
)

func TestHandleWhoami_PATReturnsScopes(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT id, username, email, role FROM users").
		WillReturnRows(sqlmockNewRows([]string{"id", "username", "email", "role"}).
			AddRow("u-test", "alice", "alice@example.com", "user"))

	w := rec()
	r := authedRequest("GET", "/api/v2/auth/whoami", "")
	// Simulate a PAT-authenticated request by attaching scopes to the context.
	r = r.WithContext(withPAT(r.Context(), "pat-1", []string{"files:read", "files:write"}))
	a.handleWhoami(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "alice") {
		t.Error("response missing username")
	}
	if !strings.Contains(body, `"auth":"pat"`) {
		t.Errorf("expected auth=pat, got %s", body)
	}
	if !strings.Contains(body, "files:write") {
		t.Error("response should list the token's scopes")
	}
}

func TestHandleWhoami_SessionHasNoScopes(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT id, username, email, role FROM users").
		WillReturnRows(sqlmockNewRows([]string{"id", "username", "email", "role"}).
			AddRow("u-test", "alice", "alice@example.com", "user"))

	w := rec()
	a.handleWhoami(w, authedRequest("GET", "/api/v2/auth/whoami", "")) // no PAT in ctx
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"auth":"session"`) {
		t.Errorf("expected auth=session for JWT request, got %s", w.Body.String())
	}
}
