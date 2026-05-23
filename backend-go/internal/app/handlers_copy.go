package app

// Copy a file or folder the caller can read into a destination they can write
// to (their own drive, or any folder they have editor access to). Unlike a
// move, a copy never touches the source: it creates new rows owned by the
// destination's owner, sharing the underlying encrypted blobs via blob_refs
// (no bytes are re-encrypted or duplicated on disk) and reserving quota against
// the new owner — the same accounting dedup uploads use.
//
// Access model: Viewer on the source is enough to copy (you can copy anything
// you can read); the destination is resolved via resolveUploadOwner, which
// requires Editor on a non-root target and attributes the copy to that folder's
// owner. A nil/empty destination means the caller's own root.

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"mycloud/backend-go/internal/httpapi"
)

// copySuffix inserts " (copy)" before a file's extension (or at the end when
// there is none): "report.pdf" → "report (copy).pdf", "notes" → "notes (copy)".
func copySuffix(name string) string {
	dot := strings.LastIndex(name, ".")
	if dot <= 0 || dot == len(name)-1 {
		return name + " (copy)"
	}
	return name[:dot] + " (copy)" + name[dot:]
}

// copyName returns a name that won't visually collide with an existing
// non-deleted item of the same name in the destination folder for ownerID.
func (a *App) copyName(ctx context.Context, tx *sql.Tx, ownerID, folderID, name string) (string, error) {
	where := "folder_id IS NULL"
	args := []any{ownerID, name}
	if folderID != "" {
		where = "folder_id = ?"
		args = []any{ownerID, name, folderID}
	}
	var n int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM files WHERE user_id=? AND name=? AND is_deleted=0 AND "+where,
		args...).Scan(&n); err != nil {
		return "", err
	}
	if n == 0 {
		return name, nil
	}
	return copySuffix(name), nil
}

// copyFile copies srcFileID into destFolderID ("" = own root). Returns the new
// file id. The new row shares the source blob (ref-counted) and key bundle, so
// it decrypts identically; quota is reserved against the destination owner.
func (a *App) copyFile(ctx context.Context, copierID, srcFileID, destFolderID string) (string, error) {
	if _, err := a.canAccessFile(ctx, copierID, srcFileID, AccessViewer); err != nil {
		return "", err // sql.ErrNoRows when not accessible
	}
	destOwnerID, err := a.resolveUploadOwner(ctx, copierID, destFolderID)
	if err != nil {
		return "", err // ErrFolderNotFound when the destination isn't writable
	}

	var name, mimeType, encKey, iv, tag, storagePath string
	var contentHash sql.NullString
	var size int64
	if err := a.DB.QueryRowContext(ctx, `
		SELECT name, mime_type, encryption_key_enc, encryption_iv, encryption_tag, storage_path, size_bytes, content_sha256
		FROM files WHERE id=? AND is_deleted=0`, srcFileID).
		Scan(&name, &mimeType, &encKey, &iv, &tag, &storagePath, &size, &contentHash); err != nil {
		return "", err
	}

	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	if size > 0 {
		if err := reserveQuota(ctx, tx, destOwnerID, size); err != nil {
			return "", err // ErrQuotaExceeded
		}
	}
	finalName, err := a.copyName(ctx, tx, destOwnerID, destFolderID, name)
	if err != nil {
		return "", err
	}
	newID := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO files (id, name, name_normalised, storage_path, size_bytes, mime_type, user_id, folder_id,
		                   encryption_key_enc, encryption_iv, encryption_tag, content_sha256, is_deleted, is_starred)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0)`,
		newID, finalName, normaliseName(finalName), storagePath, size, mimeType, destOwnerID, nullableString(destFolderID),
		encKey, iv, tag, contentHash); err != nil {
		return "", err
	}
	if err := acquireBlobRef(ctx, tx, storagePath); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return newID, nil
}

// copyFolderInto recursively copies the srcFolderID subtree into destParentID
// ("" = own root). Every folder + file is recreated under the destination's
// owner; files share the source blobs (ref-counted). Quota for the whole
// subtree is reserved up front so a copy that would exceed the destination
// owner's cap is rejected before any rows are written.
func (a *App) copyFolderInto(ctx context.Context, copierID, srcFolderID, destParentID string) (string, error) {
	if _, err := a.canAccessFolder(ctx, copierID, srcFolderID, AccessViewer); err != nil {
		return "", err
	}
	destOwnerID, err := a.resolveUploadOwner(ctx, copierID, destParentID)
	if err != nil {
		return "", err
	}
	srcOwnerID, err := a.folderOwner(ctx, srcFolderID)
	if err != nil {
		return "", err
	}
	// The source subtree is owned by the source's owner (a single-owner subtree
	// invariant the move/create paths maintain), so collectFolderTree resolves
	// it in one walk.
	ids, err := a.collectFolderTree(ctx, srcOwnerID, srcFolderID)
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", sql.ErrNoRows
	}
	ph, args := inClause(ids)

	type fnode struct {
		name   string
		parent sql.NullString
	}
	nodes := make(map[string]fnode, len(ids))
	frows, err := a.DB.QueryContext(ctx,
		"SELECT id, name, parent_id FROM folders WHERE id IN "+ph+" AND is_deleted=0", args...)
	if err != nil {
		return "", err
	}
	for frows.Next() {
		var id, name string
		var parent sql.NullString
		if err := frows.Scan(&id, &name, &parent); err != nil {
			frows.Close()
			return "", err
		}
		nodes[id] = fnode{name: name, parent: parent}
	}
	frows.Close()

	type frec struct {
		name, mime, encKey, iv, tag, path, folderID string
		contentHash                                 sql.NullString
		size                                        int64
	}
	var files []frec
	var total int64
	fr, err := a.DB.QueryContext(ctx, `
		SELECT name, mime_type, encryption_key_enc, encryption_iv, encryption_tag, storage_path,
		       size_bytes, content_sha256, folder_id
		FROM files WHERE folder_id IN `+ph+` AND is_deleted=0`, args...)
	if err != nil {
		return "", err
	}
	for fr.Next() {
		var f frec
		var folder sql.NullString
		if err := fr.Scan(&f.name, &f.mime, &f.encKey, &f.iv, &f.tag, &f.path, &f.size, &f.contentHash, &folder); err != nil {
			fr.Close()
			return "", err
		}
		f.folderID = folder.String
		files = append(files, f)
		total += f.size
	}
	fr.Close()

	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	if total > 0 {
		if err := reserveQuota(ctx, tx, destOwnerID, total); err != nil {
			return "", err // ErrQuotaExceeded
		}
	}

	// New id for every folder in the subtree, so we can rewire parent_id.
	newID := make(map[string]string, len(nodes))
	for oldID := range nodes {
		newID[oldID] = uuid.NewString()
	}
	for oldID, n := range nodes {
		var parent any
		if oldID == srcFolderID {
			parent = nullableString(destParentID) // graft the root under the destination
		} else if n.parent.Valid {
			parent = newID[n.parent.String]
		}
		name := n.name
		if oldID == srcFolderID {
			// Only the visible root needs collision-proofing against the dest.
			cn, err := a.copyFolderName(ctx, tx, destOwnerID, destParentID, name)
			if err != nil {
				return "", err
			}
			name = cn
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO folders (id, name, user_id, parent_id, is_deleted) VALUES (?, ?, ?, ?, 0)",
			newID[oldID], name, destOwnerID, parent); err != nil {
			return "", err
		}
	}
	for _, f := range files {
		dest := newID[f.folderID]
		fid := uuid.NewString()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO files (id, name, name_normalised, storage_path, size_bytes, mime_type, user_id, folder_id,
			                   encryption_key_enc, encryption_iv, encryption_tag, content_sha256, is_deleted, is_starred)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0)`,
			fid, f.name, normaliseName(f.name), f.path, f.size, f.mime, destOwnerID, dest,
			f.encKey, f.iv, f.tag, f.contentHash); err != nil {
			return "", err
		}
		if err := acquireBlobRef(ctx, tx, f.path); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return newID[srcFolderID], nil
}

