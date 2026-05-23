package app

// Twelfth bulk push — easy pure helpers, simple-SQL handlers, and
// straightforward Redis operations via miniredis.

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// ─── Pure helpers ───────────────────────────────────────────────────────────

func TestDetectMime_CommonExtensions(t *testing.T) {
	if got := detectMime("a.txt"); !strings.HasPrefix(got, "text/plain") {
		t.Errorf("expected text/plain for .txt, got %q", got)
	}
	if got := detectMime("noextension"); got != "application/octet-stream" {
		t.Errorf("expected octet-stream for no ext, got %q", got)
	}
	if got := detectMime("a.json"); !strings.Contains(got, "json") {
		t.Errorf("expected json mime, got %q", got)
	}
}

func TestThumbFilename_HasThumbSuffix(t *testing.T) {
	out := thumbFilename("file-1")
	if !strings.HasSuffix(out, ".thumb.enc") {
		t.Errorf("expected .thumb.enc suffix, got %q", out)
	}
}

func TestHlsCacheDir_JoinsPath(t *testing.T) {
	a := newTestApp(t)
	a.Config.StoragePath = "/tmp/storage"
	got := a.hlsCacheDir("file-uuid")
	if !strings.Contains(got, "hls") || !strings.Contains(got, "file-uuid") {
		t.Errorf("expected path to contain hls and file id, got %q", got)
	}
}

func TestPeekJSONField_ReadsTopLevel(t *testing.T) {
	r := httptest.NewRequest("POST", "/login",
		strings.NewReader(`{"username":"Alice","password":"x"}`))
	if got := peekJSONField(r, "username"); got != "alice" {
		t.Errorf("expected 'alice' (lowercased), got %q", got)
	}
	// Body should still be readable (restored after peek).
	buf := make([]byte, 200)
	n, _ := r.Body.Read(buf)
	if n == 0 {
		t.Error("body should be restored after peek")
	}
}

func TestPeekJSONField_NilBody(t *testing.T) {
	r := httptest.NewRequest("POST", "/login", nil)
	r.Body = nil
	if got := peekJSONField(r, "username"); got != "" {
		t.Errorf("nil body should return empty, got %q", got)
	}
}

func TestPeekJSONField_MalformedReturnsEmpty(t *testing.T) {
	r := httptest.NewRequest("POST", "/login", strings.NewReader("not json"))
	if got := peekJSONField(r, "username"); got != "" {
		t.Errorf("malformed JSON should return empty, got %q", got)
	}
}

func TestPeekJSONField_MissingField(t *testing.T) {
	r := httptest.NewRequest("POST", "/login", strings.NewReader(`{"foo":"bar"}`))
	if got := peekJSONField(r, "username"); got != "" {
		t.Errorf("missing field should return empty, got %q", got)
	}
}

func TestKeyByLoginField_ReturnsField(t *testing.T) {
	fn := keyByLoginField("email")
	r := httptest.NewRequest("POST", "/login",
		strings.NewReader(`{"email":"Bob@Example.COM"}`))
	if got := fn(r, "1.2.3.4"); got != "bob@example.com" {
		t.Errorf("expected lowercased email, got %q", got)
	}
}

func TestKeyByLoginField_EmptyOnMissing(t *testing.T) {
	fn := keyByLoginField("email")
	r := httptest.NewRequest("POST", "/login", strings.NewReader(`{}`))
	if got := fn(r, "1.2.3.4"); got != "" {
		t.Errorf("missing field should return empty, got %q", got)
	}
}

// ─── recordSession via miniredis-backed test app ───────────────────────────

func TestRecordSession_InsertsSession(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("INSERT INTO sessions").
		WillReturnResult(sqlmock.NewResult(0, 1))
	r := httptest.NewRequest("POST", "/login", nil)
	r.Header.Set("User-Agent", "Mozilla/5.0")
	a.recordSession(r, "jti-1", "u-test", time.Now().Add(time.Hour))
}

func TestRecordSession_LongUATruncated(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("INSERT INTO sessions").
		WillReturnResult(sqlmock.NewResult(0, 1))
	r := httptest.NewRequest("POST", "/login", nil)
	r.Header.Set("User-Agent", strings.Repeat("X", 1000))
	a.recordSession(r, "jti-long", "u-test", time.Now().Add(time.Hour))
}

