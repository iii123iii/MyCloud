package app

import (
	"context"
	"database/sql"
	"errors"
)

// AccessLevel is the minimum permission required to perform an action on a
// shared resource. The order matters: Owner > Editor > Viewer.
type AccessLevel int

const (
	AccessViewer AccessLevel = iota
	AccessEditor
	AccessOwner
)

// canAccessFile reports whether userID may act on fileID with at least min
// permission. Returns (ownerID, nil) on success, or sql.ErrNoRows when the
// file does not exist, is in trash, or the user has insufficient permission.
//
// The ownership check is the primary path. Once share_grants exists
// (F1: per-user sharing), this helper will also consult that table for
// non-owner access.
func (a *App) canAccessFile(ctx context.Context, userID, fileID string, min AccessLevel) (string, error) {
	var ownerID string
	err := a.DB.QueryRowContext(ctx,
		"SELECT user_id FROM files WHERE id=? AND is_deleted=0", fileID,
	).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", sql.ErrNoRows
	}
	if err != nil {
		return "", err
	}
	if ownerID == userID {
		return ownerID, nil
	}
	level, err := a.lookupGrantFile(ctx, userID, fileID)
	if err != nil {
		return "", err
	}
	if level < 0 || level < min {
		return "", sql.ErrNoRows
	}
	return ownerID, nil
}

// canAccessFolder mirrors canAccessFile but only returns the error since the
// caller almost always already has the folder ID.
func (a *App) canAccessFolder(ctx context.Context, userID, folderID string, min AccessLevel) error {
	var ownerID string
	err := a.DB.QueryRowContext(ctx,
		"SELECT user_id FROM folders WHERE id=? AND is_deleted=0", folderID,
	).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.ErrNoRows
	}
	if err != nil {
		return err
	}
	if ownerID == userID {
		return nil
	}
	level, err := a.lookupGrantFolder(ctx, userID, folderID)
	if err != nil {
		return err
	}
	if level < 0 || level < min {
		return sql.ErrNoRows
	}
	return nil
}

// lookupGrantFile returns the highest AccessLevel granted to userID on the
// given file. Checks direct file grants AND any grant on an ancestor folder.
// Returns -1 when no grant exists.
func (a *App) lookupGrantFile(ctx context.Context, userID, fileID string) (AccessLevel, error) {
	var perm string
	err := a.DB.QueryRowContext(ctx, `
		WITH RECURSIVE ancestors AS (
			SELECT folder_id AS id FROM files WHERE id = ?
			UNION ALL
			SELECT f.parent_id FROM folders f
			JOIN ancestors a ON f.id = a.id
			WHERE f.parent_id IS NOT NULL
		)
		SELECT g.permission
		FROM share_grants g
		WHERE g.grantee_user_id = ?
		  AND (
		    g.file_id = ?
		    OR g.folder_id IN (SELECT id FROM ancestors WHERE id IS NOT NULL)
		  )
		ORDER BY FIELD(g.permission, 'owner', 'editor', 'viewer') ASC
		LIMIT 1`, fileID, userID, fileID).Scan(&perm)
	if errors.Is(err, sql.ErrNoRows) {
		return -1, nil
	}
	if err != nil {
		return -1, err
	}
	return parseAccessLevel(perm), nil
}

// lookupGrantFolder walks the folder tree upward from folderID checking for
// any grant to userID. Returns the highest matching level or -1.
func (a *App) lookupGrantFolder(ctx context.Context, userID, folderID string) (AccessLevel, error) {
	var perm string
	err := a.DB.QueryRowContext(ctx, `
		WITH RECURSIVE ancestors AS (
			SELECT id FROM folders WHERE id = ?
			UNION ALL
			SELECT f.parent_id FROM folders f
			JOIN ancestors a ON f.id = a.id
			WHERE f.parent_id IS NOT NULL
		)
		SELECT g.permission
		FROM share_grants g
		WHERE g.grantee_user_id = ?
		  AND g.folder_id IN (SELECT id FROM ancestors WHERE id IS NOT NULL)
		ORDER BY FIELD(g.permission, 'owner', 'editor', 'viewer') ASC
		LIMIT 1`, folderID, userID).Scan(&perm)
	if errors.Is(err, sql.ErrNoRows) {
		return -1, nil
	}
	if err != nil {
		return -1, err
	}
	return parseAccessLevel(perm), nil
}

func parseAccessLevel(s string) AccessLevel {
	switch s {
	case "owner":
		return AccessOwner
	case "editor":
		return AccessEditor
	default:
		return AccessViewer
	}
}

func (l AccessLevel) String() string {
	switch l {
	case AccessOwner:
		return "owner"
	case AccessEditor:
		return "editor"
	default:
		return "viewer"
	}
}
