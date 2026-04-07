package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/iii123iii/mycloud/backend/internal/auth"
	"github.com/iii123iii/mycloud/backend/internal/utils"
)

// FoldersHandler handles /api/folders/* routes.
type FoldersHandler struct {
	db *sql.DB
}

func NewFoldersHandler(db *sql.DB) *FoldersHandler {
	return &FoldersHandler{db: db}
}

// ListFolders handles GET /api/folders
func (h *FoldersHandler) ListFolders(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	parentID := r.URL.Query().Get("parent_id")

	where := "FROM folders WHERE user_id=? AND is_deleted=0"
	args := []any{userID}
	if parentID == "" {
		where += " AND parent_id IS NULL"
	} else {
		where += " AND parent_id=?"
		args = append(args, parentID)
	}

	var total int64
	if err := h.db.QueryRowContext(r.Context(), "SELECT COUNT(*) AS total "+where, args...).Scan(&total); err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	rows, err := h.db.QueryContext(r.Context(),
		"SELECT id,name,parent_id,created_at,updated_at "+where+" ORDER BY name ASC", args...)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type folderItem struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		ParentID  *string `json:"parent_id,omitempty"`
		CreatedAt string  `json:"created_at"`
		UpdatedAt *string `json:"updated_at,omitempty"`
	}
	folders := []folderItem{}
	for rows.Next() {
		var f folderItem
		var parentIDNull sql.NullString
		var updatedAtNull sql.NullString
		if err := rows.Scan(&f.ID, &f.Name, &parentIDNull, &f.CreatedAt, &updatedAtNull); err != nil {
			utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		if parentIDNull.Valid {
			f.ParentID = &parentIDNull.String
		}
		if updatedAtNull.Valid {
			f.UpdatedAt = &updatedAtNull.String
		}
		folders = append(folders, f)
	}

	utils.OkJSON(w, map[string]any{
		"folders": folders,
		"total":   total,
	})
}

// CreateFolder handles POST /api/folders
func (h *FoldersHandler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	var body struct {
		Name     string  `json:"name"`
		ParentID *string `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.ErrorJSON(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if body.Name == "" {
		utils.ErrorJSON(w, http.StatusBadRequest, "name required")
		return
	}

	id := utils.GenerateUUID()
	var parentIDArg any
	if body.ParentID != nil && *body.ParentID != "" {
		parentIDArg = *body.ParentID
	}

	_, err := h.db.ExecContext(r.Context(),
		"INSERT INTO folders (id,name,user_id,parent_id) VALUES (?,?,?,?)",
		id, body.Name, userID, parentIDArg)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	utils.CreatedJSON(w, map[string]any{"id": id, "name": body.Name})
}

// GetFolder handles GET /api/folders/{id}
func (h *FoldersHandler) GetFolder(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	folderID := chi.URLParam(r, "id")

	var (
		id        string
		name      string
		parentID  sql.NullString
		createdAt string
		updatedAt sql.NullString
	)
	err := h.db.QueryRowContext(r.Context(),
		"SELECT id,name,parent_id,created_at,updated_at FROM folders WHERE id=? AND user_id=? AND is_deleted=0",
		folderID, userID).Scan(&id, &name, &parentID, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		utils.ErrorJSON(w, http.StatusNotFound, "Folder not found")
		return
	}
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]any{
		"id":         id,
		"name":       name,
		"created_at": createdAt,
	}
	if parentID.Valid {
		resp["parent_id"] = parentID.String
	}
	if updatedAt.Valid {
		resp["updated_at"] = updatedAt.String
	}
	utils.OkJSON(w, resp)
}

// UpdateFolder handles PATCH /api/folders/{id}
func (h *FoldersHandler) UpdateFolder(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	folderID := chi.URLParam(r, "id")

	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.ErrorJSON(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if rawName, ok := body["name"]; ok {
		var name string
		if err := json.Unmarshal(rawName, &name); err != nil || name == "" {
			utils.ErrorJSON(w, http.StatusBadRequest, "Invalid name")
			return
		}
		_, err := h.db.ExecContext(r.Context(),
			"UPDATE folders SET name=? WHERE id=? AND user_id=? AND is_deleted=0",
			name, folderID, userID)
		if err != nil {
			utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		utils.OkJSON(w, map[string]string{"message": "Updated"})
		return
	}

	if rawParent, ok := body["parent_id"]; ok {
		if string(rawParent) == "null" {
			_, err := h.db.ExecContext(r.Context(),
				"UPDATE folders SET parent_id=NULL WHERE id=? AND user_id=? AND is_deleted=0",
				folderID, userID)
			if err != nil {
				utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
				return
			}
		} else {
			var parentID string
			if err := json.Unmarshal(rawParent, &parentID); err != nil {
				utils.ErrorJSON(w, http.StatusBadRequest, "Invalid parent_id")
				return
			}
			_, err := h.db.ExecContext(r.Context(),
				"UPDATE folders SET parent_id=? WHERE id=? AND user_id=? AND is_deleted=0",
				parentID, folderID, userID)
			if err != nil {
				utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		utils.OkJSON(w, map[string]string{"message": "Updated"})
		return
	}

	utils.ErrorJSON(w, http.StatusBadRequest, "Nothing to update")
}

// DeleteFolder handles DELETE /api/folders/{id}
func (h *FoldersHandler) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	folderID := chi.URLParam(r, "id")

	res, err := h.db.ExecContext(r.Context(),
		"UPDATE folders SET is_deleted=1, deleted_at=NOW() WHERE id=? AND user_id=? AND is_deleted=0",
		folderID, userID)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		utils.ErrorJSON(w, http.StatusNotFound, "Folder not found")
		return
	}

	// Also soft-delete all children recursively
	_, _ = h.db.ExecContext(r.Context(),
		"UPDATE folders SET is_deleted=1, deleted_at=NOW() WHERE user_id=? AND is_deleted=0 AND "+
			"id IN (SELECT id FROM (SELECT id FROM folders WHERE user_id=? AND parent_id=? AND is_deleted=0) AS sub)",
		userID, userID, folderID)

	utils.NoContent(w)
}