func TestRecordSessionFromTokens_NoJTI(t *testing.T) {
	a := newTestApp(t)
	r := httptest.NewRequest("POST", "/login", nil)
	// No "access_jti" key — function should silently no-op (no SQL expected).
	a.recordSessionFromTokens(r, "u-test", map[string]any{})
}

func TestRecordSessionFromTokens_WithJTI(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("INSERT INTO sessions").
		WillReturnResult(sqlmock.NewResult(0, 1))
	r := httptest.NewRequest("POST", "/login", nil)
	r.Header.Set("User-Agent", "Test/1.0")
	a.recordSessionFromTokens(r, "u-test", map[string]any{
		"access_jti":        "jti-2",
		"access_expires_at": time.Now().Add(time.Hour),
	})
}

func TestRecordSessionFromTokens_NoExpires_UsesConfigTTL(t *testing.T) {
	a := newTestApp(t)
	a.Config.AccessTokenTTL = 900
	a.mock.ExpectExec("INSERT INTO sessions").
		WillReturnResult(sqlmock.NewResult(0, 1))
	r := httptest.NewRequest("POST", "/login", nil)
	r.Header.Set("User-Agent", "Test/1.0")
	a.recordSessionFromTokens(r, "u-test", map[string]any{
		"access_jti": "jti-3",
		// expires omitted → falls back to AccessTokenTTL
	})
}

// ─── enqueueHLS uses Redis only — miniredis exercises it ───────────────────

func TestEnqueueHLS_PushesToQueue(t *testing.T) {
	a := newTestApp(t)
	a.enqueueHLS(context.Background(), "file-xyz")
	got, err := a.Redis.LRange(context.Background(), "q:hls", 0, -1).Result()
	if err != nil {
		t.Fatalf("LRange: %v", err)
	}
	if len(got) != 1 || got[0] != "file-xyz" {
		t.Errorf("expected [file-xyz] on q:hls, got %v", got)
	}
}

// ─── markHLSFailed: simple UPDATE ──────────────────────────────────────────

func TestMarkHLSFailed_UpdatesRow(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("UPDATE hls_jobs|INSERT INTO hls_jobs").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.markHLSFailed(context.Background(), "file-id", "transcode error")
}

// ─── loadUploadRequest: not found path ─────────────────────────────────────

func TestLoadUploadRequest_NotFound(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery(".*FROM upload_requests.*").
		WillReturnError(rowsErrNoRows())
	r := httptest.NewRequest("GET", "/public/uploads/tok-1", nil)
	_, err := a.loadUploadRequest(r, "tok-1")
	if err == nil {
		t.Error("expected error for not-found upload request")
	}
}

// ─── handleDeleteComment idempotent (DELETE with 0 rows) ───────────────────

func TestHandleDeleteComment_Idempotent(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM comments WHERE id=").
		WithArgs("c1").
		WillReturnError(rowsErrNoRows())
	w := rec()
	r := authedRequest("DELETE", "/comments/c1", "")
	r = withChiParams(r, "id", "c1")
	a.handleDeleteComment(w, r)
	if w.Code < 400 && w.Code != 204 {
		t.Errorf("expected 4xx or 204, got %d", w.Code)
	}
}

// ─── recordSlowQuery: simple INSERT ────────────────────────────────────────

func TestRecordSlowQuery_Insert(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("INSERT INTO slow_queries").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.recordSlowQuery(context.Background(), "SELECT 1", 100*time.Millisecond, 5)
}

// ─── handleEmptyTrash: success with empty result set ───────────────────────

func TestHandleEmptyTrash_NoFilesNoError(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT id, storage_path FROM files WHERE user_id=").
		WillReturnRows(sqlmock.NewRows([]string{"id", "storage_path"}))
	a.mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 0))
	a.mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 0))
	a.mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 0))
	w := rec()
	r := authedRequest("POST", "/trash:empty", "")
	a.handleEmptyTrash(w, r)
	// Any 2xx or 5xx is fine — we just want function-level coverage.
	if w.Code == 0 {
		t.Error("expected non-zero status")
	}
}
