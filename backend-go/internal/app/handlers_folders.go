package app

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"mycloud/backend-go/internal/httpapi"
)

func (a *App) handleListFolders(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	parentID := r.URL.Query().Get("parent_id")
	query := `
		SELECT id, name, parent_id, created_at, updated_at
		FROM folders
		WHERE user_id=? AND is_deleted=0`
	args := []any{userID}
	if parentID == "" {
		query += " AND parent_id IS NULL"
	} else {
		query += " AND parent_id=?"
		args = append(args, parentID)
	}
	query += " ORDER BY name ASC"

	rows, err := a.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()
	folders := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, createdAt, updatedAt string
		var parent sql.NullString
		if err := rows.Scan(&id, &name, &parent, &createdAt, &updatedAt); err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		item := map[string]any{
			"id":         id,
			"name":       name,
			"created_at": createdAt,
			"updated_at": updatedAt,
		}
		if parent.Valid {
			item["parent_id"] = parent.String
		}
		folders = append(folders, item)
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"folders": folders}, nil)
}

func (a *App) handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Name     string  `json:"name"`
		ParentID *string `json:"parent_id"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.Name == "" {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Folder name is required")
		return
	}
	userID := userIDFrom(r)
	id := uuid.NewString()
	if payload.ParentID != nil && *payload.ParentID != "" {
		var exists int
		if err := a.DB.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM folders WHERE id=? AND user_id=? AND is_deleted=0", *payload.ParentID, userID).Scan(&exists); err != nil || exists == 0 {
			httpapi.Error(w, http.StatusBadRequest, "invalid_parent", "Parent folder not found")
			return
		}
	}
	_, err := a.DB.ExecContext(r.Context(),
		"INSERT INTO folders (id, name, user_id, parent_id, is_deleted) VALUES (?, ?, ?, ?, 0)",
		id, payload.Name, userID, payload.ParentID)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "folder_create", "folder", id, clientIP(r), map[string]any{"name": payload.Name})
	httpapi.JSON(w, http.StatusCreated, map[string]any{"id": id, "name": payload.Name, "parent_id": payload.ParentID}, nil)
}

func (a *App) handleGetFolder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	var folderID, name, createdAt, updatedAt string
	var parent sql.NullString
	err := a.DB.QueryRowContext(r.Context(), `
		SELECT id, name, parent_id, created_at, updated_at
		FROM folders WHERE id=? AND user_id=?`, id, userID).
		Scan(&folderID, &name, &parent, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Folder not found")
		return
	}
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	data := map[string]any{"id": folderID, "name": name, "created_at": createdAt, "updated_at": updatedAt}
	if parent.Valid {
		data["parent_id"] = parent.String
	}
	httpapi.JSON(w, http.StatusOK, data, nil)
}

func (a *App) handleUpdateFolder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	var payload struct {
		Name     *string `json:"name"`
		ParentID *string `json:"parent_id"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	switch {
	case payload.Name != nil:
		name := strings.TrimSpace(*payload.Name)
		if name == "" {
			httpapi.Error(w, http.StatusBadRequest, "validation_error", "Folder name is required")
			return
		}
		_, err := a.DB.ExecContext(r.Context(), "UPDATE folders SET name=? WHERE id=? AND user_id=? AND is_deleted=0", name, id, userID)
		if err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
	case payload.ParentID != nil:
		_, err := a.DB.ExecContext(r.Context(), "UPDATE folders SET parent_id=? WHERE id=? AND user_id=? AND is_deleted=0", payload.ParentID, id, userID)
		if err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
	default:
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Nothing to update")
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"message": "Updated"}, nil)
}

func (a *App) handleDeleteFolder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	if err := a.markFolderDeleted(r.Context(), userID, id); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "delete_failed", err.Error())
		return
	}
	httpapi.NoContent(w)
}
