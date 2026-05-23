package app

// Tests dedicated to the FOLDER MOVE path (handleUpdateFolder with a
// parent_id body). The move handler is unusually complex: it opens a tx,
// reads the current parent for the activity log, validates the new parent
// (ownership + depth cap + cycle CTE), runs the UPDATE, commits, then
// writes a move-shaped activity entry. We test each branch separately so
// failures pinpoint exactly which guard regressed.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ─── Validation rejection ───────────────────────────────────────────────────

// Move folder into itself — the handler rejects via a parent_id==id string
// compare BEFORE opening the tx, so no SQL should be issued.
func TestHandleUpdateFolder_MoveIntoSelfReturns400(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("A").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	w := rec()
	r := authedRequest("PATCH", "/folders/A", `{"parent_id":"A"}`)
	r = withChiParams(r, "id", "A")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Folder cannot be moved into itself") {
		t.Errorf("expected self-move message, got %s", w.Body.String())
	}
}

// Malformed parent_id (non-string, non-null JSON) → 400.
func TestHandleUpdateFolder_MoveMalformedParentId400(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("A").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	w := rec()
	r := authedRequest("PATCH", "/folders/A", `{"parent_id":42}`)
	r = withChiParams(r, "id", "A")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "parent_id must be a string or null") {
		t.Errorf("expected parent_id validation message")
	}
}

// Empty body — handler treats len(payload)==0 as nothing to update.
func TestHandleUpdateFolder_MoveEmptyBody400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("PATCH", "/folders/A", `{}`)
	r = withChiParams(r, "id", "A")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── Folder not found / cross-user ──────────────────────────────────────────

// The handler reads the current parent_id inside the tx; ErrNoRows there →
// 404 not_found (also covers cross-user — foreign folder won't match
// user_id predicate).
func TestHandleUpdateFolder_MoveOnNonexistentFolder404(t *testing.T) {
	a := newTestApp(t)
	// canAccessFolder finds no such folder → 404 before the tx opens.
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("ghost").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}))
	w := rec()
	r := authedRequest("PATCH", "/folders/ghost", `{"parent_id":"B"}`)
	r = withChiParams(r, "id", "ghost")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

// Cross-user folder — same flow as nonexistent, the user_id predicate
// filters it out. The audit calls for 404 (not 403) to avoid existence
// leak.
func TestHandleUpdateFolder_MoveForeignFolder404(t *testing.T) {
	a := newTestApp(t)
	// Foreign folder: access query returns another owner, grant lookup empty
	// → canAccessFolder returns ErrNoRows → 404 (no existence leak).
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("foreign-folder").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("other-user"))
	a.mock.ExpectQuery("WITH RECURSIVE ancestors AS").
		WithArgs("foreign-folder", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"permission"}))
	w := rec()
	r := authedRequest("PATCH", "/folders/foreign-folder", `{"parent_id":"B"}`)
	r = withChiParams(r, "id", "foreign-folder")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── Foreign parent ─────────────────────────────────────────────────────────

// Parent owned by another user — parent existence check returns 0 → 400
// invalid_parent.
func TestHandleUpdateFolder_MoveIntoForeignParent400(t *testing.T) {
	a := newTestApp(t)
	// Source folder owned by the requester.
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("A").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WithArgs("A").
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))
	// Destination owned by someone else with no grant → canAccessFolder
	// returns ErrNoRows → invalid_parent (400).
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("foreign-parent").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("other-user"))
	a.mock.ExpectQuery("WITH RECURSIVE ancestors AS").
		WithArgs("foreign-parent", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"permission"}))
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("PATCH", "/folders/A", `{"parent_id":"foreign-parent"}`)
	r = withChiParams(r, "id", "A")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid_parent") {
		t.Errorf("expected invalid_parent code")
	}
}

// ─── Depth cap ──────────────────────────────────────────────────────────────

// Parent already at MaxFolderDepth → 400 folder_too_deep. The check uses
// `folderDepth >= MaxFolderDepth` so equality should also be rejected.
func TestHandleUpdateFolder_MoveExceedsDepthCap400(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("A").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WithArgs("A").
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("deep-parent").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("WITH RECURSIVE chain AS").
		WithArgs("deep-parent").
		WillReturnRows(sqlmock.NewRows([]string{"MAX(depth)"}).AddRow(MaxFolderDepth))
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("PATCH", "/folders/A", `{"parent_id":"deep-parent"}`)
	r = withChiParams(r, "id", "A")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "folder_too_deep") {
		t.Errorf("expected folder_too_deep code, got %s", w.Body.String())
	}
}

// Just under the cap — accepted.
func TestHandleUpdateFolder_MoveAtDepthCapMinusOneAllowed(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("A").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WithArgs("A").
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("parent").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("WITH RECURSIVE chain AS").
		WithArgs("parent").
		WillReturnRows(sqlmock.NewRows([]string{"MAX(depth)"}).AddRow(MaxFolderDepth - 1))
	// Cycle CTE returns only the folder itself (no descendants).
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree AS").
		WithArgs("A", "u-test", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("A"))
	a.mock.ExpectExec("UPDATE folders SET parent_id=").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	// Activity write.
	a.mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("PATCH", "/folders/A", `{"parent_id":"parent"}`)
	r = withChiParams(r, "id", "A")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── Cycle detection ────────────────────────────────────────────────────────

// Moving a folder into one of its own descendants is a cycle. The CTE
// returns the descendant id in the subtree of A; the handler detects the
// match and rejects.
func TestHandleUpdateFolder_MoveIntoDescendantReturns400(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("A").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WithArgs("A").
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("B").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("WITH RECURSIVE chain AS").
		WithArgs("B").
		WillReturnRows(sqlmock.NewRows([]string{"MAX(depth)"}).AddRow(1))
	// Subtree of A includes A and B — match means cycle.
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree AS").
		WithArgs("A", "u-test", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("A").AddRow("B"))
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("PATCH", "/folders/A", `{"parent_id":"B"}`)
	r = withChiParams(r, "id", "A")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "descendant") {
		t.Errorf("expected descendant error message, got %s", w.Body.String())
	}
}

