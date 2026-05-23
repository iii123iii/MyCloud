package app

// Per-folder activity feed.
//
// GET /api/v2/folders/{id}/activity — historic activity rows that touched
// this folder or any descendant file. Cursor pagination on
// activity_log.id.
//
// Live updates flow through the WS topic "folder:<id>" which writeActivity
// publishes to whenever a row is written that names this folder as either
// resource_id (resource_type='folder') or via the detail map.
//
// Per-folder permission scope: the caller must have at least viewer access
// on the folder. The endpoint does NOT filter results by row owner — every
// action on the folder is visible to anyone with folder access, which is
// the intent of a collaboration feed.

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"mycloud/backend-go/internal/httpapi"
)

func (a *App) handleFolderActivity(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	folderID := chi.URLParam(r, "id")
	if _, err := a.canAccessFolder(r.Context(), userID, folderID, AccessViewer); err != nil {
		httpapi.Error(w, http.StatusForbidden, "forbidden", "no folder access")
		return
	}

	limit := qInt(r, "limit", 50, 1, 200)
	beforeID := r.URL.Query().Get("before_id")

	args := []any{folderID, folderID}
	q := `
		SELECT id, user_id, action, resource_type, resource_id, details, created_at
		FROM activity_log
		WHERE (
		    (resource_type = 'folder' AND resource_id = ?) OR
		    (resource_type = 'file' AND resource_id IN (
		        SELECT id FROM files WHERE folder_id = ?
		    ))
		)`
	if beforeID != "" {
		q += " AND id < ?"
		args = append(args, beforeID)
	}
	q += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := a.DB.QueryContext(r.Context(), q, args...)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0, limit)
	var lastID int64
	for rows.Next() {
		var (
			id           int64
			rowUser      sql.NullString
			action       string
			resourceType sql.NullString
			resourceID   sql.NullString
			details      sql.NullString
			createdAt    time.Time
		)
		if err := rows.Scan(&id, &rowUser, &action, &resourceType, &resourceID, &details, &createdAt); err != nil {
			respondDBError(w, r, err)
			return
		}
		entry := map[string]any{
			"id":         id,
			"action":     action,
			"created_at": createdAt,
		}
		if rowUser.Valid {
			entry["user_id"] = rowUser.String
		}
		if resourceType.Valid {
			entry["resource_type"] = resourceType.String
		}
		if resourceID.Valid {
			entry["resource_id"] = resourceID.String
		}
		if details.Valid {
			entry["details"] = sql.RawBytes(details.String)
		}
		out = append(out, entry)
		lastID = id
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{
		"activity": out,
		"next_before_id": func() string {
			if len(out) < limit {
				return ""
			}
			return strconv.FormatInt(lastID, 10)
		}(),
	}, nil)
}
