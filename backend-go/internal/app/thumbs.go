package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/go-chi/chi/v5"
	"github.com/rwcarlsen/goexif/exif"

	"mycloud/backend-go/internal/httpapi"
	"mycloud/backend-go/internal/storage"
)

const thumbMaxEdge = 256

// processThumb generates a thumbnail for the file at fileID and extracts EXIF
// metadata. The thumbnail is encrypted with its own per-file key. Called by
// the q:thumb worker after a successful image upload.
func (a *App) processThumb(ctx context.Context, fileID string) error {
	var mimeType, encKey, iv, tag, blobPath, userID string
	err := a.DB.QueryRowContext(ctx, `
		SELECT mime_type, encryption_key_enc, encryption_iv, encryption_tag, storage_path, user_id
		FROM files WHERE id = ? AND is_deleted = 0`, fileID,
	).Scan(&mimeType, &encKey, &iv, &tag, &blobPath, &userID)
	if err != nil {
		return fmt.Errorf("load file %s: %w", fileID, err)
	}
	if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return nil
	}

	fileKey, err := storage.UnwrapKey(a.Config.MasterEncryptionKey, storage.EncryptedKeyBundle{
		IVHex: iv, EncKeyHex: encKey, TagHex: tag,
	})
	if err != nil {
		return fmt.Errorf("unwrap key: %w", err)
	}

	// Decrypt the full image to a buffer (typical photo uploads — a few MB).
	var plainBuf bytes.Buffer
	if err := storage.DecryptFileToWriter(blobPath, &plainBuf, fileKey); err != nil {
		return fmt.Errorf("decrypt: %w", err)
	}
	plain := plainBuf.Bytes()

	// EXIF (best effort).
	exifJSON := ""
	var takenAt sql.NullTime
	if x, err := exif.Decode(bytes.NewReader(plain)); err == nil {
		if rawJSON, err := x.MarshalJSON(); err == nil {
			exifJSON = string(rawJSON)
		}
		if t, err := x.DateTime(); err == nil {
			takenAt = sql.NullTime{Time: t, Valid: true}
		}
	}

	// Decode the image.
	img, _, err := image.Decode(bytes.NewReader(plain))
	if err != nil {
		return fmt.Errorf("decode image: %w", err)
	}
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()

	// Resize keeping aspect ratio (longest edge thumbMaxEdge px).
	resized := imaging.Fit(img, thumbMaxEdge, thumbMaxEdge, imaging.Lanczos)

	// Encode as JPEG quality 85.
	var thumbPlain bytes.Buffer
	if err := imaging.Encode(&thumbPlain, resized, imaging.JPEG, imaging.JPEGQuality(85)); err != nil {
		return fmt.Errorf("encode thumb: %w", err)
	}

	// Encrypt thumbnail with a NEW per-file key (different from the main blob).
	// Store the key bundle inline so we can decrypt without the original key.
	thumbKey, err := storage.GenerateFileKey()
	if err != nil {
		return err
	}
	thumbBundle, err := storage.WrapKey(a.Config.MasterEncryptionKey, thumbKey)
	if err != nil {
		return err
	}

	thumbsDir := filepath.Join(a.Config.StoragePath, "thumbs", userID)
	if err := os.MkdirAll(thumbsDir, 0o755); err != nil {
		return err
	}
	thumbName := thumbFilename(fileID)
	thumbPath := filepath.Join(thumbsDir, thumbName)

	out, err := os.Create(thumbPath)
	if err != nil {
		return err
	}
	if _, err := storage.EncryptStream(&thumbPlain, out, thumbKey); err != nil {
		_ = out.Close()
		_ = os.Remove(thumbPath)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}

	// Persist metadata. Encode the thumb's own key bundle into a JSON field on
	// file_exif so we can keep `files` skinny while still being able to decrypt.
	thumbMeta, _ := json.Marshal(map[string]string{
		"thumb_iv":  thumbBundle.IVHex,
		"thumb_key": thumbBundle.EncKeyHex,
		"thumb_tag": thumbBundle.TagHex,
		"exif":      exifJSON,
	})
	if _, err := a.DB.ExecContext(ctx, `
		UPDATE files SET thumb_path=?, width=?, height=?, taken_at=? WHERE id=?`,
		thumbPath, width, height, takenAt, fileID); err != nil {
		return err
	}
	if _, err := a.DB.ExecContext(ctx, `
		INSERT INTO file_exif (file_id, exif_json) VALUES (?, ?)
		ON DUPLICATE KEY UPDATE exif_json = VALUES(exif_json)`,
		fileID, string(thumbMeta)); err != nil {
		return err
	}
	return nil
}

