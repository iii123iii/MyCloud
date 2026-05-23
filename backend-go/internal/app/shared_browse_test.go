package app

// Unit tests for grant-aware browsing: listing a shared folder's files +
// subfolders (keyed on the OWNER, with shared/permission metadata), and the
// breadcrumb-boundary truncation that must never reveal the owner's folders
// above the share root.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// A grantee lists a shared folder's files: results are the OWNER's files, each
// tagged shared/owner_id/permission.
func TestHandleListFiles_SharedFolder_ListsOwnerFiles(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("sharedF").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-owner")) // canAccessFolder
	a.mock.ExpectQuery("WITH RECURSIVE ancestors").
		WithArgs("sharedF", "u-test").WillReturnRows(sqlmock.NewRows([]string{"permission"}).AddRow("editor"))
	a.mock.ExpectQuery("WITH RECURSIVE ancestors").
		WithArgs("sharedF", "u-test").WillReturnRows(sqlmock.NewRows([]string{"permission"}).AddRow("editor")) // folderGrantLevel
	a.mock.ExpectQuery("SELECT COUNT").
		WithArgs("u-owner", "sharedF").WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))
	a.mock.ExpectQuery("SELECT id, name, size_bytes").
		WithArgs("u-owner", "sharedF", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "size_bytes", "mime_type", "folder_id", "is_starred", "created_at", "updated_at",
		}).AddRow("file1", "a.txt", int64(50), "text/plain", "sharedF", false, "2026-01-01", "2026-01-02"))

	w := rec()
	r := authedRequest("GET", "/files?folder_id=sharedF", "")
	a.handleListFiles(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{`"shared":true`, `"owner_id":"u-owner"`, `"permission":"editor"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
}

// A grantee lists a shared folder's subfolders: keyed on the owner, tagged shared.
func TestHandleListFolders_SharedParent_ListsOwnerSubfolders(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("sharedF").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-owner"))
	a.mock.ExpectQuery("WITH RECURSIVE ancestors").
		WithArgs("sharedF", "u-test").WillReturnRows(sqlmock.NewRows([]string{"permission"}).AddRow("editor"))
	a.mock.ExpectQuery("WITH RECURSIVE ancestors").
		WithArgs("sharedF", "u-test").WillReturnRows(sqlmock.NewRows([]string{"permission"}).AddRow("editor"))
	// favourites join keyed on requester (u-test), WHERE keyed on owner (u-owner).
	a.mock.ExpectQuery("SELECT f.id, f.name").
		WithArgs("u-test", "u-owner", "sharedF").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "parent_id", "created_at", "updated_at", "is_favorited"}).
			AddRow("sub1", "Sub", "sharedF", "2026-01-01", "2026-01-02", 0))

	w := rec()
	r := authedRequest("GET", "/folders?parent_id=sharedF", "")
	a.handleListFolders(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{`"shared":true`, `"owner_id":"u-owner"`, `"permission":"editor"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
}

// Listing a folder the caller can't access → 404 (no enumeration oracle).
func TestHandleListFiles_FolderNoAccess_404(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("foreignF").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-other"))
	a.mock.ExpectQuery("WITH RECURSIVE ancestors").
		WithArgs("foreignF", "u-test").WillReturnRows(sqlmock.NewRows([]string{"permission"})) // no grant

	w := rec()
	r := authedRequest("GET", "/files?folder_id=foreignF", "")
	a.handleListFiles(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

// SECURITY: a grantee's breadcrumb is truncated at the share boundary. With the
// grant on B and the tree A→B→C, the path returns B + C only — never A — and
// B's real parent (A) is suppressed so the UI can't navigate above the share.
func TestHandleFolderPath_GranteeBoundary_TruncatesAncestors(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("C").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-owner")) // canAccessFolder
	a.mock.ExpectQuery("WITH RECURSIVE ancestors").
		WithArgs("C", "u-test").WillReturnRows(sqlmock.NewRows([]string{"permission"}).AddRow("editor"))
	// Owner-scoped chain, root-first: A (root) → B → C.
	a.mock.ExpectQuery("WITH RECURSIVE chain").
		WithArgs("C", "u-owner", "u-owner").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "parent_id", "created_at", "updated_at"}).
			AddRow("A", "Alpha", nil, "t", "t").
			AddRow("B", "Bravo", "A", "t", "t").
			AddRow("C", "Charlie", "B", "t", "t"))
	// Directly-granted set: B.
	a.mock.ExpectQuery("SELECT folder_id FROM share_grants WHERE grantee_user_id=").
		WithArgs("u-test").WillReturnRows(sqlmock.NewRows([]string{"folder_id"}).AddRow("B"))
	a.mock.ExpectQuery("WITH RECURSIVE ancestors").
		WithArgs("C", "u-test").WillReturnRows(sqlmock.NewRows([]string{"permission"}).AddRow("editor")) // folderGrantLevel

	w := rec()
	r := withChiParams(authedRequest("GET", "/folders/C/path", ""), "id", "C")
	a.handleFolderPath(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "Alpha") {
		t.Errorf("breadcrumb leaked ancestor above the share boundary: %s", body)
	}
	if !strings.Contains(body, "Bravo") || !strings.Contains(body, "Charlie") {
		t.Errorf("breadcrumb missing in-boundary crumbs: %s", body)
	}
	if strings.Contains(body, `"parent_id":"A"`) {
		t.Errorf("boundary crumb leaked its parent: %s", body)
	}
}
