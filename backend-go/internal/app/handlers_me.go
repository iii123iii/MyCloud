package app

import (
	"database/sql"
	"net/http"

	"mycloud/backend-go/internal/httpapi"
)

// handleMyActivity returns the caller's own activity log entries.
// Mirrors handleAdminLogs but scoped to user_id = caller.
func (a *App) handleMyActivity(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	limit := qInt(r, "limit", 100, 1, 500)
	before := r.URL.Query().Get("before") // optional cursor on created_at

	query := `
		SELECT al.id, al.action, al.resource_type, al.resource_id, al.ip_address, al.created_at
		FROM activity_log al
		WHERE al.user_id = ?`
	args := []any{userID}
	if before != "" {
		query += " AND al.created_at < ?"
		args = append(args, before)
	}
	query += " ORDER BY al.created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := a.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()
	logs := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var action, createdAt string
		var resourceType, resourceID, ip sql.NullString
		if err := rows.Scan(&id, &action, &resourceType, &resourceID, &ip, &createdAt); err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		item := map[string]any{"id": id, "action": action, "created_at": createdAt}
		if resourceType.Valid {
			item["resource_type"] = resourceType.String
		}
		if resourceID.Valid {
			item["resource_id"] = resourceID.String
		}
		if ip.Valid {
			item["ip_address"] = ip.String
		}
		logs = append(logs, item)
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"logs": logs}, nil)
}
