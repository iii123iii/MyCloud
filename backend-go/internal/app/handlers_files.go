package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"mycloud/backend-go/internal/httpapi"
	"mycloud/backend-go/internal/pagination"
)

// handleListFiles serves the file browser. Supports two pagination modes:
//
//  1. **Cursor (preferred)** — pass `?cursor=<opaque>` plus optional `?limit=`.
//     Returns rows AFTER the cursor according to (sort, order) and a
//     next_cursor for the page after. Keyset query is O(limit) regardless
//     of how deep into the list the caller is.
//  2. **Offset (legacy)** — pass `?page=N&page_size=M`. Returns the total
//     row count so the client can render "Page 3 of 47". Marked deprecated
//     via a response header; clients should migrate to cursor mode.
//
// Both modes share the same WHERE chain and ORDER BY so callers get
// consistent results regardless of which they use. The legacy mode also
// emits `next_cursor` to ease incremental migration.
func (a *App) handleListFiles(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	folderID := r.URL.Query().Get("folder_id")
	sort := r.URL.Query().Get("sort")
	order := strings.ToUpper(r.URL.Query().Get("order"))
	all := r.URL.Query().Get("all") == "1"
	starred := r.URL.Query().Get("starred_only") == "1"
	sharedWithMe := r.URL.Query().Get("shared_with_me") == "1"

	allowedSort := map[string]string{
		"name":       "name",
		"size_bytes": "size_bytes",
		"created_at": "created_at",
		"updated_at": "updated_at",
	}
	if allowedSort[sort] == "" {
		sort = "name"
	}
	if order != "DESC" {
		order = "ASC"
	}
	sortColumn := allowedSort[sort]

	cursorToken := r.URL.Query().Get("cursor")
	limit := qInt(r, "limit", 0, 1, 200)
	page := qInt(r, "page", 1, 1, 100000)
	pageSize := qInt(r, "page_size", 50, 1, 200)
	offset := (page - 1) * pageSize

	useCursor := cursorToken != "" || limit > 0
	if limit == 0 {
		limit = pageSize
	}

	if sharedWithMe {
		a.listSharedFiles(w, r, userID, sortColumn, order, page, pageSize, offset)
		return
	}

	// Resolve whose files to list. A folder_id pointing at a folder owned by
	// someone else but granted to the requester lists the OWNER's files in it;
	// canAccessFolder is the security boundary. Own root / all-files / starred
	// views stay scoped to the requester.
	listOwnerID := userID
	effPerm := "owner"
	if folderID != "" && !all {
		ownerID, aerr := a.canAccessFolder(r.Context(), userID, folderID, AccessViewer)
		if aerr != nil {
			if errors.Is(aerr, sql.ErrNoRows) {
				httpapi.Error(w, http.StatusNotFound, "not_found", "Folder not found")
				return
			}
			respondDBError(w, r, aerr)
			return
		}
		listOwnerID = ownerID
		if ownerID != userID {
			perm, perr := a.folderGrantLevel(r.Context(), userID, ownerID, folderID)
			if perr != nil {
				respondDBError(w, r, perr)
				return
			}
			effPerm = perm
		}
	}
	shared := listOwnerID != userID

	cur, err := pagination.Decode(cursorToken)
	if err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_cursor", "Cursor is malformed; drop it and re-request page 1")
		return
	}
	// Validate cursor still matches the requested sort/order — clients that
	// flip the sort header on an existing cursor would otherwise see
	// undefined results. Treat as fresh page.
	if cur.ID != "" && (cur.Sort != sort || cur.Order != order) {
		cur = pagination.Cursor{}
	}

	where := " WHERE user_id=? AND is_deleted=0"
	args := []any{listOwnerID}
	if starred {
		where += " AND is_starred=1"
	}
	if !all {
		if folderID == "" {
			where += " AND folder_id IS NULL"
		} else {
			where += " AND folder_id=?"
			args = append(args, folderID)
		}
	}

	// Only the offset path needs a total — cursor mode is "give me what's
	// next" and doesn't promise pre-known counts.
	var total int
	if !useCursor {
		if err := a.DB.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM files"+where, args...).Scan(&total); err != nil {
			respondDBError(w, r, err)
			return
		}
	}

	// Append cursor predicate after the base WHERE so the existing args
	// stay aligned with their placeholders.
	cursorFrag, cursorArgs := pagination.WhereClause(cur, sortColumn)
	where += cursorFrag
	args = append(args, cursorArgs...)

	query := "SELECT id, name, size_bytes, mime_type, folder_id, is_starred, created_at, updated_at FROM files" +
		where + " ORDER BY " + sortColumn + " " + order + ", id " + order + " LIMIT ?"
	queryArgs := append(args, limit)
	if !useCursor {
		query += " OFFSET ?"
		queryArgs = append(queryArgs, offset)
	}
	rows, err := a.DB.QueryContext(r.Context(), query, queryArgs...)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	defer rows.Close()

	files := make([]map[string]any, 0)
	var lastID, lastSortVal string
	for rows.Next() {
		var id, name, mimeType, createdAt, updatedAt string
		var sizeBytes int64
		var folder sql.NullString
		var starred bool
		if err := rows.Scan(&id, &name, &sizeBytes, &mimeType, &folder, &starred, &createdAt, &updatedAt); err != nil {
			respondDBError(w, r, err)
			return
		}
		item := map[string]any{
			"id":         id,
			"name":       name,
			"size_bytes": sizeBytes,
			"mime_type":  mimeType,
			"is_starred": starred,
			"created_at": createdAt,
			"updated_at": updatedAt,
		}
		if folder.Valid {
			item["folder_id"] = folder.String
		}
		if shared {
			item["owner_id"] = listOwnerID
			item["permission"] = effPerm
			item["shared"] = true
		}
		files = append(files, item)
		lastID = id
		// Capture the sort field's string-encoded value for the next cursor.
		switch sortColumn {
		case "name":
			lastSortVal = name
		case "size_bytes":
			// Decimal string keeps the cursor stable across signed/unsigned
			// JSON parsing on the client side.
			lastSortVal = strconv.FormatInt(sizeBytes, 10)
		case "created_at":
			lastSortVal = createdAt
		case "updated_at":
			lastSortVal = updatedAt
		}
	}

	nextCursor := ""
	if len(files) == limit && lastID != "" {
		nextCursor = pagination.Encode(pagination.Cursor{
			Sort: sort, Order: order, Value: lastSortVal, ID: lastID,
		})
	}

	meta := map[string]any{
		"page_size":   limit,
		"next_cursor": nextCursor,
	}
	if !useCursor {
		// Preserve legacy fields. Deprecation header nudges clients to migrate.
		w.Header().Set("X-Pagination-Deprecated-Offset", "use ?cursor= and ?limit= instead")
		meta["total"] = total
		meta["page"] = page
		meta["has_more"] = page*pageSize < total
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"files": files}, meta)
}


