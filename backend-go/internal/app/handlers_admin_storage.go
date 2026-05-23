package app

// Admin storage breakdown.
//
//   GET /api/v2/admin/storage/by-user           → paginated list of users
//                                                 ordered by used_bytes DESC
//   GET /api/v2/admin/storage/top-files?n=50    → top-N largest files
//                                                 across all users
//
// These queries scan tables that are typically ≪ a hundred thousand rows
// each, so we just do straightforward aggregations. If MyCloud ever scales
// to millions, a materialised view fed by triggers becomes the natural
// next step.

import (
	"net/http"
	"strconv"

	"mycloud/backend-go/internal/httpapi"
)

func (a *App) handleAdminStorageByUser(w http.ResponseWriter, r *http.Request) {
	limit := qInt(r, "limit", 100, 1, 1000)
	offset := qInt(r, "offset", 0, 0, 100000)

	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT u.id, u.email, u.role, u.used_bytes, u.quota_bytes,
		       (SELECT COUNT(*) FROM files f WHERE f.user_id = u.id AND f.is_deleted = 0) AS file_count
		FROM users u
		ORDER BY u.used_bytes DESC
		LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, email, role string
		var used, quota int64
		var fileCount int64
		if err := rows.Scan(&id, &email, &role, &used, &quota, &fileCount); err != nil {
			respondDBError(w, r, err)
			return
		}
		pct := 0.0
		if quota > 0 {
			pct = float64(used) / float64(quota) * 100
		}
		out = append(out, map[string]any{
			"user_id":     id,
			"email":       email,
			"role":        role,
			"used_bytes":  used,
			"quota_bytes": quota,
			"usage_pct":   pct,
			"file_count":  fileCount,
		})
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"users": out, "limit": limit, "offset": offset}, nil)
}

func (a *App) handleAdminStorageTopFiles(w http.ResponseWriter, r *http.Request) {
	n := qInt(r, "n", 50, 1, 500)
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT f.id, f.name, f.size_bytes, f.mime_type, f.user_id, u.email, f.created_at
		FROM files f
		LEFT JOIN users u ON u.id = f.user_id
		WHERE f.is_deleted = 0
		ORDER BY f.size_bytes DESC
		LIMIT ?`, n)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0, n)
	for rows.Next() {
		var id, name, mime, userID, email, createdAt string
		var size int64
		if err := rows.Scan(&id, &name, &size, &mime, &userID, &email, &createdAt); err != nil {
			respondDBError(w, r, err)
			return
		}
		out = append(out, map[string]any{
			"file_id":      id,
			"name":         name,
			"size_bytes":   size,
			"mime_type":    mime,
			"user_id":      userID,
			"owner_email":  email,
			"created_at":   createdAt,
		})
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{
		"files": out,
		"n":     n,
	}, nil)
}

// silence the import lint if strconv unused after future refactors
var _ = strconv.Itoa
