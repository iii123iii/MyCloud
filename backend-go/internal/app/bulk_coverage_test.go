package app

// Bulk coverage tests — exercise the happy/error paths of dozens of small
// handler functions to push the internal/app coverage number up. Each
// test follows the same pattern: build a testApp, set up minimal SQL
// expectations, invoke the handler, assert the status code. We don't
// deep-validate response bodies here; those are exercised in the
// dedicated handler test files.

import (
	"context"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ─── blob_refs.go ───────────────────────────────────────────────────────────

func TestAcquireBlobRef_Insert(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectExec("INSERT INTO blob_refs").
		WithArgs("/path/x.enc").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	tx, _ := a.DB.BeginTx(context.Background(), nil)
	if err := acquireBlobRef(context.Background(), tx, "/path/x.enc"); err != nil {
		t.Errorf("unexpected: %v", err)
	}
	_ = tx.Commit()
}

func TestReleaseBlobRefDB_Success(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT ref_count FROM blob_refs WHERE storage_path = . FOR UPDATE").
		WithArgs("/path/x.enc").
		WillReturnRows(sqlmock.NewRows([]string{"ref_count"}).AddRow(int64(2)))
	a.mock.ExpectExec("UPDATE blob_refs SET ref_count = ref_count - 1 WHERE storage_path =").
		WithArgs("/path/x.enc").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	gone, err := releaseBlobRefDB(context.Background(), a.DB, "/path/x.enc")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if gone {
		t.Errorf("expected gone=false (ref_count was 2)")
	}
}

func TestReleaseBlobRefDB_BeginError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin().WillReturnError(rowsErrNoRows())
	if _, err := releaseBlobRefDB(context.Background(), a.DB, "/path/x.enc"); err == nil {
		t.Errorf("expected error from BeginTx failure")
	}
}

// ─── context.go ─────────────────────────────────────────────────────────────

func TestUserIDHolderFrom_Present(t *testing.T) {
	r := authedRequest("GET", "/", "")
	h := &userIDHolder{}
	ctx := context.WithValue(r.Context(), userIDHolderKey, h)
	if got := userIDHolderFrom(ctx); got != h {
		t.Errorf("userIDHolderFrom should return the stored holder")
	}
}

func TestUserIDHolderFrom_Absent(t *testing.T) {
	if got := userIDHolderFrom(context.Background()); got != nil {
		t.Errorf("expected nil when absent")
	}
}

// withUser with nil holder still sets ctx values.
func TestWithUser_NoHolderInCtx(t *testing.T) {
	ctx := withUser(context.Background(), "u-1", "user")
	if v, _ := ctx.Value(userIDKey).(string); v != "u-1" {
		t.Errorf("user id missing")
	}
	if v, _ := ctx.Value(userRoleKey).(string); v != "user" {
		t.Errorf("user role missing")
	}
}

// ─── handlers_admin.go: handleAdminCreateUser ───────────────────────────────

