package app

import (
	"context"
	"database/sql"
	"errors"
)

// acquireBlobRef registers a new reference to storagePath. Inserts a row with
// ref_count=1 if this is the first reference, otherwise increments.
func acquireBlobRef(ctx context.Context, tx *sql.Tx, storagePath string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO blob_refs (storage_path, ref_count) VALUES (?, 1)
		ON DUPLICATE KEY UPDATE ref_count = ref_count + 1`, storagePath)
	return err
}

// releaseBlobRef decrements the reference for storagePath. Returns true when
// the reference count reached zero, in which case the row is removed from
// blob_refs and the caller is expected to delete the on-disk blob after the
// surrounding transaction commits.
//
// Atomic by serialization: SELECT ... FOR UPDATE first takes the row lock,
// then we decrement and check the post-value within that same tx. Two
// concurrent releases now order strictly — the second sees the first's
// committed decrement, not a stale read. The previous implementation did
// UPDATE → SELECT → DELETE and could double-delete or phantom-delete a
// still-referenced blob.
func releaseBlobRef(ctx context.Context, tx *sql.Tx, storagePath string) (bool, error) {
	var refCount int64
	err := tx.QueryRowContext(ctx,
		"SELECT ref_count FROM blob_refs WHERE storage_path = ? FOR UPDATE",
		storagePath).Scan(&refCount)
	if errors.Is(err, sql.ErrNoRows) {
		// No row at all — already cleaned. Treat as "delete the disk blob"
		// only if the caller hasn't already done so (idempotent).
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if refCount <= 1 {
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM blob_refs WHERE storage_path = ?",
			storagePath); err != nil {
			return false, err
		}
		return true, nil
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE blob_refs SET ref_count = ref_count - 1 WHERE storage_path = ?",
		storagePath); err != nil {
		return false, err
	}
	return false, nil
}

// releaseBlobRefDB is the same as releaseBlobRef but uses a *sql.DB directly,
// for callers outside a transaction. Wraps the body in its own tx so the
// SELECT ... FOR UPDATE has the row lock it needs.
func releaseBlobRefDB(ctx context.Context, db *sql.DB, storagePath string) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	freed, err := releaseBlobRef(ctx, tx, storagePath)
	if err != nil {
		_ = tx.Rollback()
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return freed, nil
}
