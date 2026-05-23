package app

// Admin log viewer + CSV export.
//
// Reads from two sources:
//
//   1. activity_log table (user-facing events: uploads, shares, deletes)
//   2. the JSONL system-log file at cfg.LogFilePath (slog-emitted HTTP +
//      worker logs)
//
// Endpoints:
//   GET /api/v2/admin/logs/activity?level=&action=&user=&since=&until=&cursor=&limit=&export=csv
//   GET /api/v2/admin/logs/system?since=&until=&level=&limit=&export=csv
//
// Both endpoints accept ?export=csv to switch the response from JSON to a
// downloadable CSV (Content-Disposition: attachment).

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"mycloud/backend-go/internal/httpapi"
)

func (a *App) handleAdminActivityLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	action := q.Get("action")
	user := q.Get("user")
	since := q.Get("since")
	until := q.Get("until")
	limit := qInt(r, "limit", 200, 1, 5000)
	export := q.Get("export") == "csv"

	where := []string{"1=1"}
	args := []any{}
	if action != "" {
		where = append(where, "action = ?")
		args = append(args, action)
	}
	if user != "" {
		where = append(where, "user_id = ?")
		args = append(args, user)
	}
	if since != "" {
		where = append(where, "created_at >= ?")
		args = append(args, since)
	}
	if until != "" {
		where = append(where, "created_at <= ?")
		args = append(args, until)
	}

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id, COALESCE(user_id,''), action, COALESCE(resource_type,''), COALESCE(resource_id,''),
		        COALESCE(details, '{}'), COALESCE(ip_address, ''), created_at
		 FROM activity_log WHERE `+strings.Join(where, " AND ")+
			` ORDER BY id DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	defer rows.Close()

	type entry struct {
		ID           int64     `json:"id"`
		UserID       string    `json:"user_id"`
		Action       string    `json:"action"`
		ResourceType string    `json:"resource_type"`
		ResourceID   string    `json:"resource_id"`
		Details      string    `json:"details"`
		IPAddress    string    `json:"ip_address"`
		CreatedAt    time.Time `json:"created_at"`
	}
	out := make([]entry, 0, limit)
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.ID, &e.UserID, &e.Action, &e.ResourceType, &e.ResourceID, &e.Details, &e.IPAddress, &e.CreatedAt); err != nil {
			respondDBError(w, r, err)
			return
		}
		out = append(out, e)
	}

	if export {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="activity-log.csv"`)
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"id", "created_at", "user_id", "action", "resource_type", "resource_id", "ip_address", "details"})
		for _, e := range out {
			_ = cw.Write([]string{
				strconv.FormatInt(e.ID, 10),
				e.CreatedAt.Format(time.RFC3339),
				e.UserID, e.Action, e.ResourceType, e.ResourceID, e.IPAddress, e.Details,
			})
		}
		cw.Flush()
		return
	}

	httpapi.JSON(w, http.StatusOK, map[string]any{"activity": out}, nil)
}

func (a *App) handleAdminSystemLogs(w http.ResponseWriter, r *http.Request) {
	if a.Config.LogFilePath == "" {
		httpapi.JSON(w, http.StatusOK, map[string]any{"system_logs": []any{}, "note": "LOG_FILE_PATH unset"}, nil)
		return
	}
	limit := qInt(r, "limit", 500, 1, 5000)
	levelFilter := strings.ToLower(r.URL.Query().Get("level"))
	export := r.URL.Query().Get("export") == "csv"

	f, err := os.Open(a.Config.LogFilePath)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "log_open", err.Error())
		return
	}
	defer f.Close()
	// Tail-read: collect last N matching lines. JSONL is line-oriented;
	// linear scan is acceptable up to ~50 MiB. For larger files this
	// degrades; future versions can index by size/mtime ranges.
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1 MiB max line — slog lines stay well below
	type entry struct {
		Time  string         `json:"time"`
		Level string         `json:"level"`
		Msg   string         `json:"msg"`
		Attrs map[string]any `json:"attrs"`
	}
	rolling := make([]entry, 0, limit)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}
		level, _ := raw["level"].(string)
		if levelFilter != "" && !strings.EqualFold(level, levelFilter) {
			continue
		}
		e := entry{
			Time:  toStr(raw["time"]),
			Level: level,
			Msg:   toStr(raw["msg"]),
		}
		// Strip the standard fields into a separate attrs map for the UI.
		delete(raw, "time")
		delete(raw, "level")
		delete(raw, "msg")
		e.Attrs = raw
		// Maintain a sliding window of the last N matching entries.
		if len(rolling) >= limit {
			copy(rolling, rolling[1:])
			rolling = rolling[:limit-1]
		}
		rolling = append(rolling, e)
	}

	if export {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="system-log.csv"`)
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"time", "level", "msg", "attrs_json"})
		for _, e := range rolling {
			attrs, _ := json.Marshal(e.Attrs)
			_ = cw.Write([]string{e.Time, e.Level, e.Msg, string(attrs)})
		}
		cw.Flush()
		return
	}

	httpapi.JSON(w, http.StatusOK, map[string]any{"system_logs": rolling}, nil)
}

func toStr(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}
