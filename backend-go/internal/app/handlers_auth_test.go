package app

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"golang.org/x/crypto/bcrypt"
)

// rowsErrNoRows returns sql.ErrNoRows for sqlmock WillReturnError. Tiny
// helper so call sites don't need a database/sql import.
func rowsErrNoRows() error { return sql.ErrNoRows }

// ─── handleSetupComplete ────────────────────────────────────────────────────

func TestHandleSetupComplete_AlreadyDoneReturns409(t *testing.T) {
	a := newTestApp(t)
	// handleSetupComplete uses a literal string (no placeholder) to read
	// the setup_complete flag; expect the bare SELECT shape and return "true".
	a.mock.ExpectQuery("SELECT value FROM settings WHERE key_name='setup_complete'").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))
	w := rec()
	r := httptest.NewRequest("POST", "/setup", strings.NewReader(`{"username":"alice","email":"a@b.com","password":"longenough"}`))
	r.Header.Set("Content-Type", "application/json")
	a.handleSetupComplete(w, r)
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleSetupComplete_ShortPasswordReturns400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := httptest.NewRequest("POST", "/setup", strings.NewReader(`{"username":"a","email":"a@b.com","password":"short"}`))
	r.Header.Set("Content-Type", "application/json")
	a.handleSetupComplete(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "validation_error") {
		t.Errorf("expected validation_error code, got %s", w.Body.String())
	}
}

func TestHandleSetupComplete_MalformedJSONReturns400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := httptest.NewRequest("POST", "/setup", strings.NewReader(`{`))
	r.Header.Set("Content-Type", "application/json")
	a.handleSetupComplete(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ─── handleRegister ─────────────────────────────────────────────────────────

func TestHandleRegister_SetupNotComplete(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT value FROM settings WHERE key_name=").
		WithArgs("setup_complete").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	w := rec()
	r := httptest.NewRequest("POST", "/register", strings.NewReader(`{"username":"alice","email":"a@b.com","password":"longenough"}`))
	r.Header.Set("Content-Type", "application/json")
	a.handleRegister(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "setup_required") {
		t.Errorf("expected setup_required code")
	}
}

func TestHandleRegister_RegistrationDisabled(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT value FROM settings WHERE key_name=").
		WithArgs("setup_complete").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))
	a.mock.ExpectQuery("SELECT value FROM settings WHERE key_name=").
		WithArgs("registration_enabled").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	w := rec()
	r := httptest.NewRequest("POST", "/register", strings.NewReader(`{"username":"alice","email":"a@b.com","password":"longenough"}`))
	r.Header.Set("Content-Type", "application/json")
	a.handleRegister(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "registration_disabled") {
		t.Errorf("expected registration_disabled code")
	}
}

func TestHandleRegister_ShortPasswordReturns400(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT value FROM settings WHERE key_name=").
		WithArgs("setup_complete").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))
	a.mock.ExpectQuery("SELECT value FROM settings WHERE key_name=").
		WithArgs("registration_enabled").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))
	w := rec()
	r := httptest.NewRequest("POST", "/register", strings.NewReader(`{"username":"a","email":"a@b.com","password":"short"}`))
	r.Header.Set("Content-Type", "application/json")
	a.handleRegister(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ─── handleLogin ────────────────────────────────────────────────────────────

func TestHandleLogin_UnknownUserAbsorbsBcrypt(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT id, username, email, password_hash").
		WithArgs("ghost@example.com", "ghost@example.com").
		WillReturnError(rowsErrNoRows())
	w := rec()
	r := httptest.NewRequest("POST", "/login", strings.NewReader(`{"email":"ghost@example.com","password":"anything"}`))
	r.Header.Set("Content-Type", "application/json")
	a.handleLogin(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid_credentials") {
		t.Errorf("expected generic invalid_credentials")
	}
	// Critical: error message must NOT leak that user doesn't exist —
	// same message as wrong-password case.
	if strings.Contains(w.Body.String(), "not found") || strings.Contains(w.Body.String(), "ghost") {
		t.Errorf("response leaked user-existence: %s", w.Body.String())
	}
}

func TestHandleLogin_WrongPassword401(t *testing.T) {
	a := newTestApp(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct"), 10)
	a.mock.ExpectQuery("SELECT id, username, email, password_hash").
		WithArgs("alice@example.com", "alice@example.com").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "username", "email", "password_hash", "role", "quota_bytes", "used_bytes", "is_active", "must_change_password",
		}).AddRow("u-1", "alice", "alice@example.com", string(hash), "user", int64(0), int64(0), true, false))
	w := rec()
	r := httptest.NewRequest("POST", "/login", strings.NewReader(`{"email":"alice@example.com","password":"wrong"}`))
	r.Header.Set("Content-Type", "application/json")
	a.handleLogin(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleLogin_DisabledAccount401(t *testing.T) {
	a := newTestApp(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct"), 10)
	a.mock.ExpectQuery("SELECT id, username, email, password_hash").
		WithArgs("alice@example.com", "alice@example.com").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "username", "email", "password_hash", "role", "quota_bytes", "used_bytes", "is_active", "must_change_password",
		}).AddRow("u-1", "alice", "alice@example.com", string(hash), "user", int64(0), int64(0), false, false))
	w := rec()
	r := httptest.NewRequest("POST", "/login", strings.NewReader(`{"email":"alice@example.com","password":"correct"}`))
	r.Header.Set("Content-Type", "application/json")
	a.handleLogin(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for disabled account, got %d", w.Code)
	}
}

// ─── handleRefresh ──────────────────────────────────────────────────────────

