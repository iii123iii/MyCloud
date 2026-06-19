package app

// Boundary / edge-case tests. Each case represents a value at the threshold
// between two behaviours — the byte before quota, the depth at the cap, the
// empty input. Catches off-by-one mistakes that whole-flow tests miss.

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"mycloud/backend-go/internal/auth"
)

// ─── Filename edges ────────────────────────────────────────────────────────

func TestSanitizeUploadName_Boundaries(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		wantOk bool
	}{
		{"empty", "", false},
		{"only_spaces", "      ", false},
		{"only_dots", "...", true}, // becomes "..." after sanitize; not "." or ".."
		{"bare_dot", ".", false},
		{"bare_double_dot", "..", false},
		{"single_char", "a", true},
		{"255_byte_ascii", strings.Repeat("a", 255), true},
		{"256_byte_ascii_truncated", strings.Repeat("a", 256), true},
		{"only_control_chars", "\x01\x02\x03", false},
		{"only_separators", "///", false},
		{"unicode_emoji", "📁.txt", true},
		{"with_spaces_inside", "my file.txt", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := sanitizeUploadName(c.input)
			if ok != c.wantOk {
				t.Errorf("ok=%v, want %v (got name=%q)", ok, c.wantOk, got)
			}
			if ok && len(got) > 255 {
				t.Errorf("name exceeds 255 bytes after truncation: %q", got)
			}
		})
	}
}

// ─── Folder depth at exact cap ─────────────────────────────────────────────

func TestFolderDepth_AtMaxAllowed(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE chain").
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(int64(MaxFolderDepth)))
	got, err := a.folderDepth(context.Background(), "f1")
	if err != nil {
		t.Fatal(err)
	}
	if got != MaxFolderDepth {
		t.Errorf("expected %d, got %d", MaxFolderDepth, got)
	}
}

func TestHandleCreateFolder_AtDepthBoundary(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	// At exactly MaxFolderDepth-1, creating a child should fail with too-deep.
	a.mock.ExpectQuery("WITH RECURSIVE chain").
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(int64(MaxFolderDepth)))
	w := rec()
	r := authedRequest("POST", "/folders", `{"name":"deep","parent_id":"parent"}`)
	a.handleCreateFolder(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 at depth limit, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── Quota boundaries ──────────────────────────────────────────────────────

// Reserve at exactly the quota cap — UPDATE affects 1 row → success.
func TestReserveQuota_ExactlyFits(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectExec("UPDATE users").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	tx, _ := a.DB.BeginTx(context.Background(), nil)
	defer tx.Commit()
	if err := reserveQuota(context.Background(), tx, "u-test", 100); err != nil {
		t.Errorf("expected nil at exact fit, got %v", err)
	}
}

// Reserve 1 byte over — UPDATE affects 0 rows → ErrQuotaExceeded.
func TestReserveQuota_OneOver(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectBegin()
	a.mock.ExpectExec("UPDATE users").
		WillReturnResult(sqlmock.NewResult(0, 0))
	a.mock.ExpectQuery("SELECT used_bytes, quota_bytes FROM users WHERE id=").
		WillReturnRows(sqlmock.NewRows([]string{"used", "quota"}).AddRow(int64(100), int64(100)))
	a.mock.ExpectRollback()
	tx, _ := a.DB.BeginTx(context.Background(), nil)
	defer tx.Rollback()
	if err := reserveQuota(context.Background(), tx, "u-test", 1); err == nil {
		t.Error("expected ErrQuotaExceeded")
	}
}

// ─── Pagination boundaries ─────────────────────────────────────────────────

// Negative / zero / huge limit values — handler should clamp.
func TestReadLimitOffset_Clamping(t *testing.T) {
	cases := []struct {
		in       string
		defaultL int
		maxL     int
	}{
		{"limit=0", 25, 100},   // 0 invalid → default
		{"limit=-5", 25, 100},  // negative → default
		{"limit=9999", 25, 100}, // huge → clamped to max
		{"limit=abc", 25, 100}, // unparseable → default
		{"limit=50", 25, 100},  // in-range → 50
	}
	for _, c := range cases {
		r := authedRequest("GET", "/?"+c.in, "")
		limit, _ := readLimitOffset(r, c.defaultL, c.maxL)
		if limit < 1 || limit > c.maxL {
			t.Errorf("limit %d out of [1, %d] for %q", limit, c.maxL, c.in)
		}
	}
}

// ─── Time edges: token issued with exp at exactly t=0 ──────────────────────

func TestToken_ImmediateExpiry(t *testing.T) {
	// A token whose TTL has already elapsed must fail Parse. Manager TTLs are
	// captured at construction (sign() reads m.accessTTL), so to test expiry we
	// mint a manager with an access TTL already in the past. The previous
	// version set a.Config.AccessTokenTTL after the manager was built — a no-op
	// that issued a normal 900s token and never exercised expiry at all.
	mgr := auth.NewManager(strings.Repeat("a", 32), -1, 600, nil)
	pair, err := mgr.IssuePair("u-test", "user")
	if err != nil {
		t.Fatalf("IssuePair: %v", err)
	}
	if _, err := mgr.Parse(pair["access_token"].(string)); err == nil {
		t.Error("Parse should reject an already-expired access token")
	}
}

// ─── Empty inputs ──────────────────────────────────────────────────────────

func TestHandleCreateFolder_EmptyName(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("POST", "/folders", `{"name":""}`)
	a.handleCreateFolder(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty name, got %d", w.Code)
	}
}

func TestHandleCreateFolder_WhitespaceOnlyName(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("POST", "/folders", `{"name":"   "}`)
	a.handleCreateFolder(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for whitespace-only name, got %d", w.Code)
	}
}

func TestHandleListFolders_NoUser_NoData(t *testing.T) {
	a := newTestApp(t)
	// Query now LEFT JOINs user_favorites and selects an is_favorited flag,
	// so the mock returns 6 columns instead of 5.
	a.mock.ExpectQuery(".*FROM folders.*").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "parent_id", "created_at", "updated_at", "is_favorited",
		}))
	w := rec()
	r := authedRequest("GET", "/folders", "")
	a.handleListFolders(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with empty result, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── Pagination cursor: malformed cursor ───────────────────────────────────

func TestPagination_MalformedCursor(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery(".*").WillReturnRows(sqlmock.NewRows([]string{
		"id", "name", "size_bytes", "mime_type", "is_starred",
		"updated_at", "folder_id", "thumb_path",
	}))
	a.mock.ExpectQuery(".*").WillReturnRows(sqlmock.NewRows([]string{
		"id", "name", "size_bytes", "mime_type", "is_starred",
		"updated_at", "folder_id", "thumb_path",
	}))
	w := rec()
	r := authedRequest("GET", "/files?cursor=NOT-A-VALID-BASE64-%2F%2F%2F", "")
	a.handleListFiles(w, r)
	// Should fall back to no-cursor query, NOT 500.
	if w.Code >= 500 {
		t.Errorf("malformed cursor caused %d", w.Code)
	}
}