func thumbFilename(fileID string) string {
	// Hash the fileID so different users' files don't accidentally share
	// thumbnails just because the IDs collide (they won't — UUIDs — but the
	// hash also avoids a path that mirrors the encrypted blob layout).
	sum := sha256.Sum256([]byte(fileID))
	salt := make([]byte, 4)
	_, _ = rand.Read(salt)
	return hex.EncodeToString(sum[:8]) + ".thumb.enc"
}

// handleFileThumb streams the decrypted thumbnail for a file.
// Viewer access required. Returns 404 if no thumbnail has been generated yet.
func (a *App) handleFileThumb(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	fileID := chi.URLParam(r, "id")
	if _, err := a.canAccessFile(r.Context(), userID, fileID, AccessViewer); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var thumbPath sql.NullString
	var exifJSON sql.NullString
	if err := a.DB.QueryRowContext(r.Context(), `
		SELECT f.thumb_path, e.exif_json
		FROM files f LEFT JOIN file_exif e ON e.file_id = f.id
		WHERE f.id = ?`, fileID).Scan(&thumbPath, &exifJSON); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !thumbPath.Valid || !exifJSON.Valid {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var meta struct {
		ThumbIV  string `json:"thumb_iv"`
		ThumbKey string `json:"thumb_key"`
		ThumbTag string `json:"thumb_tag"`
	}
	if err := json.Unmarshal([]byte(exifJSON.String), &meta); err != nil {
		http.Error(w, "decode meta", http.StatusInternalServerError)
		return
	}
	key, err := storage.UnwrapKey(a.Config.MasterEncryptionKey, storage.EncryptedKeyBundle{
		IVHex: meta.ThumbIV, EncKeyHex: meta.ThumbKey, TagHex: meta.ThumbTag,
	})
	if err != nil {
		http.Error(w, "key", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	if err := storage.DecryptFileToWriter(thumbPath.String, w, key); err != nil {
		// Already wrote headers — best we can do is truncate the response.
		return
	}
}

// handleListPhotos returns image files for the caller, ordered by taken_at
// (falling back to created_at). Query params: ?from=YYYY-MM-DD&to=YYYY-MM-DD.
func (a *App) handleListPhotos(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	query := `
		SELECT id, name, size_bytes, mime_type, width, height,
		       COALESCE(taken_at, created_at) AS shot_at, created_at
		FROM files
		WHERE user_id = ? AND is_deleted = 0
		  AND mime_type LIKE 'image/%'`
	args := []any{userID}
	if from != "" {
		query += " AND COALESCE(taken_at, created_at) >= ?"
		args = append(args, from)
	}
	if to != "" {
		query += " AND COALESCE(taken_at, created_at) <= ?"
		args = append(args, to)
	}
	query += " ORDER BY shot_at DESC LIMIT 500"
	rows, err := a.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0, 100)
	for rows.Next() {
		var id, name, mime, shotAt, createdAt string
		var size sql.NullInt64
		var width, height sql.NullInt64
		if err := rows.Scan(&id, &name, &size, &mime, &width, &height, &shotAt, &createdAt); err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		item := map[string]any{
			"id":         id,
			"name":       name,
			"mime_type":  mime,
			"shot_at":    shotAt,
			"created_at": createdAt,
		}
		if size.Valid {
			item["size_bytes"] = size.Int64
		}
		if width.Valid {
			item["width"] = width.Int64
		}
		if height.Valid {
			item["height"] = height.Int64
		}
		out = append(out, item)
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"photos": out}, nil)
}

var _ = io.Copy // keep stdlib import alive
var _ time.Time
