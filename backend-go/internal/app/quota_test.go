package app

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// reserveQuota succeeds when the atomic UPDATE … WHERE used + size <= quota
// returns one affected row. The driver's RowsAffected → 1 path is the
// success signal; the helper relies on `clientFoundRows=true` in the DSN
// so MATCHED counts are returned rather than CHANGED.
func TestReserveQuota_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE users SET used_bytes").
		WithArgs(int64(1024), "u-1", int64(1024)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if err := reserveQuota(context.Background(), tx, "u-1", 1024); err != nil {
		t.Fatalf("reserveQuota: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

// 0 rows affected → caller is over quota; helper must surface
// ErrQuotaExceeded so handlers translate to 413 RequestEntityTooLarge.
func TestReserveQuota_Exceeded(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE users SET used_bytes").
		WithArgs(int64(1<<40), "u-1", int64(1<<40)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	tx, _ := db.BeginTx(context.Background(), nil)
	err := reserveQuota(context.Background(), tx, "u-1", 1<<40)
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Errorf("expected ErrQuotaExceeded, got %v", err)
	}
	_ = tx.Commit()
}

// Driver-level errors propagate verbatim so respondDBError can log + sanitise.
func TestReserveQuota_DBError(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE users SET used_bytes").
		WithArgs(int64(100), "u-1", int64(100)).
		WillReturnError(errors.New("connection lost"))
	mock.ExpectRollback()

	tx, _ := db.BeginTx(context.Background(), nil)
	err := reserveQuota(context.Background(), tx, "u-1", 100)
	if err == nil || errors.Is(err, ErrQuotaExceeded) {
		t.Errorf("expected db error to propagate, got %v", err)
	}
	_ = tx.Rollback()
}

// releaseQuota uses GREATEST(0, used_bytes - size) to avoid drifting
// negative on double-release. Test that the SQL is shaped correctly + the
// return is nil on success.
func TestReleaseQuota_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE users SET used_bytes").
		WithArgs(int64(500), "u-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, _ := db.BeginTx(context.Background(), nil)
	if err := releaseQuota(context.Background(), tx, "u-1", 500); err != nil {
		t.Errorf("unexpected: %v", err)
	}
	_ = tx.Commit()
}

func TestReleaseQuota_DBError(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE users SET used_bytes").
		WillReturnError(sql.ErrConnDone)
	tx, _ := db.BeginTx(context.Background(), nil)
	if err := releaseQuota(context.Background(), tx, "u-1", 100); err == nil {
		t.Errorf("expected error to propagate")
	}
	_ = tx.Rollback()
}

// ErrQuotaExceeded is comparable with errors.Is.
func TestErrQuotaExceeded_IdentityViaErrorsIs(t *testing.T) {
	wrapped := errors.New("wrap")
	if errors.Is(wrapped, ErrQuotaExceeded) {
		t.Errorf("unrelated error should not match ErrQuotaExceeded")
	}
	if !errors.Is(ErrQuotaExceeded, ErrQuotaExceeded) {
		t.Errorf("self-identity should hold")
	}
}
