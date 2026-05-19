package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
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
	hasher := sha256.New()
	teed := io.TeeReader(filePart, hasher)
	size, encErr := storage.EncryptStream(teed, tmpFile, fileKey)
	closeErr := tmpFile.Close()
	filePart.Close()
	contentHash := hex.EncodeToString(hasher.Sum(nil))
	if encErr != nil {
		_ = os.Remove(tmpPath)
		return nil, encErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return nil, closeErr
	}

	if _, err := storage.EnsureUserDir(a.Config.StoragePath, userID); err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	mimeType := detectMime(filename)

	// Versioning: if a non-deleted same-name file exists in the target folder,
	// replace it in place and snapshot the previous current into file_versions.
	// Keeps the file ID stable so shares/comments/tags survive the upload.
	commit, err := a.commitUploadWithVersioning(ctx, userID, filename, folderID, mimeType, contentHash, size, bundle, tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	return map[string]any{
		"id":          commit.fileID,
		"name":        filename,
		"folder_id":   nullableString(folderID),
		"size_bytes":  size,
		"mime_type":   mimeType,
		"versioned":   commit.versioned,
		"version_no":  commit.newVersionNo,
	}, nil
}

type uploadCommit struct {
	fileID       string
	versioned    bool // true when this replaced an existing file (a new version row was written)
	newVersionNo int  // version number written for the prior current, 0 if not versioned
	deduped      bool // true when the new files row points at an existing blob
}

