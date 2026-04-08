package app

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"mycloud/backend-go/internal/auth"
	"mycloud/backend-go/internal/httpapi"
	"mycloud/backend-go/internal/storage"
)

func randomToken() string {
	buf := make([]byte, 24)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func (a *App) handleListShares(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT s.id, s.token, s.permission, s.expires_at, s.created_at, s.file_id, f.name, s.folder_id
		FROM shares s
		LEFT JOIN files f ON f.id = s.file_id
		WHERE s.created_by=?
		ORDER BY s.created_at DESC`, userID)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()
	shares := make([]map[string]any, 0)
	for rows.Next() {
		var id, token, permission, createdAt string
		var expiresAt, fileID, fileName, folderID sql.NullString
		if err := rows.Scan(&id, &token, &permission, &expiresAt, &createdAt, &fileID, &fileName, &folderID); err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		item := map[string]any{"id": id, "token": token, "permission": permission, "created_at": createdAt}
		if expiresAt.Valid {
			item["expires_at"] = expiresAt.String
		}
		if fileID.Valid {
			item["file_id"] = fileID.String
		}
		if fileName.Valid {
			item["file_name"] = fileName.String
		}
		if folderID.Valid {
			item["folder_id"] = folderID.String
		}
		shares = append(shares, item)
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"shares": shares}, nil)
}

func (a *App) handleCreateShare(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	var payload struct {
		FileID     *string `json:"file_id"`
		FolderID   *string `json:"folder_id"`
		Permission string  `json:"permission"`
		Password   string  `json:"password"`
		ExpiresAt  *string `json:"expires_at"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	if payload.FileID == nil && payload.FolderID == nil {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "file_id or folder_id is required")
		return
	}
	if payload.FileID != nil && payload.FolderID != nil {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "share exactly one resource")
		return
	}
	if payload.Permission == "" {
		payload.Permission = "read"
	}
	if payload.Permission != "read" && payload.Permission != "write" {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "invalid permission")
		return
	}
	var passwordHash interface{}
	if strings.TrimSpace(payload.Password) != "" {
		hash, err := auth.HashPassword(payload.Password)
		if err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "hash_error", err.Error())
			return
		}
		passwordHash = hash
	}
	switch {
	case payload.FileID != nil:
		var exists int
		if err := a.DB.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM files WHERE id=? AND user_id=? AND is_deleted=0", *payload.FileID, userID).Scan(&exists); err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		if exists == 0 {
			httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
			return
		}
	case payload.FolderID != nil:
		var exists int
		if err := a.DB.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM folders WHERE id=? AND user_id=? AND is_deleted=0", *payload.FolderID, userID).Scan(&exists); err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		if exists == 0 {
			httpapi.Error(w, http.StatusNotFound, "not_found", "Folder not found")
			return
		}
	}
	id := uuid.NewString()
	token := randomToken()
	if _, err := a.DB.ExecContext(r.Context(), `
		INSERT INTO shares (id, token, file_id, folder_id, created_by, permission, password_hash, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, token, payload.FileID, payload.FolderID, userID, payload.Permission, passwordHash, payload.ExpiresAt); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	httpapi.JSON(w, http.StatusCreated, map[string]any{"token": token, "url": "/s/" + token}, nil)
}

func (a *App) handleDeleteShare(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	if _, err := a.DB.ExecContext(r.Context(), "DELETE FROM shares WHERE id=? AND created_by=?", id, userID); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	httpapi.NoContent(w)
}

func (a *App) loadShare(r *http.Request, token string) (map[string]any, *storage.EncryptedKeyBundle, string, error) {
	row := a.DB.QueryRowContext(r.Context(), `
		SELECT s.permission, s.password_hash, s.expires_at, s.file_id, f.name, f.size_bytes, f.mime_type, f.user_id, f.storage_path, f.encryption_key_enc, f.encryption_iv, f.encryption_tag, s.folder_id
		FROM shares s
		LEFT JOIN files f ON f.id = s.file_id AND f.is_deleted=0
		LEFT JOIN folders d ON d.id = s.folder_id
		WHERE s.token=?
		  AND (s.expires_at IS NULL OR s.expires_at > NOW())
		  AND (s.folder_id IS NULL OR d.is_deleted=0)`, token)
	var permission string
	var passwordHash, expiresAt, fileID, fileName, mimeType, ownerID, storagePath, encKey, iv, tag, folderID sql.NullString
	var sizeBytes sql.NullInt64
	if err := row.Scan(&permission, &passwordHash, &expiresAt, &fileID, &fileName, &sizeBytes, &mimeType, &ownerID, &storagePath, &encKey, &iv, &tag, &folderID); err != nil {
		return nil, nil, "", err
	}
	if passwordHash.Valid {
		sharePassword := r.Header.Get("X-Share-Password")
		if auth.ComparePassword(passwordHash.String, sharePassword) != nil {
			return nil, nil, "", errors.New("share password required")
		}
	}
	data := map[string]any{"permission": permission}
	if fileID.Valid {
		data["file_id"] = fileID.String
		data["file_name"] = fileName.String
		data["file_size"] = sizeBytes.Int64
		data["mime_type"] = mimeType.String
	}
	if folderID.Valid {
		data["folder_id"] = folderID.String
	}
	if fileID.Valid && !storagePath.Valid {
		return nil, nil, "", sql.ErrNoRows
	}
	if !fileID.Valid {
		return data, nil, "", nil
	}
	bundle := &storage.EncryptedKeyBundle{IVHex: iv.String, EncKeyHex: encKey.String, TagHex: tag.String}
	return data, bundle, storagePath.String, nil
}

func (a *App) handleResolveShare(w http.ResponseWriter, r *http.Request) {
	data, _, _, err := a.loadShare(r, chi.URLParam(r, "token"))
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Share not found")
		return
	}
	if err != nil {
		status := http.StatusInternalServerError
		code := "share_error"
		if strings.Contains(err.Error(), "password") {
			status = http.StatusUnauthorized
			code = "share_password_required"
		}
		httpapi.Error(w, status, code, err.Error())
		return
	}
	httpapi.JSON(w, http.StatusOK, data, nil)
}

func (a *App) handleDownloadShare(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	data, bundle, path, err := a.loadShare(r, token)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Share not found")
		return
	}
	if err != nil {
		status := http.StatusInternalServerError
		code := "share_error"
		if strings.Contains(err.Error(), "password") {
			status = http.StatusUnauthorized
			code = "share_password_required"
		}
		httpapi.Error(w, status, code, err.Error())
		return
	}
	if bundle == nil {
		httpapi.Error(w, http.StatusBadRequest, "unsupported", "Folder shares cannot be downloaded directly")
		return
	}
	fileKey, err := storage.UnwrapKey(a.Config.MasterEncryptionKey, *bundle)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "crypto_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", data["mime_type"].(string))
	w.Header().Set("Content-Disposition", "attachment; filename=\""+sanitizeFilename(data["file_name"].(string))+"\"")
	if err := storage.DecryptFileToWriter(path, w, fileKey); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "stream_error", err.Error())
		return
	}
}