func (a *App) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	folderID, filePart, err := a.parseUpload(r)
	if err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	filename := filePart.FileName()
	if filename == "" {
		filename = "untitled"
	}
	data, err := a.storeUploadedFile(r.Context(), userID, filename, folderID, filePart)
	if err != nil {
		status := http.StatusInternalServerError
		code := "upload_failed"
		switch {
		case errors.Is(err, ErrQuotaExceeded):
			status = http.StatusRequestEntityTooLarge
			code = "quota_exceeded"
		case errors.Is(err, ErrInvalidFilename):
			status = http.StatusBadRequest
			code = "invalid_filename"
		case errors.Is(err, ErrFolderNotFound):
			status = http.StatusBadRequest
			code = "invalid_parent"
		}
		httpapi.Error(w, status, code, err.Error())
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "file.upload", "file", data["id"].(string), clientIP(r), map[string]any{"name": filename})
	if mime, _ := data["mime_type"].(string); mime != "" {
		a.afterUploadCommit(r.Context(), data["id"].(string), mime)
	}
	httpapi.JSON(w, http.StatusCreated, data, nil)
}

// listSharedFiles returns files directly granted to the caller via share_grants.
// Renders a flat list; navigation into shared folders is not yet supported on
// the backend.
func (a *App) listSharedFiles(w http.ResponseWriter, r *http.Request, userID, sortCol, order string, page, pageSize, offset int) {
	var total int
	if err := a.DB.QueryRowContext(r.Context(), `
		SELECT COUNT(*) FROM files f
		JOIN share_grants g ON g.file_id = f.id
		WHERE g.grantee_user_id=? AND f.is_deleted=0`, userID).Scan(&total); err != nil {
		respondDBError(w, r, err)
		return
	}
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT f.id, f.name, f.size_bytes, f.mime_type, f.folder_id, f.is_starred,
		       f.created_at, f.updated_at, g.permission, f.user_id
		FROM files f
		JOIN share_grants g ON g.file_id = f.id
		WHERE g.grantee_user_id=? AND f.is_deleted=0
		ORDER BY `+sortCol+` `+order+` LIMIT ? OFFSET ?`, userID, pageSize, offset)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	defer rows.Close()
	files := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, mimeType, createdAt, updatedAt, permission, ownerID string
		var sizeBytes int64
		var folder sql.NullString
		var starred bool
		if err := rows.Scan(&id, &name, &sizeBytes, &mimeType, &folder, &starred, &createdAt, &updatedAt, &permission, &ownerID); err != nil {
			respondDBError(w, r, err)
			return
		}
		item := map[string]any{
			"id":         id,
			"name":       name,
			"size_bytes": sizeBytes,
			"mime_type":  mimeType,
			"is_starred": starred,
			"created_at": createdAt,
			"updated_at": updatedAt,
			"shared":     true,
			"permission": permission,
			"owner_id":   ownerID,
		}
		if folder.Valid {
			item["folder_id"] = folder.String
		}
		files = append(files, item)
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"files": files}, map[string]any{
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"has_more":  page*pageSize < total,
	})
}

func (a *App) handleStorageStats(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	var used, quota, filesCount, folderCount int64
	// Single round-trip via correlated subqueries — previously three serial
	// QueryRowContext calls, two of them silently swallowing errors. Lower
	// dashboard latency and reliable error reporting in one move.
	if err := a.DB.QueryRowContext(r.Context(), `
		SELECT u.used_bytes, u.quota_bytes,
		       (SELECT COUNT(*) FROM files   WHERE user_id = u.id AND is_deleted = 0),
		       (SELECT COUNT(*) FROM folders WHERE user_id = u.id AND is_deleted = 0)
		FROM users u WHERE u.id = ?`, userID).
		Scan(&used, &quota, &filesCount, &folderCount); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "User not found")
			return
		}
		respondDBError(w, r, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{
		"used_bytes":   used,
		"quota_bytes":  quota,
		"file_count":   filesCount,
		"folder_count": folderCount,
	}, nil)
}

func (a *App) handleFileInfo(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	if _, err := a.canAccessFile(r.Context(), userID, id, AccessViewer); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
			return
		}
		respondDBError(w, r, err)
		return
	}
	var fileID, name, mimeType, createdAt, updatedAt string
	var sizeBytes int64
	var folder sql.NullString
	var starred, deleted bool
	err := a.DB.QueryRowContext(r.Context(), `
		SELECT id, name, size_bytes, mime_type, folder_id, is_starred, is_deleted, created_at, updated_at
		FROM files WHERE id=?`, id).
		Scan(&fileID, &name, &sizeBytes, &mimeType, &folder, &starred, &deleted, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
		return
	}
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	data := map[string]any{"id": fileID, "name": name, "size_bytes": sizeBytes, "mime_type": mimeType, "is_starred": starred, "is_deleted": deleted, "created_at": createdAt, "updated_at": updatedAt}
	if folder.Valid {
		data["folder_id"] = folder.String
	}
	httpapi.JSON(w, http.StatusOK, data, nil)
}

func (a *App) handleUpdateFile(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	var payload map[string]json.RawMessage
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	if len(payload) == 0 {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Nothing to update")
		return
	}
	switch {
	case payload["is_starred"] != nil:
		var isStarred bool
		if err := json.Unmarshal(payload["is_starred"], &isStarred); err != nil {
			httpapi.Error(w, http.StatusBadRequest, "validation_error", "is_starred must be a boolean")
			return
		}
		// is_starred is an owner-global column on the file row, so only the
		// owner may toggle it — a grantee starring would flip it for everyone.
		ownerID, aerr := a.canAccessFile(r.Context(), userID, id, AccessViewer)
		if aerr != nil {
			if errors.Is(aerr, sql.ErrNoRows) {
				httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
				return
			}
			respondDBError(w, r, aerr)
			return
		}
		if ownerID != userID {
			httpapi.Error(w, http.StatusForbidden, "forbidden", "Only the owner can star this file")
			return
		}
		res, err := a.DB.ExecContext(r.Context(), "UPDATE files SET is_starred=? WHERE id=? AND is_deleted=0", isStarred, id)
		if err != nil {
			respondDBError(w, r, err)
			return
		}
		if affected, _ := res.RowsAffected(); affected == 0 {
			httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
			return
		}
	case payload["name"] != nil:
		var name string
		if err := json.Unmarshal(payload["name"], &name); err != nil {
			httpapi.Error(w, http.StatusBadRequest, "validation_error", "name must be a string")
			return
		}
		name = strings.TrimSpace(name)
		if name == "" {
			httpapi.Error(w, http.StatusBadRequest, "validation_error", "File name is required")
			return
		}
		if _, aerr := a.canAccessFile(r.Context(), userID, id, AccessEditor); aerr != nil {
			if errors.Is(aerr, sql.ErrNoRows) {
				httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
				return
			}
			respondDBError(w, r, aerr)
			return
		}
		// Keep name_normalised aligned with name on every rename so
		// fuzzy search continues to index the latest value.
		res, err := a.DB.ExecContext(r.Context(),
			"UPDATE files SET name=?, name_normalised=? WHERE id=? AND is_deleted=0",
			name, normaliseName(name), id)
		if err != nil {
			respondDBError(w, r, err)
			return
		}
		if affected, _ := res.RowsAffected(); affected == 0 {
			httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
			return
		}
	case payload["folder_id"] != nil:
		ownerID, aerr := a.canAccessFile(r.Context(), userID, id, AccessEditor)
		if aerr != nil {
			if errors.Is(aerr, sql.ErrNoRows) {
				httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
				return
			}
			respondDBError(w, r, aerr)
			return
		}
		var folderID *string
		destOwnerID := userID // a nil destination = the mover's own root
		if string(payload["folder_id"]) != "null" {
			var raw string
			if err := json.Unmarshal(payload["folder_id"], &raw); err != nil {
				httpapi.Error(w, http.StatusBadRequest, "validation_error", "folder_id must be a string or null")
				return
			}
			raw = strings.TrimSpace(raw)
			if raw != "" {
				// Destination must be writable. Its owner may differ from the
				// source owner — that's allowed: it's how an editor relocates a
				// shared item into their own drive or another shared folder, and
				// moveFile reassigns ownership + quota accordingly.
				owner, derr := a.canAccessFolder(r.Context(), userID, raw, AccessEditor)
				if derr != nil {
					if errors.Is(derr, sql.ErrNoRows) {
						httpapi.Error(w, http.StatusBadRequest, "invalid_parent", "Folder not found")
						return
					}
					respondDBError(w, r, derr)
					return
				}
				destOwnerID = owner
				folderID = &raw
			}
		}
		if err := a.moveFile(r.Context(), id, folderID, ownerID, destOwnerID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
				return
			}
			if errors.Is(err, ErrQuotaExceeded) {
				httpapi.Error(w, http.StatusRequestEntityTooLarge, "quota_exceeded", "Not enough storage at the destination")
				return
			}
			respondDBError(w, r, err)
			return
		}
	default:
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Nothing to update")
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "file.update", "file", id, clientIP(r), nil)
	httpapi.JSON(w, http.StatusOK, map[string]any{"message": "Updated"}, nil)
}

func (a *App) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	// Authorize as editor; a delete on a shared file releases the OWNER's quota
	// and lands the row in the owner's trash, so resolve the owner first and
	// key the mutations by id.
	ownerID, err := a.canAccessFile(r.Context(), userID, id, AccessEditor)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
			return
		}
		respondDBError(w, r, err)
		return
	}
	// Soft-delete now also releases quota — previously, trashed files still
	// counted against used_bytes, so a user near their cap was stuck and the
	// error message didn't say so. Restoration debits quota again (in
	// handleRestoreTrash) and rejects if the user is now over.
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	defer tx.Rollback()
	var size int64
	if err := tx.QueryRowContext(r.Context(),
		"SELECT size_bytes FROM files WHERE id=? AND is_deleted=0 FOR UPDATE",
		id).Scan(&size); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
			return
		}
		respondDBError(w, r, err)
		return
	}
	if _, err := tx.ExecContext(r.Context(),
		"UPDATE files SET is_deleted=1, deleted_at=NOW() WHERE id=?",
		id); err != nil {
		respondDBError(w, r, err)
		return
	}
	if size > 0 {
		if err := releaseQuota(r.Context(), tx, ownerID, size); err != nil {
			respondDBError(w, r, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		respondDBError(w, r, err)
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "file.delete", "file", id, clientIP(r), nil)
	httpapi.NoContent(w)
}

func (a *App) handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	a.serveFile(w, r, chi.URLParam(r, "id"), "attachment")
}

func (a *App) handlePreviewFile(w http.ResponseWriter, r *http.Request) {
	a.serveFile(w, r, chi.URLParam(r, "id"), "inline")
}
