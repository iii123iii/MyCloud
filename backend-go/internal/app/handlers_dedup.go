package app

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"mycloud/backend-go/internal/httpapi"
)

// handleHasFileHash answers HEAD /api/v2/files/by-hash?h=<sha256> with 200 if
// the caller already owns any file with this content hash, 404 otherwise.
// Used by the desktop client to skip uploads when the server already has the
// bytes.
func (a *App) handleHasFileHash(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	hash := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("h")))
	if len(hash) != 64 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var n int
	err := a.DB.QueryRowContext(r.Context(),
		"SELECT COUNT(*) FROM files WHERE user_id=? AND content_sha256=? AND is_deleted=0",
		userID, hash).Scan(&n)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if n == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleCreateFromHash POST /api/v2/files/by-hash creates a new files row
// referencing the blob of an existing file with the same content hash. Used
// by the desktop client after HEAD returns 200, to register a new logical
// file (different name / folder) without uploading bytes.
func (a *App) handleCreateFromHash(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	var payload struct {
		Hash     string  `json:"hash"`
		Name     string  `json:"name"`
		FolderID *string `json:"folder_id"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	payload.Hash = strings.ToLower(strings.TrimSpace(payload.Hash))
	payload.Name = strings.TrimSpace(payload.Name)
	if len(payload.Hash) != 64 || payload.Name == "" {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "hash and name are required")
		return
	}
	if payload.FolderID != nil && *payload.FolderID != "" {
		if err := a.canAccessFolder(r.Context(), userID, *payload.FolderID, AccessEditor); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Error(w, http.StatusBadRequest, "invalid_parent", "Folder not found")
				return
			}
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
	}

	// Find a donor file for the hash.
	var storagePath, key, iv, tag, mime string
	var size int64
	err := a.DB.QueryRowContext(r.Context(), `
		SELECT storage_path, encryption_key_enc, encryption_iv, encryption_tag, mime_type, size_bytes
		FROM files WHERE user_id=? AND content_sha256=? AND is_deleted=0 LIMIT 1`,
		userID, payload.Hash,
	).Scan(&storagePath, &key, &iv, &tag, &mime, &size)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Error(w, http.StatusNotFound, "not_found", "No file with this hash")
		return
	}
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := reserveQuota(r.Context(), tx, userID, size); err != nil {
		if errors.Is(err, ErrQuotaExceeded) {
			httpapi.Error(w, http.StatusRequestEntityTooLarge, "quota_exceeded", err.Error())
			return
		}
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	id := uuid.NewString()
	folderArg := any(nil)
	if payload.FolderID != nil && *payload.FolderID != "" {
		folderArg = *payload.FolderID
	}
	useMime := mime
	if useMime == "" {
		useMime = detectMime(payload.Name)
	}
	if _, err := tx.ExecContext(r.Context(), `
		INSERT INTO files (id, name, storage_path, size_bytes, mime_type, user_id, folder_id,
		                   encryption_key_enc, encryption_iv, encryption_tag,
		                   content_sha256, is_deleted, is_starred)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0)`,
		id, payload.Name, storagePath, size, useMime, userID, folderArg,
		key, iv, tag, payload.Hash); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if err := acquireBlobRef(r.Context(), tx, storagePath); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	committed = true
	writeActivity(r.Context(), a.DB, &userID, "file.upload", "file", id, clientIP(r), map[string]any{
		"name":     payload.Name,
		"size":     size,
		"deduped":  true,
		"via":      "by-hash",
	})
	httpapi.JSON(w, http.StatusCreated, map[string]any{
		"id":         id,
		"name":       payload.Name,
		"size_bytes": size,
		"mime_type":  useMime,
		"deduped":    true,
	}, nil)
}
