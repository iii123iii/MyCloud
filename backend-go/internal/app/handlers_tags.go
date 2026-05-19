package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"mycloud/backend-go/internal/httpapi"
)

var colorRE = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (a *App) handleListTags(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id, name, color, created_at FROM tags WHERE user_id=? ORDER BY name ASC`, userID)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, color, createdAt string
		if err := rows.Scan(&id, &name, &color, &createdAt); err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		out = append(out, map[string]any{
			"id":         id,
			"name":       name,
			"color":      color,
			"created_at": createdAt,
		})
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"tags": out}, nil)
}

func (a *App) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	var payload struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.Name == "" {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Tag name is required")
		return
	}
	if len(payload.Name) > 64 {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Tag name is too long")
		return
	}
	if payload.Color == "" {
		payload.Color = "#6b7280"
	}
	if !colorRE.MatchString(payload.Color) {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Color must be a #RRGGBB hex value")
		return
	}
	id := uuid.NewString()
	if _, err := a.DB.ExecContext(r.Context(),
		"INSERT INTO tags (id, user_id, name, color) VALUES (?, ?, ?, ?)",
		id, userID, payload.Name, payload.Color); err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			httpapi.Error(w, http.StatusConflict, "duplicate", "A tag with this name already exists")
			return
		}
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "tag.create", "tag", id, clientIP(r),
		map[string]any{"name": payload.Name})
	httpapi.JSON(w, http.StatusCreated, map[string]any{
		"id": id, "name": payload.Name, "color": payload.Color,
	}, nil)
}

func (a *App) handleUpdateTag(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	var payload struct {
		Name  *string `json:"name"`
		Color *string `json:"color"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	sets := make([]string, 0, 2)
	args := make([]any, 0, 4)
	if payload.Name != nil {
		name := strings.TrimSpace(*payload.Name)
		if name == "" || len(name) > 64 {
			httpapi.Error(w, http.StatusBadRequest, "validation_error", "Invalid tag name")
			return
		}
		sets = append(sets, "name=?")
		args = append(args, name)
	}
	if payload.Color != nil {
		if !colorRE.MatchString(*payload.Color) {
			httpapi.Error(w, http.StatusBadRequest, "validation_error", "Color must be a #RRGGBB hex value")
			return
		}
		sets = append(sets, "color=?")
		args = append(args, *payload.Color)
	}
	if len(sets) == 0 {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Nothing to update")
		return
	}
	args = append(args, id, userID)
	res, err := a.DB.ExecContext(r.Context(),
		"UPDATE tags SET "+strings.Join(sets, ", ")+" WHERE id=? AND user_id=?", args...)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Tag not found")
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"message": "Updated"}, nil)
}

func (a *App) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	res, err := a.DB.ExecContext(r.Context(),
		"DELETE FROM tags WHERE id=? AND user_id=?", id, userID)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Tag not found")
		return
	}
	httpapi.NoContent(w)
}

func (a *App) handleAttachTagToFile(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	fileID := chi.URLParam(r, "id")
	tagID := chi.URLParam(r, "tagId")
	if _, err := a.canAccessFile(r.Context(), userID, fileID, AccessEditor); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
			return
		}
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if err := a.ensureOwnTag(r.Context(), userID, tagID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "Tag not found")
			return
		}
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if _, err := a.DB.ExecContext(r.Context(),
		"INSERT IGNORE INTO file_tags (file_id, tag_id) VALUES (?, ?)", fileID, tagID); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "tag.attach", "file", fileID, clientIP(r),
		map[string]any{"tag_id": tagID})
	httpapi.NoContent(w)
}

func (a *App) handleDetachTagFromFile(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	fileID := chi.URLParam(r, "id")
	tagID := chi.URLParam(r, "tagId")
	if _, err := a.canAccessFile(r.Context(), userID, fileID, AccessEditor); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
			return
		}
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if _, err := a.DB.ExecContext(r.Context(),
		"DELETE FROM file_tags WHERE file_id=? AND tag_id=?", fileID, tagID); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "tag.detach", "file", fileID, clientIP(r),
		map[string]any{"tag_id": tagID})
	httpapi.NoContent(w)
}

func (a *App) handleAttachTagToFolder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	folderID := chi.URLParam(r, "id")
	tagID := chi.URLParam(r, "tagId")
	if err := a.canAccessFolder(r.Context(), userID, folderID, AccessEditor); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "Folder not found")
			return
		}
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if err := a.ensureOwnTag(r.Context(), userID, tagID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "Tag not found")
			return
		}
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if _, err := a.DB.ExecContext(r.Context(),
		"INSERT IGNORE INTO folder_tags (folder_id, tag_id) VALUES (?, ?)", folderID, tagID); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "tag.attach", "folder", folderID, clientIP(r),
		map[string]any{"tag_id": tagID})
	httpapi.NoContent(w)
}

func (a *App) handleDetachTagFromFolder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	folderID := chi.URLParam(r, "id")
	tagID := chi.URLParam(r, "tagId")
	if err := a.canAccessFolder(r.Context(), userID, folderID, AccessEditor); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "Folder not found")
			return
		}
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if _, err := a.DB.ExecContext(r.Context(),
		"DELETE FROM folder_tags WHERE folder_id=? AND tag_id=?", folderID, tagID); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "tag.detach", "folder", folderID, clientIP(r),
		map[string]any{"tag_id": tagID})
	httpapi.NoContent(w)
}

func (a *App) ensureOwnTag(ctx context.Context, userID, tagID string) error {
	var owner string
	err := a.DB.QueryRowContext(ctx, "SELECT user_id FROM tags WHERE id=?", tagID).Scan(&owner)
	if err != nil {
		return err
	}
	if owner != userID {
		return sql.ErrNoRows
	}
	return nil
}
