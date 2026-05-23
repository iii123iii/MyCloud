package app

// Tests dedicated to the FOLDER DELETE path: soft-delete via
// handleDeleteFolder/markFolderDeleted, restore via restoreFolder, the
// quota accounting on both sides, and the collectFolderTree helper that
// powers the recursive scan. The hard-delete tests (permanentlyDeleteFolder)
// would need extensive SQL setup — we focus on the soft/restore cycle which
// is where the audit found the most bugs.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ─── handleDeleteFolder (entry) ─────────────────────────────────────────────

// Empty folder soft-delete: no child files → no quota update, no file
// UPDATE, just the folder UPDATE + activity row.
func TestHandleDeleteFolder_EmptyFolder_204(t *testing.T) {
	a := newTestApp(t)
	// canAccessFolder owner path.
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("F").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	// collectFolderTree
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree AS").
		WithArgs("F", "u-test", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("F"))
	a.mock.ExpectBegin()
	a.mock.ExpectExec("UPDATE folders SET is_deleted=1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Sum of file sizes returns 0 → no quota release.
	a.mock.ExpectQuery("SELECT COALESCE\\(SUM\\(size_bytes\\), 0\\) FROM files").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(0)))
	a.mock.ExpectExec("UPDATE files SET is_deleted=1").
		WillReturnResult(sqlmock.NewResult(0, 0))
	a.mock.ExpectCommit()
	// Activity write.
	a.mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 1))

	w := rec()
	r := authedRequest("DELETE", "/folders/F", "")
	r = withChiParams(r, "id", "F")
	a.handleDeleteFolder(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d body=%s", w.Code, w.Body.String())
	}
}

// Folder with child files: quota release = SUM(size_bytes).
func TestHandleDeleteFolder_ReleasesQuotaForChildFiles(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("F").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree AS").
		WithArgs("F", "u-test", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("F"))
	a.mock.ExpectBegin()
	a.mock.ExpectExec("UPDATE folders SET is_deleted=1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// SUM = 600 bytes from 3 files.
	a.mock.ExpectQuery("SELECT COALESCE\\(SUM\\(size_bytes\\), 0\\) FROM files").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(600)))
	a.mock.ExpectExec("UPDATE files SET is_deleted=1").
		WillReturnResult(sqlmock.NewResult(0, 3))
	// releaseQuota UPDATE with 600.
	a.mock.ExpectExec("UPDATE users SET used_bytes").
		WithArgs(int64(600), "u-test").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 1))

	w := rec()
	r := authedRequest("DELETE", "/folders/F", "")
	r = withChiParams(r, "id", "F")
	a.handleDeleteFolder(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d body=%s", w.Code, w.Body.String())
	}
}

// Nested subtree: collectFolderTree returns parent + child + grandchild.
// The UPDATE folders IN-clause covers all three.
func TestHandleDeleteFolder_NestedSubtreeMarked(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("F").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree AS").
		WithArgs("F", "u-test", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).
			AddRow("F").AddRow("SubA").AddRow("SubB"))
	a.mock.ExpectBegin()
	a.mock.ExpectExec("UPDATE folders SET is_deleted=1").
		WillReturnResult(sqlmock.NewResult(0, 3))
	a.mock.ExpectQuery("SELECT COALESCE\\(SUM\\(size_bytes\\), 0\\) FROM files").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(0)))
	a.mock.ExpectExec("UPDATE files SET is_deleted=1").
		WillReturnResult(sqlmock.NewResult(0, 0))
	a.mock.ExpectCommit()
	a.mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 1))

	w := rec()
	r := authedRequest("DELETE", "/folders/F", "")
	r = withChiParams(r, "id", "F")
	a.handleDeleteFolder(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d body=%s", w.Code, w.Body.String())
	}
}

