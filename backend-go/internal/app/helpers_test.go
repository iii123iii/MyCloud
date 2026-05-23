package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// ─── clientIP ───────────────────────────────────────────────────────────────

func TestClientIP_XForwardedFor(t *testing.T) {
	cases := []struct {
		name string
		xff  string
		ra   string
		want string
	}{
		{"single xff", "1.2.3.4", "10.0.0.1:34567", "1.2.3.4"},
		{"chain takes first", "1.2.3.4, 5.6.7.8, 9.10.11.12", "10.0.0.1:34567", "1.2.3.4"},
		{"xff with spaces", "  1.2.3.4  , 5.6.7.8 ", "10.0.0.1:34567", "1.2.3.4"},
		{"falls back to RemoteAddr", "", "203.0.113.42:51000", "203.0.113.42"},
		{"RemoteAddr without port", "", "203.0.113.42", "203.0.113.42"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = c.ra
			if c.xff != "" {
				r.Header.Set("X-Forwarded-For", c.xff)
			}
			if got := clientIP(r); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// ─── readLimitOffset ────────────────────────────────────────────────────────

func TestReadLimitOffset(t *testing.T) {
	cases := []struct {
		name             string
		query            string
		defaultL, maxL   int
		wantLimit, wantOffset int
	}{
		{"defaults when missing", "", 50, 500, 50, 0},
		{"honours explicit limit", "limit=10", 50, 500, 10, 0},
		{"caps at max", "limit=99999", 50, 500, 500, 0},
		{"floors at 1", "limit=0", 50, 500, 50, 0},
		{"negative limit falls back to default", "limit=-5", 50, 500, 50, 0},
		{"non-integer limit falls back to default", "limit=banana", 50, 500, 50, 0},
		{"offset honored", "offset=100", 50, 500, 50, 100},
		{"negative offset falls back to 0", "offset=-1", 50, 500, 50, 0},
		{"combined", "limit=20&offset=40", 50, 500, 20, 40},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/?"+c.query, nil)
			gotL, gotO := readLimitOffset(r, c.defaultL, c.maxL)
			if gotL != c.wantLimit {
				t.Errorf("limit: got %d, want %d", gotL, c.wantLimit)
			}
			if gotO != c.wantOffset {
				t.Errorf("offset: got %d, want %d", gotO, c.wantOffset)
			}
		})
	}
}

// ─── boolSetting ────────────────────────────────────────────────────────────

func TestBoolSetting(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{"1", true},
		{"false", false},
		{"0", false},
		{"", false},
		{"yes", false},
		{"on", false},
		{"  true  ", false}, // strict: no trim
	}
	for _, c := range cases {
		if got := boolSetting(c.in); got != c.want {
			t.Errorf("boolSetting(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// ─── qInt ───────────────────────────────────────────────────────────────────

func TestQInt(t *testing.T) {
	cases := []struct {
		name string
		q    string
		key  string
		def, minV, maxV, want int
	}{
		{"default when missing", "", "n", 25, 1, 100, 25},
		{"honoured", "n=50", "n", 25, 1, 100, 50},
		{"clamped to max", "n=99999", "n", 25, 1, 100, 100},
		{"clamped to min", "n=-5", "n", 25, 1, 100, 1},
		{"non-int returns default", "n=foo", "n", 25, 1, 100, 25},
		{"different key untouched", "x=10", "n", 25, 1, 100, 25},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/?"+c.q, nil)
			if got := qInt(r, c.key, c.def, c.minV, c.maxV); got != c.want {
				t.Errorf("qInt = %d, want %d", got, c.want)
			}
		})
	}
}

// ─── nullable helpers ───────────────────────────────────────────────────────

func TestNullableString(t *testing.T) {
	if got := nullableString(""); got != nil {
		t.Errorf("nullableString(\"\") should be nil interface, got %v", got)
	}
	if got := nullableString("hello"); got != "hello" {
		t.Errorf("nullableString(\"hello\") = %v, want \"hello\"", got)
	}
}

func TestNullableStringPtr(t *testing.T) {
	if got := nullableStringPtr(nil); got != nil {
		t.Errorf("nullableStringPtr(nil) = %v, want nil", got)
	}
	empty := ""
	if got := nullableStringPtr(&empty); got != nil {
		t.Errorf("nullableStringPtr(&\"\") = %v, want nil", got)
	}
	s := "x"
	if got := nullableStringPtr(&s); got != "x" {
		t.Errorf("nullableStringPtr(&\"x\") = %v, want \"x\"", got)
	}
}

func TestNullableJSON(t *testing.T) {
	if got := nullableJSON(nil); got != nil {
		t.Errorf("nullableJSON(nil) = %v, want nil", got)
	}
	if got := nullableJSON([]byte{}); got != nil {
		t.Errorf("nullableJSON([]byte{}) = %v, want nil", got)
	}
	if got := nullableJSON([]byte("null")); got != nil {
		t.Errorf("nullableJSON(\"null\") = %v, want nil", got)
	}
	if got := nullableJSON([]byte(`{"a":1}`)); got != `{"a":1}` {
		t.Errorf("nullableJSON should pass through valid JSON, got %v", got)
	}
}

// ─── strPtr / ts ────────────────────────────────────────────────────────────

func TestStrPtr(t *testing.T) {
	if got := strPtr(sql.NullString{Valid: false}); got != nil {
		t.Errorf("strPtr(invalid) = %v, want nil", got)
	}
	if got := strPtr(sql.NullString{Valid: true, String: "hi"}); got == nil || *got != "hi" {
		t.Errorf("strPtr(valid) wrong: got %v", got)
	}
}

func TestTs(t *testing.T) {
	in := time.Date(2026, 5, 22, 10, 30, 0, 0, time.UTC)
	got := ts(in)
	if !strings.HasSuffix(got, "Z") {
		t.Errorf("ts should end with Z (UTC), got %q", got)
	}
	if !strings.HasPrefix(got, "2026-05-22T10:30:00") {
		t.Errorf("ts wrong prefix: %q", got)
	}
	// Round-trip back through Parse to validate the format.
	if _, err := time.Parse(time.RFC3339, got); err != nil {
		t.Errorf("ts output should be RFC3339-parseable, got error %v on %q", err, got)
	}
}

// ─── parseUserDatetime ──────────────────────────────────────────────────────

func TestParseUserDatetime(t *testing.T) {
	cases := []struct {
		name    string
		in      *string
		wantNil bool
		wantErr bool
	}{
		{"nil input", nil, true, false},
		{"empty string", strPtr2(""), true, false},
		{"whitespace", strPtr2("   "), true, false},
		{"RFC3339Nano with Z", strPtr2("2026-05-27T14:46:49.577Z"), false, false},
		{"RFC3339", strPtr2("2026-05-27T14:46:49Z"), false, false},
		{"MySQL DATETIME", strPtr2("2026-05-27 14:46:49"), false, false},
		{"date only", strPtr2("2026-05-27"), false, false},
		{"malformed", strPtr2("not-a-date"), false, true},
		{"trailing garbage", strPtr2("2026-05-27Tgarbage"), false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseUserDatetime(c.in)
			if c.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !c.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if c.wantNil && got != nil {
				t.Errorf("expected nil, got %v", got)
			}
			if !c.wantNil && !c.wantErr && got == nil {
				t.Errorf("expected non-nil time, got nil")
			}
			// UTC normalisation check
			if got != nil && got.Location() != time.UTC {
				t.Errorf("returned time should be UTC, got %v", got.Location())
			}
		})
	}
}

func strPtr2(s string) *string { return &s }

// ─── decodeJSON ─────────────────────────────────────────────────────────────

func TestDecodeJSON_RejectsUnknownFields(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"alice","extra":1}`))
	var p payload
	if err := decodeJSON(r, &p); err == nil {
		t.Errorf("decodeJSON should reject unknown fields, got nil error")
	}
}

func TestDecodeJSON_AcceptsKnownFields(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"alice"}`))
	var p payload
	if err := decodeJSON(r, &p); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "alice" {
		t.Errorf("got %q, want \"alice\"", p.Name)
	}
}

