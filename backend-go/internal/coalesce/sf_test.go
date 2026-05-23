package coalesce

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCoalescesConcurrentCalls(t *testing.T) {
	g := New()

	var calls atomic.Int32
	const N = 50
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(N)
	results := make([]int, N)

	for i := 0; i < N; i++ {
		go func(idx int) {
			defer wg.Done()
			<-start
			v, err := Do(context.Background(), g, "same-key", func(ctx context.Context) (int, error) {
				calls.Add(1)
				time.Sleep(20 * time.Millisecond) // hold the in-flight slot
				return 42, nil
			})
			if err != nil {
				t.Errorf("goroutine %d: %v", idx, err)
			}
			results[idx] = v
		}(i)
	}
	close(start)
	wg.Wait()

	// Some calls may sneak through if they happened to start after the first
	// returned — but on a 20ms in-flight window with 50 racing goroutines we
	// expect dramatically fewer than N. Assert <= 5 to keep the test stable.
	if c := calls.Load(); c > 5 {
		t.Errorf("expected ≤5 underlying calls, got %d", c)
	}
	for i, v := range results {
		if v != 42 {
			t.Errorf("result[%d] = %d, want 42", i, v)
		}
	}
}

func TestDifferentKeysDoNotCoalesce(t *testing.T) {
	g := New()
	var calls atomic.Int32
	fn := func(ctx context.Context) (string, error) {
		calls.Add(1)
		return "ok", nil
	}
	_, _ = Do(context.Background(), g, "a", fn)
	_, _ = Do(context.Background(), g, "b", fn)
	if calls.Load() != 2 {
		t.Errorf("different keys should execute separately: got %d calls", calls.Load())
	}
}

func TestNilGroupExecutesDirectly(t *testing.T) {
	var calls atomic.Int32
	v, err := Do(context.Background(), nil, "k", func(ctx context.Context) (int, error) {
		calls.Add(1)
		return 7, nil
	})
	if err != nil || v != 7 || calls.Load() != 1 {
		t.Errorf("nil group: v=%d err=%v calls=%d (want 7, nil, 1)", v, err, calls.Load())
	}
}
