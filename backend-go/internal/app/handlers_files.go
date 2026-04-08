package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"mycloud/backend-go/internal/httpapi"
)

func (a *App) handleListFiles(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	folderID := r.URL.Query().Get("folder_id")
	sort := r.URL.Query().Get("sort")
	order := strings.ToUpper(r.URL.Query().Get("order"))
	all := r.URL.Query().Get("all") == "1"
	starred := r.URL.Query().Get("starred_only") == "1"
	page := qInt(r, "page", 1, 1, 100000)
	pageSize := qInt(r, "page_size", 50, 1, 200)
	offset := (page - 1) * pageSize
	allowedSort := map[string]string{
		"name":       "name",
		"size_bytes": "size_bytes",
		"created_at": "created_at",
		"updated_at": "updated_at",
	}
	if allowedSort[sort] == "" {
		sort = "name"
	}
	if order != "DESC" {
		order = "ASC"
	}
	where := " WHERE user_id=? AND is_deleted=0"
	args := []any{userID}
	if starred {
		where += " AND is_starred=1"
	}
	if !all {
		if folderID == "" {
			where += " AND folder_id IS NULL"
		} else {
			where += " AND folder_id=?"
			args = append(args, folderID)
		}
	}
	var total int
	if err := a.DB.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM files"+where, args...).Scan(&total); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	rows, err := a.DB.QueryContext(r.Context(),
		"SELECT id, name, size_bytes, mime_type, folder_id, is_starred, created_at, updated_at FROM files"+where+
			" ORDER BY "+allowedSort[sort]+" "+order+" LIMIT ? OFFSET ?", append(args, pageSize, offset)...)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()
	files := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, mimeType, createdAt, updatedAt string
		var sizeBytes int64
		var folder sql.NullString
		var starred bool
		if err := rows.Scan(&id, &name, &sizeBytes, &mimeType, &folder, &starred, &createdAt, &updatedAt); err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		item := map[string]any{
			"id":         id,
			"name":       name,
			"size_bytes": sizeBytes,
			"mime_type":  mimeType,
			"is_starred": starred,
			"created_at": createdAt,
			"updated_at": updatedAt,
		}
		if folder.Valid {
			item["folder_id"] = folder.String
		}
		files = append(files, item)
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"files": files}, map[string]any{
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"has_more":  page*pageSize < total,
	})
}

func (a *App) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	folderID, filePart, err := a.parseUpload(r)
	if err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	filename := filePart.FileName()
	if filename == "" {
		filename = "untitled"
	}
	data, err := a.storeUploadedFile(r.Context(), userID, filename, folderID, filePart)
	if err != nil {
		status := http.StatusInternalServerError
		code := "upload_failed"
		if strings.Contains(err.Error(), "quota") {
			status = http.StatusRequestEntityTooLarge
			code = "quota_exceeded"
		} else if strings.Contains(err.Error(), "folder not found") {
			status = http.StatusBadRequest
			code = "invalid_parent"
		}
		httpapi.Error(w, status, code, err.Error())
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "file_upload", "file", data["id"].(string), clientIP(r), map[string]any{"name": filename})
	httpapi.JSON(w, http.StatusCreated, data, nil)
}

func (a *App) handleStorageStats(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	var used, quota, filesCount, folderCount int64
	if err := a.DB.QueryRowContext(r.Context(), "SELECT used_bytes, quota_bytes FROM users WHERE id=?", userID).Scan(&used, &quota); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	_ = a.DB.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM files WHERE user_id=? AND is_deleted=0", userID).Scan(&filesCount)
	_ = a.DB.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM folders WHERE user_id=? AND is_deleted=0", userID).Scan(&folderCount)
	httpapi.JSON(w, http.StatusOK, map[string]any{
		"used_bytes":   used,
		"quota_bytes":  quota,
		"file_count":   filesCount,
		"folder_count": folderCount,
	}, nil)
}

