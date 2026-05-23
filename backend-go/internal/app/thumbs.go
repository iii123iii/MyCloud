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

// thumbMaxSourceBytes caps the size of an image we'll attempt to thumbnail.
// image.Decode loads the full decoded pixel grid into memory — a 500 MB JPEG
// can decode to several GB of RGBA, OOM'ing the worker. Above the cap we
// skip thumbnailing entirely; the UI falls back to a generic icon and the
// user can still preview the original on demand.
const thumbMaxSourceBytes int64 = 50 * 1024 * 1024 // 50 MB

// processThumb generates a thumbnail for the file at fileID and extracts EXIF
// metadata. The thumbnail is encrypted with its own per-file key. Called by
// the q:thumb worker after a successful image upload.
func (a *App) processThumb(ctx context.Context, fileID string) error {
	var mimeType, encKey, iv, tag, blobPath, userID string
	var sizeBytes int64
	err := a.DB.QueryRowContext(ctx, `
		SELECT mime_type, encryption_key_enc, encryption_iv, encryption_tag, storage_path, user_id, size_bytes
		FROM files WHERE id = ? AND is_deleted = 0`, fileID,
	).Scan(&mimeType, &encKey, &iv, &tag, &blobPath, &userID, &sizeBytes)
	if err != nil {
		return fmt.Errorf("load file %s: %w", fileID, err)
	}
	if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return nil
	}
	if sizeBytes > thumbMaxSourceBytes {
		// Skip but don't fail — the job is "done" successfully and won't
		// retry. The user sees the fallback icon instead.
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
	var fileSize int64
	var fileUpdated string
	if err := a.DB.QueryRowContext(r.Context(), `
		SELECT f.thumb_path, e.exif_json, f.size_bytes, f.updated_at
		FROM files f LEFT JOIN file_exif e ON e.file_id = f.id
		WHERE f.id = ?`, fileID).Scan(&thumbPath, &exifJSON, &fileSize, &fileUpdated); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !thumbPath.Valid || !exifJSON.Valid {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	// ETag conditional GET — thumbnails are immutable for a given (size,
	// updated_at) pair because they're regenerated on file content change.
	// Returning 304 here saves the decrypt+stream cost which dominates the
	// request latency for cold cache pages.
	etag := httpapi.For(fileSize, fileUpdated)
	if httpapi.CheckIfNoneMatch(w, r, etag) {
		return
	}

	// Redis cache layer in front of the disk decrypt. Key includes the
	// ETag so a content change naturally invalidates the cache without an
	// explicit eviction step. 24h TTL and a 256 KiB size cap keep memory
	// bounded; larger thumbs (rare — disintegration outputs ~10-30 KiB
	// JPEGs at 256px) fall through to the disk path.
	redisKey := "thumb:" + fileID + ":" + etag
	if cached, err := a.Redis.Get(r.Context(), redisKey).Bytes(); err == nil && len(cached) > 0 {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "private, max-age=86400")
		w.Header().Set("X-Cache", "redis")
		_, _ = w.Write(cached)
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
	// Decrypt into a buffer (capped at 256 KiB) so we can both stream
	// to the client AND seed the Redis cache. Past the cap, we fall back
	// to streaming directly (bypassing the cache) so unusually large
	// thumbnails don't pin Redis memory.
	const cacheCap = 256 * 1024
	cacheBuf := &capBuffer{cap: cacheCap}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("X-Cache", "miss")
	multi := teeWriter(w, cacheBuf)
	if err := storage.DecryptFileToWriter(thumbPath.String, multi, key); err != nil {
		// Already wrote headers — best we can do is truncate the response.
		return
	}
	if cacheBuf.full() {
		// Source exceeded the cap; skip caching.
		return
	}
	_ = a.Redis.Set(r.Context(), redisKey, cacheBuf.bytes(), 24*time.Hour).Err()
}

// capBuffer accumulates writes up to cap, then silently drops further
// content (sets the full flag). Used by the cache to stop hoarding
// memory on outsize thumbnails.
type capBuffer struct {
	cap  int
	buf  []byte
	over bool
}

func (b *capBuffer) Write(p []byte) (int, error) {
	if b.over {
		return len(p), nil
	}
	remain := b.cap - len(b.buf)
	if len(p) > remain {
		b.buf = append(b.buf, p[:remain]...)
		b.over = true
		return len(p), nil
	}
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *capBuffer) bytes() []byte { return b.buf }
func (b *capBuffer) full() bool    { return b.over }

// teeWriter mirrors writes to two destinations without io.MultiWriter (which
// would import io into this file just for one call site).
func teeWriter(a, b interface{ Write([]byte) (int, error) }) interface {
	Write([]byte) (int, error)
} {
	return &teeWriterImpl{a: a, b: b}
}

type teeWriterImpl struct {
	a, b interface{ Write([]byte) (int, error) }
}

func (t *teeWriterImpl) Write(p []byte) (int, error) {
	n, err := t.a.Write(p)
	if err != nil {
		return n, err
	}
	// Best-effort to the cache buffer; errors don't fail the request.
	_, _ = t.b.Write(p)
	return n, nil
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
	// Hard limit + explicit offset paging replaces the previous silent
	// LIMIT 500 — that capped the response without telling the client
	// there was more data, so a user with 10k photos saw 500 with no UI
	// hint to scroll further. Cap stays sane (1000) to keep the JSON
	// payload bounded.
	limit, offset := readLimitOffset(r, 200, 1000)
	query += " ORDER BY shot_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	rows, err := a.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0, 100)
	for rows.Next() {
		var id, name, mime, shotAt, createdAt string
		var size sql.NullInt64
		var width, height sql.NullInt64
		if err := rows.Scan(&id, &name, &size, &mime, &width, &height, &shotAt, &createdAt); err != nil {
			respondDBError(w, r, err)
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
	if err := rows.Err(); err != nil {
		respondDBError(w, r, err)
		return
	}
	httpapi.JSON(w, http.StatusOK,
		map[string]any{"photos": out},
		map[string]any{"limit": limit, "offset": offset, "has_more": len(out) == limit})
}

var _ = io.Copy // keep stdlib import alive
var _ time.Time