// copyFolderName mirrors copyName for folders (no extension handling).
func (a *App) copyFolderName(ctx context.Context, tx *sql.Tx, ownerID, parentID, name string) (string, error) {
	where := "parent_id IS NULL"
	args := []any{ownerID, name}
	if parentID != "" {
		where = "parent_id = ?"
		args = []any{ownerID, name, parentID}
	}
	var n int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM folders WHERE user_id=? AND name=? AND is_deleted=0 AND "+where,
		args...).Scan(&n); err != nil {
		return "", err
	}
	if n == 0 {
		return name, nil
	}
	return name + " (copy)", nil
}

// handleCopyFile POST /files/{id}:copy  body: { "folder_id": "<dest>"|null }
func (a *App) handleCopyFile(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	var payload struct {
		FolderID *string `json:"folder_id"`
	}
	// Body is optional — no body / empty means "copy to my root".
	_ = decodeJSON(r, &payload)
	dest := ""
	if payload.FolderID != nil {
		dest = strings.TrimSpace(*payload.FolderID)
	}
	newID, err := a.copyFile(r.Context(), userID, id, dest)
	if err != nil {
		a.respondCopyError(w, r, err)
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "file.copy", "file", newID, clientIP(r), map[string]any{"source": id})
	httpapi.JSON(w, http.StatusCreated, map[string]any{"id": newID}, nil)
}

// handleCopyFolder POST /folders/{id}:copy  body: { "parent_id": "<dest>"|null }
func (a *App) handleCopyFolder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	var payload struct {
		ParentID *string `json:"parent_id"`
	}
	_ = decodeJSON(r, &payload)
	dest := ""
	if payload.ParentID != nil {
		dest = strings.TrimSpace(*payload.ParentID)
	}
	newID, err := a.copyFolderInto(r.Context(), userID, id, dest)
	if err != nil {
		a.respondCopyError(w, r, err)
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "folder.copy", "folder", newID, clientIP(r), map[string]any{"source": id})
	httpapi.JSON(w, http.StatusCreated, map[string]any{"id": newID}, nil)
}

// respondCopyError maps the shared copy/move error set to HTTP statuses.
func (a *App) respondCopyError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		httpapi.Error(w, http.StatusNotFound, "not_found", "Item not found")
	case errors.Is(err, ErrFolderNotFound):
		httpapi.Error(w, http.StatusBadRequest, "invalid_parent", "Destination folder not found")
	case errors.Is(err, ErrQuotaExceeded):
		httpapi.Error(w, http.StatusRequestEntityTooLarge, "quota_exceeded", "Not enough storage to copy")
	default:
		respondDBError(w, r, err)
	}
}
