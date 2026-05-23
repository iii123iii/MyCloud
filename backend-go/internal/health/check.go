// Package health exposes the dependency probes used by /readyz.
//
// Each check is a short-lived (≤500 ms) ping that returns a Result rather
// than just a boolean — operators want to know WHICH dependency is sick,
// not just "something failed". The /readyz endpoint aggregates these into a
// single 200/503 with a JSON body listing each result.
//
// The probes are intentionally lightweight: a SELECT 1, a Redis PING, a
// worker-heartbeat timestamp check. Avoid putting expensive validation
// here — readiness probes run every few seconds in production and would
// add real cost.
package health

import (
	"context"
	"database/sql"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// Result is the per-check outcome. Detail is human-readable and can be
// surfaced in /readyz; Latency lets dashboards spot creeping degradation
// before a hard failure.
type Result struct {
	Name    string        `json:"name"`
	OK      bool          `json:"ok"`
	Detail  string        `json:"detail,omitempty"`
	Latency time.Duration `json:"latency_ns"`
}

// CheckDB runs SELECT 1 with a short timeout.
func CheckDB(ctx context.Context, db *sql.DB) Result {
	if db == nil {
		return Result{Name: "db", OK: false, Detail: "db is nil"}
	}
	probeCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	var one int
	err := db.QueryRowContext(probeCtx, "SELECT 1").Scan(&one)
	r := Result{Name: "db", OK: err == nil && one == 1, Latency: time.Since(start)}
	if err != nil {
		r.Detail = err.Error()
	}
	return r
}

// CheckRedis runs PING.
func CheckRedis(ctx context.Context, client *redis.Client) Result {
	if client == nil {
		return Result{Name: "redis", OK: false, Detail: "redis is nil"}
	}
	probeCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	res, err := client.Ping(probeCtx).Result()
	r := Result{Name: "redis", OK: err == nil && res == "PONG", Latency: time.Since(start)}
	if err != nil {
		r.Detail = err.Error()
	}
	return r
}

// WorkerHeartbeat is an atomic timestamp updated by long-running workers
// to assert they're still alive. /readyz reads it via CheckWorker; if no
// update has occurred in `maxAge`, the worker is considered stuck.
type WorkerHeartbeat struct {
	last atomic.Int64 // unix-nanos
}

// Touch updates the heartbeat to now. Call from inside the worker loop on
// every successful iteration.
func (h *WorkerHeartbeat) Touch() {
	h.last.Store(time.Now().UnixNano())
}

// CheckWorker returns a Result describing whether the worker has heartbeat
// inside the last maxAge. A worker that has never beaten is considered down.
func CheckWorker(name string, h *WorkerHeartbeat, maxAge time.Duration) Result {
	if h == nil {
		return Result{Name: "worker:" + name, OK: false, Detail: "heartbeat is nil"}
	}
	last := h.last.Load()
	if last == 0 {
		return Result{Name: "worker:" + name, OK: false, Detail: "never started"}
	}
	age := time.Since(time.Unix(0, last))
	return Result{
		Name:    "worker:" + name,
		OK:      age <= maxAge,
		Detail:  age.String(),
		Latency: age,
	}
}
