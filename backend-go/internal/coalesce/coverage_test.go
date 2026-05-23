package coalesce

import (
	"context"
	"sync/atomic"
	"testing"
)

// Forget releases an in-flight slot so the next call re-executes the fn.
// Use a counter — the second Do under the same key should re-fire after
// Forget is called.
func TestForget_AllowsRefetch(t *testing.T) {
	g := New()
	var calls atomic.Int32
	fn := func(_ context.Context) (int, error) {
		calls.Add(1)
		return 42, nil
	}

	if _, err := Do(context.Background(), g, "k", fn); err != nil {
		t.Fatalf("first Do: %v", err)
	}
	g.Forget("k")
	if _, err := Do(context.Background(), g, "k", fn); err != nil {
		t.Fatalf("second Do: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("expected 2 calls after Forget, got %d", calls.Load())
	}
}

// nil Group falls back to direct execution.
func TestDo_NilGroupFallback(t *testing.T) {
	got, err := Do[int](context.Background(), nil, "k", func(_ context.Context) (int, error) {
		return 99, nil
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got != 99 {
		t.Errorf("expected 99, got %d", got)
	}
}