// Foreign folder: canAccessFolder now runs FIRST. The folder is owned by
// someone else and the requester has no grant, so canAccessFolder returns
// ErrNoRows → handleDeleteFolder returns 404 not_found before any
// collectFolderTree / tx work. (Previously this was a silent 204 no-op; the
// grant-aware refactor makes it an explicit 404 — still no existence leak vs.
// a real missing folder, which also 404s.)
func TestHandleDeleteFolder_ForeignFolderIsNoOp(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("foreign").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("other-user"))
	a.mock.ExpectQuery("WITH RECURSIVE ancestors AS").
		WithArgs("foreign", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"permission"}))
	w := rec()
	r := authedRequest("DELETE", "/folders/foreign", "")
	r = withChiParams(r, "id", "foreign")
	a.handleDeleteFolder(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 (foreign folder denied), got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── markFolderDeleted atomicity ───────────────────────────────────────────

// If the quota release UPDATE fails, the entire tx must roll back —
// folders + files stay live, no quota change.
func TestMarkFolderDeleted_RollbackOnQuotaError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree AS").
		WithArgs("F", "u-test", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("F"))
	a.mock.ExpectBegin()
	a.mock.ExpectExec("UPDATE folders SET is_deleted=1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectQuery("SELECT COALESCE\\(SUM\\(size_bytes\\), 0\\) FROM files").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(100)))
	a.mock.ExpectExec("UPDATE files SET is_deleted=1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// releaseQuota errors.
	a.mock.ExpectExec("UPDATE users SET used_bytes").
		WillReturnError(rowsErrNoRows())
	a.mock.ExpectRollback()

	err := a.markFolderDeleted(testCtx(), "u-test", "F")
	if err == nil {
		t.Errorf("expected error from quota release")
	}
}

// Rollback when the files UPDATE fails.
func TestMarkFolderDeleted_RollbackOnFilesUpdateError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree AS").
		WithArgs("F", "u-test", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("F"))
	a.mock.ExpectBegin()
	a.mock.ExpectExec("UPDATE folders SET is_deleted=1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectQuery("SELECT COALESCE\\(SUM\\(size_bytes\\), 0\\) FROM files").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(0)))
	a.mock.ExpectExec("UPDATE files SET is_deleted=1").
		WillReturnError(rowsErrNoRows())
	a.mock.ExpectRollback()

	err := a.markFolderDeleted(testCtx(), "u-test", "F")
	if err == nil {
		t.Errorf("expected error to propagate")
	}
}

// ─── restoreFolder ─────────────────────────────────────────────────────────

// Empty folder restore: no files to re-debit, just clears is_deleted on
// the folder row.
func TestRestoreFolder_EmptyFolder_ClearsDeletedFlag(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree AS").
		WithArgs("F", "u-test", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("F"))
	a.mock.ExpectBegin()
	// SUM size of trashed files = 0
	a.mock.ExpectQuery("SELECT COALESCE\\(SUM\\(size_bytes\\), 0\\) FROM files").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(0)))
	a.mock.ExpectExec("UPDATE folders SET is_deleted=0").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE files SET is_deleted=0").
		WillReturnResult(sqlmock.NewResult(0, 0))
	a.mock.ExpectCommit()

	err := a.restoreFolder(testCtx(), "u-test", "F")
	if err != nil {
		t.Errorf("unexpected: %v", err)
	}
}

