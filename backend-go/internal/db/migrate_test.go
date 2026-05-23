package db

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Migrate creates schema_migrations and checks for legacy install. We
// exercise via sqlmock: every CREATE/SELECT/INSERT is mocked so the
// function executes its top branches.

func TestMigrate_CreateTableErrorReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations").
		WillReturnError(errFake("create failed"))
	if err := Migrate(db); err == nil {
		t.Errorf("expected error from CREATE TABLE failure")
	}
}

func TestMigrate_CountSchemaError(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT COUNT.\\*. FROM information_schema").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
	mock.ExpectQuery("SELECT COUNT.\\*. FROM schema_migrations").
		WillReturnError(errFake("count error"))
	if err := Migrate(db); err == nil {
		t.Errorf("expected error from count schema_migrations")
	}
}

func TestLoadApplied_QueryError(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery("SELECT version FROM schema_migrations").
		WillReturnError(errFake("load failed"))
	if _, err := loadApplied(db); err == nil {
		t.Errorf("expected error")
	}
}

func TestLoadApplied_ScanError(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	// Return a row whose value can't scan into string (NULL → ok, but a
	// wrong-type would do). NULL into string column actually IS ok for
	// non-nullable scan with sql.NullString — for *string it'd error.
	// Use rows.Err() path: return rows with a close-error.
	rows := sqlmock.NewRows([]string{"version"}).AddRow("001").RowError(0, errFake("scan failed"))
	mock.ExpectQuery("SELECT version FROM schema_migrations").
		WillReturnRows(rows)
	if _, err := loadApplied(db); err == nil {
		t.Errorf("expected scan-error path")
	}
}

func TestLoadApplied_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery("SELECT version FROM schema_migrations").
		WillReturnRows(sqlmock.NewRows([]string{"version"}).
			AddRow("001").AddRow("002").AddRow("003"))
	applied, err := loadApplied(db)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(applied) != 3 {
		t.Errorf("expected 3 applied, got %d", len(applied))
	}
	if !applied["001"] || !applied["002"] || !applied["003"] {
		t.Errorf("applied map wrong: %v", applied)
	}
}

// fakeErr is a tiny string error for sqlmock returns.
type fakeErr string

func (e fakeErr) Error() string { return string(e) }
func errFake(s string) error    { return fakeErr(s) }
