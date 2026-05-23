package app

// Concurrency tests. sqlmock isn't safe for parallel queries on the same
// connection, so these tests verify:
//   - Sequential correctness under the kinds of races we care about
//     (atomic-counter race for ref counts, family revocation visibility).
//   - Lock ordering / call sequence (SELECT FOR UPDATE before UPDATE).
// Real-DB stress tests would belong in an integration build-tag suite.

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// ─── Family revocation is visible to subsequent Parse calls ────────────────

// Revoking a family must be immediately visible to any goroutine calling
// Parse. We use miniredis (in-process, single-threaded) so the visibility
// guarantee is trivially satisfied; this test codifies the property.
func TestConcurrency_FamilyRevocationVisible(t *testing.T) {
	a := newTestApp(t)
	pair, _ := a.Auth.IssuePair("u-test", "user")
	family := pair["family"].(string)

	a.Auth.RevokeFamily(family)

	var wg sync.WaitGroup
	var rejected atomic.Int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := a.Auth.Parse(pair["access_token"].(string)); err != nil {
				rejected.Add(1)
			}
		}()
	}
	wg.Wait()
	if rejected.Load() != 20 {
		t.Errorf("expected 20 rejections, got %d", rejected.Load())
	}
}

// ─── goSafe isolates panics across goroutines ──────────────────────────────

func TestConcurrency_PanicInOneGoroutineDoesNotAffectOthers(t *testing.T) {
	const N = 50
	var ok atomic.Int32
	done := make(chan struct{}, N)
	for i := 0; i < N; i++ {
		i := i
		goSafe("test", func() {
			defer func() { done <- struct{}{} }()
			if i == 7 {
				panic("controlled panic")
			}
			ok.Add(1)
		})
	}
	// Wait for all goroutines.
	for i := 0; i < N; i++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("goroutines did not all complete")
		}
	}
	if ok.Load() != N-1 {
		t.Errorf("expected %d successes, got %d", N-1, ok.Load())
	}
}

// ─── Enqueue & drain ───────────────────────────────────────────────────────

// 50 enqueueHLS calls in parallel — all entries land on q:hls. miniredis
// serializes commands, so this is more of a smoke test for the call site.
func TestConcurrency_HLSEnqueueAllSeen(t *testing.T) {
	a := newTestApp(t)
	const N = 50
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a.enqueueHLS(context.Background(), "f1")
		}(i)
	}
	wg.Wait()
	got, _ := a.Redis.LLen(context.Background(), "q:hls").Result()
	if got != int64(N) {
		t.Errorf("expected %d entries, got %d", N, got)
	}
}

// ─── deleteUserResources SELECT-FOR-UPDATE ordering ────────────────────────

// The deleteUserResources fix moves the SELECT into the transaction with
// FOR UPDATE so concurrent uploads can't slip rows past it. We verify the
// SELECT carries FOR UPDATE by inspecting the SQL pattern sqlmock sees.
func TestConcurrency_DeleteUserResources_UsesForUpdate(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(true)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT id, storage_path FROM files WHERE user_id=.* FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{"id", "storage_path"}))
	a.mock.ExpectExec("DELETE FROM shares").
		WillReturnResult(sqlmock.NewResult(0, 0))
	a.mock.ExpectExec("DELETE FROM files").
		WillReturnResult(sqlmock.NewResult(0, 0))
	a.mock.ExpectExec("DELETE FROM folders").
		WillReturnResult(sqlmock.NewResult(0, 0))
	a.mock.ExpectExec("DELETE FROM activity_log").
		WillReturnResult(sqlmock.NewResult(0, 0))
	a.mock.ExpectCommit()
	if err := a.deleteUserResources(context.Background(), "u-test"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// ─── releaseBlobRef uses FOR UPDATE (atomicity fix) ────────────────────────

func TestConcurrency_ReleaseBlobRef_UsesForUpdate(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(true)
	a.mock.ExpectBegin()
	a.mock.ExpectQuery("SELECT ref_count FROM blob_refs WHERE storage_path = . FOR UPDATE").
		WithArgs("path-1").
		WillReturnRows(sqlmock.NewRows([]string{"ref_count"}).AddRow(int64(2)))
	a.mock.ExpectExec("UPDATE blob_refs SET ref_count").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectCommit()
	tx, _ := a.DB.BeginTx(context.Background(), nil)
	defer tx.Commit()
	if _, err := releaseBlobRef(context.Background(), tx, "path-1"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}
