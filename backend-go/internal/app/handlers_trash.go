package app

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"mycloud/backend-go/internal/httpapi"
)

func (a *App) handleListTrash(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT 'file' AS item_type, id, name, size_bytes, mime_type, deleted_at
		FROM files WHERE user_id=? AND is_deleted=1
		UNION ALL
		SELECT 'folder' AS item_type, id, name, NULL, NULL, deleted_at
		FROM folders WHERE user_id=? AND is_deleted=1
		ORDER BY deleted_at DESC`, userID, userID)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var itemType, id, name, deletedAt string
		var size sql.NullInt64
		var mime sql.NullString
		if err := rows.Scan(&itemType, &id, &name, &size, &mime, &deletedAt); err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		item := map[string]any{"type": itemType, "id": id, "name": name, "deleted_at": deletedAt}
		if size.Valid {
			item["size_bytes"] = size.Int64
		}
		if mime.Valid {
			item["mime_type"] = mime.String
		}
		items = append(items, item)
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"items": items}, nil)
}

func (a *App) handleRestoreTrash(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	res, err := a.DB.ExecContext(r.Context(), "UPDATE files SET is_deleted=0, deleted_at=NULL WHERE id=? AND user_id=? AND is_deleted=1", id, userID)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	affected, _ := res.RowsAffected()
	if affected > 0 {
		httpapi.JSON(w, http.StatusOK, map[string]any{"message": "Restored"}, nil)
		return
	}
	var exists int
	if err := a.DB.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM folders WHERE id=? AND user_id=? AND is_deleted=1", id, userID).Scan(&exists); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if exists == 0 {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Trash item not found")
		return
	}
	if err := a.restoreFolder(r.Context(), userID, id); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "restore_failed", err.Error())
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"message": "Restored"}, nil)
}

func (a *App) handleDeleteTrashItem(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	if err := a.permanentlyDeleteFile(r.Context(), userID, id); err == nil {
		httpapi.NoContent(w)
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Error(w, http.StatusInternalServerError, "delete_failed", err.Error())
		return
	}
	if err := a.permanentlyDeleteFolder(r.Context(), userID, id); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "delete_failed", err.Error())
		return
	}
	httpapi.NoContent(w)
}

func (a *App) handleEmptyTrash(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	fileRows, err := a.DB.QueryContext(r.Context(), "SELECT id FROM files WHERE user_id=? AND is_deleted=1", userID)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer fileRows.Close()
	var fileIDs []string
	for fileRows.Next() {
		var id string
		if err := fileRows.Scan(&id); err == nil {
			fileIDs = append(fileIDs, id)
		}
	}
	for _, id := range fileIDs {
		if err := a.permanentlyDeleteFile(r.Context(), userID, id); err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "delete_failed", err.Error())
			return
		}
	}
	folderRows, err := a.DB.QueryContext(r.Context(), "SELECT id FROM folders WHERE user_id=? AND is_deleted=1 AND (parent_id IS NULL OR parent_id NOT IN (SELECT id FROM folders WHERE user_id=? AND is_deleted=1))", userID, userID)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer folderRows.Close()
	var folderIDs []string
	for folderRows.Next() {
		var id string
		if err := folderRows.Scan(&id); err == nil {
			folderIDs = append(folderIDs, id)
		}
	}
	for _, id := range folderIDs {
		if err := a.permanentlyDeleteFolder(r.Context(), userID, id); err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "delete_failed", err.Error())
			return
		}
	}
	httpapi.NoContent(w)
}
