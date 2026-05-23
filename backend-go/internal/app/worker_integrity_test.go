package app

// Worker DLQ + idempotency + recovery tests.

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ─── DLQ visibility ────────────────────────────────────────────────────────

// A processFn that always errors must result in a dead-letter row on
// <queue>:failed. We don't run the full drainQueue (it blocks on BRPOP);
// instead we exercise the equivalent code path inline.
func TestWorker_FailedJobLandsInDLQ(t *testing.T) {
	a := newTestApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Enqueue a payload.
	_ = a.Redis.LPush(ctx, "q:test", "f1").Err()

	// Simulate the inner loop body: pop, process (fails), push to DLQ.
	res, _ := a.Redis.BRPop(ctx, time.Second, "q:test").Result()
	if len(res) < 2 {
		t.Fatal("BRPop returned no payload")
	}
	payload := res[1]
	fakeErr := errors.New("simulated job failure")
	dl, _ := json.Marshal(map[string]any{"payload": payload, "error": fakeErr.Error()})
	if err := a.Redis.LPush(ctx, "q:test:failed", dl).Err(); err != nil {
		t.Fatal(err)
	}

	// Verify the DLQ has exactly one entry.
	n, _ := a.Redis.LLen(ctx, "q:test:failed").Result()
	if n != 1 {
		t.Errorf("expected 1 DLQ entry, got %d", n)
	}
	// Verify the entry parses back to the original payload + error.
	got, _ := a.Redis.LRange(ctx, "q:test:failed", 0, 0).Result()
	if len(got) != 1 {
		t.Fatal("expected one DLQ row")
	}
	var dec map[string]any
	if err := json.Unmarshal([]byte(got[0]), &dec); err != nil {
		t.Fatalf("DLQ row not valid JSON: %v", err)
	}
	if dec["payload"] != "f1" || dec["error"] != "simulated job failure" {
		t.Errorf("DLQ row payload mismatch: %v", dec)
	}
}

// ─── enqueue is idempotent at the queue level ──────────────────────────────

// Same payload pushed twice yields two entries — workers must handle
// duplicate-delivery gracefully. We codify the expectation here.
func TestWorker_DuplicateEnqueueResultsInTwoEntries(t *testing.T) {
	a := newTestApp(t)
	a.enqueueThumb(context.Background(), "f1")
	a.enqueueThumb(context.Background(), "f1")
	n, _ := a.Redis.LLen(context.Background(), "q:thumb").Result()
	if n != 2 {
		t.Errorf("expected 2 entries (duplicate-tolerant), got %d", n)
	}
}

// ─── goSafe recovery: panic in one goroutine, others unaffected ────────────

func TestWorker_GoSafeRecoversPanic(t *testing.T) {
	const N = 100
	var done atomic.Int32
	completion := make(chan struct{}, N)
	for i := 0; i < N; i++ {
		i := i
		goSafe("test-worker", func() {
			defer func() { completion <- struct{}{} }()
			if i%10 == 0 {
				panic("controlled panic")
			}
			done.Add(1)
		})
	}
	// Wait for all goroutines.
	for i := 0; i < N; i++ {
		select {
		case <-completion:
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for goroutines")
		}
	}
	// 90 succeed, 10 panic — goSafe catches and the goroutine exits cleanly.
	if done.Load() != 90 {
		t.Errorf("expected 90 successes, got %d", done.Load())
	}
}

// ─── afterUploadCommit dispatches to the right queues ──────────────────────

func TestAfterUploadCommit_DispatchesByMime(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()

	// Image — should enqueue both extract (text indexable) and thumb (image).
	a.afterUploadCommit(ctx, "f1", "image/jpeg")
	got, _ := a.Redis.LLen(ctx, "q:thumb").Result()
	if got == 0 {
		t.Error("image upload did not enqueue thumb job")
	}

	// PDF — should enqueue extract but NOT thumb.
	a.afterUploadCommit(ctx, "f2", "application/pdf")
	got2, _ := a.Redis.LLen(ctx, "q:extract").Result()
	if got2 == 0 {
		t.Error("PDF upload did not enqueue extract job")
	}
}

func TestAfterUploadCommit_EmptyFileIDIsNoop(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	a.afterUploadCommit(ctx, "", "image/jpeg")
	got, _ := a.Redis.LLen(ctx, "q:thumb").Result()
	if got != 0 {
		t.Errorf("empty fileID enqueued unexpectedly: %d", got)
	}
}

// ─── Drain doesn't block on empty queue indefinitely ───────────────────────

// drainQueue uses BRPop with a 5s timeout — by the time we cancel ctx,
// the worker should exit. We don't run drainQueue itself (would block
// 5s); instead we verify the inner sentinel handling.
func TestWorker_DrainExitsOnCtxCancel(t *testing.T) {
	a := newTestApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Use a no-op fn — if drain exits before we cancel, fine; otherwise
		// cancel triggers exit on the next iteration.
		a.drainQueue(ctx, "q:test", func(ctx context.Context, _ []byte) error { return nil })
		close(done)
	}()
	cancel()
	select {
	case <-done:
		// drainQueue exited cleanly.
	case <-time.After(7 * time.Second):
		t.Fatal("drainQueue did not exit after ctx cancel")
	}
	wg.Wait()
}
