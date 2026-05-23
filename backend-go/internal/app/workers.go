package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"mycloud/backend-go/internal/logging"
)

// goSafe launches a goroutine that catches panics and logs them with stack
// trace under the supplied name. Background workers must use this — the
// HTTP middleware.Recoverer only saves request handlers; a panic in a
// long-running worker kills it silently until process restart.
func goSafe(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("worker panic",
					"worker", name,
					"panic", r,
					"stack", string(debug.Stack()))
			}
		}()
		fn()
	}()
}

// startWorkers launches background goroutines for the async work items.
// Each worker drains a Redis list using BRPOP. The context, normally derived
// from the application's lifetime, signals when to stop. Workers exit on
// context cancellation; transient errors are logged with a backoff sleep so
// a flaky Redis doesn't burn CPU.
func (a *App) startWorkers(ctx context.Context) {
	goSafe("q:thumb", func() { a.drainQueue(ctx, "q:thumb", a.processThumbJob) })
	goSafe("q:extract", func() { a.drainQueue(ctx, "q:extract", a.processExtractJob) })
	// HLS transcoder. Single worker; the underlying ffmpeg already
	// saturates a core, and queueing multiple concurrent transcodes on
	// the same host would starve everything else.
	goSafe("q:hls", func() { a.drainQueue(ctx, "q:hls", a.processHLSJob) })
	goSafe("maintenance", func() { a.startMaintenanceTicker(ctx) })
	// Metrics sampler runs on a slower cadence than maintenance — once every
	// 15s gives Prometheus a fresh value at the default 15s scrape interval
	// without burning Redis QPS on LLEN queries.
	goSafe("metrics", func() { a.startMetricsSampler(ctx) })
}

// startMetricsSampler periodically refreshes the Prometheus gauges that
// observe state we don't see at request time: prepared-statement count,
// Redis queue depths. Histograms and counters are bumped at the call site so
// they don't need a sampler.
func (a *App) startMetricsSampler(ctx context.Context) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	queues := []string{"q:thumb", "q:extract", "q:hls"}
	hb := a.Heartbeats["metrics"]
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if hb != nil {
				hb.Touch()
			}
			if a.Metrics != nil && a.Stmts != nil {
				a.Metrics.StmtCacheSize.Set(float64(a.Stmts.Len()))
			}
			for _, q := range queues {
				if n, err := a.Redis.LLen(ctx, q).Result(); err == nil && a.Metrics != nil {
					a.Metrics.QueueDepth.WithLabelValues(q).Set(float64(n))
				}
			}
		}
	}
}

// Method wrappers that adapt the worker's []byte payload signature to the
// string-fileID signature the real processors expose. The implementations
// live in thumbs.go and extract.go.
func (a *App) processThumbJob(ctx context.Context, payload []byte) error {
	return a.processThumb(ctx, string(payload))
}

func (a *App) processExtractJob(ctx context.Context, payload []byte) error {
	return a.processExtract(ctx, string(payload))
}

// processFn handles one queued payload. Implementations should be safe to
// call concurrently if called from multiple workers — for the single-worker
// pattern below they need not be.
type processFn func(ctx context.Context, payload []byte) error

func (a *App) drainQueue(ctx context.Context, queue string, fn processFn) {
	// Build a queue-scoped logger once so every line carries worker=queue
	// without re-attaching the field on each call site.
	qLog := slog.With("worker", queue)
	// Heartbeat name strips the "q:" prefix to match the canonical key in
	// app.Heartbeats ("thumb", "extract", etc.).
	hbName := strings.TrimPrefix(queue, "q:")
	hb := a.Heartbeats[hbName]
	backoff := 0
	for {
		if ctx.Err() != nil {
			return
		}
		// Touch heartbeat every loop iteration — even when the queue is
		// empty — so /readyz knows the worker is alive and not stuck.
		if hb != nil {
			hb.Touch()
		}
		// BRPOP with a 5s timeout — returns ("", "", redis.Nil) when nothing arrives.
		res, err := a.Redis.BRPop(ctx, 5*time.Second, queue).Result()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// redis.Nil is returned on timeout — that's normal. Use the
			// typed sentinel via errors.Is so a wrap or a driver-version
			// change in the error string doesn't break this loop.
			if errors.Is(err, redis.Nil) {
				continue
			}
			backoff++
			if backoff > 6 {
				backoff = 6
			}
			qLog.Warn("brpop failed", "error", err, "backoff_seconds", 1<<backoff)
			time.Sleep(time.Duration(1<<backoff) * time.Second)
			continue
		}
		backoff = 0
		if len(res) < 2 {
			continue
		}
		payload := []byte(res[1])
		workerCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		// Attach the queue-scoped logger so deeper functions (processThumb,
		// processExtract) can pick it up via logging.FromContext.
		jobLog := qLog.With("job_payload", string(payload))
		workerCtx = logging.IntoContext(workerCtx, jobLog)
		// Catch panics from a single bad job so the worker stays up.
		// image.Decode on adversarial JPEGs is one historical panic source;
		// crypto streamers can panic on truncated input. Either case
		// shouldn't take down the queue.
		runJob := func() (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("panic: %v", r)
					jobLog.Error("job panic", "panic", r, "stack", string(debug.Stack()))
				}
			}()
			return fn(workerCtx, payload)
		}
		if err := runJob(); err != nil {
			jobLog.Error("job failed", "error", err)
			// Dead-letter: push to <queue>:failed with the error attached.
			dl, _ := json.Marshal(map[string]any{"payload": string(payload), "error": err.Error()})
			if dlErr := a.Redis.LPush(ctx, queue+":failed", dl).Err(); dlErr != nil {
				jobLog.Warn("failed to push to dead-letter queue", "error", dlErr)
			}
			if a.Metrics != nil {
				a.Metrics.DLQPushed.WithLabelValues(queue).Inc()
			}
		}
		cancel()
	}
}

