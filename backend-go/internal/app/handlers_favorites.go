package app

// Per-user ordered favourites.
//
//   GET /api/v2/me/favorites          → ordered list { folder_id, name, position }
//   PUT /api/v2/me/favorites          → { folder_ids: [...] }  // full reorder
//   POST /api/v2/me/favorites         → { folder_id }          // add at end
//   DELETE /api/v2/me/favorites/{id}  → remove
//
// PUT replaces the whole list — simpler than computing per-row position
// deltas and works fine for the typical "a dozen favourites" case.

import (
	"database/sql"
	"net/http"

	"github.com/go-chi/chi/v5"

	"mycloud/backend-go/internal/httpapi"
)

func (a *App) handleListFavorites(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT uf.folder_id, f.name, uf.position
		FROM user_favorites uf
		JOIN folders f ON f.id = uf.folder_id AND f.is_deleted = 0
		WHERE uf.user_id = ?
		ORDER BY uf.position ASC, uf.created_at ASC`, userID)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var folderID, name string
		var position int
		if err := rows.Scan(&folderID, &name, &position); err != nil {
			respondDBError(w, r, err)
			return
		}
		out = append(out, map[string]any{
			"folder_id": folderID,
			"name":      name,
			"position":  position,
		})
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"favorites": out}, nil)
}

func (a *App) handleAddFavorite(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	var body struct {
		FolderID string `json:"folder_id"`
	}
	if err := decodeJSON(r, &body); err != nil || body.FolderID == "" {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "folder_id required")
		return
	}
	// Verify the user actually has access to this folder before pinning it
	// — otherwise the sidebar could leak the existence of folders the user
	// can't open.
	if _, err := a.canAccessFolder(r.Context(), userID, body.FolderID, AccessViewer); err != nil {
		httpapi.Error(w, http.StatusForbidden, "forbidden", "no folder access")
		return
	}
	var maxPos sql.NullInt64
	_ = a.DB.QueryRowContext(r.Context(),
		`SELECT MAX(position) FROM user_favorites WHERE user_id = ?`, userID,
	).Scan(&maxPos)
	pos := int64(0)
	if maxPos.Valid {
		pos = maxPos.Int64 + 1
	}
	if _, err := a.DB.ExecContext(r.Context(), `
		INSERT INTO user_favorites (user_id, folder_id, position)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE position = VALUES(position)`,
		userID, body.FolderID, pos); err != nil {
		respondDBError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleRemoveFavorite(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	folderID := chi.URLParam(r, "folderID")
	if _, err := a.DB.ExecContext(r.Context(),
		`DELETE FROM user_favorites WHERE user_id = ? AND folder_id = ?`,
		userID, folderID); err != nil {
		respondDBError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleReorderFavorites(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	var body struct {
		FolderIDs []string `json:"folder_ids"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if len(body.FolderIDs) == 0 {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "folder_ids must be non-empty")
		return
	}

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	defer tx.Rollback()
	// Wipe + reinsert. Cheap for ~dozen rows and avoids per-row UPDATE
	// permutation logic.
	if _, err := tx.ExecContext(r.Context(),
		`DELETE FROM user_favorites WHERE user_id = ?`, userID); err != nil {
		respondDBError(w, r, err)
		return
	}
	for i, fid := range body.FolderIDs {
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO user_favorites (user_id, folder_id, position)
			VALUES (?, ?, ?)`, userID, fid, i); err != nil {
			respondDBError(w, r, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		respondDBError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
