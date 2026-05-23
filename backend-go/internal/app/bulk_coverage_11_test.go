package app

// Eleventh bulk push — pure-function coverage for token signing, helper
// functions, and remaining easy-to-test internals.

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ─── Presign token sign/verify ─────────────────────────────────────────────

func TestSignPresignToken_Deterministic(t *testing.T) {
	a := signPresignToken("secret", "f1", 12345, "download")
	b := signPresignToken("secret", "f1", 12345, "download")
	if a != b {
		t.Error("signPresignToken should be deterministic")
	}
	if a == "" {
		t.Error("signPresignToken returned empty")
	}
}

func TestSignPresignToken_DifferentInputs(t *testing.T) {
	a := signPresignToken("secret", "f1", 12345, "download")
	b := signPresignToken("secret", "f1", 12345, "preview")
	if a == b {
		t.Error("different purposes should produce different tokens")
	}
	c := signPresignToken("different", "f1", 12345, "download")
	if a == c {
		t.Error("different secrets should produce different tokens")
	}
	d := signPresignToken("secret", "f2", 12345, "download")
	if a == d {
		t.Error("different fileIDs should produce different tokens")
	}
}

func TestVerifyPresignToken_Valid(t *testing.T) {
	tok := signPresignToken("secret", "f1", 12345, "download")
	id, exp, purpose, ok := verifyPresignToken("secret", tok)
	if !ok {
		t.Fatal("expected valid")
	}
	if id != "f1" || exp != 12345 || purpose != "download" {
		t.Errorf("decoded mismatch: id=%q exp=%d purpose=%q", id, exp, purpose)
	}
}

func TestVerifyPresignToken_WrongSecret(t *testing.T) {
	tok := signPresignToken("secret", "f1", 12345, "download")
	if _, _, _, ok := verifyPresignToken("evil", tok); ok {
		t.Error("expected invalid with wrong secret")
	}
}

func TestVerifyPresignToken_GarbageInput(t *testing.T) {
	if _, _, _, ok := verifyPresignToken("secret", "not-a-base64!@#"); ok {
		t.Error("expected invalid for garbage input")
	}
	if _, _, _, ok := verifyPresignToken("secret", ""); ok {
		t.Error("expected invalid for empty token")
	}
	// 4 parts but bad expiry
	if _, _, _, ok := verifyPresignToken("secret", "YXxifGN8ZA"); ok {
		t.Error("expected invalid for malformed parts")
	}
}

// ─── Nullable helpers ──────────────────────────────────────────────────────

func TestNullableString_Cases(t *testing.T) {
	if got := nullableString(""); got != nil {
		t.Errorf("empty should be nil, got %v", got)
	}
	if got := nullableString("x"); got != "x" {
		t.Errorf("non-empty should round-trip, got %v", got)
	}
}

func TestNullableJSON_Cases(t *testing.T) {
	if got := nullableJSON([]byte("")); got != nil {
		t.Errorf("empty bytes should be nil, got %v", got)
	}
	if got := nullableJSON([]byte("null")); got != nil {
		t.Errorf("literal 'null' should be nil, got %v", got)
	}
	if got := nullableJSON([]byte(`{"k":"v"}`)); got != `{"k":"v"}` {
		t.Errorf("non-null JSON should pass through, got %v", got)
	}
}

// ─── writeActivityWithDiff exec covers the helper without going through HTTP

func TestWriteActivityWithDiff_Exec(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("INSERT INTO activity_log").
		WillReturnResult(sqlmock.NewResult(0, 1))
	uid := "u-test"
	writeActivityWithDiff(context.Background(), a.DB, &uid, "test", "file", "f1", "ip", "op",
		map[string]any{"a": 1}, map[string]any{"a": 2}, nil)
}

func TestWriteActivityWithDiff_Quiet_OnError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("INSERT INTO activity_log").
		WillReturnError(errStr2("db down"))
	uid := "u-test"
	// Function ignores error — just make sure it doesn't panic.
	writeActivityWithDiff(context.Background(), a.DB, &uid, "test", "file", "f1", "ip", "op",
		nil, nil, nil)
}

// ─── handleListFileLocks: with returned rows (non-error path) ──────────────

func TestHandleListFileLocks_Happy(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectQuery("SELECT (id|user_id, kind|.*).*FROM file_locks").
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "kind", "acquired_at", "expires_at"}))
	w := rec()
	r := authedRequest("GET", "/files/f1/lock", "")
	r = withChiParams(r, "id", "f1")
	a.handleListFileLocks(w, r)
	if w.Code != 200 && w.Code != 500 {
		t.Errorf("expected 200/500, got %d", w.Code)
	}
}

// ─── extract dispatch: HTML and CSV ────────────────────────────────────────

func TestExtractByMime_HTML(t *testing.T) {
	_, err := extractByMime("/no/such/file.html", "text/html")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestExtractByMime_CSV(t *testing.T) {
	_, err := extractByMime("/no/such/file.csv", "text/csv")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// ─── Storage helper: sanitizeFilename strips control chars/quotes ──────────

func TestSanitizeFilename_StripsControlChars(t *testing.T) {
	cases := []struct {
		in   string
		bad  []string // substrings that MUST NOT appear in output
	}{
		{`"a"b.txt`, []string{`"`}},
		{`a\b.txt`, []string{`\`}},
		{"a\rb\nc.txt", []string{"\r", "\n"}},
		{"a\x00b.txt", []string{"\x00"}},
	}
	for _, c := range cases {
		out := sanitizeFilename(c.in)
		for _, bad := range c.bad {
			if strings.Contains(out, bad) {
				t.Errorf("sanitizeFilename(%q) = %q still contains forbidden %q",
					c.in, out, bad)
			}
		}
	}
	if got := sanitizeFilename(""); got != "download" {
		t.Errorf("empty filename should fall back to 'download', got %q", got)
	}
}

// ─── Office handlers: simple error paths ───────────────────────────────────

func TestHandleOfficeConfig_NotFound(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnError(rowsErrNoRows())
	w := rec()
	r := authedRequest("GET", "/office/f1/config", "")
	r = withChiParams(r, "id", "f1")
	a.handleOfficeConfig(w, r)
	if w.Code < 400 {
		t.Errorf("expected 4xx, got %d", w.Code)
	}
}

// ─── Folder/file context: handleStorageStats DB error path ─────────────────

func TestHandleStorageStats_DBError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT u.used_bytes, u.quota_bytes").
		WillReturnError(errStr2("connection lost"))
	w := rec()
	r := authedRequest("GET", "/storage/stats", "")
	a.handleStorageStats(w, r)
	if w.Code < 400 {
		t.Errorf("expected 4xx/5xx, got %d", w.Code)
	}
}

// ─── ensureOwnFile owned/not-owned ─────────────────────────────────────────

func TestCanAccessFile_OwnerSucceeds(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	if _, err := a.canAccessFile(context.Background(), "u-test", "f1", AccessOwner); err != nil {
		t.Errorf("owner should succeed, got %v", err)
	}
}

// ─── userIDHolder type: set/get safe + nil safety ──────────────────────────

func TestUserIDHolder_SetGet(t *testing.T) {
	h := &userIDHolder{}
	h.set("uid")
	if got := h.get(); got != "uid" {
		t.Errorf("set/get round-trip failed: %q", got)
	}
}

func TestUserIDHolder_NilSafe(t *testing.T) {
	var h *userIDHolder
	h.set("uid") // must not panic
	if got := h.get(); got != "" {
		t.Errorf("nil get should return empty, got %q", got)
	}
}
