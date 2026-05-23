package app

// Background reindexer.
//
// Triggered by admins from POST /admin/reindex. Walks every (non-deleted)
// file that should be in the text index, requeues it on q:extract, and
// streams progress to admins via the WS topic "admin:reindex".

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"mycloud/backend-go/internal/httpapi"
	"mycloud/backend-go/internal/wsHub"
)

// reindexState lives in Redis under key "reindex:progress" so any backend
// instance can render the same dashboard. Single-instance for now; the
// schema is forward-compatible.
const reindexStateKey = "reindex:progress"

type reindexProgress struct {
	StartedAt time.Time `json:"started_at"`
	Total     int       `json:"total"`
	Processed int       `json:"processed"`
	Done      bool      `json:"done"`
	Error     string    `json:"error,omitempty"`
}

// handleAdminReindex kicks off a reindex. Atomic via Redis SET NX so two
// admins clicking the button concurrently can't both spawn sweeps.
func (a *App) handleAdminReindex(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode string `json:"mode"` // "missing" (default) | "all"
	}
	_ = decodeJSON(r, &body)
	if body.Mode == "" {
		body.Mode = "missing"
	}

	// Atomic lock: SET NX with a 1-hour safety TTL so a crashed sweep
	// doesn't permanently wedge the button. The runReindexSweep goroutine
	// clears the lock when it finishes via writeReindexState(Done:true).
	initial := reindexProgress{StartedAt: time.Now().UTC()}
	payload, _ := json.Marshal(initial)
	ok, err := a.Redis.SetNX(r.Context(), reindexStateKey, payload, time.Hour).Result()
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "redis_error", err.Error())
		return
	}
	if !ok {
		// Lock taken — read back to surface the existing run's start time.
		raw, _ := a.Redis.Get(r.Context(), reindexStateKey).Result()
		var existing reindexProgress
		_ = json.Unmarshal([]byte(raw), &existing)
		if existing.Done {
			// Stale Done state in the key — replace it with our fresh start.
			_ = a.Redis.Set(r.Context(), reindexStateKey, payload, time.Hour).Err()
		} else {
			httpapi.Error(w, http.StatusConflict, "already_running",
				"reindex started at "+existing.StartedAt.Format(time.RFC3339))
			return
		}
	}

	// Run the sweep in a goroutine so the HTTP request returns 202 fast.
	go a.runReindexSweep(context.Background(), body.Mode)

	httpapi.JSON(w, http.StatusAccepted, map[string]any{
		"ok":   true,
		"mode": body.Mode,
	}, nil)
}

// handleAdminReindexStatus exposes the current progress so non-WS clients
// can poll.
func (a *App) handleAdminReindexStatus(w http.ResponseWriter, r *http.Request) {
	raw, err := a.Redis.Get(r.Context(), reindexStateKey).Result()
	if err != nil || raw == "" {
		httpapi.JSON(w, http.StatusOK, map[string]any{"status": "idle"}, nil)
		return
	}
	var p reindexProgress
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "bad_state", err.Error())
		return
	}
	httpapi.JSON(w, http.StatusOK, p, nil)
}

// runReindexSweep walks the files table, requeueing each row on q:extract.
// Updates reindexStateKey every 100 rows or every 2 s, whichever comes
// first. Publishes the same payload to "admin:reindex" so live UI updates.
func (a *App) runReindexSweep(ctx context.Context, mode string) {
	lg := slog.With("worker", "reindex", "mode", mode)
	lg.Info("reindex starting")
	start := time.Now()

	// Count first so the UI can show "X / Y".
	totalQuery := `SELECT COUNT(*) FROM files WHERE is_deleted = 0`
	if mode == "missing" {
		totalQuery = `SELECT COUNT(*) FROM files f
		              WHERE f.is_deleted = 0
		                AND NOT EXISTS (SELECT 1 FROM file_text t WHERE t.file_id = f.id)`
	}
	var total int
	if err := a.DB.QueryRowContext(ctx, totalQuery).Scan(&total); err != nil {
		a.writeReindexState(ctx, reindexProgress{StartedAt: start, Done: true, Error: err.Error()})
		return
	}

	// Walk in batches of 500 keyed by id to avoid offset-degradation on big tables.
	var lastID string
	processed := 0
	a.writeReindexState(ctx, reindexProgress{StartedAt: start, Total: total})

	for {
		listQuery := `SELECT id FROM files WHERE is_deleted = 0 AND id > ? ORDER BY id LIMIT 500`
		if mode == "missing" {
			listQuery = `SELECT f.id FROM files f
			             WHERE f.is_deleted = 0 AND f.id > ?
			               AND NOT EXISTS (SELECT 1 FROM file_text t WHERE t.file_id = f.id)
			             ORDER BY f.id LIMIT 500`
		}
		rows, err := a.DB.QueryContext(ctx, listQuery, lastID)
		if err != nil {
			a.writeReindexState(ctx, reindexProgress{
				StartedAt: start, Total: total, Processed: processed, Done: true, Error: err.Error(),
			})
			return
		}
		batch := make([]string, 0, 500)
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err == nil {
				batch = append(batch, id)
			}
		}
		rows.Close()
		if len(batch) == 0 {
			break
		}
		for _, id := range batch {
			a.enqueueExtract(ctx, id)
			processed++
		}
		lastID = batch[len(batch)-1]
		a.writeReindexState(ctx, reindexProgress{
			StartedAt: start, Total: total, Processed: processed,
		})
	}

	a.writeReindexState(ctx, reindexProgress{
		StartedAt: start, Total: total, Processed: processed, Done: true,
	})
	lg.Info("reindex complete", "processed", processed, "duration", time.Since(start))
}

func (a *App) writeReindexState(ctx context.Context, p reindexProgress) {
	payload, err := json.Marshal(p)
	if err != nil {
		return
	}
	_ = a.Redis.Set(ctx, reindexStateKey, payload, 24*time.Hour).Err()
	if a.Hub != nil {
		_ = a.Hub.Broker().Publish(ctx, "admin:reindex", wsHub.Envelope{
			Type:  "event",
			Topic: "admin:reindex",
			Data:  p,
		})
	}
}

// Suppress an unused-import lint when only one branch references strconv.
var _ = strconv.Itoa
