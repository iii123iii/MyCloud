package app

import (
	"context"
	"database/sql"
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
func releaseBlobRef(ctx context.Context, tx *sql.Tx, storagePath string) (bool, error) {
	if _, err := tx.ExecContext(ctx,
		"UPDATE blob_refs SET ref_count = ref_count - 1 WHERE storage_path = ?",
		storagePath); err != nil {
		return false, err
	}
	var refCount int64
	err := tx.QueryRowContext(ctx,
		"SELECT ref_count FROM blob_refs WHERE storage_path = ?",
		storagePath).Scan(&refCount)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if refCount <= 0 {
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM blob_refs WHERE storage_path = ?",
			storagePath); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// releaseBlobRefDB is the same as releaseBlobRef but uses a *sql.DB directly,
// for callers outside a transaction. Used by purge paths that have already
// committed metadata changes.
func releaseBlobRefDB(ctx context.Context, db *sql.DB, storagePath string) (bool, error) {
	if _, err := db.ExecContext(ctx,
		"UPDATE blob_refs SET ref_count = ref_count - 1 WHERE storage_path = ?",
		storagePath); err != nil {
		return false, err
	}
	var refCount int64
	err := db.QueryRowContext(ctx,
		"SELECT ref_count FROM blob_refs WHERE storage_path = ?",
		storagePath).Scan(&refCount)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if refCount <= 0 {
		if _, err := db.ExecContext(ctx,
			"DELETE FROM blob_refs WHERE storage_path = ?",
			storagePath); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}
