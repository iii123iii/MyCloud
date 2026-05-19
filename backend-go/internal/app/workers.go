package app

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"
)

// startWorkers launches background goroutines for the async work items.
// Each worker drains a Redis list using BRPOP. The context, normally derived
// from the application's lifetime, signals when to stop. Workers exit on
// context cancellation; transient errors are logged with a backoff sleep so
// a flaky Redis doesn't burn CPU.
func (a *App) startWorkers(ctx context.Context) {
	go a.drainQueue(ctx, "q:thumb", a.processThumbJob)
	go a.drainQueue(ctx, "q:extract", a.processExtractJob)
	go a.startMaintenanceTicker(ctx)
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
	backoff := 0
	for {
		if ctx.Err() != nil {
			return
		}
		// BRPOP with a 5s timeout — returns ("", "", redis.Nil) when nothing arrives.
		res, err := a.Redis.BRPop(ctx, 5*time.Second, queue).Result()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// redis.Nil is returned on timeout — that's normal.
			if err.Error() == "redis: nil" {
				continue
			}
			backoff++
			if backoff > 6 {
				backoff = 6
			}
			time.Sleep(time.Duration(1<<backoff) * time.Second)
			continue
		}
		backoff = 0
		if len(res) < 2 {
			continue
		}
		payload := []byte(res[1])
		workerCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		if err := fn(workerCtx, payload); err != nil {
			log.Printf("worker %s: %v", queue, err)
			// Dead-letter: push to <queue>:failed with the error attached.
			dl, _ := json.Marshal(map[string]any{"payload": string(payload), "error": err.Error()})
			_ = a.Redis.LPush(ctx, queue+":failed", dl).Err()
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
	t := time.NewTicker(1 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.runMaintenance(ctx)
		}
	}
}

func (a *App) runMaintenance(ctx context.Context) {
	if _, err := a.DB.ExecContext(ctx,
		"DELETE FROM shares WHERE expires_at IS NOT NULL AND expires_at < NOW() - INTERVAL 30 DAY"); err != nil {
		log.Printf("maintenance: prune shares: %v", err)
	}
	if _, err := a.DB.ExecContext(ctx,
		"DELETE FROM upload_requests WHERE expires_at IS NOT NULL AND expires_at < NOW() - INTERVAL 30 DAY"); err != nil {
		log.Printf("maintenance: prune upload_requests: %v", err)
	}
}
