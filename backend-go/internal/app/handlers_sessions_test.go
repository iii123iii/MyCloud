package app

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// Sessions endpoints — per-device session management. Critical security
// guarantees: only own sessions are listed, only own sessions are revoked,
// self-revoke is blocked (would strand the caller), JTI gets blacklisted in
// Redis on revoke.

func TestHandleListMySessions_FlagsCurrentJTI(t *testing.T) {
	a := newTestApp(t)
	now := time.Now()
	a.mock.ExpectQuery("SELECT jti").
		WithArgs("u-test").
		WillReturnRows(sqlmock.NewRows([]string{
			"jti", "device_label", "user_agent", "ip_address", "created_at", "last_seen_at", "expires_at",
		}).
			AddRow("jti-test", "Chrome on macOS", "Mozilla", "1.1.1.1", now, now, nil).
			AddRow("jti-other", "Firefox on Linux", "Mozilla", "2.2.2.2", now, now, nil))
	w := rec()
	r := authedRequest("GET", "/me/sessions", "")
	a.handleListMySessions(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// jti-test is the caller's own (set in authedRequest), should be marked current.
	if !strings.Contains(body, `"is_current":true`) {
		t.Errorf("expected is_current=true for caller's JTI, got %s", body)
	}
}

func TestHandleRevokeMySession_SelfRevokeBlocked(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("DELETE", "/me/sessions/jti-test", "")
	r = withChiParams(r, "jti", "jti-test") // matches authedRequest's JTI
	a.handleRevokeMySession(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "self_revoke") {
		t.Errorf("expected self_revoke code")
	}
}

func TestHandleRevokeMySession_MissingJTIReturns400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("DELETE", "/me/sessions/", "")
	r = withChiParams(r, "jti", "")
	a.handleRevokeMySession(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// DELETE matches by (jti, user_id) — a foreign JTI matches 0 rows → 404.
func TestHandleRevokeMySession_ForeignJTIReturns404(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("DELETE FROM sessions WHERE jti").
		WithArgs("jti-someone-else", "u-test").
		WillReturnResult(sqlmock.NewResult(0, 0))
	w := rec()
	r := authedRequest("DELETE", "/me/sessions/jti-someone-else", "")
	r = withChiParams(r, "jti", "jti-someone-else")
	a.handleRevokeMySession(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

// Successful revoke deletes the row AND blacklists the JTI in Redis so any
// in-flight access tokens stop working immediately.
func TestHandleRevokeMySession_Success_BlacklistsJTI(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("DELETE FROM sessions WHERE jti").
		WithArgs("jti-other-device", "u-test").
		WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("DELETE", "/me/sessions/jti-other-device", "")
	r = withChiParams(r, "jti", "jti-other-device")
	a.handleRevokeMySession(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d body=%s", w.Code, w.Body.String())
	}
	if !a.miniRds.Exists("bl:jti-other-device") {
		t.Errorf("JTI should be blacklisted in Redis")
	}
}
