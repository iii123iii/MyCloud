package app

// Slow-query log + viewer.
//
// The stmtcache layer ALWAYS times every query it runs (cheap, no allocation
// beyond the timestamps). Queries that exceed the threshold are written to
// the slow_queries table AND broadcast on the WS topic "admin:slow-queries"
// so the admin dashboard's live tail shows them in real time.
//
// We deliberately do NOT log the bound parameters: those may contain
// password hashes, share tokens, file content snippets. Only the query text
// (which is a fixed string per call site) and the timing are recorded.

import (
	"context"
	"net/http"
	"time"

	"mycloud/backend-go/internal/httpapi"
	"mycloud/backend-go/internal/wsHub"
)

// slowQueryThreshold is the wall-clock cutoff above which a query gets
// recorded. 200 ms is a generous default — anything slower than that is
// noticeable to a human user and worth investigating.
const slowQueryThreshold = 200 * time.Millisecond

// slowQueryMax caps the table size so a sustained issue doesn't fill disk.
// Oldest rows are pruned by the maintenance ticker once this is exceeded.
const slowQueryMax = 10_000

// recordSlowQuery is called from a Goroutine spawned by the stmtcache
// instrumentation. It MUST be cheap — failures are swallowed so a database
// blip doesn't bubble back into the request that produced the slow log.
func (a *App) recordSlowQuery(ctx context.Context, query string, duration time.Duration, rowsReturned int) {
	if duration < slowQueryThreshold {
		return
	}
	_, err := a.DB.ExecContext(ctx, `
		INSERT INTO slow_queries (query_text, duration_ms, rows_returned)
		VALUES (?, ?, ?)`,
		query, int(duration.Milliseconds()), rowsReturned)
	if err != nil {
		return
	}
	if a.Hub != nil {
		_ = a.Hub.Broker().Publish(ctx, "admin:slow-queries", wsHub.Envelope{
			Type:  "event",
			Topic: "admin:slow-queries",
			Data: map[string]any{
				"query_text":     query,
				"duration_ms":    duration.Milliseconds(),
				"rows_returned":  rowsReturned,
				"captured_at":    time.Now().UTC(),
			},
		})
	}
}

// handleAdminSlowQueries lists recent slow queries. Pagination via
// ?since= (unix-ms) and ?limit=.
func (a *App) handleAdminSlowQueries(w http.ResponseWriter, r *http.Request) {
	limit := qInt(r, "limit", 100, 1, 1000)
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT id, query_text, duration_ms, rows_returned, captured_at
		FROM slow_queries
		ORDER BY captured_at DESC
		LIMIT ?`, limit)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0, limit)
	for rows.Next() {
		var id int64
		var query string
		var duration int
		var rowsRet *int
		var captured time.Time
		if err := rows.Scan(&id, &query, &duration, &rowsRet, &captured); err != nil {
			respondDBError(w, r, err)
			return
		}
		out = append(out, map[string]any{
			"id":             id,
			"query_text":     query,
			"duration_ms":    duration,
			"rows_returned":  rowsRet,
			"captured_at":    captured,
		})
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"slow_queries": out}, nil)
}
