package app

import (
	"database/sql"
	"net/http"

	"mycloud/backend-go/internal/httpapi"
)

func (a *App) handleSearch(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	q := "%" + r.URL.Query().Get("q") + "%"
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT 'file' AS item_type, id, name, size_bytes, mime_type, is_starred, updated_at
		FROM files WHERE user_id=? AND is_deleted=0 AND name LIKE ?
		UNION ALL
		SELECT 'folder' AS item_type, id, name, NULL, NULL, NULL, updated_at
		FROM folders WHERE user_id=? AND is_deleted=0 AND name LIKE ?
		ORDER BY updated_at DESC LIMIT 100`, userID, q, userID, q)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()
	results := make([]map[string]any, 0)
	for rows.Next() {
		var itemType, id, name, updatedAt string
		var size sql.NullInt64
		var mime sql.NullString
		var starred sql.NullBool
		if err := rows.Scan(&itemType, &id, &name, &size, &mime, &starred, &updatedAt); err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		item := map[string]any{"type": itemType, "id": id, "name": name, "updated_at": updatedAt}
		if size.Valid {
			item["size_bytes"] = size.Int64
		}
		if mime.Valid {
			item["mime_type"] = mime.String
		}
		if starred.Valid {
			item["is_starred"] = starred.Bool
		}
		results = append(results, item)
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"results": results}, nil)
}
