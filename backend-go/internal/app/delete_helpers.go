package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
)

// collectFolderTree returns all folder IDs in the subtree rooted at folderID (inclusive).
// Uses a SELECT CTE which is supported on all MariaDB versions that have recursive CTEs.
func (a *App) collectFolderTree(ctx context.Context, userID, folderID string) ([]string, error) {
	rows, err := a.DB.QueryContext(ctx, `
		WITH RECURSIVE folder_tree AS (
			SELECT id FROM folders WHERE id=? AND user_id=?
			UNION ALL
			SELECT f.id FROM folders f JOIN folder_tree ft ON f.parent_id = ft.id WHERE f.user_id=?
		)
		SELECT id FROM folder_tree`, folderID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// inClause builds a "(?,?,?)" placeholder string and an []any arg slice for the given string IDs.
func inClause(ids []string) (string, []any) {
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = "(" + placeholders[:len(placeholders)-1] + ")"
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return placeholders, args
}

func (a *App) markFolderDeleted(ctx context.Context, userID, folderID string) error {
	ids, err := a.collectFolderTree(ctx, userID, folderID)
	if err != nil || len(ids) == 0 {
		return err
	}
	ph, args := inClause(ids)
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf("UPDATE folders SET is_deleted=1, deleted_at=NOW() WHERE id IN %s", ph),
		args...); err != nil {
		return err
	}
	// Sum sizes of files about to be soft-deleted so we can release that
	// quota atomically with the trash move. Without this, trashing a folder
	// preserves its bytes against the user's cap — confusing UX, identical
	// to the file-level fix in handleDeleteFile.
	fileArgs := append([]any{userID}, args...)
	var freed sql.NullInt64
	if err := tx.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COALESCE(SUM(size_bytes), 0) FROM files WHERE user_id=? AND folder_id IN %s AND is_deleted=0", ph),
		fileArgs...).Scan(&freed); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf("UPDATE files SET is_deleted=1, deleted_at=NOW() WHERE user_id=? AND folder_id IN %s AND is_deleted=0", ph),
		fileArgs...); err != nil {
		return err
	}
	if freed.Int64 > 0 {
		if err := releaseQuota(ctx, tx, userID, freed.Int64); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (a *App) restoreFolder(ctx context.Context, userID, folderID string) error {
	ids, err := a.collectFolderTree(ctx, userID, folderID)
	if err != nil || len(ids) == 0 {
		return err
	}
	ph, args := inClause(ids)
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Re-debit quota for the soft-deleted files we're restoring. If that
	// would exceed cap, refuse — caller surfaces as 413.
	fileArgs := append([]any{userID}, args...)
	var needed sql.NullInt64
	if err := tx.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COALESCE(SUM(size_bytes), 0) FROM files WHERE user_id=? AND folder_id IN %s AND is_deleted=1", ph),
		fileArgs...).Scan(&needed); err != nil {
		return err
	}
	if needed.Int64 > 0 {
		if err := reserveQuota(ctx, tx, userID, needed.Int64); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf("UPDATE folders SET is_deleted=0, deleted_at=NULL WHERE id IN %s", ph),
		args...); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf("UPDATE files SET is_deleted=0, deleted_at=NULL WHERE user_id=? AND folder_id IN %s", ph),
		fileArgs...); err != nil {
		return err
	}
	return tx.Commit()
}

