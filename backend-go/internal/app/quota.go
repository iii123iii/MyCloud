package app

import (
	"context"
	"database/sql"
	"errors"
	"log"
)

// ErrQuotaExceeded is returned by reserveQuota when adding size to the user's
// used_bytes would exceed their quota_bytes.
var ErrQuotaExceeded = errors.New("storage quota exceeded")

// reserveQuota atomically increments users.used_bytes by size if the new
// total stays within quota_bytes. Returns ErrQuotaExceeded otherwise.
// Used by both the legacy single-shot upload path and the tus post-finish
// hook, and by dedup hits where a new file row is created against an
// existing blob.
func reserveQuota(ctx context.Context, tx *sql.Tx, userID string, size int64) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE users
		SET used_bytes = used_bytes + ?
		WHERE id=? AND used_bytes + ? <= quota_bytes`, size, userID, size)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		// Log the actual quota state so server operators can see why the
		// reservation was rejected — distinguishes "user is over quota" from
		// "user row doesn't exist".
		var used, quota int64
		_ = tx.QueryRowContext(ctx,
			"SELECT used_bytes, quota_bytes FROM users WHERE id=?", userID,
		).Scan(&used, &quota)
		log.Printf("reserveQuota rejected: user=%s size=%d used=%d quota=%d", userID, size, used, quota)
		return ErrQuotaExceeded
	}
	return nil
}

// releaseQuota decrements users.used_bytes by size with a floor of zero.
// Used when a file is permanently deleted or a blob ref is dropped (F7).
func releaseQuota(ctx context.Context, ex sqlExec, userID string, size int64) error {
	_, err := ex.ExecContext(ctx, `
		UPDATE users
		SET used_bytes = GREATEST(0, used_bytes - ?)
		WHERE id=?`, size, userID)
	return err
}

// sqlExec abstracts *sql.DB and *sql.Tx so quota helpers work both inside and
// outside transactions.
type sqlExec interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}
