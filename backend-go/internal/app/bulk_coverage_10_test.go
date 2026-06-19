package app

// Tenth bulk push — target pure helper functions and unexported methods
// with simple SQL signatures. These don't go through HTTP; they go
// straight at the function so we get covered statements per test.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ─── Pure helpers (no DB, no I/O) ──────────────────────────────────────────

func TestValidSegmentName_Cases(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"playlist.m3u8", true},
		{"seg0.ts", true},
		{"seg999.ts", true},
		{"seg12345.ts", true},
		{"seg.ts", false},
		{"seg999999.ts", false},
		{"segABC.ts", false},
		{"../etc/passwd", false},
		{"", false},
		{"index.html", false},
		{"seg001.tsx", false},
		{"abcseg001.ts", false},
	}
	for _, c := range cases {
		if got := validSegmentName(c.in); got != c.want {
			t.Errorf("validSegmentName(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestOfficeDocumentType_Cases(t *testing.T) {
	cases := map[string]string{
		"docx": "word",
		"doc":  "word",
		"odt":  "word",
		"rtf":  "word",
		"txt":  "word",
		"xlsx": "cell",
		"xls":  "cell",
		"csv":  "cell",
		"ods":  "cell",
		"pptx": "slide",
		"ppt":  "slide",
		"odp":  "slide",
		"":     "word", // default
		"png":  "word", // default
	}
	for in, want := range cases {
		if got := officeDocumentType(in); got != want {
			t.Errorf("officeDocumentType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsTextMime_Cases(t *testing.T) {
	yes := []string{
		"text/plain", "text/html", "text/css", "TEXT/PLAIN",
		"application/json", "application/xml",
		"application/yaml", "application/x-yaml",
		"application/javascript", "application/typescript",
	}
	for _, m := range yes {
		if !isTextMime(m) {
			t.Errorf("isTextMime(%q) should be true", m)
		}
	}
	no := []string{
		"image/jpeg", "application/pdf", "video/mp4", "audio/mpeg",
		"application/octet-stream", "",
	}
	for _, m := range no {
		if isTextMime(m) {
			t.Errorf("isTextMime(%q) should be false", m)
		}
	}
}

func TestContainsLite_IndexOf(t *testing.T) {
	if !containsLite("hello world", "world") {
		t.Error("containsLite should find 'world' in 'hello world'")
	}
	if containsLite("abc", "") {
		t.Error("containsLite with empty needle should be false")
	}
	if containsLite("ab", "abc") {
		t.Error("needle longer than haystack should be false")
	}
	if indexOf("hello", "lo") != 3 {
		t.Errorf("indexOf returned wrong position")
	}
	if indexOf("abc", "xyz") != -1 {
		t.Error("indexOf should return -1 for not found")
	}
}

func TestRandomHex_Length(t *testing.T) {
	out := randomHex(8)
	if len(out) != 16 {
		t.Errorf("randomHex(8) expected length 16, got %d", len(out))
	}
}

func TestNewSHA256_HashesDeterministically(t *testing.T) {
	h := newSHA256()
	_, _ = h.Write([]byte("hello"))
	first := h.HexSum()

	h2 := newSHA256()
	_, _ = h2.Write([]byte("hello"))
	if first != h2.HexSum() {
		t.Error("sha256 should be deterministic")
	}
}

// ─── publicBackendURL with various inputs ──────────────────────────────────

func TestPublicBackendURL_FromConfig(t *testing.T) {
	a := newTestApp(t)
	a.Config.PublicBackendURL = "https://api.example.com/"
	r := httptest.NewRequest("GET", "http://localhost/", nil)
	got := a.publicBackendURL(r)
	if got != "https://api.example.com" {
		t.Errorf("expected trimmed URL, got %q", got)
	}
}

func TestPublicBackendURL_FromRequest(t *testing.T) {
	a := newTestApp(t)
	r := httptest.NewRequest("GET", "http://localhost/", nil)
	r.Host = "myhost:8080"
	got := a.publicBackendURL(r)
	if got != "http://myhost:8080" {
		t.Errorf("expected scheme+host, got %q", got)
	}
}

func TestPublicBackendURL_FromForwardedHeaders(t *testing.T) {
	a := newTestApp(t)
	r := httptest.NewRequest("GET", "http://backend:8080/", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "192.168.1.142:8443")
	got := a.publicBackendURL(r)
	if got != "https://192.168.1.142:8443" {
		t.Errorf("expected forwarded public URL, got %q", got)
	}
}

// ─── lookupUsername ─────────────────────────────────────────────────────────

func TestLookupUsername_Found(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT username FROM users WHERE id=").
		WithArgs("u-1").
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("alice"))
	got, err := a.lookupUsername(context.Background(), "u-1")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got != "alice" {
		t.Errorf("expected 'alice', got %q", got)
	}
}

func TestLookupUsername_NotFound(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT username FROM users WHERE id=").
		WithArgs("ghost").
		WillReturnError(rowsErrNoRows())
	got, err := a.lookupUsername(context.Background(), "ghost")
	if err != nil {
		t.Fatalf("not-found should not error, got: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty username, got %q", got)
	}
}

func TestLookupUsername_DBError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT username FROM users WHERE id=").
		WithArgs("u-1").
		WillReturnError(errStr2("connection lost"))
	if _, err := a.lookupUsername(context.Background(), "u-1"); err == nil {
		t.Error("expected db error to propagate")
	}
}

// ─── batchSetStarred ───────────────────────────────────────────────────────

func TestBatchSetStarred_NoAccess(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnError(rowsErrNoRows())
	res := a.batchSetStarred(context.Background(), "u-test", "f1", true, "1.2.3.4", "op-1")
	if res.OK {
		t.Errorf("expected OK=false on no-access, got %+v", res)
	}
}

func TestBatchSetStarred_Success(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectExec("UPDATE files SET is_starred").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("INSERT INTO activity_log").
		WillReturnResult(sqlmock.NewResult(0, 1))
	res := a.batchSetStarred(context.Background(), "u-test", "f1", true, "1.2.3.4", "op-1")
	if !res.OK {
		t.Errorf("expected OK, got error=%q", res.Error)
	}
}

func TestBatchSetStarred_NoRowsUpdated(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectExec("UPDATE files SET is_starred").
		WillReturnResult(sqlmock.NewResult(0, 0))
	res := a.batchSetStarred(context.Background(), "u-test", "f1", true, "1.2.3.4", "op-1")
	if res.OK {
		t.Error("expected OK=false when 0 rows updated")
	}
}

// ─── batchSoftDelete ───────────────────────────────────────────────────────

func TestBatchSoftDelete_NoAccess(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnError(rowsErrNoRows())
	res := a.batchSoftDelete(context.Background(), "u-test", "f1", "ip", "op")
	if res.OK {
		t.Error("expected OK=false on no-access")
	}
}

func TestBatchSoftDelete_Success(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectExec("UPDATE files SET is_deleted").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("INSERT INTO activity_log").
		WillReturnResult(sqlmock.NewResult(0, 1))
	res := a.batchSoftDelete(context.Background(), "u-test", "f1", "ip", "op")
	if !res.OK {
		t.Errorf("expected OK, got error=%q", res.Error)
	}
}

func TestBatchSoftDelete_AlreadyDeleted(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectExec("UPDATE files SET is_deleted").
		WillReturnResult(sqlmock.NewResult(0, 0))
	res := a.batchSoftDelete(context.Background(), "u-test", "f1", "ip", "op")
	if res.OK {
		t.Error("expected OK=false when already deleted")
	}
}

// ─── batchRestore ──────────────────────────────────────────────────────────

func TestBatchRestore_Success(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectExec("UPDATE files SET is_deleted = 0").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("INSERT INTO activity_log").
		WillReturnResult(sqlmock.NewResult(0, 1))
	res := a.batchRestore(context.Background(), "u-test", "f1", "ip", "op")
	if !res.OK {
		t.Errorf("expected OK, got error=%q", res.Error)
	}
}

func TestBatchRestore_NotInTrash(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectExec("UPDATE files SET is_deleted = 0").
		WillReturnResult(sqlmock.NewResult(0, 0))
	res := a.batchRestore(context.Background(), "u-test", "f1", "ip", "op")
	if res.OK {
		t.Error("expected OK=false when not in trash")
	}
}

// ─── batchMove ─────────────────────────────────────────────────────────────

func TestBatchMove_ToRoot(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("SELECT folder_id FROM files WHERE id").
		WillReturnRows(sqlmock.NewRows([]string{"folder_id"}).AddRow(nil))
	a.mock.ExpectExec("UPDATE files SET folder_id").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("INSERT INTO activity_log").
		WillReturnResult(sqlmock.NewResult(0, 1))
	res := a.batchMove(context.Background(), "u-test", "f1", nil, "ip", "op")
	if !res.OK {
		t.Errorf("expected OK, got error=%q", res.Error)
	}
}

func TestBatchMove_DestNotFound(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("dest").
		WillReturnError(rowsErrNoRows())
	dst := "dest"
	res := a.batchMove(context.Background(), "u-test", "f1", &dst, "ip", "op")
	if res.OK {
		t.Error("expected OK=false when destination missing")
	}
}

func TestBatchMove_NoAccess(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnError(errors.New("no access"))
	dst := "dest"
	res := a.batchMove(context.Background(), "u-test", "f1", &dst, "ip", "op")
	if res.OK {
		t.Error("expected OK=false on no-access")
	}
}

// ─── publicBackendURL + handleHLSPlaylist quick error paths ───────────────

func TestHandleHLSPlaylist_NoAccess(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnError(rowsErrNoRows())
	w := rec()
	r := authedRequest("GET", "/files/f1/hls/playlist.m3u8", "")
	r = withChiParams(r, "id", "f1")
	a.handleHLSPlaylist(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleHLSSegment_BadName(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("GET", "/files/f1/hls/../etc/passwd", "")
	r = withChiParams(r, "id", "f1", "seg", "../etc/passwd")
	a.handleHLSSegment(w, r)
	if w.Code < 400 {
		t.Errorf("expected 4xx for bad segment name, got %d", w.Code)
	}
}

// ─── handleEmptyTrash error path ───────────────────────────────────────────

func TestHandleEmptyTrash_DBError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery(".*").WillReturnError(errStr2("connection lost"))
	w := rec()
	r := authedRequest("POST", "/trash:empty", "")
	a.handleEmptyTrash(w, r)
	if w.Code < 400 {
		t.Errorf("expected 4xx/5xx, got %d", w.Code)
	}
}

// ─── ensureOwnTag error paths ──────────────────────────────────────────────

func TestEnsureOwnTag_NotFound(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM tags WHERE id=").
		WithArgs("t1").
		WillReturnError(rowsErrNoRows())
	if err := a.ensureOwnTag(context.Background(), "u-test", "t1"); err == nil {
		t.Error("expected error when tag missing")
	}
}

func TestEnsureOwnTag_Owned(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM tags WHERE id=").
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	if err := a.ensureOwnTag(context.Background(), "u-test", "t1"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestEnsureOwnTag_NotOwner(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM tags WHERE id=").
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("other"))
	if err := a.ensureOwnTag(context.Background(), "u-test", "t1"); err == nil {
		t.Error("expected access denied for non-owner")
	}
}
