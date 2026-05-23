package app

import (
	"context"
	"database/sql"
	"errors"

	"mycloud/backend-go/internal/logging"
)

// ErrQuotaExceeded is returned by reserveQuota when adding size to the user's
// used_bytes would exceed their quota_bytes.
var ErrQuotaExceeded = errors.New("storage quota exceeded")

// ErrInvalidFilename is returned by upload paths when the supplied filename
// fails structural validation (empty after cleaning, traversal, control
// chars). Handlers surface this as 400 bad_request.
var ErrInvalidFilename = errors.New("invalid filename")

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
		logging.FromContext(ctx).Warn("reserveQuota rejected",
			"user_id", userID,
			"size", size,
			"used", used,
			"quota", quota,
		)
		return ErrQuotaExceeded
	}
	return nil
}

// releaseQuota decrements users.used_bytes by size with a floor of zero.
// Used when a file is permanently deleted or a blob ref is dropped.
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
