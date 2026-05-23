// Package coalesce deduplicates concurrent identical reads inside a single
// process using golang.org/x/sync/singleflight.
//
// The use case is the "browser refresh + open three tabs" pattern: a user
// triggers N identical SELECTs against the same folder/search query within
// milliseconds. Without coalescing, each lands a separate query (and behind
// it, a DB roundtrip + Go decoding cost). With coalescing, the first call
// in flight wins and the others piggy-back on its result.
//
// Scope: in-process only. Multi-instance deployments would need a Redis-
// backed coalescer (set NX + poll) — out of scope for v1. The plan promotes
// this when horizontal scale ships.
//
// Cache vs. coalesce: this is NOT a cache. The result lives only for the
// duration of the first caller's roundtrip. Once that caller returns, the
// next request triggers a fresh query. Use this when:
//   - The work is expensive (DB query, decryption, JSON marshal of a large blob)
//   - The work is idempotent for a given key
//   - The hit rate is bursty (many concurrent requests, but not a steady stream)
package coalesce

import (
	"context"

	"golang.org/x/sync/singleflight"
)

// Group wraps singleflight.Group with a Do helper that adds context support
// and a typed return signature. The stdlib singleflight.Do is `(any, error)`
// which forces a type assertion at every call site — annoying for handlers.
type Group struct {
	g singleflight.Group
}

// New constructs a fresh coalescer. Each Group keeps its own in-flight map;
// pass one per concern (e.g. one for folder listings, one for searches) so
// keys can't accidentally collide between unrelated operations.
func New() *Group {
	return &Group{}
}

// Do coalesces calls that share the same key. The first caller runs fn; any
// caller that arrives while fn is in flight blocks and gets the same result
// once fn returns. fn's ctx is the first caller's ctx — if that caller
// cancels, all waiters see context.Canceled.
//
// Generic over T so handlers don't type-assert. The cost is one closure
// allocation per call; acceptable for the bursty workloads this targets.
func Do[T any](ctx context.Context, gr *Group, key string, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	if gr == nil {
		// Safety: nil group falls back to direct execution so a feature flag
		// can disable coalescing without rewriting handlers.
		return fn(ctx)
	}
	v, err, _ := gr.g.Do(key, func() (any, error) {
		return fn(ctx)
	})
	if err != nil {
		return zero, err
	}
	cast, ok := v.(T)
	if !ok {
		// Should be unreachable — same key always carries same return type.
		return zero, nil
	}
	return cast, nil
}

// Forget releases the in-flight slot for key. Useful when a caller knows
// the underlying state has changed mid-flight (e.g. a mutation arrived) and
// they don't want followers to get the now-stale result.
func (gr *Group) Forget(key string) {
	gr.g.Forget(key)
}
