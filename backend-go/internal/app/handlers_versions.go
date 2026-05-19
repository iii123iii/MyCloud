package app

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"mycloud/backend-go/internal/httpapi"
	"mycloud/backend-go/internal/storage"
)

// handleListVersions returns the version history for a file (newest first).
// Viewer access required.
func (a *App) handleListVersions(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	fileID := chi.URLParam(r, "id")
	if _, err := a.canAccessFile(r.Context(), userID, fileID, AccessViewer); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
			return
		}
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT v.version_no, v.size_bytes, v.created_at, v.created_by, u.username
		FROM file_versions v
		LEFT JOIN users u ON u.id = v.created_by
		WHERE v.file_id = ?
		ORDER BY v.version_no DESC`, fileID)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var versionNo int
		var size int64
		var createdAt, createdBy string
		var username sql.NullString
		if err := rows.Scan(&versionNo, &size, &createdAt, &createdBy, &username); err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		item := map[string]any{
			"version_no": versionNo,
			"size_bytes": size,
			"created_at": createdAt,
			"created_by": createdBy,
		}
		if username.Valid {
			item["username"] = username.String
		}
		out = append(out, item)
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"versions": out}, nil)
}

// handleRestoreVersion makes the named historical version the current.
// The previously-current contents become a NEW version (so restore is reversible).
// Editor access required.
func (a *App) handleRestoreVersion(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	fileID := chi.URLParam(r, "id")
	versionNoStr := chi.URLParam(r, "vno")
	versionNo, err := strconv.Atoi(versionNoStr)
	if err != nil || versionNo < 1 {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Invalid version number")
		return
	}
	if _, err := a.canAccessFile(r.Context(), userID, fileID, AccessEditor); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
			return
		}
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

	// Current row data.
	var ownerID, curKey, curIV, curTag, curPath string
	var curSize int64
	if err := tx.QueryRowContext(r.Context(), `
		SELECT user_id, storage_path, size_bytes, encryption_key_enc, encryption_iv, encryption_tag
		FROM files WHERE id = ? AND is_deleted = 0`, fileID,
	).Scan(&ownerID, &curPath, &curSize, &curKey, &curIV, &curTag); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	// Target version data.
	var verPath, verKey, verIV, verTag string
	var verSize int64
	if err := tx.QueryRowContext(r.Context(), `
		SELECT storage_path, size_bytes, encryption_key_enc, encryption_iv, encryption_tag
		FROM file_versions WHERE file_id = ? AND version_no = ?`, fileID, versionNo,
	).Scan(&verPath, &verSize, &verKey, &verIV, &verTag); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "Version not found")
			return
		}
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	// Snapshot the current into a fresh version_no slot, then swap blobs on disk.
	var maxNo sql.NullInt64
	if err := tx.QueryRowContext(r.Context(),
		"SELECT COALESCE(MAX(version_no), 0) FROM file_versions WHERE file_id = ?", fileID,
	).Scan(&maxNo); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	newVersionNo := int(maxNo.Int64) + 1
	newVersionPath := storage.VersionPath(a.Config.StoragePath, ownerID, fileID, newVersionNo)
	snapshotID := uuid.NewString()

	if _, err := tx.ExecContext(r.Context(), `
		INSERT INTO file_versions (id, file_id, version_no, storage_path, size_bytes,
		                           encryption_key_enc, encryption_iv, encryption_tag, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snapshotID, fileID, newVersionNo, newVersionPath, curSize,
		curKey, curIV, curTag, userID); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	// Delete the version row we're restoring (its blob becomes the new current).
	if _, err := tx.ExecContext(r.Context(),
		"DELETE FROM file_versions WHERE file_id = ? AND version_no = ?",
		fileID, versionNo); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	// Update files row to point at the restored content.
	if _, err := tx.ExecContext(r.Context(), `
		UPDATE files
		SET size_bytes = ?, encryption_key_enc = ?, encryption_iv = ?, encryption_tag = ?
		WHERE id = ?`,
		verSize, verKey, verIV, verTag, fileID); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	// Three-step swap via a temp filename so a partial failure never leaves
	// both blobs pointing at the same path:
	//   curPath  → tmpSwap     (move current out of the way)
	//   verPath  → curPath     (target version becomes current)
	//   tmpSwap  → newVersionPath  (former current becomes a new version)
	tmpSwap := curPath + ".restore-swap"
	if err := os.Rename(curPath, tmpSwap); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "fs_error", err.Error())
		return
	}
	if err := os.Rename(verPath, curPath); err != nil {
		_ = os.Rename(tmpSwap, curPath) // restore original current
		httpapi.Error(w, http.StatusInternalServerError, "fs_error", err.Error())
		return
	}
	if err := os.Rename(tmpSwap, newVersionPath); err != nil {
		// Roll back both prior renames so the DB row still matches disk.
		_ = os.Rename(curPath, verPath)
		_ = os.Rename(tmpSwap, curPath)
		httpapi.Error(w, http.StatusInternalServerError, "fs_error", err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
		// Best-effort full unwind on commit failure.
		_ = os.Rename(newVersionPath, tmpSwap)
		_ = os.Rename(curPath, verPath)
		_ = os.Rename(tmpSwap, curPath)
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	committed = true
	writeActivity(r.Context(), a.DB, &userID, "version.restore", "file", fileID, clientIP(r),
		map[string]any{"version_no": versionNo})
	httpapi.JSON(w, http.StatusOK, map[string]any{"message": "Restored", "now_current": versionNo}, nil)
}

// handleDownloadVersion streams a specific historical version as an
// attachment. Viewer access required.
func (a *App) handleDownloadVersion(w http.ResponseWriter, r *http.Request) {
	a.streamVersion(w, r, "attachment")
}

// handlePreviewVersion streams a historical version with an inline disposition
// so the browser renders it (image, PDF, etc.) rather than downloading.
// Viewer access required.
func (a *App) handlePreviewVersion(w http.ResponseWriter, r *http.Request) {
	a.streamVersion(w, r, "inline")
}

// streamVersion is the shared implementation behind the download and preview
// endpoints for file_versions. `disposition` is "attachment" or "inline".
func (a *App) streamVersion(w http.ResponseWriter, r *http.Request, disposition string) {
	userID := userIDFrom(r)
	fileID := chi.URLParam(r, "id")
	versionNo, err := strconv.Atoi(chi.URLParam(r, "vno"))
	if err != nil || versionNo < 1 {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Invalid version number")
		return
	}
	if _, err := a.canAccessFile(r.Context(), userID, fileID, AccessViewer); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
			return
		}
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	var name, mimeType, encKey, iv, tag, path string
	err = a.DB.QueryRowContext(r.Context(), `
		SELECT f.name, f.mime_type, v.encryption_key_enc, v.encryption_iv, v.encryption_tag, v.storage_path
		FROM file_versions v JOIN files f ON f.id = v.file_id
		WHERE v.file_id = ? AND v.version_no = ?`, fileID, versionNo,
	).Scan(&name, &mimeType, &encKey, &iv, &tag, &path)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Version not found")
		return
	}
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	fileKey, err := storage.UnwrapKey(a.Config.MasterEncryptionKey, storage.EncryptedKeyBundle{
		IVHex: iv, EncKeyHex: encKey, TagHex: tag,
	})
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "crypto_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", disposition+"; filename=\""+sanitizeFilename(name)+"\"")
	if err := storage.DecryptFileToWriter(path, w, fileKey); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "stream_error", err.Error())
		return
	}
}
