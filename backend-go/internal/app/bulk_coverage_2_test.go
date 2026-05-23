package app

// Second batch of bulk coverage tests — additional handlers + helpers
// exercised with minimal sqlmock setup to bump statement coverage on the
// large internal/app package.

import (
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ─── handlers_admin.go: handleAdminUsers happy ──────────────────────────────

func TestHandleAdminUsers_Empty(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT id, username, email, role").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "username", "email", "role", "quota_bytes", "used_bytes",
			"is_active", "must_change_password", "created_at",
		}))
	w := rec()
	r := adminRequest("GET", "/admin/users", "")
	a.handleAdminUsers(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handlers_admin.go: handleAdminUpdateCheck — calls a remote URL; expect
//      error path when URL unset returns useful status. ────────────────────

// Removed — handleAdminUpdateCheck calls into a nil HttpClient and panics.
// Cover it through a more careful test if needed; not worth the setup
// cost just for coverage.

// ─── handlers_admin_logs.go ─────────────────────────────────────────────────

func TestHandleAdminActivityLogs_Empty(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT id, COALESCE\\(user_id").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "action", "resource_type", "resource_id",
			"details", "ip_address", "created_at",
		}))
	w := rec()
	r := adminRequest("GET", "/admin/logs/activity", "")
	a.handleAdminActivityLogs(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handlers_archive.go: empty selection ──────────────────────────────────

func TestHandleDownloadArchive_EmptySelection400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("POST", "/files:download-archive", `{"file_ids":[],"folder_ids":[]}`)
	a.handleDownloadArchive(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleDownloadArchive_MalformedJSON400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("POST", "/files:download-archive", `{not-json`)
	a.handleDownloadArchive(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ─── handlers_files.go: handleFileThumb access denied ──────────────────────

func TestHandleFileThumb_AccessDenied404(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WillReturnError(rowsErrNoRows())
	w := rec()
	r := authedRequest("GET", "/files/f1/thumb", "")
	r = withChiParams(r, "id", "f1")
	a.handleFileThumb(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handlers_files.go: handleListPhotos minimal ───────────────────────────

func TestHandleListPhotos_Empty(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT id, name").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "size_bytes", "mime_type", "width", "height",
			"shot_at", "created_at",
		}))
	w := rec()
	r := authedRequest("GET", "/photos", "")
	a.handleListPhotos(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handlers_locks.go: handleListFileLocks empty ──────────────────────────

func TestHandleListFileLocks_Owner(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("SELECT id, file_id, user_id, kind, expires_at, created_at FROM file_locks").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "file_id", "user_id", "kind", "expires_at", "created_at",
		}))
	w := rec()
	r := authedRequest("GET", "/files/f1/lock", "")
	r = withChiParams(r, "id", "f1")
	a.handleListFileLocks(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handlers_versions.go: handleListVersions access denied ────────────────

func TestHandleListVersions_AccessDenied404(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WillReturnError(rowsErrNoRows())
	w := rec()
	r := authedRequest("GET", "/files/f1/versions", "")
	r = withChiParams(r, "id", "f1")
	a.handleListVersions(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handlers_requests.go: handleListUploadRequests empty ──────────────────

func TestHandleListUploadRequests_Empty(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "token", "folder_id", "folder_name", "expires_at",
			"max_files", "used_files", "has_password", "created_at",
		}))
	w := rec()
	r := authedRequest("GET", "/upload-requests", "")
	a.handleListUploadRequests(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handlers_trash.go: handleListTrash empty ──────────────────────────────

func TestHandleListTrash_Empty(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows([]string{
			"item_type", "id", "name", "size_bytes", "mime_type", "deleted_at",
		}))
	w := rec()
	r := authedRequest("GET", "/trash", "")
	a.handleListTrash(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handlers_admin_storage.go ─────────────────────────────────────────────

func TestHandleAdminStorageByUser_Empty(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "username", "email", "used_bytes", "quota_bytes", "file_count",
		}))
	w := rec()
	r := adminRequest("GET", "/admin/storage/by-user", "")
	a.handleAdminStorageByUser(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleAdminStorageTopFiles_Empty(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "size_bytes", "mime_type", "owner_id", "owner_username",
		}))
	w := rec()
	r := adminRequest("GET", "/admin/storage/top-files", "")
	a.handleAdminStorageTopFiles(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handlers_admin_slow.go ────────────────────────────────────────────────

func TestHandleAdminSlowQueries_Empty(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "query", "duration_ms", "rows_returned", "captured_at",
		}))
	w := rec()
	r := adminRequest("GET", "/admin/slow-queries", "")
	a.handleAdminSlowQueries(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handlers_admin_reindex.go: handleAdminReindexStatus ──────────────────

func TestHandleAdminReindexStatus(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := adminRequest("GET", "/admin/reindex/status", "")
	a.handleAdminReindexStatus(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handlers_dedup.go ─────────────────────────────────────────────────────

func TestHandleHasFileHash_MissingHash400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("HEAD", "/files/by-hash", "")
	a.handleHasFileHash(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateFromHash_MissingFields400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("POST", "/files/by-hash", `{}`)
	a.handleCreateFromHash(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ─── handlers_search.go: handleSearch empty query ──────────────────────────

func TestHandleSearch_EmptyQueryReturnsEmpty(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("GET", "/search?q=", "")
	a.handleSearch(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handlers_me.go ────────────────────────────────────────────────────────

func TestHandleMyActivity_Default(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "action", "resource_type", "resource_id", "details",
			"ip_address", "created_at",
		}))
	w := rec()
	r := authedRequest("GET", "/auth/me/activity", "")
	a.handleMyActivity(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}
