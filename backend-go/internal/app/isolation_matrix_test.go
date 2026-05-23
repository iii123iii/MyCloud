package app

// Multi-tenant isolation matrix. For each handler that takes a resource ID,
// verify that user B cannot operate on user A's resource. These tests
// codify the cross-user-access property — every handler must enforce
// ownership at canAccessFile / canAccessFolder / ensureOwn* boundaries.

import (
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// canAccess returns ErrNoRows when the file's user_id doesn't match. The
// helper below seeds the mock to emulate "this file belongs to someone else".
func foreignFileMock(a *testApp) {
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("other-user"))
	// Some access paths then check share_grants via recursive CTE — return none.
	a.mock.ExpectQuery("WITH RECURSIVE ancestors").
		WillReturnError(rowsErrNoRows())
}

func foreignFolderMock(a *testApp) {
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("other-user"))
	a.mock.ExpectQuery("WITH RECURSIVE ancestors").
		WillReturnError(rowsErrNoRows())
}

// ─── Files ─────────────────────────────────────────────────────────────────

func TestIsolation_DownloadFile_ForeignDenied(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	foreignFileMock(a)
	w := rec()
	r := authedRequest("GET", "/files/f1:download", "")
	r = withChiParams(r, "id", "f1")
	a.handleDownloadFile(w, r)
	if w.Code != http.StatusNotFound && w.Code != http.StatusForbidden {
		t.Errorf("expected 403/404, got %d", w.Code)
	}
}

