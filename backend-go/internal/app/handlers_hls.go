package app

// HLS streaming handlers. See internal/media/hls.go for pipeline notes.
//
// Endpoints:
//   GET /api/v2/files/{id}/hls/playlist.m3u8     — playlist or 202 with retry
//   GET /api/v2/files/{id}/hls/{segment}.ts      — individual segment
//
// Cache lives at <StoragePath>/hls/<file_id>/. The maintenance ticker is
// responsible for evicting entries for deleted source files.

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"mycloud/backend-go/internal/httpapi"
	"mycloud/backend-go/internal/logging"
	"mycloud/backend-go/internal/media"
	"mycloud/backend-go/internal/storage"
)

// hlsCacheDir returns the on-disk dir for this file. We don't include the
// user id in the path: file_id is a UUID and the canAccessFile check
// already prevents cross-user access.
func (a *App) hlsCacheDir(fileID string) string {
	return filepath.Join(a.Config.StoragePath, "hls", fileID)
}

// handleHLSPlaylist returns the cached playlist or kicks off a transcode.
func (a *App) handleHLSPlaylist(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	fileID := chi.URLParam(r, "id")
	if _, err := a.canAccessFile(r.Context(), userID, fileID, AccessViewer); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "file not accessible")
			return
		}
		respondDBError(w, r, err)
		return
	}

	// Check job state.
	var status sql.NullString
	_ = a.DB.QueryRowContext(r.Context(),
		`SELECT status FROM hls_jobs WHERE file_id = ?`, fileID).Scan(&status)

	playlistPath := filepath.Join(a.hlsCacheDir(fileID), "playlist.m3u8")
	if status.Valid && status.String == "ready" {
		if _, err := os.Stat(playlistPath); err == nil {
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			http.ServeFile(w, r, playlistPath)
			return
		}
		// Status says ready but file vanished — fall through to re-transcode.
	}

	switch {
	case !status.Valid:
		// First request — insert pending row and enqueue.
		if _, err := a.DB.ExecContext(r.Context(),
			`INSERT INTO hls_jobs (file_id, status) VALUES (?, 'pending')
			 ON DUPLICATE KEY UPDATE status='pending', error=NULL`, fileID); err != nil {
			respondDBError(w, r, err)
			return
		}
		a.enqueueHLS(r.Context(), fileID)
		w.Header().Set("Retry-After", "5")
		httpapi.Error(w, http.StatusAccepted, "transcoding", "HLS transcode queued")
		return
	case status.String == "failed":
		// Reset and retry.
		if _, err := a.DB.ExecContext(r.Context(),
			`UPDATE hls_jobs SET status='pending', error=NULL WHERE file_id=?`, fileID); err != nil {
			respondDBError(w, r, err)
			return
		}
		a.enqueueHLS(r.Context(), fileID)
		w.Header().Set("Retry-After", "10")
		httpapi.Error(w, http.StatusAccepted, "transcoding", "HLS transcode requeued")
		return
	default:
		// pending or running: just wait.
		w.Header().Set("Retry-After", "5")
		httpapi.Error(w, http.StatusAccepted, "transcoding", "HLS transcode in progress")
		return
	}
}

// handleHLSSegment streams a single .ts segment from cache. We avoid using
// http.ServeFile's range support directly because segments are tiny and we
// want a stricter path-validation step (no directory traversal).
func (a *App) handleHLSSegment(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	fileID := chi.URLParam(r, "id")
	segment := chi.URLParam(r, "segment")
	if !validSegmentName(segment) {
		httpapi.Error(w, http.StatusBadRequest, "bad_segment", "invalid segment filename")
		return
	}
	if _, err := a.canAccessFile(r.Context(), userID, fileID, AccessViewer); err != nil {
		httpapi.Error(w, http.StatusNotFound, "not_found", "file not accessible")
		return
	}
	path := filepath.Join(a.hlsCacheDir(fileID), segment)
	// Defence-in-depth: ensure the resolved path didn't escape the cache dir.
	resolved, err := filepath.Abs(path)
	if err != nil || !strings.HasPrefix(resolved, a.hlsCacheDir(fileID)) {
		httpapi.Error(w, http.StatusBadRequest, "bad_path", "")
		return
	}
	w.Header().Set("Content-Type", "video/mp2t")
	http.ServeFile(w, r, path)
}

