package app

import (
	"net/http"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ─── handleRestoreTrash ─────────────────────────────────────────────────────

// Re-debiting quota when restoring a file: if reserveQuota fails with
// ErrQuotaExceeded, the handler must surface 413 (not 500), so the UI can
// explain the user is out of space.
func TestHandleRestoreTrash_FileQuotaExceededReturns413(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WithArgs("f1", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"size_bytes"}).AddRow(int64(10_000)))
	// reserveQuota's atomic UPDATE matches zero rows → ErrQuotaExceeded.
	a.mock.ExpectExec("UPDATE users SET used_bytes").
		WithArgs(int64(10_000), "u-test", int64(10_000)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	// reserveQuota also runs a diagnostic SELECT for logging.
	a.mock.ExpectQuery("SELECT used_bytes, quota_bytes FROM users WHERE id=").
		WithArgs("u-test").
		WillReturnRows(sqlmock.NewRows([]string{"used", "quota"}).AddRow(int64(0), int64(0)))
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("POST", "/trash/f1:restore", "")
	r = withChiParams(r, "id", "f1")
	a.handleRestoreTrash(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "quota_exceeded") {
		t.Errorf("expected quota_exceeded code")
	}
}

// Successful restore: re-debits quota then clears is_deleted.
func TestHandleRestoreTrash_FileSuccess(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WithArgs("f1", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"size_bytes"}).AddRow(int64(500)))
	a.mock.ExpectExec("UPDATE users SET used_bytes").
		WithArgs(int64(500), "u-test", int64(500)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE files SET is_deleted=0").
		WithArgs("f1", "u-test").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	// writeActivity insert (any).
	a.mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("POST", "/trash/f1:restore", "")
	r = withChiParams(r, "id", "f1")
	a.handleRestoreTrash(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// Zero-byte file restore: no quota re-debit needed.
func TestHandleRestoreTrash_ZeroByteFileSkipsQuotaReserve(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WithArgs("f1", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"size_bytes"}).AddRow(int64(0)))
	// No reserveQuota call expected for zero size.
	a.mock.ExpectExec("UPDATE files SET is_deleted=0").
		WithArgs("f1", "u-test").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("POST", "/trash/f1:restore", "")
	r = withChiParams(r, "id", "f1")
	a.handleRestoreTrash(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// Neither file nor folder found → 404.
func TestHandleRestoreTrash_NotFound404(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WithArgs("ghost", "u-test").
		WillReturnError(rowsErrNoRows())
	a.mock.ExpectRollback()
	a.mock.ExpectQuery("SELECT COUNT.\\*. FROM folders WHERE id=").
		WithArgs("ghost", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
	w := rec()
	r := authedRequest("POST", "/trash/ghost:restore", "")
	r = withChiParams(r, "id", "ghost")
	a.handleRestoreTrash(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handleDeleteFile (soft-delete + quota release) ─────────────────────────

func TestHandleDeleteFile_ReleasesQuota(t *testing.T) {
	a := newTestApp(t)
	// canAccessFile owner path.
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"size_bytes"}).AddRow(int64(1000)))
	a.mock.ExpectExec("UPDATE files SET is_deleted=1").
		WithArgs("f1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// releaseQuota
	a.mock.ExpectExec("UPDATE users SET used_bytes").
		WithArgs(int64(1000), "u-test").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("DELETE", "/files/f1", "")
	r = withChiParams(r, "id", "f1")
	a.handleDeleteFile(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleDeleteFile_NotFound404(t *testing.T) {
	a := newTestApp(t)
	// canAccessFile finds no such file → 404 before the tx opens.
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("ghost").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}))
	w := rec()
	r := authedRequest("DELETE", "/files/ghost", "")
	r = withChiParams(r, "id", "ghost")
	a.handleDeleteFile(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}