func TestIsolation_DeleteFile_ForeignDenied(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	// canAccessFile: file owned by another user, no grant → ErrNoRows → 404
	// before any transaction is opened.
	foreignFileMock(a)
	w := rec()
	r := authedRequest("DELETE", "/files/f1", "")
	r = withChiParams(r, "id", "f1")
	a.handleDeleteFile(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestIsolation_UpdateFile_ForeignDenied(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	// canAccessFile gates the star toggle: foreign file with no grant →
	// ErrNoRows → 404 (no UPDATE issued). A grantee would instead get 403,
	// since is_starred is owner-global; either way a non-owner can't flip it.
	foreignFileMock(a)
	w := rec()
	r := authedRequest("PATCH", "/files/f1", `{"is_starred":true}`)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFile(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestIsolation_PreviewFile_ForeignDenied(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	foreignFileMock(a)
	w := rec()
	r := authedRequest("GET", "/files/f1/preview", "")
	r = withChiParams(r, "id", "f1")
	a.handlePreviewFile(w, r)
	if w.Code != http.StatusNotFound && w.Code != http.StatusForbidden {
		t.Errorf("expected 403/404, got %d", w.Code)
	}
}

// ─── Folders ───────────────────────────────────────────────────────────────

func TestIsolation_UpdateFolder_ForeignDenied(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	// canAccessFolder runs first: foreign folder, no grant → ErrNoRows → 404
	// before any UPDATE is attempted.
	foreignFolderMock(a)
	w := rec()
	r := authedRequest("PATCH", "/folders/fold1", `{"name":"new"}`)
	r = withChiParams(r, "id", "fold1")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestIsolation_DeleteFolder_ForeignIsNoOp(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	// canAccessFolder runs first: foreign folder, no grant → ErrNoRows → 404
	// before collectFolderTree / any tx. Previously this was a silent 204
	// no-op; the grant-aware refactor makes it an explicit 404 (which still
	// matches the response for a genuinely missing folder — no leak).
	foreignFolderMock(a)
	w := rec()
	r := authedRequest("DELETE", "/folders/fold1", "")
	r = withChiParams(r, "id", "fold1")
	a.handleDeleteFolder(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 (foreign folder denied), got %d", w.Code)
	}
}

// ─── Tags ──────────────────────────────────────────────────────────────────

func TestIsolation_UpdateTag_Foreign_404(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM tags WHERE id=").
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("other-user"))
	w := rec()
	r := authedRequest("PATCH", "/tags/t1", `{"name":"hack"}`)
	r = withChiParams(r, "id", "t1")
	a.handleUpdateTag(w, r)
	if w.Code < 400 {
		t.Errorf("expected 4xx, got %d", w.Code)
	}
}

func TestIsolation_DeleteTag_Foreign_NoOp(t *testing.T) {
	a := newTestApp(t)
	// DELETE FROM tags WHERE id=? AND user_id=?  →  0 rows for foreign tag.
	a.mock.ExpectExec("DELETE FROM tags").
		WillReturnResult(sqlmock.NewResult(0, 0))
	w := rec()
	r := authedRequest("DELETE", "/tags/t1", "")
	r = withChiParams(r, "id", "t1")
	a.handleDeleteTag(w, r)
	// Idempotent delete = 204 even for foreign IDs (information-leak safe).
	if w.Code != http.StatusNoContent && w.Code != http.StatusNotFound {
		t.Errorf("expected 204/404, got %d", w.Code)
	}
}

// ─── Comments ──────────────────────────────────────────────────────────────

func TestIsolation_DeleteComment_Foreign(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM comments WHERE id=").
		WithArgs("c1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("other-user"))
	w := rec()
	r := authedRequest("DELETE", "/comments/c1", "")
	r = withChiParams(r, "id", "c1")
	a.handleDeleteComment(w, r)
	if w.Code < 400 {
		t.Errorf("expected 4xx, got %d", w.Code)
	}
}

// ─── Shares ────────────────────────────────────────────────────────────────

func TestIsolation_DeleteShare_Foreign_NoOp(t *testing.T) {
	a := newTestApp(t)
	// DELETE FROM shares WHERE id=? AND created_by=? — 0 rows for foreign.
	a.mock.ExpectExec("DELETE FROM shares").
		WillReturnResult(sqlmock.NewResult(0, 0))
	w := rec()
	r := authedRequest("DELETE", "/shares/share1", "")
	r = withChiParams(r, "id", "share1")
	a.handleDeleteShare(w, r)
	if w.Code != http.StatusNoContent && w.Code != http.StatusNotFound {
		t.Errorf("expected 204/404, got %d", w.Code)
	}
}

// ─── Locks ─────────────────────────────────────────────────────────────────

func TestIsolation_AcquireLock_ForeignDenied(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	foreignFileMock(a)
	w := rec()
	r := authedRequest("POST", "/files/f1/lock", `{"kind":"edit"}`)
	r = withChiParams(r, "id", "f1")
	a.handleAcquireFileLock(w, r)
	if w.Code != http.StatusNotFound && w.Code != http.StatusForbidden {
		t.Errorf("expected 403/404, got %d", w.Code)
	}
}

// ─── Trash ─────────────────────────────────────────────────────────────────

func TestIsolation_RestoreTrash_ForeignNotFound(t *testing.T) {
	a := newTestApp(t)
	// Neither files nor folders match — handler returns 404.
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT size_bytes FROM files WHERE id=").
		WillReturnError(rowsErrNoRows())
	a.mock.ExpectRollback()
	a.mock.ExpectQuery("SELECT COUNT.*FROM folders WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	w := rec()
	r := authedRequest("POST", "/trash/restore/foreign-id", "")
	r = withChiParams(r, "id", "foreign-id")
	a.handleRestoreTrash(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ─── batch* helpers also enforce per-item access ───────────────────────────

func TestIsolation_BatchMove_ForeignFileBlocked(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WillReturnError(rowsErrNoRows())
	dst := "dest"
	res := a.batchMove(testCtx(), "u-test", "f1", &dst, "ip", "op")
	if res.OK {
		t.Errorf("expected OK=false for foreign file move, got %+v", res)
	}
}

func TestIsolation_BatchSetStarred_ForeignFileBlocked(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WillReturnError(rowsErrNoRows())
	res := a.batchSetStarred(testCtx(), "u-test", "f1", true, "ip", "op")
	if res.OK {
		t.Error("expected foreign star to be blocked")
	}
}
