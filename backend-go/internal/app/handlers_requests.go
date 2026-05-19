package app

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"mycloud/backend-go/internal/auth"
	"mycloud/backend-go/internal/httpapi"
)

// ─── Authenticated CRUD ──────────────────────────────────────────────────────

func (a *App) handleListUploadRequests(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT ur.id, ur.token, ur.folder_id, fo.name, ur.expires_at, ur.max_files, ur.used_files, ur.created_at,
		       ur.password_hash IS NOT NULL
		FROM upload_requests ur
		LEFT JOIN folders fo ON fo.id = ur.folder_id
		WHERE ur.created_by = ?
		ORDER BY ur.created_at DESC`, userID)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, token, createdAt string
		var folderID, folderName, expiresAt sql.NullString
		var maxFiles sql.NullInt64
		var usedFiles int64
		var hasPassword bool
		if err := rows.Scan(&id, &token, &folderID, &folderName, &expiresAt, &maxFiles, &usedFiles, &createdAt, &hasPassword); err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		item := map[string]any{
			"id":           id,
			"token":        token,
			"used_files":   usedFiles,
			"created_at":   createdAt,
			"has_password": hasPassword,
			"url":          "/u/" + token,
		}
		if folderID.Valid {
			item["folder_id"] = folderID.String
			item["folder_name"] = folderName.String
		}
		if expiresAt.Valid {
			item["expires_at"] = expiresAt.String
		}
		if maxFiles.Valid {
			item["max_files"] = maxFiles.Int64
		}
		out = append(out, item)
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"requests": out}, nil)
}

func (a *App) handleCreateUploadRequest(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	var payload struct {
		FolderID  *string `json:"folder_id"`
		ExpiresAt *string `json:"expires_at"`
		MaxFiles  *int    `json:"max_files"`
		Password  string  `json:"password"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	if payload.FolderID != nil && *payload.FolderID != "" {
		if err := a.canAccessFolder(r.Context(), userID, *payload.FolderID, AccessEditor); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Error(w, http.StatusNotFound, "not_found", "Folder not found")
				return
			}
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
	}
	if payload.MaxFiles != nil && *payload.MaxFiles < 1 {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "max_files must be at least 1")
		return
	}
	var passwordHash any
	if strings.TrimSpace(payload.Password) != "" {
		hash, err := auth.HashPassword(payload.Password)
		if err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "hash_error", err.Error())
			return
		}
		passwordHash = hash
	}
	id := uuid.NewString()
	token := randomToken()
	if _, err := a.DB.ExecContext(r.Context(), `
		INSERT INTO upload_requests (id, token, folder_id, created_by, expires_at, max_files, password_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, token, payload.FolderID, userID, payload.ExpiresAt, payload.MaxFiles, passwordHash,
	); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "request.create", "upload_request", id, clientIP(r), nil)
	httpapi.JSON(w, http.StatusCreated, map[string]any{"id": id, "token": token, "url": "/u/" + token}, nil)
}

func (a *App) handleDeleteUploadRequest(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	res, err := a.DB.ExecContext(r.Context(),
		"DELETE FROM upload_requests WHERE id=? AND created_by=?", id, userID)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Request not found")
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "request.delete", "upload_request", id, clientIP(r), nil)
	httpapi.NoContent(w)
}

// ─── Public endpoints ────────────────────────────────────────────────────────

// uploadRequestRow describes a resolved request after token + (optional) password validation.
type uploadRequestRow struct {
	ID         string
	FolderID   sql.NullString
	CreatedBy  string
	ExpiresAt  sql.NullString
	MaxFiles   sql.NullInt64
	UsedFiles  int64
	FolderName sql.NullString
}

func (a *App) loadUploadRequest(r *http.Request, token string) (*uploadRequestRow, error) {
	row := a.DB.QueryRowContext(r.Context(), `
		SELECT ur.id, ur.folder_id, ur.created_by, ur.expires_at, ur.max_files, ur.used_files,
		       ur.password_hash, fo.name
		FROM upload_requests ur
		LEFT JOIN folders fo ON fo.id = ur.folder_id
		WHERE ur.token = ?`, token)
	var rec uploadRequestRow
	var passwordHash sql.NullString
	if err := row.Scan(&rec.ID, &rec.FolderID, &rec.CreatedBy, &rec.ExpiresAt,
		&rec.MaxFiles, &rec.UsedFiles, &passwordHash, &rec.FolderName); err != nil {
		return nil, err
	}
	// Limit / expiry checks first so a wrong password on an exhausted link
	// returns the more meaningful 410.
	if rec.MaxFiles.Valid && rec.UsedFiles >= rec.MaxFiles.Int64 {
		return nil, ErrShareGone
	}
	if rec.ExpiresAt.Valid {
		if t, err := time.Parse("2006-01-02 15:04:05", rec.ExpiresAt.String); err == nil && time.Now().UTC().After(t) {
			return nil, ErrShareGone
		}
	}
	if passwordHash.Valid {
		given := r.Header.Get("X-Share-Password")
		if auth.ComparePassword(passwordHash.String, given) != nil {
			return nil, errors.New("share password required")
		}
	}
	return &rec, nil
}

func (a *App) handleResolveUploadRequest(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	rec, err := a.loadUploadRequest(r, token)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Upload link not found")
		return
	}
	if errors.Is(err, ErrShareGone) {
		httpapi.Error(w, http.StatusGone, "share_gone", "Upload link has expired or reached its limit")
		return
	}
	if err != nil {
		if strings.Contains(err.Error(), "password") {
			httpapi.Error(w, http.StatusUnauthorized, "share_password_required", err.Error())
			return
		}
		httpapi.Error(w, http.StatusInternalServerError, "share_error", err.Error())
		return
	}
	data := map[string]any{"token": token, "used_files": rec.UsedFiles}
	if rec.MaxFiles.Valid {
		data["max_files"] = rec.MaxFiles.Int64
		data["uploads_remaining"] = rec.MaxFiles.Int64 - rec.UsedFiles
	}
	if rec.ExpiresAt.Valid {
		data["expires_at"] = rec.ExpiresAt.String
	}
	if rec.FolderName.Valid {
		data["folder_name"] = rec.FolderName.String
	}
	httpapi.JSON(w, http.StatusOK, data, nil)
}

func (a *App) handlePublicUploadToRequest(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	rec, err := a.loadUploadRequest(r, token)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Upload link not found")
		return
	}
	if errors.Is(err, ErrShareGone) {
		httpapi.Error(w, http.StatusGone, "share_gone", "Upload link has expired or reached its limit")
		return
	}
	if err != nil {
		if strings.Contains(err.Error(), "password") {
			httpapi.Error(w, http.StatusUnauthorized, "share_password_required", err.Error())
			return
		}
		httpapi.Error(w, http.StatusInternalServerError, "share_error", err.Error())
		return
	}

	// Atomic limit reservation — wins the race against parallel uploads at the
	// last unit of capacity. If 0 rows, capacity is exhausted.
	res, err := a.DB.ExecContext(r.Context(), `
		UPDATE upload_requests
		SET used_files = used_files + 1
		WHERE token = ?
		  AND (expires_at IS NULL OR expires_at > NOW())
		  AND (max_files IS NULL OR used_files < max_files)`, token)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpapi.Error(w, http.StatusGone, "share_gone", "Upload link has reached its limit")
		return
	}

	folderID := ""
	if rec.FolderID.Valid {
		folderID = rec.FolderID.String
	}
	parsedFolderID, filePart, err := a.parseUpload(r)
	if err != nil {
		// roll back the reservation
		_, _ = a.DB.ExecContext(r.Context(),
			"UPDATE upload_requests SET used_files = used_files - 1 WHERE token = ?", token)
		httpapi.Error(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	// Uploaders cannot override the folder; ignore any value they sent.
	_ = parsedFolderID
	filename := filePart.FileName()
	if filename == "" {
		filename = "untitled"
	}
	// Owner of the resulting file is the request creator, NOT the anonymous uploader.
	data, err := a.storeUploadedFile(r.Context(), rec.CreatedBy, filename, folderID, filePart)
	if err != nil {
		_, _ = a.DB.ExecContext(r.Context(),
			"UPDATE upload_requests SET used_files = used_files - 1 WHERE token = ?", token)
		_, _ = io.Copy(io.Discard, r.Body) // drain
		if errors.Is(err, ErrQuotaExceeded) {
			httpapi.Error(w, http.StatusRequestEntityTooLarge, "quota_exceeded", err.Error())
			return
		}
		httpapi.Error(w, http.StatusInternalServerError, "upload_failed", err.Error())
		return
	}
	creator := rec.CreatedBy
	writeActivity(r.Context(), a.DB, &creator, "file.upload",
		"file", data["id"].(string), clientIP(r),
		map[string]any{"via_request": rec.ID, "name": filename})
	httpapi.JSON(w, http.StatusCreated, map[string]any{
		"id":   data["id"],
		"name": filename,
	}, nil)
}
