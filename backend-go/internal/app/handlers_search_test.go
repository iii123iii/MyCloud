package app

// Tests for folder-scoped search (the optional ?folder_id param on
// handleSearch). These follow the package's mock-based convention: assert the
// SQL shape via regex AND the bound args via WithArgs (a bare ".*" body would
// match anything, so the arg lists are what actually lock the scoping in).
//
// Regexes are chosen to be mutually exclusive across the queries a single
// request fires, so MatchExpectationsInOrder(false) can't mis-route:
//   - subtree CTE      → "WITH RECURSIVE folder_tree"
//   - name UNION (scoped) → "parent_id IN"   (folders arm; CTE uses "parent_id =")
//   - name UNION (global) → "UNION ALL"      (content has no UNION)
//   - content          → "JOIN file_text"
//   - DSL files query  → "folder_id IN.*LIMIT 100"

import (
	"net/http"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// searchRowCols is the 7-column shape every name/content search query returns.
var searchRowCols = []string{
	"item_type", "id", "name", "size_bytes", "mime_type", "is_starred", "updated_at",
}

// Folder-scoped scope=both: the subtree CTE runs first, then both the name
// UNION and the content query carry the subtree ids in the right positions.
func TestHandleSearch_FolderScoped_NameAndContent(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)

	// collectFolderTree(ctx, userID, folderID) → args (folderID, userID, userID)
	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WithArgs("F", "u-test", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("F").AddRow("A").AddRow("B"))

	// Name UNION: files arm folder_id IN, folders arm parent_id IN.
	// Args: userID, like, q, <ids>, userID, like, <ids>. The two LIKE
	// patterns go through search.NormaliseName, so match them loosely.
	a.mock.ExpectQuery("parent_id IN").
		WithArgs("u-test", sqlmock.AnyArg(), "report", "F", "A", "B", "u-test", sqlmock.AnyArg(), "F", "A", "B").
		WillReturnRows(sqlmock.NewRows(searchRowCols))

	// Content: inner subquery gains AND f.folder_id IN. Args: q, userID, q, <ids>.
	a.mock.ExpectQuery("JOIN file_text").
		WithArgs("report", "u-test", "report", "F", "A", "B").
		WillReturnRows(sqlmock.NewRows(searchRowCols))

	w := rec()
	r := authedRequest("GET", "/search?q=report&scope=both&folder_id=F", "")
	a.handleSearch(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if err := a.mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// Folder-scoped DSL query: the files query must append the subtree filter
// BEFORE the relocated LIMIT 100. ("starred:true" compiles to a single
// is_starred=? arg, so the subtree ids land at positions 3..5.)
func TestHandleSearch_FolderScoped_DSL(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)

	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WithArgs("F", "u-test", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("F").AddRow("A").AddRow("B"))

	// Args: userID, <1 dsl arg>, <ids>. Match the dsl arg loosely.
	a.mock.ExpectQuery("folder_id IN.*LIMIT 100").
		WithArgs("u-test", sqlmock.AnyArg(), "F", "A", "B").
		WillReturnRows(sqlmock.NewRows(searchRowCols))

	w := rec()
	r := authedRequest("GET", "/search?q=starred:true&folder_id=F", "")
	a.handleSearch(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if err := a.mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// An unknown / non-owned folder yields an empty subtree, so the handler must
// short-circuit to empty results WITHOUT running any name/content query. The
// absence of other expectations is the assertion: if the handler kept going,
// the unmatched query would error and the response would be 500.
func TestHandleSearch_FolderScoped_UnknownFolderReturnsEmpty(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)

	a.mock.ExpectQuery("WITH RECURSIVE folder_tree").
		WithArgs("ghost", "u-test", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // zero rows

	w := rec()
	r := authedRequest("GET", "/search?q=report&scope=both&folder_id=ghost", "")
	a.handleSearch(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"results":[]`) {
		t.Errorf("expected empty results, got body=%s", w.Body.String())
	}
	if err := a.mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// Regression guard: with no folder_id the global path must be unchanged — no
// CTE, no folder_id/parent_id IN, and the original arg lists.
func TestHandleSearch_RootStaysGlobal(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)

	// Name UNION, global: args userID, like, q, userID, like (no ids).
	a.mock.ExpectQuery("UNION ALL").
		WithArgs("u-test", sqlmock.AnyArg(), "report", "u-test", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(searchRowCols))

	// Content, global: args q, userID, q (no ids).
	a.mock.ExpectQuery("JOIN file_text").
		WithArgs("report", "u-test", "report").
		WillReturnRows(sqlmock.NewRows(searchRowCols))

	w := rec()
	r := authedRequest("GET", "/search?q=report&scope=both", "")
	a.handleSearch(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if err := a.mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
