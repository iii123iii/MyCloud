package app

// Attack-surface tests. Each case represents a real attack vector we should
// reject. Names follow the format Test<Vector>_<Endpoint>_Rejected.

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang-jwt/jwt/v5"
)

// ─── JWT algorithm-confusion ───────────────────────────────────────────────

// Forging an unsigned ("alg":"none") token and presenting it must fail
// regardless of how the rest of the token is shaped. jwt/v5 refuses "none"
// outright; we also pin to HS256 to catch caller-chosen RS/ES swaps that
// would interpret the HMAC secret as a public key.
func TestJWTAlgorithmNone_Rejected(t *testing.T) {
	a := newTestApp(t)
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"uid":"u-test","role":"admin","exp":9999999999}`))
	// No signature
	tok := header + "." + payload + "."
	if _, err := a.Auth.Parse(tok); err == nil {
		t.Error("alg:none token should be rejected")
	}
}

// HS-vs-RS algorithm confusion: a token signed with an attacker-chosen
// algorithm should be rejected since we pin HS256.
func TestJWTAlgorithmRS256_Rejected(t *testing.T) {
	a := newTestApp(t)
	// Build an RS256 header but sign with a random byte string. Parse should
	// short-circuit on the method check before signature verification runs.
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"uid":"u-test","role":"admin","exp":9999999999}`))
	tok := header + "." + payload + ".sig"
	if _, err := a.Auth.Parse(tok); err == nil {
		t.Error("RS256-signed token should be rejected by HS256 pin")
	}
}

// An HS512 token signed with our secret should also be rejected since we
// only accept HS256. Defence-in-depth check.
func TestJWTAlgorithmHS512_Rejected(t *testing.T) {
	a := newTestApp(t)
	tok := jwt.NewWithClaims(jwt.SigningMethodHS512, jwt.MapClaims{
		"uid": "u-test", "role": "user",
		"exp": 9999999999,
	})
	signed, _ := tok.SignedString([]byte(strings.Repeat("a", 32)))
	if _, err := a.Auth.Parse(signed); err == nil {
		t.Error("HS512 token should be rejected by HS256 pin")
	}
}

// ─── Path traversal via filename ───────────────────────────────────────────

func TestPathTraversal_FilenameSanitizer(t *testing.T) {
	cases := []string{
		"../../etc/passwd",
		"..\\..\\Windows\\System32",
		"/etc/passwd",
		"..",
		".",
		"  ..  ",
		"./../../config.json",
		// Null-byte truncation
		"normal.txt\x00../../evil",
		// Unicode RTL override (visual reordering)
		"normal‮txt.exe",
	}
	for _, name := range cases {
		clean, ok := sanitizeUploadName(name)
		if !ok {
			continue // rejection is fine
		}
		// If accepted, the result must never contain path separators or be
		// "." / ".."
		if strings.ContainsAny(clean, "/\\") {
			t.Errorf("sanitizeUploadName(%q) accepted but kept separator: %q", name, clean)
		}
		if clean == "." || clean == ".." {
			t.Errorf("sanitizeUploadName(%q) accepted bare dot path: %q", name, clean)
		}
		if strings.Contains(clean, "\x00") {
			t.Errorf("sanitizeUploadName(%q) kept null byte", name)
		}
	}
}

// ─── XSS in Content-Disposition ────────────────────────────────────────────

func TestXSS_FilenameInContentDisposition(t *testing.T) {
	// sanitizeFilename strips quotes and CR/LF — both critical so the
	// attacker can't escape the Content-Disposition field and inject a new
	// header line.
	out := sanitizeFilename(`"; X-Evil: pwned` + "\r\nfilename=\"x.txt\"")
	if strings.Contains(out, "\r") || strings.Contains(out, "\n") {
		t.Errorf("filename CRLF should be stripped, got %q", out)
	}
	if strings.Contains(out, `"`) {
		t.Errorf("filename quote should be stripped, got %q", out)
	}
}

// ─── SQL injection probes (via the params layer) ───────────────────────────