// Restore folder with files: reserveQuota called with the SUM.
func TestRestoreFolder_ReDebitsQuota(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree AS").
		WithArgs("F", "u-test", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("F"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT COALESCE\\(SUM\\(size_bytes\\), 0\\) FROM files").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(500)))
	// reserveQuota's UPDATE matches successfully.
	a.mock.ExpectExec("UPDATE users SET used_bytes").
		WithArgs(int64(500), "u-test", int64(500)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE folders SET is_deleted=0").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE files SET is_deleted=0").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()

	err := a.restoreFolder(testCtx(), "u-test", "F")
	if err != nil {
		t.Errorf("unexpected: %v", err)
	}
}

// Restore over quota: reserveQuota returns ErrQuotaExceeded → tx rolled
// back → folder stays deleted.
func TestRestoreFolder_QuotaExceededRollsBack(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree AS").
		WithArgs("F", "u-test", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("F"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT COALESCE\\(SUM\\(size_bytes\\), 0\\) FROM files").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(10000)))
	// reserveQuota's UPDATE matches 0 rows → ErrQuotaExceeded.
	a.mock.ExpectExec("UPDATE users SET used_bytes").
		WithArgs(int64(10000), "u-test", int64(10000)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	// Diagnostic SELECT inside reserveQuota for logging.
	a.mock.ExpectQuery("SELECT used_bytes, quota_bytes FROM users WHERE id=").
		WithArgs("u-test").
		WillReturnRows(sqlmock.NewRows([]string{"used", "quota"}).AddRow(int64(95), int64(100)))
	a.mock.ExpectRollback()

	err := a.restoreFolder(testCtx(), "u-test", "F")
	if err == nil {
		t.Errorf("expected ErrQuotaExceeded")
	}
}

// Restore foreign folder: collectFolderTree returns empty → restoreFolder
// is a no-op (no error). Same pattern as the delete path.
func TestRestoreFolder_ForeignFolderIsNoOp(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree AS").
		WithArgs("foreign", "u-test", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if err := a.restoreFolder(testCtx(), "u-test", "foreign"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ─── markFolderDeleted helper, foreign / empty ──────────────────────────────

func TestMarkFolderDeleted_NonexistentNoOp(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree AS").
		WithArgs("ghost", "u-test", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	err := a.markFolderDeleted(testCtx(), "u-test", "ghost")
	if err != nil {
		t.Errorf("expected nil for empty tree, got %v", err)
	}
}

// ─── handleDeleteFolder error surfacing ────────────────────────────────────

// Internal error from markFolderDeleted surfaces as 500 delete_failed.
func TestHandleDeleteFolder_InternalErrorReturns500(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("F").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree AS").
		WithArgs("F", "u-test", "u-test").
		WillReturnError(rowsErrNoRows()) // Forces an error path different from "empty tree".
	w := rec()
	r := authedRequest("DELETE", "/folders/F", "")
	r = withChiParams(r, "id", "F")
	a.handleDeleteFolder(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "delete_failed") {
		t.Errorf("expected delete_failed code")
	}
}

// ─── Quota-net-zero invariant ───────────────────────────────────────────────

// Delete then restore: the SUM in both directions matches. Verify that the
// quota release amount (delete) equals the quota reserve amount (restore)
// for the same dataset.
func TestDeleteThenRestore_QuotaArithmeticBalances(t *testing.T) {
	const folderSize int64 = 250
	a := newTestApp(t)

	// Delete leg.
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree AS").
		WithArgs("F", "u-test", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("F"))
	a.mock.ExpectBegin()
	a.mock.ExpectExec("UPDATE folders SET is_deleted=1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectQuery("SELECT COALESCE\\(SUM\\(size_bytes\\), 0\\) FROM files").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(folderSize))
	a.mock.ExpectExec("UPDATE files SET is_deleted=1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE users SET used_bytes").
		WithArgs(folderSize, "u-test").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()

	// Restore leg.
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree AS").
		WithArgs("F", "u-test", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("F"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT COALESCE\\(SUM\\(size_bytes\\), 0\\) FROM files").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(folderSize))
	a.mock.ExpectExec("UPDATE users SET used_bytes").
		WithArgs(folderSize, "u-test", folderSize).
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE folders SET is_deleted=0").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE files SET is_deleted=0").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()

	if err := a.markFolderDeleted(testCtx(), "u-test", "F"); err != nil {
		t.Fatalf("delete leg: %v", err)
	}
	if err := a.restoreFolder(testCtx(), "u-test", "F"); err != nil {
		t.Fatalf("restore leg: %v", err)
	}
}
