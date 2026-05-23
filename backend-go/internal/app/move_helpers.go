package app

// Move helpers shared by the single-item (handlers_files/handlers_folders) and
// batch (handlers_batch) move paths.
//
// A move within one owner's tree is a plain parent/folder_id update. A move
// that crosses owners (e.g. an editor pulling a shared file into their own
// drive, or relocating it into a different owner's shared folder) reassigns
// ownership to the destination owner and transfers the bytes between the two
// owners' quotas — without this the relocated rows would be invisible to the
// destination owner, whose listings key on user_id. Blobs are never moved on
// disk: storage_path is an opaque, ref-counted reference, so only the DB owner
// changes. The mover's own grant on the relocated item is dropped so it stops
// showing under "shared with me" once they own it.
//
// Callers must have already verified editor access to BOTH the source and the
// destination (and run any cycle/depth checks); these helpers assume that and
// focus on the relocation + accounting.

import (
	"context"
	"database/sql"
	"fmt"
)

// moveFile relocates fileID into destFolderID (nil = destOwnerID's root).
func (a *App) moveFile(ctx context.Context, fileID string, destFolderID *string, srcOwnerID, destOwnerID string) error {
	if srcOwnerID == destOwnerID {
		res, err := a.DB.ExecContext(ctx,
			"UPDATE files SET folder_id=? WHERE id=? AND is_deleted=0",
			nullableStringPtr(destFolderID), fileID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return sql.ErrNoRows
		}
		return nil
	}

	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var size int64
	if err := tx.QueryRowContext(ctx,
		"SELECT size_bytes FROM files WHERE id=? AND is_deleted=0 FOR UPDATE", fileID).Scan(&size); err != nil {
		return err
	}
	if size > 0 {
		if err := releaseQuota(ctx, tx, srcOwnerID, size); err != nil {
			return err
		}
		if err := reserveQuota(ctx, tx, destOwnerID, size); err != nil {
			return err // ErrQuotaExceeded
		}
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE files SET folder_id=?, user_id=? WHERE id=? AND is_deleted=0",
		nullableStringPtr(destFolderID), destOwnerID, fileID); err != nil {
		return err
	}
	// The mover now owns the file; drop their own grant so it doesn't linger
	// in their "shared with me" list. Other users' grants are left intact.
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM share_grants WHERE file_id=? AND grantee_user_id=?", fileID, destOwnerID); err != nil {
		return err
	}
	return tx.Commit()
}

// moveFolder relocates folderID under destParentID (nil = destOwnerID's root),
// reassigning the whole subtree on a cross-owner move. Used by the batch path
// (the single-folder handler owns its own transaction and calls
// reassignFolderTreeTx directly).
func (a *App) moveFolder(ctx context.Context, folderID string, destParentID *string, srcOwnerID, destOwnerID string) error {
	if srcOwnerID == destOwnerID {
		res, err := a.DB.ExecContext(ctx,
			"UPDATE folders SET parent_id=? WHERE id=? AND is_deleted=0",
			nullableStringPtr(destParentID), folderID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return sql.ErrNoRows
		}
		return nil
	}

	ids, err := a.collectFolderTree(ctx, srcOwnerID, folderID)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return sql.ErrNoRows
	}
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := a.reassignFolderTreeTx(ctx, tx, folderID, destParentID, srcOwnerID, destOwnerID, ids); err != nil {
		return err
	}
	return tx.Commit()
}

// reassignFolderTreeTx, within an existing tx, re-parents rootID under
// destParentID and reassigns the whole subtree (folders + files, given as ids)
// to destOwnerID, transferring the subtree's bytes between the two owners'
// quotas and dropping the mover's own grant on the root. Callers run it only
// when the owners differ.
func (a *App) reassignFolderTreeTx(ctx context.Context, tx *sql.Tx, rootID string, destParentID *string, srcOwnerID, destOwnerID string, ids []string) error {
	ph, args := inClause(ids)
	var total sql.NullInt64
	if err := tx.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COALESCE(SUM(size_bytes),0) FROM files WHERE user_id=? AND folder_id IN %s AND is_deleted=0", ph),
		append([]any{srcOwnerID}, args...)...).Scan(&total); err != nil {
		return err
	}
	if total.Int64 > 0 {
		if err := releaseQuota(ctx, tx, srcOwnerID, total.Int64); err != nil {
			return err
		}
		if err := reserveQuota(ctx, tx, destOwnerID, total.Int64); err != nil {
			return err // ErrQuotaExceeded
		}
	}
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf("UPDATE folders SET user_id=? WHERE id IN %s", ph),
		append([]any{destOwnerID}, args...)...); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf("UPDATE files SET user_id=? WHERE folder_id IN %s", ph),
		append([]any{destOwnerID}, args...)...); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE folders SET parent_id=? WHERE id=?", nullableStringPtr(destParentID), rootID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM share_grants WHERE folder_id=? AND grantee_user_id=?", rootID, destOwnerID); err != nil {
		return err
	}
	return nil
}
