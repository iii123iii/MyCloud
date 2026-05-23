package app

// Targeted-100% coverage for the file/folder move + delete code paths.
// Every branch in handleUpdateFile, handleDeleteFile, handleUpdateFolder,
// handleDeleteFolder, handleRestoreTrash, handleEmptyTrash, the batch*
// methods, the rollback* methods, and the unexported helpers
// (collectFolderTree, markFolderDeleted, restoreFolder, folderDepth) is
// driven from here.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ─── handleUpdateFile — every branch ───────────────────────────────────────

func TestHandleUpdateFile_IsStarred_BadType(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("PATCH", "/files/f1", `{"is_starred":"yes"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFile(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleUpdateFile_IsStarred_DBError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectExec("UPDATE files SET is_starred").
		WillReturnError(errStr2("connection lost"))
	w := rec()
	r := authedRequest("PATCH", "/files/f1", `{"is_starred":true}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFile(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleUpdateFile_IsStarred_NotFound(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectExec("UPDATE files SET is_starred").
		WillReturnResult(sqlmock.NewResult(0, 0))
	w := rec()
	r := authedRequest("PATCH", "/files/f1", `{"is_starred":true}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFile(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleUpdateFile_IsStarred_Success(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectExec("UPDATE files SET is_starred").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("INSERT INTO activity_log").
		WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("PATCH", "/files/f1", `{"is_starred":true}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFile(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateFile_Name_BadType(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("PATCH", "/files/f1", `{"name":42}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFile(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleUpdateFile_Name_EmptyAfterTrim(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("PATCH", "/files/f1", `{"name":"   "}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFile(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleUpdateFile_Name_DBError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectExec("UPDATE files SET name").
		WillReturnError(errStr2("connection lost"))
	w := rec()
	r := authedRequest("PATCH", "/files/f1", `{"name":"new.txt"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFile(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleUpdateFile_Name_NotFound(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectExec("UPDATE files SET name").
		WillReturnResult(sqlmock.NewResult(0, 0))
	w := rec()
	r := authedRequest("PATCH", "/files/f1", `{"name":"new.txt"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFile(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleUpdateFile_Name_Success(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectExec("UPDATE files SET name").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("INSERT INTO activity_log").
		WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("PATCH", "/files/f1", `{"name":"new.txt"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFile(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleUpdateFile_FolderID_BadType(t *testing.T) {
	a := newTestApp(t)
	// canAccessFile (editor) runs before the folder_id is parsed.
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	w := rec()
	r := authedRequest("PATCH", "/files/f1", `{"folder_id":42}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFile(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleUpdateFile_FolderID_MoveToRoot(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	// canAccessFile owner path; move-to-root by owner skips the dest query.
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectExec("UPDATE files SET folder_id").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("INSERT INTO activity_log").
		WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("PATCH", "/files/f1", `{"folder_id":null}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFile(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateFile_FolderID_TargetNotFound(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	// Destination canAccessFolder finds no folder → invalid_parent.
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("missing").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}))
	w := rec()
	r := authedRequest("PATCH", "/files/f1", `{"folder_id":"missing"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFile(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateFile_FolderID_ExistsCheckDBError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	// Destination folder access check hits a DB error.
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("dest").
		WillReturnError(errStr2("connection lost"))
	w := rec()
	r := authedRequest("PATCH", "/files/f1", `{"folder_id":"dest"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFile(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleUpdateFile_FolderID_UpdateDBError(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("dest").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectExec("UPDATE files SET folder_id").
		WillReturnError(errStr2("db down"))
	w := rec()
	r := authedRequest("PATCH", "/files/f1", `{"folder_id":"dest"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFile(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleUpdateFile_FolderID_UpdateNoRows(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("dest").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectExec("UPDATE files SET folder_id").
		WillReturnResult(sqlmock.NewResult(0, 0))
	w := rec()
	r := authedRequest("PATCH", "/files/f1", `{"folder_id":"dest"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFile(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleUpdateFile_FolderID_Success(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("dest").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectExec("UPDATE files SET folder_id").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("INSERT INTO activity_log").
		WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("PATCH", "/files/f1", `{"folder_id":"dest"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFile(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateFile_UnknownField_400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("PATCH", "/files/f1", `{"unknown":"x"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFile(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ─── handleDeleteFile — every branch ───────────────────────────────────────

func TestHandleDeleteFile_BeginTxError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin().WillReturnError(errStr2("tx failed"))
	w := rec()
	r := authedRequest("DELETE", "/files/f1", "")
	r = withChiParams(r, "id", "f1")
	a.handleDeleteFile(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleDeleteFile_NotFound(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WillReturnError(rowsErrNoRows())
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("DELETE", "/files/f1", "")
	r = withChiParams(r, "id", "f1")
	a.handleDeleteFile(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleDeleteFile_SelectDBError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WillReturnError(errStr2("db down"))
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("DELETE", "/files/f1", "")
	r = withChiParams(r, "id", "f1")
	a.handleDeleteFile(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleDeleteFile_UpdateDBError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"size"}).AddRow(int64(100)))
	a.mock.ExpectExec("UPDATE files SET is_deleted=1").
		WillReturnError(errStr2("db down"))
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("DELETE", "/files/f1", "")
	r = withChiParams(r, "id", "f1")
	a.handleDeleteFile(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleDeleteFile_ReleaseQuotaError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"size"}).AddRow(int64(100)))
	a.mock.ExpectExec("UPDATE files SET is_deleted=1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE users").
		WillReturnError(errStr2("quota release failed"))
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("DELETE", "/files/f1", "")
	r = withChiParams(r, "id", "f1")
	a.handleDeleteFile(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleDeleteFile_CommitError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"size"}).AddRow(int64(100)))
	a.mock.ExpectExec("UPDATE files SET is_deleted=1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE users").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit().WillReturnError(errStr2("commit failed"))
	w := rec()
	r := authedRequest("DELETE", "/files/f1", "")
	r = withChiParams(r, "id", "f1")
	a.handleDeleteFile(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleDeleteFile_Success_SizeZero(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"size"}).AddRow(int64(0)))
	a.mock.ExpectExec("UPDATE files SET is_deleted=1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// No UPDATE users expected — size is 0
	a.mock.ExpectCommit()
	a.mock.ExpectExec("INSERT INTO activity_log").
		WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("DELETE", "/files/f1", "")
	r = withChiParams(r, "id", "f1")
	a.handleDeleteFile(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleDeleteFile_Success_SizeNonZero(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"size"}).AddRow(int64(500)))
	a.mock.ExpectExec("UPDATE files SET is_deleted=1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE users").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec("INSERT INTO activity_log").
		WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("DELETE", "/files/f1", "")
	r = withChiParams(r, "id", "f1")
	a.handleDeleteFile(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handleUpdateFolder — every branch ─────────────────────────────────────

func TestHandleUpdateFolder_Name_BadType(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	w := rec()
	r := authedRequest("PATCH", "/folders/f1", `{"name":42}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleUpdateFolder_Name_EmptyAfterTrim(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	w := rec()
	r := authedRequest("PATCH", "/folders/f1", `{"name":"   "}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleUpdateFolder_Name_DBError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectExec("UPDATE folders SET name").
		WillReturnError(errStr2("db down"))
	w := rec()
	r := authedRequest("PATCH", "/folders/f1", `{"name":"newname"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleUpdateFolder_Name_NotFound(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectExec("UPDATE folders SET name").
		WillReturnResult(sqlmock.NewResult(0, 0))
	w := rec()
	r := authedRequest("PATCH", "/folders/f1", `{"name":"newname"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleUpdateFolder_Name_Success(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectExec("UPDATE folders SET name").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("INSERT INTO activity_log").
		WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("PATCH", "/folders/f1", `{"name":"newname"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateFolder_ParentID_BadType(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	w := rec()
	r := authedRequest("PATCH", "/folders/f1", `{"parent_id":42}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleUpdateFolder_ParentID_MoveIntoSelf(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("self").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	w := rec()
	r := authedRequest("PATCH", "/folders/self", `{"parent_id":"self"}`)
	r = withChiParams(r, "id", "self")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleUpdateFolder_ParentID_BeginTxError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin().WillReturnError(errStr2("tx failed"))
	w := rec()
	r := authedRequest("PATCH", "/folders/f1", `{"parent_id":"dest"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleUpdateFolder_ParentID_OldParent_NotFound(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WillReturnError(rowsErrNoRows())
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("PATCH", "/folders/f1", `{"parent_id":"dest"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleUpdateFolder_ParentID_OldParent_DBError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
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

func TestHandleUpdateFolder_ParentID_NewParent_NotFound(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))
	// Destination canAccessFolder finds no folder → invalid_parent.
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("missing").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}))
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("PATCH", "/folders/f1", `{"parent_id":"missing"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateFolder_ParentID_NewParent_DescendantCycle(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))
	// Destination canAccessFolder owner path — same owner as source.
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("dest").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("WITH RECURSIVE chain").
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(int64(5)))
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).
			AddRow("f1").
			AddRow("dest")) // dest is in f1's descendants → cycle
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("PATCH", "/folders/f1", `{"parent_id":"dest"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateFolder_ParentID_DepthTooDeep(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("deep").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("WITH RECURSIVE chain").
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(int64(MaxFolderDepth + 1)))
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("PATCH", "/folders/f1", `{"parent_id":"deep"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateFolder_ParentID_Success(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow("oldparent"))
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("newparent").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("WITH RECURSIVE chain").
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(int64(2)))
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("f1")) // no cycle
	a.mock.ExpectExec("UPDATE folders SET parent_id").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec("INSERT INTO activity_log").
		WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("PATCH", "/folders/f1", `{"parent_id":"newparent"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateFolder_ParentID_NullMoveToRoot(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow("oldparent"))
	a.mock.ExpectExec("UPDATE folders SET parent_id").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec("INSERT INTO activity_log").
		WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("PATCH", "/folders/f1", `{"parent_id":null}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateFolder_ParentID_UpdateRowsZero(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))
	a.mock.ExpectExec("UPDATE folders SET parent_id").
		WillReturnResult(sqlmock.NewResult(0, 0))
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("PATCH", "/folders/f1", `{"parent_id":null}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleUpdateFolder_Default_NoFields(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	w := rec()
	r := authedRequest("PATCH", "/folders/f1", `{"unknown":"field"}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ─── handleDeleteFolder — DB error path (success is already 100%) ──────────

func TestHandleDeleteFolder_DBError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnError(errStr2("db down"))
	w := rec()
	r := authedRequest("DELETE", "/folders/f1", "")
	r = withChiParams(r, "id", "f1")
	a.handleDeleteFolder(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ─── collectFolderTree — error/scan/empty branches ─────────────────────────

func TestCollectFolderTree_QueryError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnError(errStr2("db down"))
	if _, err := a.collectFolderTree(context.Background(), "u-test", "f1"); err == nil {
		t.Error("expected error")
	}
}

func TestCollectFolderTree_ScanError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).
			AddRow(nil)) // NULL into string → scan fails
	if _, err := a.collectFolderTree(context.Background(), "u-test", "f1"); err == nil {
		t.Error("expected scan error")
	}
}

func TestCollectFolderTree_Empty(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	ids, err := a.collectFolderTree(context.Background(), "u-test", "f1")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected empty, got %v", ids)
	}
}

func TestCollectFolderTree_Multi(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).
			AddRow("f1").AddRow("f2").AddRow("f3"))
	ids, err := a.collectFolderTree(context.Background(), "u-test", "f1")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(ids) != 3 {
		t.Errorf("expected 3 ids, got %v", ids)
	}
}

// ─── markFolderDeleted — every error branch ────────────────────────────────

func TestMarkFolderDeleted_CollectError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnError(errStr2("db down"))
	if err := a.markFolderDeleted(context.Background(), "u-test", "f1"); err == nil {
		t.Error("expected error")
	}
}

func TestMarkFolderDeleted_EmptyTreeIsNoop(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if err := a.markFolderDeleted(context.Background(), "u-test", "f1"); err != nil {
		t.Errorf("empty tree should be nil, got %v", err)
	}
}

func TestMarkFolderDeleted_BeginTxError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("f1"))
	a.mock.ExpectBegin().WillReturnError(errStr2("tx failed"))
	if err := a.markFolderDeleted(context.Background(), "u-test", "f1"); err == nil {
		t.Error("expected tx begin error")
	}
}

func TestMarkFolderDeleted_UpdateFoldersError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("f1"))
	a.mock.ExpectBegin()
	a.mock.ExpectExec("UPDATE folders SET is_deleted").
		WillReturnError(errStr2("update failed"))
	a.mock.ExpectRollback()
	if err := a.markFolderDeleted(context.Background(), "u-test", "f1"); err == nil {
		t.Error("expected update error")
	}
}

func TestMarkFolderDeleted_SumError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("f1"))
	a.mock.ExpectBegin()
	a.mock.ExpectExec("UPDATE folders SET is_deleted").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectQuery("SELECT COALESCE.SUM.size_bytes").
		WillReturnError(errStr2("sum failed"))
	a.mock.ExpectRollback()
	if err := a.markFolderDeleted(context.Background(), "u-test", "f1"); err == nil {
		t.Error("expected sum error")
	}
}

func TestMarkFolderDeleted_UpdateFilesError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("f1"))
	a.mock.ExpectBegin()
	a.mock.ExpectExec("UPDATE folders SET is_deleted").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectQuery("SELECT COALESCE.SUM.size_bytes").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(0)))
	a.mock.ExpectExec("UPDATE files SET is_deleted").
		WillReturnError(errStr2("update files failed"))
	a.mock.ExpectRollback()
	if err := a.markFolderDeleted(context.Background(), "u-test", "f1"); err == nil {
		t.Error("expected files update error")
	}
}

func TestMarkFolderDeleted_ReleaseQuotaError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("f1"))
	a.mock.ExpectBegin()
	a.mock.ExpectExec("UPDATE folders SET is_deleted").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectQuery("SELECT COALESCE.SUM.size_bytes").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(100)))
	a.mock.ExpectExec("UPDATE files SET is_deleted").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE users").
		WillReturnError(errStr2("release failed"))
	a.mock.ExpectRollback()
	if err := a.markFolderDeleted(context.Background(), "u-test", "f1"); err == nil {
		t.Error("expected releaseQuota error")
	}
}

func TestMarkFolderDeleted_CommitError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("f1"))
	a.mock.ExpectBegin()
	a.mock.ExpectExec("UPDATE folders SET is_deleted").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectQuery("SELECT COALESCE.SUM.size_bytes").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(0)))
	a.mock.ExpectExec("UPDATE files SET is_deleted").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit().WillReturnError(errStr2("commit failed"))
	if err := a.markFolderDeleted(context.Background(), "u-test", "f1"); err == nil {
		t.Error("expected commit error")
	}
}

func TestMarkFolderDeleted_Success(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("f1"))
	a.mock.ExpectBegin()
	a.mock.ExpectExec("UPDATE folders SET is_deleted").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectQuery("SELECT COALESCE.SUM.size_bytes").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(0)))
	a.mock.ExpectExec("UPDATE files SET is_deleted").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	if err := a.markFolderDeleted(context.Background(), "u-test", "f1"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// ─── restoreFolder — every error branch ────────────────────────────────────

func TestRestoreFolder_CollectError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnError(errStr2("db down"))
	if err := a.restoreFolder(context.Background(), "u-test", "f1"); err == nil {
		t.Error("expected collect error")
	}
}

func TestRestoreFolder_EmptyTreeNoop(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if err := a.restoreFolder(context.Background(), "u-test", "f1"); err != nil {
		t.Errorf("empty tree should be no-op, got %v", err)
	}
}

func TestRestoreFolder_BeginTxError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("f1"))
	a.mock.ExpectBegin().WillReturnError(errStr2("tx failed"))
	if err := a.restoreFolder(context.Background(), "u-test", "f1"); err == nil {
		t.Error("expected tx begin error")
	}
}

func TestRestoreFolder_SumError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("f1"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT COALESCE.SUM.size_bytes").
		WillReturnError(errStr2("sum failed"))
	a.mock.ExpectRollback()
	if err := a.restoreFolder(context.Background(), "u-test", "f1"); err == nil {
		t.Error("expected sum error")
	}
}

func TestRestoreFolder_QuotaExceeded(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("f1"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT COALESCE.SUM.size_bytes").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(1000)))
	a.mock.ExpectExec("UPDATE users.*used_bytes.*\\+").
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows → ErrQuotaExceeded
	a.mock.ExpectQuery("SELECT used_bytes, quota_bytes FROM users WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"used", "quota"}).AddRow(int64(900), int64(1000)))
	a.mock.ExpectRollback()
	err := a.restoreFolder(context.Background(), "u-test", "f1")
	if err == nil {
		t.Error("expected quota-exceeded error")
	}
}

func TestRestoreFolder_UpdateFoldersError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("f1"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT COALESCE.SUM.size_bytes").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(0)))
	a.mock.ExpectExec("UPDATE folders SET is_deleted=0").
		WillReturnError(errStr2("update failed"))
	a.mock.ExpectRollback()
	if err := a.restoreFolder(context.Background(), "u-test", "f1"); err == nil {
		t.Error("expected update folders error")
	}
}

func TestRestoreFolder_UpdateFilesError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("f1"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT COALESCE.SUM.size_bytes").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(0)))
	a.mock.ExpectExec("UPDATE folders SET is_deleted=0").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE files SET is_deleted=0").
		WillReturnError(errStr2("update files failed"))
	a.mock.ExpectRollback()
	if err := a.restoreFolder(context.Background(), "u-test", "f1"); err == nil {
		t.Error("expected update files error")
	}
}

func TestRestoreFolder_CommitError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("f1"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT COALESCE.SUM.size_bytes").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(0)))
	a.mock.ExpectExec("UPDATE folders SET is_deleted=0").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE files SET is_deleted=0").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit().WillReturnError(errStr2("commit failed"))
	if err := a.restoreFolder(context.Background(), "u-test", "f1"); err == nil {
		t.Error("expected commit error")
	}
}

func TestRestoreFolder_Success_NeedNoQuota(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("f1"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT COALESCE.SUM.size_bytes").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(0)))
	a.mock.ExpectExec("UPDATE folders SET is_deleted=0").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE files SET is_deleted=0").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	if err := a.restoreFolder(context.Background(), "u-test", "f1"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestRestoreFolder_Success_WithQuota(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("f1"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT COALESCE.SUM.size_bytes").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(500)))
	a.mock.ExpectExec("UPDATE users.*used_bytes.*\\+").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE folders SET is_deleted=0").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE files SET is_deleted=0").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	if err := a.restoreFolder(context.Background(), "u-test", "f1"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// ─── folderDepth ────────────────────────────────────────────────────────────

func TestFolderDepth_EmptyParent(t *testing.T) {
	a := newTestApp(t)
	got, err := a.folderDepth(context.Background(), "")
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestFolderDepth_DBError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE chain").
		WillReturnError(errStr2("db down"))
	if _, err := a.folderDepth(context.Background(), "f1"); err == nil {
		t.Error("expected error")
	}
}

func TestFolderDepth_NULL(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE chain").
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(nil))
	got, err := a.folderDepth(context.Background(), "f1")
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Errorf("expected 0 for NULL, got %d", got)
	}
}

func TestFolderDepth_ValidNumber(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE chain").
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(int64(42)))
	got, err := a.folderDepth(context.Background(), "f1")
	if err != nil {
		t.Fatal(err)
	}
	if got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
}

// ─── handleRestoreTrash — every branch ─────────────────────────────────────

func TestHandleRestoreTrash_BeginTxError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin().WillReturnError(errStr2("tx failed"))
	w := rec()
	r := authedRequest("POST", "/trash/restore/x", "")
	r = withChiParams(r, "id", "x")
	a.handleRestoreTrash(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleRestoreTrash_FileSuccess_WithQuota(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"size"}).AddRow(int64(100)))
	a.mock.ExpectExec("UPDATE users.*used_bytes.*\\+").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE files SET is_deleted=0").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec("INSERT INTO activity_log").
		WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("POST", "/trash/restore/f1", "")
	r = withChiParams(r, "id", "f1")
	a.handleRestoreTrash(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleRestoreTrash_FileZeroSizeSuccess(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"size"}).AddRow(int64(0)))
	a.mock.ExpectExec("UPDATE files SET is_deleted=0").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec("INSERT INTO activity_log").
		WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("POST", "/trash/restore/f1", "")
	r = withChiParams(r, "id", "f1")
	a.handleRestoreTrash(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleRestoreTrash_FileQuotaExceeded(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"size"}).AddRow(int64(1000)))
	a.mock.ExpectExec("UPDATE users.*used_bytes.*\\+").
		WillReturnResult(sqlmock.NewResult(0, 0))
	a.mock.ExpectQuery("SELECT used_bytes, quota_bytes FROM users WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"used", "quota"}).AddRow(int64(900), int64(1000)))
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("POST", "/trash/restore/f1", "")
	r = withChiParams(r, "id", "f1")
	a.handleRestoreTrash(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleRestoreTrash_FileUpdateError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"size"}).AddRow(int64(0)))
	a.mock.ExpectExec("UPDATE files SET is_deleted=0").
		WillReturnError(errStr2("update failed"))
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("POST", "/trash/restore/f1", "")
	r = withChiParams(r, "id", "f1")
	a.handleRestoreTrash(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleRestoreTrash_FileCommitError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"size"}).AddRow(int64(0)))
	a.mock.ExpectExec("UPDATE files SET is_deleted=0").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit().WillReturnError(errStr2("commit failed"))
	w := rec()
	r := authedRequest("POST", "/trash/restore/f1", "")
	r = withChiParams(r, "id", "f1")
	a.handleRestoreTrash(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleRestoreTrash_FileSelectOtherError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WillReturnError(errStr2("db down"))
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("POST", "/trash/restore/x", "")
	r = withChiParams(r, "id", "x")
	a.handleRestoreTrash(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleRestoreTrash_NotFile_FolderCountError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WillReturnError(rowsErrNoRows())
	a.mock.ExpectRollback()
	a.mock.ExpectQuery("SELECT COUNT.*FROM folders WHERE id=").
		WillReturnError(errStr2("db down"))
	w := rec()
	r := authedRequest("POST", "/trash/restore/x", "")
	r = withChiParams(r, "id", "x")
	a.handleRestoreTrash(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleRestoreTrash_NotFound(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WillReturnError(rowsErrNoRows())
	a.mock.ExpectRollback()
	a.mock.ExpectQuery("SELECT COUNT.*FROM folders WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	w := rec()
	r := authedRequest("POST", "/trash/restore/x", "")
	r = withChiParams(r, "id", "x")
	a.handleRestoreTrash(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleRestoreTrash_Folder_QuotaExceeded(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WillReturnError(rowsErrNoRows())
	a.mock.ExpectRollback()
	a.mock.ExpectQuery("SELECT COUNT.*FROM folders WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// restoreFolder path
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("f1"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT COALESCE.SUM.size_bytes").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(1000)))
	a.mock.ExpectExec("UPDATE users.*used_bytes.*\\+").
		WillReturnResult(sqlmock.NewResult(0, 0))
	a.mock.ExpectQuery("SELECT used_bytes, quota_bytes FROM users WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"used", "quota"}).AddRow(int64(900), int64(1000)))
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("POST", "/trash/restore/f1", "")
	r = withChiParams(r, "id", "f1")
	a.handleRestoreTrash(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleRestoreTrash_Folder_OtherError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WillReturnError(rowsErrNoRows())
	a.mock.ExpectRollback()
	a.mock.ExpectQuery("SELECT COUNT.*FROM folders WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnError(errStr2("db down"))
	w := rec()
	r := authedRequest("POST", "/trash/restore/f1", "")
	r = withChiParams(r, "id", "f1")
	a.handleRestoreTrash(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleRestoreTrash_Folder_Success(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WillReturnError(rowsErrNoRows())
	a.mock.ExpectRollback()
	a.mock.ExpectQuery("SELECT COUNT.*FROM folders WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("f1"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT COALESCE.SUM.size_bytes").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(0)))
	a.mock.ExpectExec("UPDATE folders SET is_deleted=0").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE files SET is_deleted=0").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec("INSERT INTO activity_log").
		WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("POST", "/trash/restore/f1", "")
	r = withChiParams(r, "id", "f1")
	a.handleRestoreTrash(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handleEmptyTrash — every branch ───────────────────────────────────────

func TestHandleEmptyTrash_FileSelectError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT id FROM files WHERE user_id=.* AND is_deleted=1").
		WillReturnError(errStr2("db down"))
	w := rec()
	r := authedRequest("POST", "/trash:empty", "")
	a.handleEmptyTrash(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleEmptyTrash_PermanentlyDeleteFileError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT id FROM files WHERE user_id=.* AND is_deleted=1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("f1"))
	a.mock.ExpectQuery("SELECT storage_path, size_bytes FROM files WHERE id=").
		WillReturnError(errStr2("db down"))
	w := rec()
	r := authedRequest("POST", "/trash:empty", "")
	a.handleEmptyTrash(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleEmptyTrash_FolderSelectError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT id FROM files WHERE user_id=.* AND is_deleted=1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	a.mock.ExpectQuery("SELECT id FROM folders WHERE user_id=.* AND is_deleted=1").
		WillReturnError(errStr2("db down"))
	w := rec()
	r := authedRequest("POST", "/trash:empty", "")
	a.handleEmptyTrash(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleEmptyTrash_NoItems_Success(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT id FROM files WHERE user_id=.* AND is_deleted=1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	a.mock.ExpectQuery("SELECT id FROM folders WHERE user_id=.* AND is_deleted=1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	a.mock.ExpectExec("INSERT INTO activity_log").
		WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("POST", "/trash:empty", "")
	a.handleEmptyTrash(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── batchMove additional branches ─────────────────────────────────────────

func TestBatchMove_OldFolderNotValid_NewNil(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("SELECT folder_id FROM files WHERE id").
		WillReturnRows(sqlmock.NewRows([]string{"folder_id"}).AddRow("oldfolder"))
	a.mock.ExpectExec("UPDATE files SET folder_id").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("INSERT INTO activity_log").
		WillReturnResult(sqlmock.NewResult(0, 1))
	res := a.batchMove(context.Background(), "u-test", "f1", nil, "ip", "op")
	if !res.OK {
		t.Errorf("expected OK, got error=%q", res.Error)
	}
}

func TestBatchMove_SelectFolderIDError(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("SELECT folder_id FROM files WHERE id").
		WillReturnError(errStr2("db down"))
	res := a.batchMove(context.Background(), "u-test", "f1", nil, "ip", "op")
	if res.OK {
		t.Error("expected OK=false on select error")
	}
}

func TestBatchMove_UpdateError(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("SELECT folder_id FROM files WHERE id").
		WillReturnRows(sqlmock.NewRows([]string{"folder_id"}).AddRow(nil))
	a.mock.ExpectExec("UPDATE files SET folder_id").
		WillReturnError(errStr2("db down"))
	res := a.batchMove(context.Background(), "u-test", "f1", nil, "ip", "op")
	if res.OK {
		t.Error("expected OK=false on update error")
	}
}

func TestBatchMove_UpdateZeroRows(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("SELECT folder_id FROM files WHERE id").
		WillReturnRows(sqlmock.NewRows([]string{"folder_id"}).AddRow(nil))
	a.mock.ExpectExec("UPDATE files SET folder_id").
		WillReturnResult(sqlmock.NewResult(0, 0))
	res := a.batchMove(context.Background(), "u-test", "f1", nil, "ip", "op")
	if res.OK {
		t.Error("expected OK=false when 0 rows updated")
	}
}

func TestBatchMove_NewFolderEmptyString_AccessFails(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("dest").
		WillReturnError(errStr2("permission denied"))
	dst := "dest"
	res := a.batchMove(context.Background(), "u-test", "f1", &dst, "ip", "op")
	if res.OK {
		t.Error("expected OK=false on dest access failure")
	}
}

// ─── batchSoftDelete: UPDATE error path ────────────────────────────────────

func TestBatchSoftDelete_UpdateError(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectExec("UPDATE files SET is_deleted").
		WillReturnError(errStr2("db down"))
	res := a.batchSoftDelete(context.Background(), "u-test", "f1", "ip", "op")
	if res.OK {
		t.Error("expected OK=false on update error")
	}
}

// ─── batchRestore: UPDATE error path ───────────────────────────────────────

func TestBatchRestore_NoAccess(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnError(rowsErrNoRows())
	res := a.batchRestore(context.Background(), "u-test", "f1", "ip", "op")
	if res.OK {
		t.Error("expected OK=false on no-access")
	}
}

func TestBatchRestore_UpdateError(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectExec("UPDATE files SET is_deleted = 0").
		WillReturnError(errStr2("db down"))
	res := a.batchRestore(context.Background(), "u-test", "f1", "ip", "op")
	if res.OK {
		t.Error("expected OK=false on update error")
	}
}

// ─── rollbackFileRename — every branch ─────────────────────────────────────

func TestRollbackFileRename_BadOldJSON(t *testing.T) {
	a := newTestApp(t)
	err := rollbackFileRename(context.Background(), a.App, "f1",
		json.RawMessage(`not-json`), json.RawMessage(`{"name":"new"}`))
	if err == nil {
		t.Error("expected decode error")
	}
}

func TestRollbackFileRename_BadNewJSON(t *testing.T) {
	a := newTestApp(t)
	err := rollbackFileRename(context.Background(), a.App, "f1",
		json.RawMessage(`{"name":"old"}`), json.RawMessage(`not-json`))
	if err == nil {
		t.Error("expected decode error")
	}
}

func TestRollbackFileRename_UpdateError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("UPDATE files SET name").
		WillReturnError(errStr2("db down"))
	err := rollbackFileRename(context.Background(), a.App, "f1",
		json.RawMessage(`{"name":"old"}`), json.RawMessage(`{"name":"new"}`))
	if err == nil {
		t.Error("expected db error")
	}
}

func TestRollbackFileRename_ZeroRows(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("UPDATE files SET name").
		WillReturnResult(sqlmock.NewResult(0, 0))
	err := rollbackFileRename(context.Background(), a.App, "f1",
		json.RawMessage(`{"name":"old"}`), json.RawMessage(`{"name":"new"}`))
	if err == nil {
		t.Error("expected error when name has changed since")
	}
}

func TestRollbackFileRename_Success(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("UPDATE files SET name").
		WillReturnResult(sqlmock.NewResult(0, 1))
	err := rollbackFileRename(context.Background(), a.App, "f1",
		json.RawMessage(`{"name":"old"}`), json.RawMessage(`{"name":"new"}`))
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// ─── rollbackFileMove — every branch ───────────────────────────────────────

func TestRollbackFileMove_BadOldJSON(t *testing.T) {
	a := newTestApp(t)
	err := rollbackFileMove(context.Background(), a.App, "f1",
		json.RawMessage(`not-json`), json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected decode error")
	}
}

func TestRollbackFileMove_BadNewJSON(t *testing.T) {
	a := newTestApp(t)
	err := rollbackFileMove(context.Background(), a.App, "f1",
		json.RawMessage(`{}`), json.RawMessage(`not-json`))
	if err == nil {
		t.Error("expected decode error")
	}
}

func TestRollbackFileMove_UpdateError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("UPDATE files SET folder_id").
		WillReturnError(errStr2("db down"))
	err := rollbackFileMove(context.Background(), a.App, "f1",
		json.RawMessage(`{"folder_id":null}`), json.RawMessage(`{"folder_id":"new"}`))
	if err == nil {
		t.Error("expected db error")
	}
}

func TestRollbackFileMove_ZeroRows(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("UPDATE files SET folder_id").
		WillReturnResult(sqlmock.NewResult(0, 0))
	err := rollbackFileMove(context.Background(), a.App, "f1",
		json.RawMessage(`{"folder_id":null}`), json.RawMessage(`{"folder_id":"new"}`))
	if err == nil {
		t.Error("expected error when folder has changed since")
	}
}

func TestRollbackFileMove_Success(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("UPDATE files SET folder_id").
		WillReturnResult(sqlmock.NewResult(0, 1))
	err := rollbackFileMove(context.Background(), a.App, "f1",
		json.RawMessage(`{"folder_id":null}`), json.RawMessage(`{"folder_id":"new"}`))
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// ─── rollbackFileSoftDelete — every branch ─────────────────────────────────

func TestRollbackFileSoftDelete_UpdateError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("UPDATE files SET is_deleted = 0").
		WillReturnError(errStr2("db down"))
	err := rollbackFileSoftDelete(context.Background(), a.App, "f1", nil, nil)
	if err == nil {
		t.Error("expected db error")
	}
}

func TestRollbackFileSoftDelete_ZeroRows(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("UPDATE files SET is_deleted = 0").
		WillReturnResult(sqlmock.NewResult(0, 0))
	err := rollbackFileSoftDelete(context.Background(), a.App, "f1", nil, nil)
	if err == nil {
		t.Error("expected error when file not in trash")
	}
}

func TestRollbackFileSoftDelete_Success(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("UPDATE files SET is_deleted = 0").
		WillReturnResult(sqlmock.NewResult(0, 1))
	err := rollbackFileSoftDelete(context.Background(), a.App, "f1", nil, nil)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}