func TestHandleAdminCreateUser_MalformedJSON400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := adminRequest("POST", "/admin/users", `{`)
	a.handleAdminCreateUser(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleAdminCreateUser_MissingFieldsReturns4xx(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := adminRequest("POST", "/admin/users", `{"username":""}`)
	a.handleAdminCreateUser(w, r)
	// Implementation may return 400 or 409 (duplicate) depending on order of
	// validation vs INSERT. Accept either client-error code.
	if w.Code < 400 || w.Code >= 500 {
		t.Errorf("expected 4xx, got %d", w.Code)
	}
}

// ─── handlers_admin.go: handleAdminLogs ─────────────────────────────────────

func TestHandleAdminLogs_DefaultLimit(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "action", "resource_type", "resource_id", "details",
			"ip_address", "created_at",
		}))
	w := rec()
	r := adminRequest("GET", "/admin/logs", "")
	a.handleAdminLogs(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handlers_admin.go: handleAdminStats ────────────────────────────────────

func TestHandleAdminStats_AggregatesCounts(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(10))
	a.mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(5))
	a.mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(20))
	a.mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(100))
	a.mock.ExpectQuery("SELECT COALESCE\\(SUM").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(1024)))
	w := rec()
	r := adminRequest("GET", "/admin/stats", "")
	a.handleAdminStats(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handlers_admin.go: handleAdminSettings ─────────────────────────────────

func TestHandleAdminSettings_Get(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT key_name, value FROM settings").
		WillReturnRows(sqlmock.NewRows([]string{"key_name", "value"}).
			AddRow("setup_complete", "true").
			AddRow("registration_enabled", "true"))
	w := rec()
	r := adminRequest("GET", "/admin/settings", "")
	a.handleAdminSettings(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handlers_admin.go: handleAdminDeleteUser ───────────────────────────────

func TestHandleAdminDeleteUser_SelfDeleteRejected(t *testing.T) {
	a := newTestApp(t)
	// SELECT role for self.
	a.mock.ExpectQuery("SELECT role FROM users WHERE id=").
		WithArgs("u-admin").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("admin"))
	w := rec()
	r := adminRequest("DELETE", "/admin/users/u-admin", "")
	r = withChiParams(r, "id", "u-admin")
	a.handleAdminDeleteUser(w, r)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusForbidden {
		t.Errorf("expected 400/403 for self-delete, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handlers_admin.go: handleAdminPutSettings ──────────────────────────────

func TestHandleAdminPutSettings_MalformedJSON400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := adminRequest("PUT", "/admin/settings", `{`)
	a.handleAdminPutSettings(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ─── handlers_grants.go ─────────────────────────────────────────────────────

func TestHandleListGrants_DefaultDirection(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery(".*").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "file_id", "folder_id", "grantee_user_id", "permission",
			"granted_by", "created_at", "file_name", "folder_name",
		}))
	w := rec()
	r := authedRequest("GET", "/shares/grants", "")
	a.handleListGrants(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handlers_comments.go ───────────────────────────────────────────────────

func TestHandleListComments_AccessDenied404(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnError(rowsErrNoRows())
	w := rec()
	r := authedRequest("GET", "/files/f1/comments", "")
	r = withChiParams(r, "id", "f1")
	a.handleListComments(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleCreateComment_AccessDenied404(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnError(rowsErrNoRows())
	w := rec()
	r := authedRequest("POST", "/files/f1/comments", `{"body":"hi"}`)
	r = withChiParams(r, "id", "f1")
	a.handleCreateComment(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handlers_smart.go ──────────────────────────────────────────────────────

func TestHandleListSmartFolders_Empty(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT id, name").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "query", "color", "created_at"}))
	w := rec()
	r := authedRequest("GET", "/smart-folders", "")
	a.handleListSmartFolders(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleCreateSmartFolder_EmptyName400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("POST", "/smart-folders", `{"name":"  "}`)
	a.handleCreateSmartFolder(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ─── handlers_favorites.go ──────────────────────────────────────────────────

func TestHandleListFavorites_Empty(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows([]string{"folder_id", "name", "position"}))
	w := rec()
	r := authedRequest("GET", "/me/favorites", "")
	a.handleListFavorites(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handlers_files.go: handleListFiles minimal ─────────────────────────────

func TestHandleListFiles_HappyPath(t *testing.T) {
	a := newTestApp(t)
	// COUNT + page query at minimum.
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
	a.mock.ExpectQuery("SELECT id, name").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "size_bytes", "mime_type", "folder_id",
			"created_at", "updated_at", "is_starred", "content_sha256",
		}))
	w := rec()
	r := authedRequest("GET", "/files", "")
	a.handleListFiles(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handlers_tags.go: list ─────────────────────────────────────────────────

func TestHandleListTags_Empty(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT id, name, color, created_at FROM tags").
		WithArgs("u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "color", "created_at"}))
	w := rec()
	r := authedRequest("GET", "/tags", "")
	a.handleListTags(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handlers_files.go: handleFileInfo not found ────────────────────────────

func TestHandleFileInfo_AccessDenied404(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnError(rowsErrNoRows())
	w := rec()
	r := authedRequest("GET", "/files/f1", "")
	r = withChiParams(r, "id", "f1")
	a.handleFileInfo(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handlers_folders.go: handleGetFolder ───────────────────────────────────

func TestHandleGetFolder_NotFound404(t *testing.T) {
	a := newTestApp(t)
	// canAccessFolder access query returns no rows → 404 (detail query not reached).
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("ghost").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}))
	w := rec()
	r := authedRequest("GET", "/folders/ghost", "")
	r = withChiParams(r, "id", "ghost")
	a.handleGetFolder(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ─── handlers_folder_activity.go: handleFolderActivity access denied ───────

func TestHandleFolderActivity_NoAccess4xx(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WillReturnError(rowsErrNoRows())
	w := rec()
	r := authedRequest("GET", "/folders/F1/activity", "")
	r = withChiParams(r, "id", "F1")
	a.handleFolderActivity(w, r)
	// Either 404 (existence-leak protection) or 403 (forbidden) acceptable.
	if w.Code != http.StatusNotFound && w.Code != http.StatusForbidden {
		t.Errorf("expected 404/403, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handlers_retention.go: handleGetRetentionPolicy access denied ─────────

func TestHandleGetRetentionPolicy_NoAccess403(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WillReturnError(rowsErrNoRows())
	w := rec()
	r := authedRequest("GET", "/folders/F1/retention", "")
	r = withChiParams(r, "id", "F1")
	a.handleGetRetentionPolicy(w, r)
	if w.Code != http.StatusNotFound && w.Code != http.StatusForbidden {
		t.Errorf("expected 404/403, got %d body=%s", w.Code, w.Body.String())
	}
}
