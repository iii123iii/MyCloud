package app

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// canAccessFile happy path: owner returns ownerID + nil.
func TestCanAccessFile_OwnerSuccess(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	owner, err := a.canAccessFile(context.Background(), "u-test", "f1", AccessViewer)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if owner != "u-test" {
		t.Errorf("got %q, want u-test", owner)
	}
}

// canAccessFile not found → sql.ErrNoRows.
func TestCanAccessFile_NotFound(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("ghost").
		WillReturnError(sql.ErrNoRows)
	_, err := a.canAccessFile(context.Background(), "u-test", "ghost", AccessViewer)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected ErrNoRows, got %v", err)
	}
}

// canAccessFile DB error propagates.
func TestCanAccessFile_DBError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnError(errors.New("connection lost"))
	_, err := a.canAccessFile(context.Background(), "u-test", "f1", AccessViewer)
	if err == nil || errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected db error to propagate, got %v", err)
	}
}

// canAccessFile non-owner without grant → ErrNoRows.
func TestCanAccessFile_NonOwnerNoGrant(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("other-user"))
	// Grant lookup returns no rows.
	a.mock.ExpectQuery("WITH RECURSIVE ancestors AS").
		WithArgs("f1", "u-test", "f1").
		WillReturnError(sql.ErrNoRows)
	_, err := a.canAccessFile(context.Background(), "u-test", "f1", AccessViewer)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected ErrNoRows for no-grant, got %v", err)
	}
}

// canAccessFile non-owner with viewer grant requesting editor → ErrNoRows.
func TestCanAccessFile_GrantBelowMinReturnsErrNoRows(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("other-user"))
	a.mock.ExpectQuery("WITH RECURSIVE ancestors AS").
		WithArgs("f1", "u-test", "f1").
		WillReturnRows(sqlmock.NewRows([]string{"permission"}).AddRow("viewer"))
	_, err := a.canAccessFile(context.Background(), "u-test", "f1", AccessEditor)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected ErrNoRows when grant < min, got %v", err)
	}
}

// canAccessFile non-owner with editor grant requesting viewer → ok.
func TestCanAccessFile_GrantAboveMin(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM files WHERE id=").
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("other-user"))
	a.mock.ExpectQuery("WITH RECURSIVE ancestors AS").
		WithArgs("f1", "u-test", "f1").
		WillReturnRows(sqlmock.NewRows([]string{"permission"}).AddRow("editor"))
	owner, err := a.canAccessFile(context.Background(), "u-test", "f1", AccessViewer)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if owner != "other-user" {
		t.Errorf("owner mismatch: %q", owner)
	}
}

// canAccessFolder mirrors canAccessFile.
func TestCanAccessFolder_OwnerSuccess(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("F1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u-test"))
	owner, err := a.canAccessFolder(context.Background(), "u-test", "F1", AccessViewer)
	if err != nil {
		t.Errorf("unexpected: %v", err)
	}
	if owner != "u-test" {
		t.Errorf("owner mismatch: %q", owner)
	}
}

func TestCanAccessFolder_NotFound(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("ghost").
		WillReturnError(sql.ErrNoRows)
	if _, err := a.canAccessFolder(context.Background(), "u-test", "ghost", AccessViewer); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected ErrNoRows, got %v", err)
	}
}

func TestCanAccessFolder_DBError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("F1").
		WillReturnError(errors.New("boom"))
	if _, err := a.canAccessFolder(context.Background(), "u-test", "F1", AccessViewer); err == nil {
		t.Errorf("expected db error")
	}
}

func TestCanAccessFolder_NonOwnerNoGrant(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("F1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("other-user"))
	a.mock.ExpectQuery("WITH RECURSIVE ancestors AS").
		WithArgs("F1", "u-test").
		WillReturnError(sql.ErrNoRows)
	if _, err := a.canAccessFolder(context.Background(), "u-test", "F1", AccessViewer); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected ErrNoRows, got %v", err)
	}
}

func TestCanAccessFolder_GrantBelowMin(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT user_id FROM folders WHERE id=").
		WithArgs("F1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("other-user"))
	a.mock.ExpectQuery("WITH RECURSIVE ancestors AS").
		WithArgs("F1", "u-test").
		WillReturnRows(sqlmock.NewRows([]string{"permission"}).AddRow("viewer"))
	if _, err := a.canAccessFolder(context.Background(), "u-test", "F1", AccessEditor); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected ErrNoRows below min, got %v", err)
	}
}

// parseAccessLevel covers all branches.
func TestParseAccessLevel(t *testing.T) {
	cases := []struct {
		in   string
		want AccessLevel
	}{
		{"owner", AccessOwner},
		{"editor", AccessEditor},
		{"viewer", AccessViewer},
		{"", AccessViewer},        // default branch
		{"anything else", AccessViewer}, // default branch
	}
	for _, c := range cases {
		if got := parseAccessLevel(c.in); got != c.want {
			t.Errorf("parseAccessLevel(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// AccessLevel.String covers all branches.
func TestAccessLevelString(t *testing.T) {
	cases := []struct {
		in   AccessLevel
		want string
	}{
		{AccessOwner, "owner"},
		{AccessEditor, "editor"},
		{AccessViewer, "viewer"},
		{AccessLevel(99), "viewer"}, // default branch
	}
	for _, c := range cases {
		if got := c.in.String(); got != c.want {
			t.Errorf("%v.String() = %q, want %q", c.in, got, c.want)
		}
	}
}

// lookupGrantFile DB error propagates.
func TestLookupGrantFile_DBError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE ancestors AS").
		WithArgs("f1", "u-test", "f1").
		WillReturnError(errors.New("query failed"))
	if _, err := a.lookupGrantFile(context.Background(), "u-test", "f1"); err == nil {
		t.Errorf("expected error from grant query")
	}
}

func TestLookupGrantFolder_DBError(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("WITH RECURSIVE ancestors AS").
		WithArgs("F1", "u-test").
		WillReturnError(errors.New("query failed"))
	if _, err := a.lookupGrantFolder(context.Background(), "u-test", "F1"); err == nil {
		t.Errorf("expected error from grant query")
	}
}
