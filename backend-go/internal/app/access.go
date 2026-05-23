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
// (per-user sharing), this helper will also consult that table for
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

// canAccessFolder mirrors canAccessFile: it returns the folder's owner ID on
// success, or sql.ErrNoRows when the folder does not exist, is in trash, or the
// user has insufficient permission. Shared-folder write paths use the returned
// ownerID to attribute new rows / quota / trash to the resource owner rather
// than the acting grantee.
func (a *App) canAccessFolder(ctx context.Context, userID, folderID string, min AccessLevel) (string, error) {
	var ownerID string
	err := a.DB.QueryRowContext(ctx,
		"SELECT user_id FROM folders WHERE id=? AND is_deleted=0", folderID,
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
	level, err := a.lookupGrantFolder(ctx, userID, folderID)
	if err != nil {
		return "", err
	}
	if level < 0 || level < min {
		return "", sql.ErrNoRows
	}
	return ownerID, nil
}

// folderOwner returns the user_id that owns a (non-deleted) folder. Used by
// paths that have already established access via canAccessFolder and just need
// the owner without re-running the grant lookup.
func (a *App) folderOwner(ctx context.Context, folderID string) (string, error) {
	var ownerID string
	err := a.DB.QueryRowContext(ctx,
		"SELECT user_id FROM folders WHERE id=? AND is_deleted=0", folderID,
	).Scan(&ownerID)
	return ownerID, err
}

// folderGrantLevel resolves the caller's effective permission string
// ("owner"|"editor"|"viewer") on a folder they can access. ownerID is the
// folder's owner (from canAccessFolder); when it equals userID the caller is
// the owner. Returns "" when there is no grant (should not happen after a
// successful canAccessFolder for a non-owner).
func (a *App) folderGrantLevel(ctx context.Context, userID, ownerID, folderID string) (string, error) {
	if ownerID == userID {
		return "owner", nil
	}
	level, err := a.lookupGrantFolder(ctx, userID, folderID)
	if err != nil {
		return "", err
	}
	if level < 0 {
		return "", nil
	}
	return level.String(), nil
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

// grantedFolderSet returns the set of folder IDs directly granted to userID
// (the share boundaries). Used by handleFolderPath to truncate a grantee's
// breadcrumb at the share root rather than revealing the owner's ancestors.
func (a *App) grantedFolderSet(ctx context.Context, userID string) (map[string]bool, error) {
	rows, err := a.DB.QueryContext(ctx,
		"SELECT folder_id FROM share_grants WHERE grantee_user_id=? AND folder_id IS NOT NULL", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		set[id] = true
	}
	return set, rows.Err()
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
