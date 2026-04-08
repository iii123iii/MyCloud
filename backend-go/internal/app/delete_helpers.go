package app

import (
	"context"
	"database/sql"
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
	if _, err := a.DB.ExecContext(ctx,
		fmt.Sprintf("UPDATE folders SET is_deleted=1, deleted_at=NOW() WHERE id IN %s", ph),
		args...); err != nil {
		return err
	}
	fileArgs := append([]any{userID}, args...)
	_, err = a.DB.ExecContext(ctx,
		fmt.Sprintf("UPDATE files SET is_deleted=1, deleted_at=NOW() WHERE user_id=? AND folder_id IN %s", ph),
		fileArgs...)
	return err
}

func (a *App) restoreFolder(ctx context.Context, userID, folderID string) error {
	ids, err := a.collectFolderTree(ctx, userID, folderID)
	if err != nil || len(ids) == 0 {
		return err
	}
	ph, args := inClause(ids)
	if _, err := a.DB.ExecContext(ctx,
		fmt.Sprintf("UPDATE folders SET is_deleted=0, deleted_at=NULL WHERE id IN %s", ph),
		args...); err != nil {
		return err
	}
	fileArgs := append([]any{userID}, args...)
	_, err = a.DB.ExecContext(ctx,
		fmt.Sprintf("UPDATE files SET is_deleted=0, deleted_at=NULL WHERE user_id=? AND folder_id IN %s", ph),
		fileArgs...)
	return err
}

func (a *App) permanentlyDeleteFile(ctx context.Context, userID, fileID string) error {
	var path string
	var size int64
	err := a.DB.QueryRowContext(ctx, "SELECT storage_path, size_bytes FROM files WHERE id=? AND user_id=? AND is_deleted=1", fileID, userID).Scan(&path, &size)
	if err != nil {
		return err
	}
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM files WHERE id=? AND user_id=? AND is_deleted=1", fileID, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE users SET used_bytes=GREATEST(0, used_bytes-?) WHERE id=?", size, userID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_ = os.Remove(path)
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

	// Collect file paths and total size before deleting.
	fileArgs := append([]any{userID}, folderArgs...)
	rows, err := a.DB.QueryContext(ctx,
		fmt.Sprintf("SELECT storage_path, size_bytes FROM files WHERE user_id=? AND folder_id IN %s", ph),
		fileArgs...)
	if err != nil {
		return err
	}
	defer rows.Close()
	var total int64
	var paths []string
	for rows.Next() {
		var path string
		var size int64
		if err := rows.Scan(&path, &size); err != nil {
			return err
		}
		paths = append(paths, path)
		total += size
	}
	rows.Close()

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
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, path := range paths {
		_ = os.Remove(path)
	}
	return nil
}

func (a *App) deleteUserResources(ctx context.Context, userID string) error {
	rows, err := a.DB.QueryContext(ctx, "SELECT storage_path FROM files WHERE user_id=?", userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return err
		}
		paths = append(paths, path)
	}
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM shares WHERE created_by=?", userID); err != nil && err != sql.ErrNoRows {
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
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, path := range paths {
		_ = os.Remove(path)
	}
	return nil
}
