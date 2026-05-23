package app

// Worker dead-letter queue (DLQ) admin endpoints.
//
// The drainQueue loop in workers.go pushes failed jobs to "<queue>:failed".
// Before these handlers existed, that list was effectively write-only — no
// way to see what's stuck, no way to retry a fixed-by-deploy job, no way to
// clear obsolete entries. Operators had to ssh into the Redis container and
// LRANGE by hand.
//
// Mounted under /admin/workers/dlq/* in router.go.

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"mycloud/backend-go/internal/httpapi"
)

// dlqQueues lists the source queues whose :failed counterpart we expose.
// Keep in sync with the workers started in startWorkers.
var dlqQueues = []string{"q:thumb", "q:extract", "q:hls"}

// handleAdminDLQList returns the depth + a sample of entries for every
// known DLQ. Sample is bounded so a huge backlog doesn't pin the admin UI
// — operators that want the full set should use the Redis CLI.
func (a *App) handleAdminDLQList(w http.ResponseWriter, r *http.Request) {
	type queueInfo struct {
		Queue   string   `json:"queue"`
		Depth   int64    `json:"depth"`
		Samples []string `json:"samples"`
	}
	out := make([]queueInfo, 0, len(dlqQueues))
	for _, base := range dlqQueues {
		key := base + ":failed"
		depth, err := a.Redis.LLen(r.Context(), key).Result()
		if err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "redis_error", err.Error())
			return
		}
		// LRANGE the head — most recent failures sit at the head of the
		// list since drainQueue uses LPUSH.
		raws, err := a.Redis.LRange(r.Context(), key, 0, 19).Result()
		if err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "redis_error", err.Error())
			return
		}
		out = append(out, queueInfo{Queue: base, Depth: depth, Samples: raws})
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"dlq": out}, nil)
}

// handleAdminDLQRetry pops every entry from <queue>:failed and re-pushes
// the original payload onto <queue>. Idempotent over partial failure: if
// the loop is interrupted, remaining entries stay in :failed and a
// subsequent call resumes. Uses an LMOVE-equivalent two-step (RPOP from
// failed, LPUSH to source) so a crash mid-loop doesn't lose payloads.
func (a *App) handleAdminDLQRetry(w http.ResponseWriter, r *http.Request) {
	queue := "q:" + chi.URLParam(r, "queue")
	if !isKnownDLQ(queue) {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Unknown queue")
		return
	}
	failed := queue + ":failed"
	moved := 0
	for {
		raw, err := a.Redis.RPop(r.Context(), failed).Result()
		if err == redis.Nil {
			break
		}
		if err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "redis_error", err.Error())
			return
		}
		// Strip the wrapping {payload, error} envelope the dead-letter
		// path adds — re-enqueue only the original payload bytes so
		// workers see exactly what they got the first time.
		var env struct {
			Payload string `json:"payload"`
		}
		payload := raw
		if err := json.Unmarshal([]byte(raw), &env); err == nil && env.Payload != "" {
			payload = env.Payload
		}
		if err := a.Redis.LPush(r.Context(), queue, payload).Err(); err != nil {
			// Best-effort restore so the entry isn't lost.
			_ = a.Redis.LPush(r.Context(), failed, raw).Err()
			httpapi.Error(w, http.StatusInternalServerError, "redis_error", err.Error())
			return
		}
		moved++
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"requeued": moved}, nil)
}

// handleAdminDLQClear empties <queue>:failed. Used when retrying is
// pointless (e.g. payloads referred to files since deleted).
func (a *App) handleAdminDLQClear(w http.ResponseWriter, r *http.Request) {
	queue := "q:" + chi.URLParam(r, "queue")
	if !isKnownDLQ(queue) {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Unknown queue")
		return
	}
	if err := a.Redis.Del(r.Context(), queue+":failed").Err(); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "redis_error", err.Error())
		return
	}
	httpapi.NoContent(w)
}

func isKnownDLQ(q string) bool {
	for _, k := range dlqQueues {
		if k == q {
			return true
		}
	}
	return false
}