// validSegmentName accepts seg000.ts through seg999.ts and playlist.m3u8.
// Anything else fails closed.
func validSegmentName(s string) bool {
	if s == "playlist.m3u8" {
		return true
	}
	if !strings.HasPrefix(s, "seg") || !strings.HasSuffix(s, ".ts") {
		return false
	}
	mid := strings.TrimSuffix(strings.TrimPrefix(s, "seg"), ".ts")
	if len(mid) < 1 || len(mid) > 5 {
		return false
	}
	for _, c := range mid {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// enqueueHLS adds the file to the q:hls queue. Workers in workers.go drain it.
func (a *App) enqueueHLS(ctx context.Context, fileID string) {
	_ = a.Redis.LPush(ctx, "q:hls", fileID).Err()
}

// processHLSJob is the worker callback. Decrypts source to a temp file,
// runs ffmpeg, moves the output into the cache dir, updates hls_jobs.
func (a *App) processHLSJob(ctx context.Context, payload []byte) error {
	fileID := string(payload)
	lg := logging.FromContext(ctx).With("file_id", fileID)

	// Mark running.
	if _, err := a.DB.ExecContext(ctx,
		`UPDATE hls_jobs SET status='running' WHERE file_id=?`, fileID); err != nil {
		return err
	}
	// Load + decrypt source.
	var encKey, iv, tag, blobPath string
	if err := a.DB.QueryRowContext(ctx, `
		SELECT encryption_key_enc, encryption_iv, encryption_tag, storage_path
		FROM files WHERE id=? AND is_deleted=0`, fileID,
	).Scan(&encKey, &iv, &tag, &blobPath); err != nil {
		_ = a.markHLSFailed(ctx, fileID, err.Error())
		return err
	}
	key, err := storage.UnwrapKey(a.Config.MasterEncryptionKey, storage.EncryptedKeyBundle{
		IVHex: iv, EncKeyHex: encKey, TagHex: tag,
	})
	if err != nil {
		_ = a.markHLSFailed(ctx, fileID, err.Error())
		return err
	}

	tmpDir, err := os.MkdirTemp("", "mycloud-hls-")
	if err != nil {
		_ = a.markHLSFailed(ctx, fileID, err.Error())
		return err
	}
	defer os.RemoveAll(tmpDir)
	srcPath := filepath.Join(tmpDir, "src")
	srcF, err := os.Create(srcPath)
	if err != nil {
		_ = a.markHLSFailed(ctx, fileID, err.Error())
		return err
	}
	if err := storage.DecryptFileToWriter(blobPath, srcF, key); err != nil {
		_ = srcF.Close()
		_ = a.markHLSFailed(ctx, fileID, "decrypt: "+err.Error())
		return err
	}
	_ = srcF.Close()

	// Stage output into a fresh dir then atomically move into cache.
	stage := filepath.Join(tmpDir, "out")
	if _, err := media.TranscodeToHLS(ctx, media.TranscodeOpts{
		SourcePath: srcPath,
		OutputDir:  stage,
	}); err != nil {
		_ = a.markHLSFailed(ctx, fileID, err.Error())
		return err
	}

	cacheDir := a.hlsCacheDir(fileID)
	_ = os.RemoveAll(cacheDir)
	if err := os.MkdirAll(filepath.Dir(cacheDir), 0o755); err != nil {
		_ = a.markHLSFailed(ctx, fileID, err.Error())
		return err
	}
	if err := os.Rename(stage, cacheDir); err != nil {
		_ = a.markHLSFailed(ctx, fileID, "move stage: "+err.Error())
		return err
	}

	if _, err := a.DB.ExecContext(ctx,
		`UPDATE hls_jobs SET status='ready', cache_dir=?, error=NULL WHERE file_id=?`,
		cacheDir, fileID); err != nil {
		return err
	}
	lg.Info("hls ready", "cache_dir", cacheDir)
	return nil
}

func (a *App) markHLSFailed(ctx context.Context, fileID, msg string) error {
	_, err := a.DB.ExecContext(ctx,
		`UPDATE hls_jobs SET status='failed', error=? WHERE file_id=?`, msg, fileID)
	return err
}

// context import dance kept inline to avoid a separate file.
type ctxRequester = http.Request

var _ = (*ctxRequester)(nil)