// enqueueExtract queues a file for text extraction.
func (a *App) enqueueExtract(ctx context.Context, fileID string) {
	_ = a.Redis.LPush(ctx, "q:extract", fileID).Err()
}

// enqueueThumb queues a file for thumbnail+EXIF generation.
func (a *App) enqueueThumb(ctx context.Context, fileID string) {
	_ = a.Redis.LPush(ctx, "q:thumb", fileID).Err()
}

// afterUploadCommit enqueues background jobs for a freshly uploaded file.
// Called by upload paths (single-shot + tus) after the DB transaction commits.
// Safe to call with an empty mimeType — uninteresting types are no-ops.
func (a *App) afterUploadCommit(ctx context.Context, fileID, mimeType string) {
	if fileID == "" {
		return
	}
	lower := strings.ToLower(mimeType)
	if isIndexableMime(lower) {
		a.enqueueExtract(ctx, fileID)
	}
	if strings.HasPrefix(lower, "image/") {
		a.enqueueThumb(ctx, fileID)
	}
}

func isIndexableMime(mime string) bool {
	if strings.HasPrefix(mime, "text/") {
		return true
	}
	switch mime {
	case "application/pdf",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/msword",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.ms-excel":
		return true
	}
	return false
}

// startMaintenanceTicker fires once an hour to prune shares past expiry by
// more than 30 days, expired upload-requests, and orphaned blob_refs with
// ref_count == 0.
func (a *App) startMaintenanceTicker(ctx context.Context) {
	// Tick a short heartbeat inside the long maintenance interval so
	// /readyz doesn't flap red between hourly runs.
	hbT := time.NewTicker(30 * time.Second)
	defer hbT.Stop()
	t := time.NewTicker(1 * time.Hour)
	defer t.Stop()
	hb := a.Heartbeats["maintenance"]
	if hb != nil {
		hb.Touch()
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-hbT.C:
			if hb != nil {
				hb.Touch()
			}
		case <-t.C:
			a.runMaintenance(ctx)
			if hb != nil {
				hb.Touch()
			}
		}
	}
}

func (a *App) runMaintenance(ctx context.Context) {
	mLog := slog.With("worker", "maintenance")
	if _, err := a.DB.ExecContext(ctx,
		"DELETE FROM shares WHERE expires_at IS NOT NULL AND expires_at < NOW() - INTERVAL 30 DAY"); err != nil {
		mLog.Error("prune shares", "error", err)
	}
	if _, err := a.DB.ExecContext(ctx,
		"DELETE FROM upload_requests WHERE expires_at IS NOT NULL AND expires_at < NOW() - INTERVAL 30 DAY"); err != nil {
		mLog.Error("prune upload_requests", "error", err)
	}
	// Expired file locks. Cheap to clear and prevents stale locks
	// from showing up in handleListFileLocks (the read path already
	// filters by expires_at > NOW(), but disk-space hygiene matters).
	if _, err := a.DB.ExecContext(ctx,
		"DELETE FROM file_locks WHERE expires_at < NOW()"); err != nil {
		mLog.Error("prune file_locks", "error", err)
	}
	// Apply per-folder retention policies.
	a.runRetentionSweep(ctx)
	// Cap slow_queries to the most recent N rows. DELETE-by-rownumber
	// is cheap with the captured_at index.
	if _, err := a.DB.ExecContext(ctx, `
		DELETE FROM slow_queries
		WHERE id NOT IN (
		    SELECT id FROM (
		        SELECT id FROM slow_queries ORDER BY captured_at DESC LIMIT ?
		    ) sub
		)`, 10000); err != nil {
		mLog.Error("prune slow_queries", "error", err)
	}
	// Activity log retention. The table grows forever otherwise — a year of
	// normal use produces millions of rows, ballooning backup size and
	// slowing every admin listing. 90 days is a balance between forensic
	// value and storage cost; operators who need longer retention should
	// ship to a SIEM and bump this. We delete in batches to avoid lock
	// escalation on a fat table.
	for batch := 0; batch < 100; batch++ {
		res, err := a.DB.ExecContext(ctx,
			"DELETE FROM activity_log WHERE created_at < NOW() - INTERVAL 90 DAY LIMIT 5000")
		if err != nil {
			mLog.Error("prune activity_log", "error", err)
			break
		}
		n, _ := res.RowsAffected()
		if n < 5000 {
			break
		}
	}
	// Soft-delete cascade for orphan share grants. A share_grants row
	// pointing at a file/folder soft-deleted more than 30 days ago is
	// guaranteed never to be useful again — the file is past trash retention
	// for permanent delete in most setups, and the recipient sees nothing in
	// listings either way. Pruning keeps the join cheap.
	if _, err := a.DB.ExecContext(ctx, `
		DELETE g FROM share_grants g
		LEFT JOIN files f ON f.id = g.file_id
		LEFT JOIN folders d ON d.id = g.folder_id
		WHERE (f.is_deleted = 1 AND f.deleted_at < NOW() - INTERVAL 30 DAY)
		   OR (d.is_deleted = 1 AND d.deleted_at < NOW() - INTERVAL 30 DAY)`); err != nil {
		mLog.Error("prune orphan share_grants", "error", err)
	}
}
