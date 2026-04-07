package app

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"mycloud/backend-go/internal/httpapi"
	"mycloud/backend-go/internal/storage"
)

func detectMime(filename string) string {
	if ext := filepath.Ext(filename); ext != "" {
		if kind := mime.TypeByExtension(ext); kind != "" {
			return kind
		}
	}
	return "application/octet-stream"
}

func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer("\"", "", "\\", "", "\r", "", "\n", "", "\x00", "")
	name = replacer.Replace(name)
	if name == "" {
		return "download"
	}
	return name
}

func (a *App) parseUpload(r *http.Request) (string, *multipart.Part, error) {
	reader, err := r.MultipartReader()
	if err != nil {
		return "", nil, err
	}
	var folderID string
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", nil, err
		}
		switch part.FormName() {
		case "folder_id":
			raw, _ := io.ReadAll(io.LimitReader(part, 256))
			folderID = strings.TrimSpace(string(raw))
			part.Close()
		case "file":
			return folderID, part, nil
		default:
			part.Close()
		}
	}
	return folderID, nil, fmt.Errorf("no file part")
}

func (a *App) storeUploadedFile(ctx context.Context, userID, filename, folderID string, filePart *multipart.Part) (map[string]any, error) {
	fileID := uuid.NewString()
	fileKey, err := storage.GenerateFileKey()
	if err != nil {
		return nil, err
	}
	bundle, err := storage.WrapKey(a.Config.MasterEncryptionKey, fileKey)
	if err != nil {
		return nil, err
	}
	tmpPath := storage.TempPath(a.Config.StoragePath, fileID)
	if err := os.MkdirAll(filepath.Dir(tmpPath), 0o755); err != nil {
		return nil, err
	}
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return nil, err
	}
	size, encErr := storage.EncryptStream(filePart, tmpFile, fileKey)
	closeErr := tmpFile.Close()
	filePart.Close()
	if encErr != nil {
		_ = os.Remove(tmpPath)
		return nil, encErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return nil, closeErr
	}

	finalDir, err := storage.EnsureUserDir(a.Config.StoragePath, userID)
	if err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	finalPath := filepath.Join(finalDir, fileID+".enc")
	mimeType := detectMime(filename)

	tx, err := a.DB.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	defer tx.Rollback()

	if folderID != "" {
		var exists int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM folders WHERE id=? AND user_id=? AND is_deleted=0", folderID, userID).Scan(&exists); err != nil || exists == 0 {
			_ = os.Remove(tmpPath)
			return nil, fmt.Errorf("folder not found")
		}
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE users
		SET used_bytes = used_bytes + ?
		WHERE id=? AND used_bytes + ? <= quota_bytes`, size, userID, size)
	if err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("storage quota exceeded")
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO files (id, name, storage_path, size_bytes, mime_type, user_id, folder_id, encryption_key_enc, encryption_iv, encryption_tag, is_deleted, is_starred)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0)`,
		fileID, filename, finalPath, size, mimeType, userID, nullableString(folderID), bundle.EncKeyHex, bundle.IVHex, bundle.TagHex); err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		_ = os.Remove(finalPath)
		return nil, err
	}
	return map[string]any{
		"id":         fileID,
		"name":       filename,
		"folder_id":  nullableString(folderID),
		"size_bytes": size,
		"mime_type":  mimeType,
	}, nil
}

func (a *App) serveFile(w http.ResponseWriter, r *http.Request, fileID, disposition string) {
	userID := userIDFrom(r)
	row, err := a.DB.QueryContext(r.Context(), `
		SELECT user_id, name, mime_type, encryption_key_enc, encryption_iv, encryption_tag, storage_path
		FROM files WHERE id=? AND user_id=? AND is_deleted=0`, fileID, userID)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer row.Close()
	if !row.Next() {
		httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
		return
	}
	var ownerID, name, mimeType, encKey, iv, tag, path string
	if err := row.Scan(&ownerID, &name, &mimeType, &encKey, &iv, &tag, &path); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	fileKey, err := storage.UnwrapKey(a.Config.MasterEncryptionKey, storage.EncryptedKeyBundle{
		IVHex:     iv,
		EncKeyHex: encKey,
		TagHex:    tag,
	})
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "crypto_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=\"%s\"", disposition, sanitizeFilename(name)))
	if err := storage.DecryptFileToWriter(path, w, fileKey); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "stream_error", err.Error())
		return
	}
}

func (a *App) removeEncryptedFile(path string) {
	if path == "" {
		return
	}
	_ = os.Remove(path)
}
