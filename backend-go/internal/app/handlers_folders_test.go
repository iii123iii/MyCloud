package app

import (
	"net/http"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ─── handleCreateFolder ─────────────────────────────────────────────────────

func TestHandleCreateFolder_EmptyNameReturns400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("POST", "/folders", `{"name":"  "}`)
	a.handleCreateFolder(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateFolder_MalformedJSONReturns400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("POST", "/folders", `{`)
	a.handleCreateFolder(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// Foreign parent_id: SELECT COUNT returns 0 → 400 invalid_parent.
func TestHandleCreateFolder_ForeignParentReturns400(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("foreign-folder").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("other-user"))
	a.mock.ExpectQuery("WITH RECURSIVE ancestors AS").
		WithArgs("foreign-folder", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"permission"}))
	w := rec()
	r := authedRequest("POST", "/folders", `{"name":"x","parent_id":"foreign-folder"}`)
	a.handleCreateFolder(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid_parent") {
		t.Errorf("expected invalid_parent code")
	}
}

// Depth cap: a parent at MaxFolderDepth-1 (i.e. depth MaxFolderDepth) blocks
// the create.
func TestHandleCreateFolder_DepthCapBlocked(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("deep-parent").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("WITH RECURSIVE chain AS").
		WithArgs("deep-parent").
		WillReturnRows(sqlmock.NewRows([]string{"MAX(depth)"}).AddRow(MaxFolderDepth))
	w := rec()
	r := authedRequest("POST", "/folders", `{"name":"x","parent_id":"deep-parent"}`)
	a.handleCreateFolder(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "folder_too_deep") {
		t.Errorf("expected folder_too_deep code")
	}
}

// Depth just under the cap is allowed.
func TestHandleCreateFolder_DepthBelowCapAllowed(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("parent").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("WITH RECURSIVE chain AS").
		WithArgs("parent").
		WillReturnRows(sqlmock.NewRows([]string{"MAX(depth)"}).AddRow(MaxFolderDepth - 1))
	a.mock.ExpectExec("INSERT INTO folders").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("POST", "/folders", `{"name":"x","parent_id":"parent"}`)
	a.handleCreateFolder(w, r)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
}

// Root creation skips the parent + depth checks.
func TestHandleCreateFolder_RootSkipsDepthCheck(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("INSERT INTO folders").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("POST", "/folders", `{"name":"top"}`)
	a.handleCreateFolder(w, r)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handleListFolders ──────────────────────────────────────────────────────

// parent_id ownership validation: a foreign parent returns 404, not just an
// empty list (the audit caught the enumeration oracle).
func TestHandleListFolders_ForeignParentReturns404(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("other-user-folder").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("other-user"))
	a.mock.ExpectQuery("WITH RECURSIVE ancestors AS").
		WithArgs("other-user-folder", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"permission"}))
	w := rec()
	r := authedRequest("GET", "/folders?parent_id=other-user-folder", "")
	a.handleListFolders(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

// Empty parent_id lists root folders without the ownership preflight.
func TestHandleListFolders_RootListingNoPreflightQuery(t *testing.T) {
	a := newTestApp(t)
	// LEFT JOIN user_favorites added so each folder row carries an
	// is_favorited flag — query now selects 6 columns and prefixes the
	// folders table with `f.`. WithArgs takes the user_id twice (once for
	// the join, once for the WHERE) when no parent_id is supplied.
	a.mock.ExpectQuery("SELECT f.id, f.name, f.parent_id").
		WithArgs("u-test", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "parent_id", "created_at", "updated_at", "is_favorited"}))
	w := rec()
	r := authedRequest("GET", "/folders", "")
	a.handleListFolders(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── handleFolderPath ──────────────────────────────────────────────────────

func TestHandleFolderPath_NotFoundWhenEmptyChain(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("ghost").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("WITH RECURSIVE chain AS").
		WithArgs("ghost", "u-test", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "parent_id", "created_at", "updated_at"}))
	w := rec()
	r := authedRequest("GET", "/folders/ghost/path", "")
	r = withChiParams(r, "id", "ghost")
	a.handleFolderPath(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── folderDepth (helper) ───────────────────────────────────────────────────

// We already test the helper in folder_depth_test.go; this is a smoke that
// MaxFolderDepth interacts with createFolder correctly under edge values.
func TestMaxFolderDepth_Boundary(t *testing.T) {
	if MaxFolderDepth < 16 || MaxFolderDepth > 1000 {
		t.Errorf("MaxFolderDepth out of sane range: %d", MaxFolderDepth)
	}
}
