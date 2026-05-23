// Package stmtcache memoises database/sql prepared statements keyed by query
// string. Handlers call Prepare(ctx, "SELECT ... WHERE id=?") on the hot path
// and get back a *sql.Stmt that the driver re-uses across requests, avoiding
// the per-call parse+plan cost MariaDB otherwise pays.
//
// The cache is safe for concurrent use. A single *sql.Stmt is shared across
// goroutines — that matches the database/sql contract and lets us keep the
// cache simple (no per-key locks).
//
// Design notes:
//   - Statements are never evicted: the hot-query set is small and bounded
//     by the number of distinct literal SQL strings in the codebase. If a
//     handler ever generates SQL dynamically (e.g. variable IN-clauses), it
//     should NOT go through this cache because every distinct shape would
//     fill the map; in those cases use db.QueryContext directly.
//   - Close() ranges and closes every cached stmt. Call it from the app's
//     cleanup func so server shutdown releases driver-side state cleanly.
//   - Prepare's contract: if PrepareContext fails (DB closed, syntax error)
//     the error is returned and nothing is cached. Callers can retry without
//     poisoning the slot.
package stmtcache

import (
	"context"
	"database/sql"
	"sync"
	"time"
)

// SlowHook receives a callback whenever a wrapped Query/Exec exceeds the
// installed threshold. Set via SetSlowHook from the app's startup. Default
// is a no-op.
type SlowHook func(ctx context.Context, query string, duration time.Duration, rowsReturned int)

// Cache is a thread-safe map of query string → *sql.Stmt.
type Cache struct {
	db   *sql.DB
	m    sync.Map // map[string]*sql.Stmt
	hook SlowHook
}

// SetSlowHook installs the slow-query callback. nil disables. Safe to call
// once at startup; not safe under concurrent calls (no atomicity guarantees).
func (c *Cache) SetSlowHook(fn SlowHook) {
	c.hook = fn
}

// New constructs a Cache backed by db. The Cache does not take ownership of
// db; callers are responsible for closing db (and calling Cache.Close to
// release prepared-statement state before db.Close to avoid a leak warning
// from some drivers).
func New(db *sql.DB) *Cache {
	return &Cache{db: db}
}

// Prepare returns a *sql.Stmt for query, preparing it on first call and
// reusing the cached value on subsequent calls. Concurrent first-call races
// are resolved by LoadOrStore — at most one prepared statement remains in
// the cache per query; the loser is closed.
func (c *Cache) Prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	if cached, ok := c.m.Load(query); ok {
		return cached.(*sql.Stmt), nil
	}
	stmt, err := c.db.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	actual, loaded := c.m.LoadOrStore(query, stmt)
	if loaded {
		// Lost the race; another goroutine cached its own. Close ours.
		_ = stmt.Close()
	}
	return actual.(*sql.Stmt), nil
}

// QueryContext prepares (or retrieves) a statement and runs Query against it.
// Sugar for the most common call shape: `rows, err := cache.QueryContext(ctx, q, args...)`.
// Instrumentation: timing is captured around the Query and fed to the
// slow-query hook on completion.
func (c *Cache) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	stmt, err := c.Prepare(ctx, query)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	rows, err := stmt.QueryContext(ctx, args...)
	c.observe(ctx, query, time.Since(start), -1)
	return rows, err
}

// QueryRowContext prepares (or retrieves) a statement and runs QueryRow.
// QueryRow's contract is that errors are deferred to .Scan() — to preserve
// that semantic we fall back to db.QueryRowContext on prepare failure so the
// caller's existing error handling kicks in.
func (c *Cache) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	stmt, err := c.Prepare(ctx, query)
	if err != nil {
		// Caller still expects a *sql.Row whose Scan reports the error.
		return c.db.QueryRowContext(ctx, query, args...)
	}
	start := time.Now()
	row := stmt.QueryRowContext(ctx, args...)
	c.observe(ctx, query, time.Since(start), 1)
	return row
}

// ExecContext prepares (or retrieves) a statement and runs Exec.
func (c *Cache) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	stmt, err := c.Prepare(ctx, query)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	res, err := stmt.ExecContext(ctx, args...)
	rows := -1
	if err == nil && res != nil {
		if n, e := res.RowsAffected(); e == nil {
			rows = int(n)
		}
	}
	c.observe(ctx, query, time.Since(start), rows)
	return res, err
}

// observe runs the slow-query hook on a background goroutine so a slow DB
// write to slow_queries can't extend the caller's request latency. nil hook
// skips immediately to keep the no-instrumentation path zero-cost.
func (c *Cache) observe(ctx context.Context, query string, dur time.Duration, rows int) {
	if c.hook == nil {
		return
	}
	go c.hook(ctx, query, dur, rows)
}

// Close releases every cached statement. Safe to call once; subsequent calls
// are no-ops because the map is emptied as it iterates.
func (c *Cache) Close() error {
	var firstErr error
	c.m.Range(func(key, val any) bool {
		if stmt, ok := val.(*sql.Stmt); ok {
			if err := stmt.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		c.m.Delete(key)
		return true
	})
	return firstErr
}

// Len returns how many distinct queries are currently cached. Useful for
// metrics — sum of Len() across processes is the "prepared stmt count"
// gauge an operator wants to track for connection-pool sizing.
func (c *Cache) Len() int {
	n := 0
	c.m.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}