func TestDecodeJSON_MalformedJSON(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":`))
	var p payload
	if err := decodeJSON(r, &p); err == nil {
		t.Errorf("should reject malformed JSON")
	}
}

// ─── normaliseName ──────────────────────────────────────────────────────────

func TestNormaliseName(t *testing.T) {
	// Mirrors search.NormaliseName behaviour — empty in, empty out; non-empty
	// at least returns a lowercased/trimmed result.
	if got := normaliseName(""); got != "" {
		t.Errorf("normaliseName(\"\") = %q, want \"\"", got)
	}
	if got := normaliseName("Hello.TXT"); got == "" {
		t.Errorf("normaliseName should return non-empty for valid input")
	}
}

// ─── getUserSession / getUserSessionFull ────────────────────────────────────

func TestGetUserSessionFull(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT role, is_active, must_change_password FROM users WHERE id=").
		WithArgs("u-1").
		WillReturnRows(sqlmock.NewRows([]string{"role", "is_active", "must_change_password"}).
			AddRow("admin", true, false))

	role, active, mustChange, err := getUserSessionFull(context.Background(), db, "u-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if role != "admin" || !active || mustChange {
		t.Errorf("wrong values: role=%q active=%v mustChange=%v", role, active, mustChange)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestGetUserSessionFull_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery("SELECT role, is_active, must_change_password FROM users WHERE id=").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)
	_, _, _, err := getUserSessionFull(context.Background(), db, "missing")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestGetUserSession_DelegatesToFull(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery("SELECT role, is_active, must_change_password FROM users WHERE id=").
		WithArgs("u-2").
		WillReturnRows(sqlmock.NewRows([]string{"role", "is_active", "must_change_password"}).
			AddRow("user", false, true))

	role, active, err := getUserSession(context.Background(), db, "u-2")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if role != "user" || active {
		t.Errorf("wrong: role=%q active=%v", role, active)
	}
}

// ─── getSetting ─────────────────────────────────────────────────────────────

func TestGetSetting_PresentValue(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery("SELECT value FROM settings WHERE key_name=").
		WithArgs("registration_enabled").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))

	got, err := getSetting(context.Background(), db, "registration_enabled", "false")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got != "true" {
		t.Errorf("got %q, want \"true\"", got)
	}
}

