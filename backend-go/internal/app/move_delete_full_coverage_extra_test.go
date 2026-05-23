package app

// Last-mile branches to push the move/delete handlers to literal 100%
// statement coverage.

import (
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ─── handleUpdateFolder uncovered branches ─────────────────────────────────

func TestHandleUpdateFolder_MalformedJSON_400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("PATCH", "/folders/f1", `{`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleUpdateFolder_ExistsCheckDBError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))
	a.mock.ExpectQuery("SELECT COUNT.*FROM folders WHERE id=").
		WillReturnError(errStr2("db down"))
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("PATCH", "/folders/f1", `{"parent_id":"dest"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleUpdateFolder_FolderDepthError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))
	a.mock.ExpectQuery("SELECT COUNT.*FROM folders WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	a.mock.ExpectQuery("WITH RECURSIVE chain").
		WillReturnError(errStr2("db down"))
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("PATCH", "/folders/f1", `{"parent_id":"dest"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleUpdateFolder_CycleQueryError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))
	a.mock.ExpectQuery("SELECT COUNT.*FROM folders WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	a.mock.ExpectQuery("WITH RECURSIVE chain").
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(int64(2)))
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnError(errStr2("db down"))
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("PATCH", "/folders/f1", `{"parent_id":"dest"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleUpdateFolder_CycleScanError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))
	a.mock.ExpectQuery("SELECT COUNT.*FROM folders WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	a.mock.ExpectQuery("WITH RECURSIVE chain").
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(int64(2)))
	// NULL into a non-nullable string scan target → scan err.
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(nil))
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("PATCH", "/folders/f1", `{"parent_id":"dest"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleUpdateFolder_UpdateError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))
	a.mock.ExpectExec("UPDATE folders SET parent_id").
		WillReturnError(errStr2("db down"))
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("PATCH", "/folders/f1", `{"parent_id":null}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleUpdateFolder_CommitError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))
	a.mock.ExpectExec("UPDATE folders SET parent_id").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit().WillReturnError(errStr2("commit failed"))
	w := rec()
	r := authedRequest("PATCH", "/folders/f1", `{"parent_id":null}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ─── handleRestoreTrash uncovered branch (lines 74-75: non-quota error) ────

func TestHandleRestoreTrash_ReserveQuotaNonQuotaError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"size"}).AddRow(int64(100)))
	// reserveQuota's UPDATE returns a generic DB error (not zero rows → not ErrQuotaExceeded)
	a.mock.ExpectExec("UPDATE users.*used_bytes.*\\+").
		WillReturnError(errStr2("db down"))
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("POST", "/trash/restore/f1", "")
	r = withChiParams(r, "id", "f1")
	a.handleRestoreTrash(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ─── handleEmptyTrash: folder permanentlyDelete error + scan body ──────────

func TestHandleEmptyTrash_PermanentlyDeleteFolderError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT id FROM files WHERE user_id=.* AND is_deleted=1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	a.mock.ExpectQuery("SELECT id FROM folders WHERE user_id=.* AND is_deleted=1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("fold1"))
	// permanentlyDeleteFolder will check existence
	a.mock.ExpectQuery("SELECT COUNT.*FROM folders WHERE id=.* AND user_id=.* AND is_deleted=1").
		WillReturnError(errStr2("db down"))
	w := rec()
	r := authedRequest("POST", "/trash:empty", "")
	a.handleEmptyTrash(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d body=%s", w.Code, w.Body.String())
	}
}
