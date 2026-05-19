package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"mycloud/backend-go/internal/httpapi"
)

// SmartQuery is the on-disk JSON DSL describing a saved filter.
type SmartQuery struct {
	Q              string   `json:"q,omitempty"`
	MimePrefix     string   `json:"mime_prefix,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	ModifiedAfter  string   `json:"modified_after,omitempty"`
	ModifiedBefore string   `json:"modified_before,omitempty"`
	Starred        bool     `json:"starred,omitempty"`
	InFolder       string   `json:"in_folder,omitempty"`
}

func (a *App) handleListSmartFolders(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	rows, err := a.DB.QueryContext(r.Context(),
		"SELECT id, name, query_json, color, created_at FROM smart_folders WHERE user_id=? ORDER BY name ASC",
		userID)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, q, createdAt string
		var color sql.NullString
		if err := rows.Scan(&id, &name, &q, &color, &createdAt); err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		var query SmartQuery
		_ = json.Unmarshal([]byte(q), &query)
		item := map[string]any{
			"id":         id,
			"name":       name,
			"query":      query,
			"created_at": createdAt,
		}
		if color.Valid {
			item["color"] = color.String
		}
		out = append(out, item)
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"smart_folders": out}, nil)
}

func (a *App) handleCreateSmartFolder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	var payload struct {
		Name  string     `json:"name"`
		Query SmartQuery `json:"query"`
		Color string     `json:"color"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.Name == "" {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Name is required")
		return
	}
	id := uuid.NewString()
	queryJSON, _ := json.Marshal(payload.Query)
	var colorArg any
	if payload.Color != "" {
		colorArg = payload.Color
	}
	if _, err := a.DB.ExecContext(r.Context(),
		"INSERT INTO smart_folders (id, user_id, name, query_json, color) VALUES (?, ?, ?, ?, ?)",
		id, userID, payload.Name, string(queryJSON), colorArg); err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			httpapi.Error(w, http.StatusConflict, "duplicate", "A smart folder with this name already exists")
			return
		}
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	httpapi.JSON(w, http.StatusCreated, map[string]any{"id": id}, nil)
}

func (a *App) handleUpdateSmartFolder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	var payload struct {
		Name  *string     `json:"name"`
		Query *SmartQuery `json:"query"`
		Color *string     `json:"color"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	sets := make([]string, 0, 3)
	args := make([]any, 0, 5)
	if payload.Name != nil {
		sets = append(sets, "name=?")
		args = append(args, *payload.Name)
	}
	if payload.Query != nil {
		queryJSON, _ := json.Marshal(*payload.Query)
		sets = append(sets, "query_json=?")
		args = append(args, string(queryJSON))
	}
	if payload.Color != nil {
		sets = append(sets, "color=?")
		args = append(args, *payload.Color)
	}
	if len(sets) == 0 {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Nothing to update")
		return
	}
	args = append(args, id, userID)
	res, err := a.DB.ExecContext(r.Context(),
		"UPDATE smart_folders SET "+strings.Join(sets, ", ")+" WHERE id=? AND user_id=?", args...)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Smart folder not found")
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"message": "Updated"}, nil)
}

func (a *App) handleDeleteSmartFolder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	res, err := a.DB.ExecContext(r.Context(),
		"DELETE FROM smart_folders WHERE id=? AND user_id=?", id, userID)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Smart folder not found")
		return
	}
	httpapi.NoContent(w)
}

// handleSmartFolderResults runs the saved query and returns matching files.
func (a *App) handleSmartFolderResults(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	var name, queryJSON string
	if err := a.DB.QueryRowContext(r.Context(),
		"SELECT name, query_json FROM smart_folders WHERE id=? AND user_id=?",
		id, userID,
	).Scan(&name, &queryJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "Smart folder not found")
			return
		}
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	var q SmartQuery
	if err := json.Unmarshal([]byte(queryJSON), &q); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "decode_error", err.Error())
		return
	}

	// Build the SQL.
	conditions := []string{"f.user_id=?", "f.is_deleted=0"}
	args := []any{userID}
	joins := ""
	if q.MimePrefix != "" {
		conditions = append(conditions, "f.mime_type LIKE ?")
		args = append(args, q.MimePrefix+"%")
	}
	if q.Starred {
		conditions = append(conditions, "f.is_starred=1")
	}
	if q.InFolder != "" {
		conditions = append(conditions, "f.folder_id=?")
		args = append(args, q.InFolder)
	}
	if q.ModifiedAfter != "" {
		conditions = append(conditions, "f.updated_at >= ?")
		args = append(args, q.ModifiedAfter)
	}
	if q.ModifiedBefore != "" {
		conditions = append(conditions, "f.updated_at <= ?")
		args = append(args, q.ModifiedBefore)
	}
	if q.Q != "" {
		// Use FULLTEXT on name when long enough; LIKE as fallback.
		if len(q.Q) >= 3 {
			conditions = append(conditions, "MATCH(f.name) AGAINST(? IN NATURAL LANGUAGE MODE)")
			args = append(args, q.Q)
		} else {
			conditions = append(conditions, "f.name LIKE ?")
			args = append(args, "%"+q.Q+"%")
		}
	}
	if len(q.Tags) > 0 {
		joins += " JOIN file_tags ft ON ft.file_id = f.id"
		ph, tagArgs := inClause(q.Tags)
		conditions = append(conditions, "ft.tag_id IN "+ph)
		args = append(args, tagArgs...)
		// Require ALL tags (not ANY): GROUP BY + HAVING COUNT
	}

	query := "SELECT DISTINCT f.id, f.name, f.size_bytes, f.mime_type, f.folder_id, f.is_starred, f.created_at, f.updated_at FROM files f"
	query += joins
	query += " WHERE " + strings.Join(conditions, " AND ")
	if len(q.Tags) > 0 {
		query += fmt.Sprintf(" GROUP BY f.id HAVING COUNT(DISTINCT ft.tag_id) = %d", len(q.Tags))
	}
	query += " ORDER BY f.updated_at DESC LIMIT 500"

	rows, err := a.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()
	files := make([]map[string]any, 0)
	for rows.Next() {
		var id, fname, mime, createdAt, updatedAt string
		var size int64
		var folder sql.NullString
		var starred bool
		if err := rows.Scan(&id, &fname, &size, &mime, &folder, &starred, &createdAt, &updatedAt); err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		item := map[string]any{
			"id":         id,
			"name":       fname,
			"size_bytes": size,
			"mime_type":  mime,
			"is_starred": starred,
			"created_at": createdAt,
			"updated_at": updatedAt,
		}
		if folder.Valid {
			item["folder_id"] = folder.String
		}
		files = append(files, item)
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"files": files, "name": name}, nil)
}