func TestGetSetting_MissingFallsToDefault(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery("SELECT value FROM settings WHERE key_name=").
		WithArgs("absent").
		WillReturnError(sql.ErrNoRows)

	got, err := getSetting(context.Background(), db, "absent", "fallback")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got != "fallback" {
		t.Errorf("expected fallback, got %q", got)
	}
}

func TestGetSetting_NullValueFallsToDefault(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery("SELECT value FROM settings WHERE key_name=").
		WithArgs("nullable").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(nil))

	got, err := getSetting(context.Background(), db, "nullable", "default")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got != "default" {
		t.Errorf("NULL setting should fall back to default, got %q", got)
	}
}

func TestGetSetting_DBError(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery("SELECT value FROM settings WHERE key_name=").
		WithArgs("k").
		WillReturnError(errors.New("connection lost"))
	if _, err := getSetting(context.Background(), db, "k", "x"); err == nil {
		t.Errorf("expected error to propagate")
	}
}

// ─── respondDBError ─────────────────────────────────────────────────────────

func TestRespondDBError_NoLeakage(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v2/files", nil)
	respondDBError(w, r, errors.New("Duplicate entry 'foo@bar' for key 'users.email'"))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want 500", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "Duplicate") || strings.Contains(body, "users.email") {
		t.Errorf("response leaked DB error text: %s", body)
	}
	if !strings.Contains(body, "internal_error") {
		t.Errorf("expected generic error code, got %s", body)
	}
}

func TestRespondDBError_NilIsNoOp(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	respondDBError(w, r, nil)
	if w.Code != http.StatusOK {
		t.Errorf("nil error should not write response, default 200 expected, got %d", w.Code)
	}
}
