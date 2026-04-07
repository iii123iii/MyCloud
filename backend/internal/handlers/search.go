package handlers

import (
	"database/sql"
	"net/http"

	"github.com/iii123iii/mycloud/backend/internal/auth"
	"github.com/iii123iii/mycloud/backend/internal/utils"
)

// SearchHandler handles /api/search.
type SearchHandler struct {
	db *sql.DB
}

func NewSearchHandler(db *sql.DB) *SearchHandler {
	return &SearchHandler{db: db}
}

// Search handles GET /api/search?q=...
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	q := r.URL.Query().Get("q")
	if q == "" {
		utils.OkJSON(w, map[string]any{"files": []any{}, "folders": []any{}})
		return
	}
	pattern := "%" + q + "%"

	// Search files
	fileRows, err := h.db.QueryContext(r.Context(),
		"SELECT id,name,size_bytes,mime_type,folder_id,is_starred,created_at,updated_at "+
			"FROM files WHERE user_id=? AND is_deleted=0 AND name LIKE ? LIMIT 50",
		userID, pattern)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer fileRows.Close()

	type fileItem struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		SizeBytes int64   `json:"size_bytes"`
		MimeType  string  `json:"mime_type"`
		FolderID  *string `json:"folder_id,omitempty"`
		IsStarred bool    `json:"is_starred"`
		CreatedAt string  `json:"created_at"`
		UpdatedAt string  `json:"updated_at"`
	}
	files := []fileItem{}
	for fileRows.Next() {
		var f fileItem
		var folderID sql.NullString
		if err := fileRows.Scan(&f.ID, &f.Name, &f.SizeBytes, &f.MimeType, &folderID, &f.IsStarred, &f.CreatedAt, &f.UpdatedAt); err != nil {
			continue
		}
		if folderID.Valid {
			f.FolderID = &folderID.String
		}
		files = append(files, f)
	}

	// Search folders
	folderRows, err := h.db.QueryContext(r.Context(),
		"SELECT id,name,parent_id,created_at FROM folders WHERE user_id=? AND is_deleted=0 AND name LIKE ? LIMIT 50",
		userID, pattern)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer folderRows.Close()

	type folderItem struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		ParentID  *string `json:"parent_id,omitempty"`
		CreatedAt string  `json:"created_at"`
	}
	folders := []folderItem{}
	for folderRows.Next() {
		var f folderItem
		var parentID sql.NullString
		if err := folderRows.Scan(&f.ID, &f.Name, &parentID, &f.CreatedAt); err != nil {
			continue
		}
		if parentID.Valid {
			f.ParentID = &parentID.String
		}
		folders = append(folders, f)
	}

	utils.OkJSON(w, map[string]any{
		"files":   files,
		"folders": folders,
	})
}