func (a *App) permanentlyDeleteFile(ctx context.Context, userID, fileID string) error {
	var path string
	var size int64
	err := a.DB.QueryRowContext(ctx, "SELECT storage_path, size_bytes FROM files WHERE id=? AND user_id=? AND is_deleted=1", fileID, userID).Scan(&path, &size)
	if err != nil {
		return err
	}

	verRows, err := a.DB.QueryContext(ctx,
		"SELECT storage_path, size_bytes FROM file_versions WHERE file_id=?", fileID)
	if err != nil {
		return err
	}
	var verPaths []string
	var verBytes int64
	for verRows.Next() {
		var p string
		var s int64
		if err := verRows.Scan(&p, &s); err != nil {
			verRows.Close()
			return err
		}
		verPaths = append(verPaths, p)
		verBytes += s
	}
	verRows.Close()

	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM files WHERE id=? AND user_id=? AND is_deleted=1", fileID, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE users SET used_bytes=GREATEST(0, used_bytes-?) WHERE id=?", size+verBytes, userID); err != nil {
		return err
	}

	// Release blob_refs. Only physically remove a blob from disk when the
	// last reference is dropped.
	deleteAfter := []string{}
	gone, err := releaseBlobRef(ctx, tx, path)
	if err != nil {
		return err
	}
	if gone {
		deleteAfter = append(deleteAfter, path)
	}
	for _, p := range verPaths {
		gone, err := releaseBlobRef(ctx, tx, p)
		if err != nil {
			return err
		}
		if gone {
			deleteAfter = append(deleteAfter, p)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	for _, p := range deleteAfter {
		_ = os.Remove(p)
	}
	return nil
}

func (a *App) permanentlyDeleteFolder(ctx context.Context, userID, folderID string) error {
	var exists int
	if err := a.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM folders WHERE id=? AND user_id=? AND is_deleted=1", folderID, userID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return sql.ErrNoRows
	}
	ids, err := a.collectFolderTree(ctx, userID, folderID)
	if err != nil || len(ids) == 0 {
		if err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	ph, folderArgs := inClause(ids)

	// Collect file paths, file IDs and total size before deleting.
	fileArgs := append([]any{userID}, folderArgs...)
	rows, err := a.DB.QueryContext(ctx,
		fmt.Sprintf("SELECT id, storage_path, size_bytes FROM files WHERE user_id=? AND folder_id IN %s", ph),
		fileArgs...)
	if err != nil {
		return err
	}
	defer rows.Close()
	var total int64
	var paths []string
	var fileIDs []string
	for rows.Next() {
		var id, path string
		var size int64
		if err := rows.Scan(&id, &path, &size); err != nil {
			return err
		}
		paths = append(paths, path)
		fileIDs = append(fileIDs, id)
		total += size
	}
	rows.Close()

	// Add up version blob paths for those files.
	if len(fileIDs) > 0 {
		filePH, fileIDArgs := inClause(fileIDs)
		vrows, err := a.DB.QueryContext(ctx,
			fmt.Sprintf("SELECT storage_path, size_bytes FROM file_versions WHERE file_id IN %s", filePH),
			fileIDArgs...)
		if err != nil {
			return err
		}
		for vrows.Next() {
			var p string
			var s int64
			if err := vrows.Scan(&p, &s); err != nil {
				vrows.Close()
				return err
			}
			paths = append(paths, p)
			total += s
		}
		vrows.Close()
	}

	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf("DELETE FROM files WHERE user_id=? AND folder_id IN %s", ph),
		fileArgs...); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf("DELETE FROM folders WHERE id IN %s", ph),
		folderArgs...); err != nil {
		return err
	}
	if total > 0 {
		if _, err := tx.ExecContext(ctx, "UPDATE users SET used_bytes=GREATEST(0, used_bytes-?) WHERE id=?", total, userID); err != nil {
			return err
		}
	}

	// Release blob_refs for every collected path. Only physically delete a blob
	// when the last reference is dropped.
	deleteAfter := []string{}
	for _, p := range paths {
		gone, err := releaseBlobRef(ctx, tx, p)
		if err != nil {
			return err
		}
		if gone {
			deleteAfter = append(deleteAfter, p)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	for _, p := range deleteAfter {
		_ = os.Remove(p)
	}
	return nil
}

func (a *App) deleteUserResources(ctx context.Context, userID string) error {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Lock the user's file rows inside the tx and gather paths + version
	// paths. Reading outside the transaction created a TOCTOU window where
	// a concurrent upload's row would be wiped by DELETE WHERE user_id=?
	// but its on-disk blob never reached this slice — leaving an orphan.
	// SELECT ... FOR UPDATE serialises against new inserts holding the
	// row lock; the outer DELETE then runs against the same snapshot.
	fileRows, err := tx.QueryContext(ctx,
		"SELECT id, storage_path FROM files WHERE user_id=? FOR UPDATE", userID)
	if err != nil {
		return err
	}
	type fileRow struct{ id, path string }
	var files []fileRow
	for fileRows.Next() {
		var f fileRow
		if err := fileRows.Scan(&f.id, &f.path); err != nil {
			fileRows.Close()
			return err
		}
		files = append(files, f)
	}
	if err := fileRows.Err(); err != nil {
		fileRows.Close()
		return err
	}
	fileRows.Close()

	// Version paths for those files — same locking concern.
	var paths []string
	for _, f := range files {
		paths = append(paths, f.path)
	}
	if len(files) > 0 {
		ids := make([]string, 0, len(files))
		for _, f := range files {
			ids = append(ids, f.id)
		}
		ph, args := inClause(ids)
		verRows, err := tx.QueryContext(ctx,
			fmt.Sprintf("SELECT storage_path FROM file_versions WHERE file_id IN %s FOR UPDATE", ph),
			args...)
		if err != nil {
			return err
		}
		for verRows.Next() {
			var p string
			if err := verRows.Scan(&p); err != nil {
				verRows.Close()
				return err
			}
			paths = append(paths, p)
		}
		if err := verRows.Err(); err != nil {
			verRows.Close()
			return err
		}
		verRows.Close()
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM shares WHERE created_by=?", userID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM files WHERE user_id=?", userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM folders WHERE user_id=?", userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM activity_log WHERE user_id=?", userID); err != nil {
		return err
	}

	// Route every blob path through releaseBlobRef so a dedup-shared blob
	// (referenced by ANOTHER user) survives this user's deletion. The
	// previous version unconditionally `os.Remove`'d every path under the
	// assumption that paths were per-user — true for non-dedup blobs, but
	// wrong as soon as cross-user dedup is in play, and silently destroyed
	// the other user's still-referenced blob.
	deleteAfter := []string{}
	for _, p := range paths {
		gone, err := releaseBlobRef(ctx, tx, p)
		if err != nil {
			return err
		}
		if gone {
			deleteAfter = append(deleteAfter, p)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	for _, p := range deleteAfter {
		_ = os.Remove(p)
	}
	return nil
}
