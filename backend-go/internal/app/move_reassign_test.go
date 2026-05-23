package app

// Unit tests for cross-owner move + ownership reassignment (move_helpers.go,
// the handleUpdateFile/handleUpdateFolder move branches). A move that keeps the
// same owner is a plain re-parent; a move whose destination has a different
// owner reassigns the moved item(s) to that owner and transfers the bytes
// between the two owners' quotas. This includes the "move from my own folder
// into a shared folder" direction.
//
// sqlmock regexp matcher: patterns avoid (, ), ?, +, * by stopping before them;
// releaseQuota vs reserveQuota are told apart by "GREATEST" vs "used_bytes =
// used_bytes".

import (
	"context"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// The batch move path also reassigns on a cross-owner move. Exercises
// batchMove directly (the multi-select toolbar calls it per item).
func TestBatchMove_CrossOwner_Reassigns(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-owner"))
	a.mock.ExpectQuery("WITH RECURSIVE ancestors").
		WithArgs("f1", "u-test", "f1").WillReturnRows(sqlmock.NewRows([]string{"permission"}).AddRow("editor"))
	// Capture current folder for the diff.
	a.mock.ExpectQuery("SELECT folder_id FROM files WHERE id =").
		WithArgs("f1").WillReturnRows(sqlmock.NewRows([]string{"folder_id"}).AddRow("sharedF"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WithArgs("f1").WillReturnRows(sqlmock.NewRows([]string{"size_bytes"}).AddRow(int64(100)))
	a.mock.ExpectExec("UPDATE users SET used_bytes = GREATEST").
		WithArgs(int64(100), "u-owner").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE users SET used_bytes = used_bytes").
		WithArgs(int64(100), "u-test", int64(100)).WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE files SET folder_id").
		WithArgs(nil, "u-test", "f1").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("DELETE FROM share_grants WHERE file_id=").
		WithArgs("f1", "u-test").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec("INSERT INTO activity_log").WillReturnResult(sqlmock.NewResult(0, 1))

	res := a.batchMove(context.Background(), "u-test", "f1", nil, "1.2.3.4", "op1")
	if !res.OK {
		t.Fatalf("expected ok, got error: %s", res.Error)
	}
}

// Editor pulls a shared file into their own root: cross-owner reassignment,
// quota moves from the owner to the editor, the editor's grant is dropped.
func TestMoveFile_SharedToOwnRoot_Reassigns(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-owner"))
	a.mock.ExpectQuery("WITH RECURSIVE ancestors").
		WithArgs("f1", "u-test", "f1").WillReturnRows(sqlmock.NewRows([]string{"permission"}).AddRow("editor"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WithArgs("f1").WillReturnRows(sqlmock.NewRows([]string{"size_bytes"}).AddRow(int64(100)))
	a.mock.ExpectExec("UPDATE users SET used_bytes = GREATEST").
		WithArgs(int64(100), "u-owner").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE users SET used_bytes = used_bytes").
		WithArgs(int64(100), "u-test", int64(100)).WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE files SET folder_id").
		WithArgs(nil, "u-test", "f1").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("DELETE FROM share_grants WHERE file_id=").
		WithArgs("f1", "u-test").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec("INSERT INTO activity_log").WillReturnResult(sqlmock.NewResult(0, 1))

	w := rec()
	r := withChiParams(authedRequest("PATCH", "/files/f1", `{"folder_id":null}`), "id", "f1")
	a.handleUpdateFile(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// Owner moves their OWN file into a folder shared with them (editor): the file
// is reassigned to the shared folder's owner and charged to that owner's quota.
func TestMoveFile_RegularIntoSharedFolder_Reassigns(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("sharedF").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-owner"))
	a.mock.ExpectQuery("WITH RECURSIVE ancestors").
		WithArgs("sharedF", "u-test").WillReturnRows(sqlmock.NewRows([]string{"permission"}).AddRow("editor"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WithArgs("f1").WillReturnRows(sqlmock.NewRows([]string{"size_bytes"}).AddRow(int64(100)))
	a.mock.ExpectExec("UPDATE users SET used_bytes = GREATEST").
		WithArgs(int64(100), "u-test").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE users SET used_bytes = used_bytes").
		WithArgs(int64(100), "u-owner", int64(100)).WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE files SET folder_id").
		WithArgs("sharedF", "u-owner", "f1").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("DELETE FROM share_grants WHERE file_id=").
		WithArgs("f1", "u-owner").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec("INSERT INTO activity_log").WillReturnResult(sqlmock.NewResult(0, 1))

	w := rec()
	r := withChiParams(authedRequest("PATCH", "/files/f1", `{"folder_id":"sharedF"}`), "id", "f1")
	a.handleUpdateFile(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// Same-owner move: a plain re-parent, no transaction / reassignment / quota.
func TestMoveFile_SameOwner_SimpleMove(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("F2").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectExec("UPDATE files SET folder_id").
		WithArgs("F2", "f1").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("INSERT INTO activity_log").WillReturnResult(sqlmock.NewResult(0, 1))

	w := rec()
	r := withChiParams(authedRequest("PATCH", "/files/f1", `{"folder_id":"F2"}`), "id", "f1")
	a.handleUpdateFile(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// A viewer cannot move (the move path needs editor access) → 404.
func TestMoveFile_ViewerCannotMove(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-owner"))
	a.mock.ExpectQuery("WITH RECURSIVE ancestors").
		WithArgs("f1", "u-test", "f1").WillReturnRows(sqlmock.NewRows([]string{"permission"}).AddRow("viewer"))

	w := rec()
	r := withChiParams(authedRequest("PATCH", "/files/f1", `{"folder_id":null}`), "id", "f1")
	a.handleUpdateFile(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

// Cross-owner move whose destination owner is over quota → 413.
func TestMoveFile_CrossOwnerQuotaExceeded(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-owner"))
	a.mock.ExpectQuery("WITH RECURSIVE ancestors").
		WithArgs("f1", "u-test", "f1").WillReturnRows(sqlmock.NewRows([]string{"permission"}).AddRow("editor"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WithArgs("f1").WillReturnRows(sqlmock.NewRows([]string{"size_bytes"}).AddRow(int64(100)))
	a.mock.ExpectExec("UPDATE users SET used_bytes = GREATEST").
		WithArgs(int64(100), "u-owner").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE users SET used_bytes = used_bytes").
		WillReturnResult(sqlmock.NewResult(0, 0)) // dest over quota
	a.mock.ExpectQuery("SELECT used_bytes, quota_bytes FROM users").
		WillReturnRows(sqlmock.NewRows([]string{"used_bytes", "quota_bytes"}).AddRow(int64(990), int64(1000)))
	a.mock.ExpectRollback()

	w := rec()
	r := withChiParams(authedRequest("PATCH", "/files/f1", `{"folder_id":null}`), "id", "f1")
	a.handleUpdateFile(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d body=%s", w.Code, w.Body.String())
	}
}

// Editor moves a shared FOLDER into their own root: whole subtree reassigned,
// quota transferred, the editor's grant on the moved root dropped.
func TestMoveFolder_SharedToOwnRoot_Reassigns(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("F1").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-owner")) // access
	a.mock.ExpectQuery("WITH RECURSIVE ancestors").
		WithArgs("F1", "u-test").WillReturnRows(sqlmock.NewRows([]string{"permission"}).AddRow("editor"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WithArgs("F1").WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow("oldp"))
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WithArgs("F1", "u-owner", "u-owner").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("F1")) // collectFolderTree
	a.mock.ExpectQuery("SELECT COALESCE").
		WithArgs("u-owner", "F1").WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(200)))
	a.mock.ExpectExec("UPDATE users SET used_bytes = GREATEST").
		WithArgs(int64(200), "u-owner").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE users SET used_bytes = used_bytes").
		WithArgs(int64(200), "u-test", int64(200)).WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE folders SET user_id").
		WithArgs("u-test", "F1").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE files SET user_id").
		WithArgs("u-test", "F1").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE folders SET parent_id").
		WithArgs(nil, "F1").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("DELETE FROM share_grants WHERE folder_id=").
		WithArgs("F1", "u-test").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec("INSERT INTO activity_log").WillReturnResult(sqlmock.NewResult(0, 1))

	w := rec()
	r := withChiParams(authedRequest("PATCH", "/folders/F1", `{"parent_id":null}`), "id", "F1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// Owner moves their OWN folder into a shared folder (editor): subtree reassigned
// to the shared folder's owner. Exercises the dest access + depth + cycle checks
// before the cross-owner reassignment.
func TestMoveFolder_RegularIntoSharedFolder_Reassigns(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("F1").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test")) // access (own)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WithArgs("F1").WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("sharedF").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-owner")) // dest access
	a.mock.ExpectQuery("WITH RECURSIVE ancestors").
		WithArgs("sharedF", "u-test").WillReturnRows(sqlmock.NewRows([]string{"permission"}).AddRow("editor"))
	a.mock.ExpectQuery("WITH RECURSIVE chain").
		WithArgs("sharedF").WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(int64(1))) // folderDepth
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WithArgs("F1", "u-test", "u-test").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("F1")) // cycle check
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WithArgs("F1", "u-test", "u-test").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("F1")) // collectFolderTree
	a.mock.ExpectQuery("SELECT COALESCE").
		WithArgs("u-test", "F1").WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(200)))
	a.mock.ExpectExec("UPDATE users SET used_bytes = GREATEST").
		WithArgs(int64(200), "u-test").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE users SET used_bytes = used_bytes").
		WithArgs(int64(200), "u-owner", int64(200)).WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE folders SET user_id").
		WithArgs("u-owner", "F1").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE files SET user_id").
		WithArgs("u-owner", "F1").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE folders SET parent_id").
		WithArgs("sharedF", "F1").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("DELETE FROM share_grants WHERE folder_id=").
		WithArgs("F1", "u-owner").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec("INSERT INTO activity_log").WillReturnResult(sqlmock.NewResult(0, 1))

	w := rec()
	r := withChiParams(authedRequest("PATCH", "/folders/F1", `{"parent_id":"sharedF"}`), "id", "F1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}