// Cycle detection only triggers when the target is actually a descendant.
// A leaf folder moving to a sibling-tree leaf is fine.
func TestHandleUpdateFolder_MoveBetweenSiblingsAllowed(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("A").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WithArgs("A").
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("B").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("WITH RECURSIVE chain AS").
		WithArgs("B").
		WillReturnRows(sqlmock.NewRows([]string{"MAX(depth)"}).AddRow(1))
	// Subtree of A is just A — B is not a descendant.
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree AS").
		WithArgs("A", "u-test", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("A"))
	a.mock.ExpectExec("UPDATE folders SET parent_id=").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("PATCH", "/folders/A", `{"parent_id":"B"}`)
	r = withChiParams(r, "id", "A")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── Move to root ───────────────────────────────────────────────────────────

// parent_id=null means move to root. The handler skips the parent-exists,
// depth, and cycle checks (no destination to validate) and goes straight
// to the UPDATE with NULL.
func TestHandleUpdateFolder_MoveToRootBypassesParentChecks(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("A").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WithArgs("A").
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow("oldparent"))
	// No parent exists / depth / cycle queries — straight to UPDATE.
	a.mock.ExpectExec("UPDATE folders SET parent_id=").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("PATCH", "/folders/A", `{"parent_id":null}`)
	r = withChiParams(r, "id", "A")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── UPDATE 0 rows affected ─────────────────────────────────────────────────

// If the UPDATE finds 0 rows (race: someone soft-deleted the folder
// between our SELECT and the UPDATE), the handler should return 404.
func TestHandleUpdateFolder_MoveUpdateZeroRowsReturns404(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("A").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WithArgs("A").
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))
	a.mock.ExpectExec("UPDATE folders SET parent_id=").
		WillReturnResult(sqlmock.NewResult(0, 0))
	a.mock.ExpectRollback()
	w := rec()
	r := authedRequest("PATCH", "/folders/A", `{"parent_id":null}`)
	r = withChiParams(r, "id", "A")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── Activity write captures old parent ─────────────────────────────────────

// The handler captures old_parent_id BEFORE the UPDATE so the activity log
// has the correct value. Test the SQL ordering: SELECT parent_id happens
// before UPDATE.
func TestHandleUpdateFolder_MoveCapturesOldParentBeforeUpdate(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(true)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("A").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WithArgs("A").
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow("old-parent"))
	a.mock.ExpectExec("UPDATE folders SET parent_id=").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("PATCH", "/folders/A", `{"parent_id":null}`)
	r = withChiParams(r, "id", "A")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if err := a.mock.ExpectationsWereMet(); err != nil {
		t.Errorf("ordering violated: %v", err)
	}
}

// ─── Move with empty parent_id string ───────────────────────────────────────

// parent_id="" after trimming is treated as null (move to root) per handler
// logic: `if raw != "" { parentID = &raw }`. Verify it doesn't error.
func TestHandleUpdateFolder_MoveEmptyParentIdStringTreatedAsRoot(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("A").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WithArgs("A").
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))
	a.mock.ExpectExec("UPDATE folders SET parent_id=").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("PATCH", "/folders/A", `{"parent_id":""}`)
	r = withChiParams(r, "id", "A")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// Whitespace-only parent_id gets trimmed to empty → treated as root.
func TestHandleUpdateFolder_MoveWhitespaceParentIdTrimmedToRoot(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("A").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT parent_id FROM folders WHERE id=").
		WithArgs("A").
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))
	a.mock.ExpectExec("UPDATE folders SET parent_id=").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("PATCH", "/folders/A", `{"parent_id":"   "}`)
	r = withChiParams(r, "id", "A")
	a.handleUpdateFolder(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ─── collectFolderTree (helper used inside move + delete) ───────────────────

// CTE returns target plus all descendants. Confirm SQL shape and scoping.
func TestCollectFolderTree_ReturnsAllDescendants(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree AS").
		WithArgs("A", "u-test", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("A").AddRow("B").AddRow("C"))
	got, err := a.collectFolderTree(testCtx(), "u-test", "A")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 ids, got %v", got)
	}
}

// Empty result when folder doesn't exist for user.
func TestCollectFolderTree_NonexistentReturnsEmpty(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree AS").
		WithArgs("ghost", "u-test", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	got, err := a.collectFolderTree(testCtx(), "u-test", "ghost")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

// Single-folder tree (leaf) returns just itself.
func TestCollectFolderTree_LeafReturnsSelfOnly(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree AS").
		WithArgs("leaf", "u-test", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("leaf"))
	got, err := a.collectFolderTree(testCtx(), "u-test", "leaf")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(got) != 1 || got[0] != "leaf" {
		t.Errorf("expected [leaf], got %v", got)
	}
}