func TestHandleRefresh_InvalidTokenReturns401(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := httptest.NewRequest("POST", "/refresh", strings.NewReader(`{"refresh_token":"garbage"}`))
	r.Header.Set("Content-Type", "application/json")
	a.handleRefresh(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleRefresh_AccessTokenPresentedAsRefreshReturns401(t *testing.T) {
	a := newTestApp(t)
	tokens, _ := a.Auth.IssuePair("u-1", "user")
	// Present the ACCESS token in the refresh slot — typ mismatch.
	body, _ := json.Marshal(map[string]string{"refresh_token": tokens["access_token"].(string)})
	w := rec()
	r := httptest.NewRequest("POST", "/refresh", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	a.handleRefresh(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("type-confusion not rejected, got %d", w.Code)
	}
}

func TestHandleRefresh_ReuseDetectionTriggers(t *testing.T) {
	a := newTestApp(t)
	tokens, _ := a.Auth.IssuePair("u-1", "user")
	refresh := tokens["refresh_token"].(string)
	refreshJTI := tokens["refresh_jti"].(string)

	// Pre-blacklist the JTI as if it had already been rotated once.
	a.Auth.RevokeJTI(refreshJTI)

	body, _ := json.Marshal(map[string]string{"refresh_token": refresh})
	w := rec()
	r := httptest.NewRequest("POST", "/refresh", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	a.handleRefresh(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("reuse should be detected, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "refresh_reuse") {
		t.Errorf("expected refresh_reuse code, got %s", w.Body.String())
	}
	// Family must now be revoked.
	family := tokens["family"].(string)
	if !a.miniRds.Exists("bl:family:" + family) {
		t.Errorf("family blacklist key should be set after reuse detection")
	}
}

// ─── handleLogout ───────────────────────────────────────────────────────────

func TestHandleLogout_NoBearerStillRevokesRefresh(t *testing.T) {
	a := newTestApp(t)
	tokens, _ := a.Auth.IssuePair("u-1", "user")
	refresh := tokens["refresh_token"].(string)
	refreshJTI := tokens["refresh_jti"].(string)

	body, _ := json.Marshal(map[string]string{"refresh_token": refresh})
	w := rec()
	r := httptest.NewRequest("POST", "/logout", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	a.handleLogout(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !a.miniRds.Exists("bl:" + refreshJTI) {
		t.Errorf("refresh JTI should be blacklisted")
	}
}

func TestHandleLogout_RevokesBearerFamily(t *testing.T) {
	a := newTestApp(t)
	tokens, _ := a.Auth.IssuePair("u-1", "user")
	access := tokens["access_token"].(string)
	family := tokens["family"].(string)

	w := rec()
	r := httptest.NewRequest("POST", "/logout", strings.NewReader(`{}`))
	r.Header.Set("Authorization", "Bearer "+access)
	r.Header.Set("Content-Type", "application/json")
	a.handleLogout(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !a.miniRds.Exists("bl:family:" + family) {
		t.Errorf("logout should revoke the bearer's family")
	}
}

func TestHandleLogout_MalformedBodyDoesNotPanic(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := httptest.NewRequest("POST", "/logout", strings.NewReader(`{`))
	r.Header.Set("Content-Type", "application/json")
	// Should NOT panic.
	a.handleLogout(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("malformed body should still return 200, got %d", w.Code)
	}
}

// ─── handleMe ───────────────────────────────────────────────────────────────

func TestHandleMe_Success(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT id, username, email, role, quota_bytes, used_bytes, is_active, must_change_password, created_at").
		WithArgs("u-test").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "username", "email", "role", "quota_bytes", "used_bytes", "is_active", "must_change_password", "created_at",
		}).AddRow("u-test", "alice", "a@b.com", "user", int64(1024), int64(0), true, false, "2026-05-22"))
	w := rec()
	r := authedRequest("GET", "/auth/me", "")
	a.handleMe(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "alice") {
		t.Errorf("body should contain username, got %s", w.Body.String())
	}
}

func TestHandleMe_NotFoundReturns404(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT id, username, email, role").
		WithArgs("u-test").
		WillReturnError(rowsErrNoRows())
	w := rec()
	r := authedRequest("GET", "/auth/me", "")
	a.handleMe(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ─── handleChangePassword ───────────────────────────────────────────────────

func TestHandleChangePassword_ShortNew400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("POST", "/auth/change-password", `{"old_password":"x","new_password":"short"}`)
	a.handleChangePassword(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleChangePassword_WrongOld401(t *testing.T) {
	a := newTestApp(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct"), 10)
	a.mock.ExpectQuery("SELECT password_hash FROM users WHERE id=").
		WithArgs("u-test").
		WillReturnRows(sqlmock.NewRows([]string{"password_hash"}).AddRow(string(hash)))
	w := rec()
	r := authedRequest("POST", "/auth/change-password", `{"old_password":"wrong","new_password":"longenough"}`)
	a.handleChangePassword(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ─── handleDeleteAccount ────────────────────────────────────────────────────

func TestHandleDeleteAccount_WrongPassword401(t *testing.T) {
	a := newTestApp(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct"), 10)
	a.mock.ExpectQuery("SELECT password_hash FROM users WHERE id=").
		WithArgs("u-test").
		WillReturnRows(sqlmock.NewRows([]string{"password_hash"}).AddRow(string(hash)))
	w := rec()
	r := authedRequest("DELETE", "/auth/account", `{"password":"wrong"}`)
	a.handleDeleteAccount(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleDeleteAccount_NotFound404(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT password_hash FROM users WHERE id=").
		WithArgs("u-test").
		WillReturnError(rowsErrNoRows())
	w := rec()
	r := authedRequest("DELETE", "/auth/account", `{"password":"x"}`)
	a.handleDeleteAccount(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
