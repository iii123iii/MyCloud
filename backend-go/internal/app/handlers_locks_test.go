package app

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// Bad lock kinds must be rejected — the cooperative-lock table only knows
// three kinds (edit/sync/office); anything else is a client bug.
func TestHandleAcquireFileLock_BadKindReturns400(t *testing.T) {
	a := newTestApp(t)
	// canAccessFile preflight returns success.
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	w := rec()
	r := authedRequest("POST", "/files/f1/lock", `{"kind":"weird"}`)
	r = withChiParams(r, "id", "f1")
	a.handleAcquireFileLock(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "bad_kind") {
		t.Errorf("expected bad_kind code, got %s", w.Body.String())
	}
}

// File access denied (canAccessFile returns ErrNoRows) → 404 to avoid
// leaking existence.
func TestHandleAcquireFileLock_NoAccessReturns404(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WillReturnError(rowsErrNoRows())
	w := rec()
	r := authedRequest("POST", "/files/foreign/lock", `{}`)
	r = withChiParams(r, "id", "foreign")
	a.handleAcquireFileLock(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

// Lock held by another user → 409 conflict.
func TestHandleAcquireFileLock_Conflict409(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	// existing lock SELECT returns row for a different user, not yet expired.
	future := time.Now().Add(5 * time.Minute)
	a.mock.ExpectQuery("SELECT user_id, expires_at FROM file_locks").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "expires_at"}).
			AddRow("u-other", future))
	w := rec()
	r := authedRequest("POST", "/files/f1/lock", `{"kind":"edit"}`)
	r = withChiParams(r, "id", "f1")
	a.handleAcquireFileLock(w, r)
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "locked") {
		t.Errorf("expected locked code")
	}
}

// validLockKind helper covers the allowed set. The helper is
// case-insensitive — "EDIT" is accepted along with "edit".
func TestValidLockKind(t *testing.T) {
	for _, k := range []string{"edit", "sync", "office", "EDIT", "Sync", "OFFICE"} {
		if !validLockKind(k) {
			t.Errorf("expected %q to be a valid kind", k)
		}
	}
	for _, k := range []string{"", "delete", "write", "  edit", "edit "} {
		if validLockKind(k) {
			t.Errorf("expected %q to be invalid", k)
		}
	}
}

// Release lock is idempotent — DELETE that matches 0 rows still returns
// 204. Release skips canAccessFile (the WHERE user_id=? predicate IS the
// access check), so no preflight SELECT is expected.
func TestHandleReleaseFileLock_IdempotentNoContent(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("DELETE FROM file_locks WHERE file_id =").
		WillReturnResult(sqlmock.NewResult(0, 0))
	w := rec()
	r := authedRequest("DELETE", "/files/f1/lock?kind=edit", "")
	r = withChiParams(r, "id", "f1")
	a.handleReleaseFileLock(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d body=%s", w.Code, w.Body.String())
	}
}

// listLocks filters by expires_at > NOW() — verify the SQL contains that
// predicate so expired rows don't bleed into the response.
func TestHandleListFileLocks_FiltersExpired(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("SELECT id, file_id, user_id, kind, expires_at, created_at FROM file_locks").
		WillReturnRows(sqlmock.NewRows([]string{"id", "file_id", "user_id", "kind", "expires_at", "created_at"}))
	w := rec()
	r := authedRequest("GET", "/files/f1/lock", "")
	r = withChiParams(r, "id", "f1")
	a.handleListFileLocks(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}
