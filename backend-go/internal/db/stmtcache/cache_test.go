package stmtcache

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// Prepare caches by query string — the second call returns the same *sql.Stmt
// without re-preparing.
func TestPrepare_CachesStmt(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	// Only one Prepare expected — second call hits the cache.
	mock.ExpectPrepare("SELECT 1")

	c := New(db)
	defer c.Close()

	s1, err := c.Prepare(context.Background(), "SELECT 1")
	if err != nil {
		t.Fatalf("Prepare#1: %v", err)
	}
	s2, err := c.Prepare(context.Background(), "SELECT 1")
	if err != nil {
		t.Fatalf("Prepare#2: %v", err)
	}
	if s1 != s2 {
		t.Errorf("expected cache hit (same *sql.Stmt), got distinct values")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet: %v", err)
	}
}

// PrepareContext failure is not cached — subsequent calls retry.
func TestPrepare_FailureNotCached(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectPrepare("SELECT bad").WillReturnError(context.DeadlineExceeded)
	mock.ExpectPrepare("SELECT bad") // second attempt — would succeed

	c := New(db)
	defer c.Close()
	if _, err := c.Prepare(context.Background(), "SELECT bad"); err == nil {
		t.Errorf("expected failure on first call")
	}
	if _, err := c.Prepare(context.Background(), "SELECT bad"); err != nil {
		t.Errorf("retry should hit a fresh Prepare attempt: %v", err)
	}
}

// Len reflects the cache size.
func TestLen(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectPrepare("Q1")
	mock.ExpectPrepare("Q2")

	c := New(db)
	defer c.Close()
	if got := c.Len(); got != 0 {
		t.Errorf("empty cache should have Len 0, got %d", got)
	}
	_, _ = c.Prepare(context.Background(), "Q1")
	_, _ = c.Prepare(context.Background(), "Q2")
	if got := c.Len(); got != 2 {
		t.Errorf("expected Len 2, got %d", got)
	}
}

// Close releases every cached statement.
func TestClose_ReleasesAll(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectPrepare("Q1").WillBeClosed()
	mock.ExpectPrepare("Q2").WillBeClosed()

	c := New(db)
	_, _ = c.Prepare(context.Background(), "Q1")
	_, _ = c.Prepare(context.Background(), "Q2")
	if err := c.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if got := c.Len(); got != 0 {
		t.Errorf("after Close, Len should be 0, got %d", got)
	}
}

// Concurrent Prepare with the same key resolves to a single *sql.Stmt —
// LoadOrStore handles the race; the loser's stmt is closed.
func TestPrepare_ConcurrentSameKey(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	// Could prepare once or twice depending on which goroutine wins —
	// sqlmock seeds two expectations; the loser's Close is also expected.
	mock.ExpectPrepare("SELECT cc")
	mock.ExpectPrepare("SELECT cc")

	c := New(db)
	defer c.Close()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.Prepare(context.Background(), "SELECT cc")
		}()
	}
	wg.Wait()
	if got := c.Len(); got != 1 {
		t.Errorf("expected exactly one cache entry after concurrent prepare, got %d", got)
	}
}

// SetSlowHook installs the callback. ExecContext should fire it.
func TestSetSlowHook_FiresOnExec(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectPrepare("UPDATE t SET a=").ExpectExec().WillReturnResult(sqlmock.NewResult(0, 1))

	c := New(db)
	defer c.Close()

	var fired int32
	c.SetSlowHook(func(ctx context.Context, query string, dur time.Duration, rows int) {
		atomic.StoreInt32(&fired, 1)
	})
	_, err := c.ExecContext(context.Background(), "UPDATE t SET a=?", 1)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	// Hook runs on a goroutine — give it a moment.
	time.Sleep(20 * time.Millisecond)
	if atomic.LoadInt32(&fired) == 0 {
		t.Errorf("slow hook should have fired")
	}
}

// No hook installed = no-op; doesn't panic.
func TestSetSlowHook_NilSafe(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectPrepare("SELECT 1").ExpectQuery().WillReturnRows(sqlmock.NewRows([]string{"x"}).AddRow(1))

	c := New(db)
	defer c.Close()
	rows, err := c.QueryContext(context.Background(), "SELECT 1")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	_ = rows.Close()
}
