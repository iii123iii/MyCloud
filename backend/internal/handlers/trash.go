package handlers

import (
	"database/sql"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/iii123iii/mycloud/backend/internal/auth"
	"github.com/iii123iii/mycloud/backend/internal/services"
	"github.com/iii123iii/mycloud/backend/internal/utils"
)

// TrashHandler handles /api/trash/* routes.
type TrashHandler struct {
	db         *sql.DB
	storageSvc *services.StorageService
}

func NewTrashHandler(db *sql.DB, storageSvc *services.StorageService) *TrashHandler {
	return &TrashHandler{db: db, storageSvc: storageSvc}
}

// ListTrash handles GET /api/trash
func (h *TrashHandler) ListTrash(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	q := r.URL.Query()
	page := safeInt(q.Get("page"), 1, 1, 100000)
	pageSize := safeInt(q.Get("page_size"), 50, 1, 200)
	offset := (page - 1) * pageSize

	query := `
SELECT 'file' AS type, id, name, size_bytes, mime_type, deleted_at FROM files
WHERE user_id=? AND is_deleted=1
UNION ALL
SELECT 'folder' AS type, id, name, 0, '', deleted_at FROM folders
WHERE user_id=? AND is_deleted=1
ORDER BY deleted_at DESC
LIMIT ? OFFSET ?`

	rows, err := h.db.QueryContext(r.Context(), query, userID, userID, pageSize, offset)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type trashItem struct {
		Type      string  `json:"type"`
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		SizeBytes *int64  `json:"size_bytes,omitempty"`
		MimeType  *string `json:"mime_type,omitempty"`
		DeletedAt string  `json:"deleted_at"`
	}
	items := []trashItem{}
	for rows.Next() {
		var item trashItem
		var sizeBytes int64
		var mimeType string
		if err := rows.Scan(&item.Type, &item.ID, &item.Name, &sizeBytes, &mimeType, &item.DeletedAt); err != nil {
			utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		if item.Type == "file" {
			item.SizeBytes = &sizeBytes
			item.MimeType = &mimeType
		}
		items = append(items, item)
	}
	utils.OkJSON(w, map[string]any{"items": items})
}

// RestoreItem handles POST /api/trash/{id}/restore
func (h *TrashHandler) RestoreItem(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	itemID := chi.URLParam(r, "id")

	// Try file first
	res, err := h.db.ExecContext(r.Context(),
		"UPDATE files SET is_deleted=0, deleted_at=NULL WHERE id=? AND user_id=?",
		itemID, userID)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		utils.OkJSON(w, map[string]string{"message": "Restored"})
		return
	}

	// Try folder
	res, err = h.db.ExecContext(r.Context(),
		"UPDATE folders SET is_deleted=0, deleted_at=NULL WHERE id=? AND user_id=?",
		itemID, userID)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ = res.RowsAffected()
	if n == 0 {
		utils.ErrorJSON(w, http.StatusNotFound, "Item not found in trash")
		return
	}
	utils.OkJSON(w, map[string]string{"message": "Restored"})
}

// DeleteItem handles DELETE /api/trash/{id}
func (h *TrashHandler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	itemID := chi.URLParam(r, "id")

	// Try file
	var sizeBytes int64
	err := h.db.QueryRowContext(r.Context(),
		"SELECT size_bytes FROM files WHERE id=? AND user_id=? AND is_deleted=1",
		itemID, userID).Scan(&sizeBytes)
	if err == nil {
		h.storageSvc.DeleteFile(userID, itemID)
		if _, err := h.db.ExecContext(r.Context(),
			"DELETE FROM files WHERE id=? AND user_id=?", itemID, userID); err != nil {
			utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		_, _ = h.db.ExecContext(r.Context(),
			"UPDATE users SET used_bytes=GREATEST(0, used_bytes-?) WHERE id=?", sizeBytes, userID)
		utils.NoContent(w)
		return
	}
	if err != sql.ErrNoRows {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Try folder (delete all files inside, then folder)
	res, err := h.db.ExecContext(r.Context(),
		"DELETE FROM folders WHERE id=? AND user_id=? AND is_deleted=1", itemID, userID)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		utils.ErrorJSON(w, http.StatusNotFound, "Item not found in trash")
		return
	}
	utils.NoContent(w)
}

// EmptyTrash handles DELETE /api/trash/empty
func (h *TrashHandler) EmptyTrash(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())

	// Collect deleted files
	rows, err := h.db.QueryContext(r.Context(),
		"SELECT id, size_bytes FROM files WHERE user_id=? AND is_deleted=1", userID)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var totalSize int64
	for rows.Next() {
		var fileID string
		var sz int64
		if err := rows.Scan(&fileID, &sz); err == nil {
			h.storageSvc.DeleteFile(userID, fileID)
			totalSize += sz
		}
	}
	rows.Close()

	if _, err := h.db.ExecContext(r.Context(),
		"DELETE FROM files WHERE user_id=? AND is_deleted=1", userID); err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, _ = h.db.ExecContext(r.Context(),
		"UPDATE users SET used_bytes=GREATEST(0, used_bytes-?) WHERE id=?", totalSize, userID)
	_, _ = h.db.ExecContext(r.Context(),
		"DELETE FROM folders WHERE user_id=? AND is_deleted=1", userID)

	utils.NoContent(w)
}