func (a *App) handleFileInfo(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	var fileID, name, mimeType, createdAt, updatedAt string
	var sizeBytes int64
	var folder sql.NullString
	var starred, deleted bool
	err := a.DB.QueryRowContext(r.Context(), `
		SELECT id, name, size_bytes, mime_type, folder_id, is_starred, is_deleted, created_at, updated_at
		FROM files WHERE id=? AND user_id=?`, id, userID).
		Scan(&fileID, &name, &sizeBytes, &mimeType, &folder, &starred, &deleted, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
		return
	}
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	data := map[string]any{"id": fileID, "name": name, "size_bytes": sizeBytes, "mime_type": mimeType, "is_starred": starred, "is_deleted": deleted, "created_at": createdAt, "updated_at": updatedAt}
	if folder.Valid {
		data["folder_id"] = folder.String
	}
	httpapi.JSON(w, http.StatusOK, data, nil)
}

func (a *App) handleUpdateFile(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	var payload map[string]json.RawMessage
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	if len(payload) == 0 {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Nothing to update")
		return
	}
	switch {
	case payload["is_starred"] != nil:
		var isStarred bool
		if err := json.Unmarshal(payload["is_starred"], &isStarred); err != nil {
			httpapi.Error(w, http.StatusBadRequest, "validation_error", "is_starred must be a boolean")
			return
		}
		res, err := a.DB.ExecContext(r.Context(), "UPDATE files SET is_starred=? WHERE id=? AND user_id=? AND is_deleted=0", isStarred, id, userID)
		if err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		if affected, _ := res.RowsAffected(); affected == 0 {
			httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
			return
		}
	case payload["name"] != nil:
		var name string
		if err := json.Unmarshal(payload["name"], &name); err != nil {
			httpapi.Error(w, http.StatusBadRequest, "validation_error", "name must be a string")
			return
		}
		name = strings.TrimSpace(name)
		if name == "" {
			httpapi.Error(w, http.StatusBadRequest, "validation_error", "File name is required")
			return
		}
		res, err := a.DB.ExecContext(r.Context(), "UPDATE files SET name=? WHERE id=? AND user_id=? AND is_deleted=0", name, id, userID)
		if err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		if affected, _ := res.RowsAffected(); affected == 0 {
			httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
			return
		}
	case payload["folder_id"] != nil:
		var folderID *string
		if string(payload["folder_id"]) != "null" {
			var raw string
			if err := json.Unmarshal(payload["folder_id"], &raw); err != nil {
				httpapi.Error(w, http.StatusBadRequest, "validation_error", "folder_id must be a string or null")
				return
			}
			raw = strings.TrimSpace(raw)
			if raw != "" {
				var exists int
				if err := a.DB.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM folders WHERE id=? AND user_id=? AND is_deleted=0", raw, userID).Scan(&exists); err != nil {
					httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
					return
				}
				if exists == 0 {
					httpapi.Error(w, http.StatusBadRequest, "invalid_parent", "Folder not found")
					return
				}
				folderID = &raw
			}
		}
		res, err := a.DB.ExecContext(r.Context(), "UPDATE files SET folder_id=? WHERE id=? AND user_id=? AND is_deleted=0", nullableStringPtr(folderID), id, userID)
		if err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		if affected, _ := res.RowsAffected(); affected == 0 {
			httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
			return
		}
	default:
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Nothing to update")
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"message": "Updated"}, nil)
}

func (a *App) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	res, err := a.DB.ExecContext(r.Context(), "UPDATE files SET is_deleted=1, deleted_at=NOW() WHERE id=? AND user_id=? AND is_deleted=0", id, userID)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "file_delete", "file", id, clientIP(r), nil)
	httpapi.NoContent(w)
}

func (a *App) handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	a.serveFile(w, r, chi.URLParam(r, "id"), "attachment")
}

func (a *App) handlePreviewFile(w http.ResponseWriter, r *http.Request) {
	a.serveFile(w, r, chi.URLParam(r, "id"), "inline")
}
