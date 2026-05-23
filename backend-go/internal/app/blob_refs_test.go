package app

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Test the atomic releaseBlobRef path. The old implementation did UPDATE →
// SELECT → DELETE without holding a row lock, so two concurrent goroutines
// could both observe ref_count == 0 and both DELETE the row (and both
// trigger an on-disk blob removal post-commit). The new implementation
// SELECTs FOR UPDATE first, so the second caller blocks until the first
// commits, then sees the post-decrement value.
//
// sqlmock doesn't simulate real locking semantics, so we don't claim this
// test catches a race — it just asserts the SQL shape we now emit. A
// proper race test would need a real MariaDB harness, which is left as a
// follow-up (the comment in the plan says use dockertest when CI allows).

func TestReleaseBlobRef_Decrement_NotLastRef(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	const path = "/data/blobs/abc.enc"
	mock.ExpectBegin()
	// FOR UPDATE row lock. After lock the row reports ref_count = 3.
	mock.ExpectQuery("SELECT ref_count FROM blob_refs WHERE storage_path = . FOR UPDATE").
		WithArgs(path).
		WillReturnRows(sqlmock.NewRows([]string{"ref_count"}).AddRow(int64(3)))
	mock.ExpectExec("UPDATE blob_refs SET ref_count = ref_count - 1 WHERE storage_path = .").
		WithArgs(path).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	gone, err := releaseBlobRef(context.Background(), tx, path)
	if err != nil {
		t.Fatalf("releaseBlobRef: %v", err)
	}
	if gone {
		t.Errorf("expected gone=false when ref_count=3, got true")
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

func TestReleaseBlobRef_LastRefDeletes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	const path = "/data/blobs/only.enc"
	mock.ExpectBegin()
	// ref_count = 1 → this caller takes the row from 1 to 0, which means
	// the blob is now unreferenced and the caller is expected to schedule
	// the on-disk delete after commit. The DB row goes away unconditionally
	// at the same time (no DELETE-then-SELECT race that could leave a
	// zero-count orphan visible to readers).
	mock.ExpectQuery("SELECT ref_count FROM blob_refs WHERE storage_path = . FOR UPDATE").
		WithArgs(path).
		WillReturnRows(sqlmock.NewRows([]string{"ref_count"}).AddRow(int64(1)))
	mock.ExpectExec("DELETE FROM blob_refs WHERE storage_path = .").
		WithArgs(path).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	gone, err := releaseBlobRef(context.Background(), tx, path)
	if err != nil {
		t.Fatalf("releaseBlobRef: %v", err)
	}
	if !gone {
		t.Errorf("expected gone=true when ref_count=1, got false")
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

func TestReleaseBlobRef_NoRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	const path = "/data/blobs/missing.enc"
	mock.ExpectBegin()
	// No row found — idempotent path: caller is told to delete the blob
	// from disk (since the metadata reference is already gone) but no DB
	// mutations happen.
	mock.ExpectQuery("SELECT ref_count FROM blob_refs WHERE storage_path = . FOR UPDATE").
		WithArgs(path).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectCommit()

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	gone, err := releaseBlobRef(context.Background(), tx, path)
	if err != nil {
		t.Fatalf("releaseBlobRef: %v", err)
	}
	if !gone {
		t.Errorf("expected gone=true for missing row (idempotent), got false")
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

// TestSanitizeUploadName_Rejects covers the zip-slip / path-traversal
// guards added to upload entry points. The function must:
//   - strip path components ("../../etc/passwd" → "passwd")
//   - reject "." / ".." / "" outright
//   - strip control bytes (NUL, CR, LF) that could inject into
//     Content-Disposition or zip headers
//   - cap length at NAME_MAX
func TestSanitizeUploadName(t *testing.T) {
	cases := []struct {
		in       string
		want     string
		wantOk   bool
	}{
		{"normal.txt", "normal.txt", true},
		{"../../etc/passwd", "passwd", true},
		{"..\\..\\windows\\system32", "system32", true},
		{"  spaced  .pdf  ", "spaced  .pdf", true},
		{"", "", false},
		{".", "", false},
		{"..", "", false},
		{"   ", "", false},
		{"/", "", false},
		{"\x00malicious", "malicious", true},
		{"line\r\nfeed.txt", "linefeed.txt", true},
	}
	for _, c := range cases {
		got, ok := sanitizeUploadName(c.in)
		if ok != c.wantOk {
			t.Errorf("sanitizeUploadName(%q): ok=%v, want %v (got=%q)", c.in, ok, c.wantOk, got)
			continue
		}
		if ok && got != c.want {
			t.Errorf("sanitizeUploadName(%q): got %q, want %q", c.in, got, c.want)
		}
	}
}

// Ensures sanitizeUploadName is goroutine-safe — it should be pure.
// Documents the contract rather than catching a real bug, since the
// current implementation is stateless.
func TestSanitizeUploadName_Concurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got, ok := sanitizeUploadName("hello.txt"); !ok || got != "hello.txt" {
				t.Errorf("unexpected result under concurrency: got=%q ok=%v", got, ok)
			}
		}()
	}
	wg.Wait()
}
