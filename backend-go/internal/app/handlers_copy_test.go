package app

// Unit tests for the copy feature (handlers_copy.go). Copies share the source
// blob (ref-counted) and are attributed to the destination's owner, reserving
// that owner's quota — including the "copy from my own folder into a shared
// folder" direction, where the copy lands under the shared folder's owner.
//
// sqlmock uses the regexp matcher (see newTestApp), so the patterns below are
// substrings/regexes of the real SQL. Patterns deliberately avoid regex
// metacharacters ((, ), ?, +, *) by stopping before them.

import (
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// copySuffix is pure string logic — exercise it directly.
func TestCopySuffix(t *testing.T) {
	cases := []struct{ in, want string }{
		{"report.pdf", "report (copy).pdf"},
		{"notes", "notes (copy)"},
		{"archive.tar.gz", "archive.tar (copy).gz"},
		{".gitignore", ".gitignore (copy)"}, // leading dot: no real extension
		{"trailing.", "trailing. (copy)"},   // trailing dot: no real extension
	}
	for _, c := range cases {
		if got := copySuffix(c.in); got != c.want {
			t.Errorf("copySuffix(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// sourceFileRow returns the 8-column row copyFile reads from the source file.
func sourceFileRow(name string, size int64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"name", "mime_type", "encryption_key_enc", "encryption_iv",
		"encryption_tag", "storage_path", "size_bytes", "content_sha256",
	}).AddRow(name, "application/pdf", "ek", "iv", "tag", "/blob/x", size, "hash")
}

// ── copyFile ────────────────────────────────────────────────────────────────

// Own file copied into own root: no folder access query, quota on the copier.
func TestCopyFile_ToOwnRoot(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("SELECT name, mime_type").
		WithArgs("f1").WillReturnRows(sourceFileRow("doc.pdf", 100))
	a.mock.ExpectBegin()
	a.mock.ExpectExec("UPDATE users SET used_bytes = used_bytes").
		WithArgs(int64(100), "u-test", int64(100)).WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectQuery("FROM files WHERE user_id=").
		WithArgs("u-test", "doc.pdf").WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
	a.mock.ExpectExec("INSERT INTO files").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("INSERT INTO blob_refs").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec("INSERT INTO activity_log").WillReturnResult(sqlmock.NewResult(0, 1))

	w := rec()
	r := withChiParams(authedRequest("POST", "/files/f1:copy", `{}`), "id", "f1")
	a.handleCopyFile(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
}

// A grantee copies a SHARED file (owned by someone else) into their own root.
// Access resolves via the grant CTE; the copy is owned by the copier.
func TestCopyFile_SharedIntoOwnRoot(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-owner"))
	a.mock.ExpectQuery("WITH RECURSIVE ancestors").
		WithArgs("f1", "u-test", "f1").WillReturnRows(sqlmock.NewRows([]string{"permission"}).AddRow("viewer"))
	a.mock.ExpectQuery("SELECT name, mime_type").
		WithArgs("f1").WillReturnRows(sourceFileRow("doc.pdf", 100))
	a.mock.ExpectBegin()
	a.mock.ExpectExec("UPDATE users SET used_bytes = used_bytes").
		WithArgs(int64(100), "u-test", int64(100)).WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectQuery("FROM files WHERE user_id=").
		WithArgs("u-test", "doc.pdf").WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
	a.mock.ExpectExec("INSERT INTO files").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("INSERT INTO blob_refs").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec("INSERT INTO activity_log").WillReturnResult(sqlmock.NewResult(0, 1))

	w := rec()
	r := withChiParams(authedRequest("POST", "/files/f1:copy", `{}`), "id", "f1")
	a.handleCopyFile(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
}

// The headline cross-direction: an owner copies their OWN file into a folder
// shared with them (editor). The copy is attributed to the folder's owner and
// charged to the folder owner's quota.
func TestCopyFile_RegularIntoSharedFolder(t *testing.T) {
	a := newTestApp(t)
	// Source is owned by the copier.
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	// Destination folder is owned by u-owner; the copier is an editor (grant CTE).
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("sharedF").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-owner"))
	a.mock.ExpectQuery("WITH RECURSIVE ancestors").
		WithArgs("sharedF", "u-test").WillReturnRows(sqlmock.NewRows([]string{"permission"}).AddRow("editor"))
	a.mock.ExpectQuery("SELECT name, mime_type").
		WithArgs("f1").WillReturnRows(sourceFileRow("doc.pdf", 100))
	a.mock.ExpectBegin()
	// Quota charged to the folder OWNER, not the copier.
	a.mock.ExpectExec("UPDATE users SET used_bytes = used_bytes").
		WithArgs(int64(100), "u-owner", int64(100)).WillReturnResult(sqlmock.NewResult(0, 1))
	// Same-name lookup scoped to the folder owner + destination folder.
	a.mock.ExpectQuery("FROM files WHERE user_id=").
		WithArgs("u-owner", "doc.pdf", "sharedF").WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
	a.mock.ExpectExec("INSERT INTO files").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("INSERT INTO blob_refs").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec("INSERT INTO activity_log").WillReturnResult(sqlmock.NewResult(0, 1))

	w := rec()
	r := withChiParams(authedRequest("POST", "/files/f1:copy", `{"folder_id":"sharedF"}`), "id", "f1")
	a.handleCopyFile(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
}

// A same-name file in the destination triggers the " (copy)" suffix path
// (count > 0). We assert the flow still succeeds; the suffix itself is covered
// by TestCopySuffix.
func TestCopyFile_NameCollisionStillSucceeds(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("SELECT name, mime_type").
		WithArgs("f1").WillReturnRows(sourceFileRow("doc.pdf", 100))
	a.mock.ExpectBegin()
	a.mock.ExpectExec("UPDATE users SET used_bytes = used_bytes").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectQuery("FROM files WHERE user_id=").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1)) // collision
	a.mock.ExpectExec("INSERT INTO files").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("INSERT INTO blob_refs").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec("INSERT INTO activity_log").WillReturnResult(sqlmock.NewResult(0, 1))

	w := rec()
	r := withChiParams(authedRequest("POST", "/files/f1:copy", `{}`), "id", "f1")
	a.handleCopyFile(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
}

// Quota rejection (reserveQuota affects 0 rows) → 413.
func TestCopyFile_QuotaExceeded(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("SELECT name, mime_type").
		WithArgs("f1").WillReturnRows(sourceFileRow("doc.pdf", 100))
	a.mock.ExpectBegin()
	a.mock.ExpectExec("UPDATE users SET used_bytes = used_bytes").
		WillReturnResult(sqlmock.NewResult(0, 0)) // no rows → over quota
	a.mock.ExpectQuery("SELECT used_bytes, quota_bytes FROM users").
		WillReturnRows(sqlmock.NewRows([]string{"used_bytes", "quota_bytes"}).AddRow(int64(990), int64(1000)))
	a.mock.ExpectRollback()

	w := rec()
	r := withChiParams(authedRequest("POST", "/files/f1:copy", `{}`), "id", "f1")
	a.handleCopyFile(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d body=%s", w.Code, w.Body.String())
	}
}

// No read access to the source → 404.
func TestCopyFile_SourceNotAccessible(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").WillReturnRows(sqlmock.NewRows([]string{"user_id"})) // empty → ErrNoRows

	w := rec()
	r := withChiParams(authedRequest("POST", "/files/f1:copy", `{}`), "id", "f1")
	a.handleCopyFile(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

// Destination folder not writable → 400 invalid_parent.
func TestCopyFile_DestNotWritable(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	// Destination folder lookup finds nothing the caller can access.
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("badF").WillReturnRows(sqlmock.NewRows([]string{"user_id"})) // empty → ErrNoRows

	w := rec()
	r := withChiParams(authedRequest("POST", "/files/f1:copy", `{"folder_id":"badF"}`), "id", "f1")
	a.handleCopyFile(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

// ── copyFolder ──────────────────────────────────────────────────────────────

// Copy a one-folder, one-file subtree into own root.
func TestCopyFolder_ToOwnRoot(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("F1").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test")) // canAccessFolder
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("F1").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test")) // folderOwner
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WithArgs("F1", "u-test", "u-test").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("F1"))
	a.mock.ExpectQuery("SELECT id, name, parent_id FROM folders WHERE id IN").
		WithArgs("F1").WillReturnRows(sqlmock.NewRows([]string{"id", "name", "parent_id"}).AddRow("F1", "MyFolder", nil))
	a.mock.ExpectQuery("FROM files WHERE folder_id IN").
		WithArgs("F1").WillReturnRows(sqlmock.NewRows([]string{
		"name", "mime_type", "encryption_key_enc", "encryption_iv", "encryption_tag",
		"storage_path", "size_bytes", "content_sha256", "folder_id",
	}).AddRow("a.txt", "text/plain", "ek", "iv", "tag", "/blob/a", int64(50), "h", "F1"))
	a.mock.ExpectBegin()
	a.mock.ExpectExec("UPDATE users SET used_bytes = used_bytes").
		WithArgs(int64(50), "u-test", int64(50)).WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectQuery("FROM folders WHERE user_id=").
		WithArgs("u-test", "MyFolder").WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0)) // copyFolderName (root)
	a.mock.ExpectExec("INSERT INTO folders").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("INSERT INTO files").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("INSERT INTO blob_refs").WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	a.mock.ExpectExec("INSERT INTO activity_log").WillReturnResult(sqlmock.NewResult(0, 1))

	w := rec()
	r := withChiParams(authedRequest("POST", "/folders/F1:copy", `{}`), "id", "F1")
	a.handleCopyFolder(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
}

// Copy a folder whose contents would blow the destination owner's quota → 413.
func TestCopyFolder_QuotaExceeded(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("F1").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("F1").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WithArgs("F1", "u-test", "u-test").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("F1"))
	a.mock.ExpectQuery("SELECT id, name, parent_id FROM folders WHERE id IN").
		WithArgs("F1").WillReturnRows(sqlmock.NewRows([]string{"id", "name", "parent_id"}).AddRow("F1", "MyFolder", nil))
	a.mock.ExpectQuery("FROM files WHERE folder_id IN").
		WithArgs("F1").WillReturnRows(sqlmock.NewRows([]string{
		"name", "mime_type", "encryption_key_enc", "encryption_iv", "encryption_tag",
		"storage_path", "size_bytes", "content_sha256", "folder_id",
	}).AddRow("big.bin", "application/octet-stream", "ek", "iv", "tag", "/blob/b", int64(9999), "h", "F1"))
	a.mock.ExpectBegin()
	a.mock.ExpectExec("UPDATE users SET used_bytes = used_bytes").
		WillReturnResult(sqlmock.NewResult(0, 0)) // over quota
	a.mock.ExpectQuery("SELECT used_bytes, quota_bytes FROM users").
		WillReturnRows(sqlmock.NewRows([]string{"used_bytes", "quota_bytes"}).AddRow(int64(0), int64(100)))
	a.mock.ExpectRollback()

	w := rec()
	r := withChiParams(authedRequest("POST", "/folders/F1:copy", `{}`), "id", "F1")
	a.handleCopyFolder(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d body=%s", w.Code, w.Body.String())
	}
}