// Sort param fuzzing — verify the response body never echoes SQL
// metachars back. Reading handlers_files.go shows allowedSort is local —
// any unrecognised ?sort defaults to "name". If the malicious string leaked
// to SQL or to the response, this would catch it.
func TestSQLInjection_SortParamNotEchoed(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	// Permissive mocks for any query the handler issues.
	a.mock.ExpectQuery(".*").WillReturnRows(sqlmock.NewRows([]string{
		"id", "name", "size_bytes", "mime_type", "is_starred",
		"updated_at", "folder_id", "thumb_path",
	}))
	a.mock.ExpectQuery(".*").WillReturnRows(sqlmock.NewRows([]string{
		"id", "name", "size_bytes", "mime_type", "is_starred",
		"updated_at", "folder_id", "thumb_path",
	}))
	w := rec()
	bad := "name;DROP+TABLE+users--"
	r := authedRequest("GET", "/files?sort="+bad, "")
	a.handleListFiles(w, r)
	if strings.Contains(w.Body.String(), "DROP TABLE") {
		t.Errorf("response echoed malicious sort value: %s", w.Body.String())
	}
}

// Search query with SQL meta-chars should be parameter-bound, not
// concatenated. We verify by feeding ' OR 1=1-- and observing no crash.
func TestSQLInjection_SearchQuery(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery(".*").WillReturnRows(sqlmock.NewRows([]string{
		"item_type", "id", "name", "size_bytes", "mime_type", "is_starred", "updated_at",
	}))
	w := rec()
	r := authedRequest("GET", "/search?q=%27+OR+1%3D1--&scope=name", "")
	a.handleSearch(w, r)
	if w.Code >= 500 {
		t.Errorf("SQL meta-chars in search caused %d", w.Code)
	}
}

// ─── CORS bypass attempts ──────────────────────────────────────────────────

func TestCORS_NullOriginRejected(t *testing.T) {
	a := newTestApp(t)
	a.Config.AllowedOrigins = []string{"https://app.example.com"}
	cors := a.cors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Origin", "null")
	w := httptest.NewRecorder()
	cors.ServeHTTP(w, r)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got == "null" {
		t.Errorf("CORS reflected 'null' origin: %q", got)
	}
}

func TestCORS_SuffixSpoofRejected(t *testing.T) {
	a := newTestApp(t)
	a.Config.AllowedOrigins = []string{"https://app.example.com"}
	cors := a.cors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Origin", "https://app.example.com.evil.com")
	w := httptest.NewRecorder()
	cors.ServeHTTP(w, r)
	if got := w.Header().Get("Access-Control-Allow-Origin"); strings.Contains(got, "evil.com") {
		t.Errorf("CORS allowed suffix-spoofed origin: %q", got)
	}
}

func TestCORS_EmptyAllowedOriginsFailsClosed(t *testing.T) {
	a := newTestApp(t)
	a.Config.AllowedOrigins = []string{} // fail-closed
	cors := a.cors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Origin", "https://anything.com")
	w := httptest.NewRecorder()
	cors.ServeHTTP(w, r)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("CORS reflected origin with empty allowed-list: %q", got)
	}
}

// ─── Open-redirect probes ──────────────────────────────────────────────────

// Login responses must never embed a Location header with an attacker-supplied
// destination. Our handlers don't redirect, but this codifies the property.
func TestNoOpenRedirect_LoginResponse(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT id, username, email").
		WillReturnError(rowsErrNoRows())
	w := rec()
	r := httptest.NewRequest("POST", "/auth/login?redirect=//evil.com",
		strings.NewReader(`{"username":"x","password":"y"}`))
	r.Header.Set("Content-Type", "application/json")
	a.handleLogin(w, r)
	if loc := w.Header().Get("Location"); loc != "" {
		t.Errorf("login should never set Location, got %q", loc)
	}
}

// ─── Resource exhaustion ───────────────────────────────────────────────────

// Documents current behavior: decodeJSON has no body-size cap, so a 1 MB
// JSON body is fully consumed and parsed. NOTE: this is a DoS risk — see
// the spawned task to wrap decodeJSON with http.MaxBytesReader. Test
// remains green by setting up a permissive mock for the eventual SQL.
func TestResourceExhaustion_LargeJSONBody(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	// canAccessFile (editor) gates the rename; owner path here.
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	a.mock.ExpectExec("UPDATE files SET name").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("INSERT INTO activity_log").
		WillReturnResult(sqlmock.NewResult(0, 1))
	big := strings.Repeat("A", 1024*1024) // 1 MB
	body := `{"name":"` + big + `"}`
	w := rec()
	r := authedRequest("PATCH", "/files/f1", body)
	r = withChiParams(r, "id", "f1")
	a.handleUpdateFile(w, r)
	// Either accepts (current bug) or rejects (post-fix). Both are valid
	// outcomes for this regression test — we just want NO panic / NO 500.
	if w.Code >= 500 {
		t.Errorf("1 MB body caused %d", w.Code)
	}
}

