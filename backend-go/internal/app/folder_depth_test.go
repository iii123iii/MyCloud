package app

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// MaxFolderDepth caps how deep folder trees can grow to keep recursive CTEs
// (folder path lookup, access checks) bounded. The constant should be set
// generous enough for real use but well under DB recursion limits.
func TestMaxFolderDepth_SaneValue(t *testing.T) {
	if MaxFolderDepth < 16 {
		t.Errorf("MaxFolderDepth too low (got %d) — real users do nest tens deep", MaxFolderDepth)
	}
	if MaxFolderDepth > 1000 {
		t.Errorf("MaxFolderDepth (%d) exceeds typical DB cte_max_recursion_depth", MaxFolderDepth)
	}
}

// folderDepth("") = 0 — root has no ancestors. Important because
// handleCreateFolder uses parentID == "" to mean "create at root", and the
// depth-cap check must not trip on legitimate root-level inserts.
func TestFolderDepth_RootReturnsZero(t *testing.T) {
	app := &App{}
	got, err := app.folderDepth(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Errorf("root depth should be 0, got %d", got)
	}
}

// With a real parent ID, folderDepth runs the recursive CTE. Test that the
// SQL is shaped correctly + the return value passes through.
func TestFolderDepth_NonRoot(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	app := &App{DB: db}

	mock.ExpectQuery("WITH RECURSIVE chain AS").
		WithArgs("folder-id-123").
		WillReturnRows(sqlmock.NewRows([]string{"MAX(depth)"}).AddRow(7))

	got, err := app.folderDepth(context.Background(), "folder-id-123")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got != 7 {
		t.Errorf("expected 7, got %d", got)
	}
}

// A NULL aggregate result (folder doesn't exist or has been deleted) maps
// to depth 0 — the surrounding handler must already have validated the
// parent's existence; depth check is a secondary defence.
func TestFolderDepth_NullResult(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	app := &App{DB: db}

	mock.ExpectQuery("WITH RECURSIVE chain AS").
		WithArgs("ghost").
		WillReturnRows(sqlmock.NewRows([]string{"MAX(depth)"}).AddRow(nil))

	got, err := app.folderDepth(context.Background(), "ghost")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got != 0 {
		t.Errorf("NULL aggregate should map to 0, got %d", got)
	}
}