// commitUploadWithVersioning is the single place that decides whether an
// upload becomes a new file row, a new version of an existing one, or is
// fully deduplicated against an existing blob.
//
// Inputs: tmpPath holds the newly-encrypted ciphertext. The caller has
// already wrapped the source through a SHA-256 hasher and passes the digest
// in contentHash so we can dedup against existing files.
//
// Behaviour:
//   - Same-name file in same folder, same hash → no-op, return existing ID.
//   - Same-name file, different hash → snapshot current into file_versions
//     (no on-disk rename — multiple things may reference the same blob), and
//     point the files row at either an existing dedup blob OR a fresh path.
//   - No same-name conflict, hash matches an existing blob for this user →
//     insert a new files row pointing at that blob, ref_count++.
//   - Otherwise → standard fresh upload.
//
// On commit failure, any disk moves we made are reversed best-effort. The
// caller still owns os.Remove(tmpPath) for non-commit-related failures.
func (a *App) commitUploadWithVersioning(ctx context.Context, userID, filename, folderID, mimeType, contentHash string,
	size int64, bundle storage.EncryptedKeyBundle, tmpPath string) (uploadCommit, error) {

	tx, err := a.DB.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return uploadCommit{}, err
	}
	committed := false
	// Disk moves we performed inside the tx — if commit fails, undo them.
	type undo struct {
		from, to string
	}
	var undos []undo
	// Disk paths to actually delete after commit succeeds (e.g. pruned versions
	// whose ref_count dropped to 0).
	var afterCommitDeletes []string
	defer func() {
		if !committed {
			_ = tx.Rollback()
			for i := len(undos) - 1; i >= 0; i-- {
				_ = os.Rename(undos[i].to, undos[i].from)
			}
		} else {
			for _, p := range afterCommitDeletes {
				_ = os.Remove(p)
			}
		}
	}()

	if folderID != "" {
		var exists int
		if err := tx.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM folders WHERE id=? AND user_id=? AND is_deleted=0",
			folderID, userID).Scan(&exists); err != nil || exists == 0 {
			return uploadCommit{}, fmt.Errorf("folder not found")
		}
	}

	// Find existing same-name file in the same folder for this owner BEFORE
	// reserving quota — a same-content reupload is a no-op and shouldn't be
	// rejected as "quota exceeded" when the user is near their cap.
	var existingID, oldKey, oldIV, oldTag, oldPath, oldHash string
	var oldSize int64
	folderArg := nullableString(folderID)
	whereFolder := "folder_id IS NULL"
	sameNameArgs := []any{filename, userID}
	if folderID != "" {
		whereFolder = "folder_id = ?"
		sameNameArgs = []any{filename, userID, folderID}
	}
	err = tx.QueryRowContext(ctx, `
		SELECT id, encryption_key_enc, encryption_iv, encryption_tag, storage_path,
		       size_bytes, COALESCE(content_sha256, '')
		FROM files WHERE name = ? AND user_id = ? AND is_deleted = 0 AND `+whereFolder,
		sameNameArgs...).Scan(&existingID, &oldKey, &oldIV, &oldTag, &oldPath,
		&oldSize, &oldHash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return uploadCommit{}, err
	}

	// ─── Case A: same name AND same content → no-op (drop tmp, no quota change).
	if err == nil && oldHash == contentHash && contentHash != "" {
		if err := tx.Commit(); err != nil {
			return uploadCommit{}, err
		}
		committed = true
		_ = os.Remove(tmpPath)
		return uploadCommit{fileID: existingID}, nil
	}

	// All remaining paths add `size` to the user's storage usage.
	if err := reserveQuota(ctx, tx, userID, size); err != nil {
		return uploadCommit{}, err
	}

	// Helper: find a dedup candidate for the new content (any non-deleted file
	// owned by this user with the same hash, optionally excluding existingID).
	findDedup := func(excludeID string) (storagePath, key, iv, tag string, hit bool, err error) {
		query := `
			SELECT storage_path, encryption_key_enc, encryption_iv, encryption_tag
			FROM files
			WHERE user_id = ? AND content_sha256 = ? AND is_deleted = 0`
		args := []any{userID, contentHash}
		if excludeID != "" {
			query += " AND id != ?"
			args = append(args, excludeID)
		}
		query += " LIMIT 1"
		row := tx.QueryRowContext(ctx, query, args...)
		if scanErr := row.Scan(&storagePath, &key, &iv, &tag); scanErr != nil {
			if errors.Is(scanErr, sql.ErrNoRows) {
				return "", "", "", "", false, nil
			}
			return "", "", "", "", false, scanErr
		}
		return storagePath, key, iv, tag, true, nil
	}

	if errors.Is(err, sql.ErrNoRows) {
		// ─── Case B: no same-name conflict.
		// Try dedup against any other file with this content.
		dedupPath, dedupKey, dedupIV, dedupTag, dedupHit, dedupErr := findDedup("")
		if dedupErr != nil {
			return uploadCommit{}, dedupErr
		}
		fileID := uuid.NewString()
		var storagePath, keyHex, ivHex, tagHex string
		if dedupHit {
			storagePath = dedupPath
			keyHex, ivHex, tagHex = dedupKey, dedupIV, dedupTag
			_ = os.Remove(tmpPath) // discard the wasted ciphertext
		} else {
			storagePath = storage.FinalPath(a.Config.StoragePath, userID, fileID)
			keyHex, ivHex, tagHex = bundle.EncKeyHex, bundle.IVHex, bundle.TagHex
			if err := os.Rename(tmpPath, storagePath); err != nil {
				return uploadCommit{}, err
			}
			undos = append(undos, undo{from: tmpPath, to: storagePath})
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO files (id, name, storage_path, size_bytes, mime_type, user_id, folder_id,
			                   encryption_key_enc, encryption_iv, encryption_tag,
			                   content_sha256, is_deleted, is_starred)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0)`,
			fileID, filename, storagePath, size, mimeType, userID, folderArg,
			keyHex, ivHex, tagHex, contentHash); err != nil {
			return uploadCommit{}, err
		}
		if err := acquireBlobRef(ctx, tx, storagePath); err != nil {
			return uploadCommit{}, err
		}
		if err := tx.Commit(); err != nil {
			return uploadCommit{}, err
		}
		committed = true
		return uploadCommit{fileID: fileID, deduped: dedupHit}, nil
	}

	// ─── Case C: same name, different content → version + maybe dedup new content.
	var nextVer sql.NullInt64
	if err := tx.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(version_no), 0) FROM file_versions WHERE file_id = ?",
		existingID).Scan(&nextVer); err != nil {
		return uploadCommit{}, err
	}
	versionNo := int(nextVer.Int64) + 1
	versionID := uuid.NewString()

	// Snapshot the previous current as a version. The blob stays at oldPath —
	// blob_refs records the new reference from the version row.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO file_versions (id, file_id, version_no, storage_path, size_bytes,
		                           encryption_key_enc, encryption_iv, encryption_tag, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		versionID, existingID, versionNo, oldPath, oldSize,
		oldKey, oldIV, oldTag, userID); err != nil {
		return uploadCommit{}, err
	}
	if err := acquireBlobRef(ctx, tx, oldPath); err != nil {
		return uploadCommit{}, err
	}

	// Dedup new content against any other file (excluding the one we are about to overwrite).
	dedupPath, dedupKey, dedupIV, dedupTag, dedupHit, dedupErr := findDedup(existingID)
	if dedupErr != nil {
		return uploadCommit{}, dedupErr
	}
	var newPath, newKey, newIV, newTag string
	if dedupHit {
		newPath = dedupPath
		newKey, newIV, newTag = dedupKey, dedupIV, dedupTag
		_ = os.Remove(tmpPath)
	} else {
		// Use a fresh UUID-named path so we never collide with the now-versioned oldPath.
		newPath = storage.FinalPath(a.Config.StoragePath, userID, uuid.NewString())
		newKey, newIV, newTag = bundle.EncKeyHex, bundle.IVHex, bundle.TagHex
		if err := os.Rename(tmpPath, newPath); err != nil {
			return uploadCommit{}, err
		}
		undos = append(undos, undo{from: tmpPath, to: newPath})
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE files
		SET storage_path = ?, size_bytes = ?, mime_type = ?,
		    encryption_key_enc = ?, encryption_iv = ?, encryption_tag = ?,
		    content_sha256 = ?
		WHERE id = ?`,
		newPath, size, mimeType, newKey, newIV, newTag, contentHash, existingID); err != nil {
		return uploadCommit{}, err
	}
	if err := acquireBlobRef(ctx, tx, newPath); err != nil {
		return uploadCommit{}, err
	}
	deleted, err := releaseBlobRef(ctx, tx, oldPath)
	if err != nil {
		return uploadCommit{}, err
	}
	if deleted {
		afterCommitDeletes = append(afterCommitDeletes, oldPath)
	}

	// Prune older versions beyond the configured cap.
	prunedPaths, prunedSize, err := a.pruneVersionsTx(ctx, tx, userID, existingID)
	if err != nil {
		return uploadCommit{}, err
	}
	for _, p := range prunedPaths {
		gone, err := releaseBlobRef(ctx, tx, p)
		if err != nil {
			return uploadCommit{}, err
		}
		if gone {
			afterCommitDeletes = append(afterCommitDeletes, p)
		}
	}
	if prunedSize > 0 {
		if err := releaseQuota(ctx, tx, userID, prunedSize); err != nil {
			return uploadCommit{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return uploadCommit{}, err
	}
	committed = true
	return uploadCommit{
		fileID:       existingID,
		versioned:    true,
		newVersionNo: versionNo,
		deduped:      dedupHit,
	}, nil
}

// pruneVersionsTx returns the storage paths of versions exceeding the
// MaxVersionsPerFile cap and removes their rows. Callers must drop the
// returned paths' blob_refs via releaseBlobRef and only os.Remove if the ref
// count reached zero.
func (a *App) pruneVersionsTx(ctx context.Context, tx *sql.Tx, userID, fileID string) ([]string, int64, error) {
	keep := a.Config.MaxVersionsPerFile
	if keep <= 0 {
		keep = 10
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, version_no, storage_path, size_bytes FROM file_versions
		WHERE file_id = ? ORDER BY version_no DESC`, fileID)
	if err != nil {
		return nil, 0, err
	}
	type row struct {
		id   string
		no   int
		path string
		size int64
	}
	var all []row
	for rows.Next() {
		var rr row
		if err := rows.Scan(&rr.id, &rr.no, &rr.path, &rr.size); err != nil {
			rows.Close()
			return nil, 0, err
		}
		all = append(all, rr)
	}
	rows.Close()
	if len(all) <= keep {
		return nil, 0, nil
	}
	var paths []string
	var freed int64
	ids := make([]string, 0)
	for _, rr := range all[keep:] {
		paths = append(paths, rr.path)
		freed += rr.size
		ids = append(ids, rr.id)
	}
	ph, args := inClause(ids)
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM file_versions WHERE id IN "+ph, args...); err != nil {
		return nil, 0, err
	}
	return paths, freed, nil
}

func (a *App) serveFile(w http.ResponseWriter, r *http.Request, fileID, disposition string) {
	userID := userIDFrom(r)
	if _, err := a.canAccessFile(r.Context(), userID, fileID, AccessViewer); err != nil {
		if err == sql.ErrNoRows {
			httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
			return
		}
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	var name, mimeType, encKey, iv, tag, path string
	err := a.DB.QueryRowContext(r.Context(), `
		SELECT name, mime_type, encryption_key_enc, encryption_iv, encryption_tag, storage_path
		FROM files WHERE id=? AND is_deleted=0`, fileID).
		Scan(&name, &mimeType, &encKey, &iv, &tag, &path)
	if err == sql.ErrNoRows {
		httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
		return
	}
	if err != nil {
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