// Deeply-nested JSON should not stack-overflow the decoder. Build a
// 10 000-level nested array.
func TestResourceExhaustion_NestedJSON(t *testing.T) {
	a := newTestApp(t)
	depth := 10000
	body := strings.Repeat("[", depth) + strings.Repeat("]", depth)
	w := rec()
	r := authedRequest("POST", "/folders", body)
	a.handleCreateFolder(w, r)
	if w.Code >= 500 {
		t.Errorf("nested JSON should be rejected as 4xx, got %d", w.Code)
	}
}

// 10 KB-long filename should be truncated to ≤255 bytes per NAME_MAX.
// NOTE: Larger inputs (e.g. 1 MB) trigger O(n²) behavior in the truncation
// loop at storage_helpers.go:85 (each iteration calls string(runes)).
// Filed as a separate task — the loop should use byte-aware truncation.
func TestResourceExhaustion_LongFilename(t *testing.T) {
	long := strings.Repeat("x", 10*1024) // 10 KB
	got, ok := sanitizeUploadName(long)
	if !ok {
		return // outright rejection is fine
	}
	if len(got) > 255 {
		t.Errorf("expected truncation to ≤255 bytes, got len=%d", len(got))
	}
}

// ─── User enumeration timing-equivalence smoke ─────────────────────────────

// Login with a non-existent user should still execute a bcrypt compare so
// timing doesn't leak account existence. We can't measure timing in a unit
// test reliably, but we CAN assert the dummy-hash code path was reached by
// looking for the same number of bcrypt calls — which here we approximate
// by verifying the handler returns the same 401 shape regardless of whether
// the user exists.
func TestLoginTiming_NoExistsResponseShape(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT id, username, email").
		WillReturnError(rowsErrNoRows())
	w := rec()
	body := `{"username":"ghost","password":"anything"}`
	r := httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	a.handleLogin(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	// Response shape must not leak "user not found" vs "wrong password".
	body2 := strings.ToLower(w.Body.String())
	if strings.Contains(body2, "not found") || strings.Contains(body2, "no such") {
		t.Errorf("login error leaks user existence: %s", body2)
	}
}

// ─── Rate-limit bypass via header injection ────────────────────────────────

// X-Forwarded-For with multiple comma-separated IPs — we should pick the
// LEFTMOST trustable one, not the rightmost (which the attacker controls
// when sitting behind a proxy chain).
func TestRateLimit_XForwardedForParsing(t *testing.T) {
	cases := []struct {
		hdr   string
		first string
	}{
		{"203.0.113.1", "203.0.113.1"},
		{"203.0.113.1, 10.0.0.1", "203.0.113.1"},
		{"203.0.113.1, 198.51.100.2, 10.0.0.1", "203.0.113.1"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("X-Forwarded-For", c.hdr)
		got := clientIP(r)
		if got != c.first {
			t.Errorf("clientIP for %q = %q, want %q", c.hdr, got, c.first)
		}
	}
}

// ─── Token-blacklist bypass via family ─────────────────────────────────────

// After RevokeFamily, every token sharing that family must fail Parse.
func TestFamilyRevocation_AffectsAllSessions(t *testing.T) {
	a := newTestApp(t)
	pair1, _ := a.Auth.IssuePair("u-test", "user")
	pair2, _ := a.Auth.IssuePairForRefresh("u-test", "user", pair1["family"].(string))
	a.Auth.RevokeFamily(pair1["family"].(string))
	if _, err := a.Auth.Parse(pair1["access_token"].(string)); err == nil {
		t.Error("first session's access token should be rejected after family revoke")
	}
	if _, err := a.Auth.Parse(pair2["access_token"].(string)); err == nil {
		t.Error("second session's access token should be rejected after family revoke")
	}
}
