package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"mycloud/backend-go/internal/httpapi"
)

func (a *App) handleListFolders(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	parentID := r.URL.Query().Get("parent_id")
	if r.URL.Query().Get("shared_with_me") == "1" {
		a.listSharedFolders(w, r, userID)
		return
	}
	// Resolve whose children to list. For the user's own root/subfolders that's
	// the requester; for a shared subfolder (parent owned by someone else but
	// granted to the requester) we list the OWNER's children — access to the
	// parent implies access to all of its children. canAccessFolder is the
	// security boundary: once it passes, listing by the resolved owner (rather
	// than the requester) is safe and is what surfaces the shared contents.
	// A missing/zero-access parent 404s — same contract as before, and closes
	// the enumeration oracle between "empty folder" and "no such folder".
	listOwnerID := userID
	effPerm := "owner"
	if parentID != "" {
		ownerID, err := a.canAccessFolder(r.Context(), userID, parentID, AccessViewer)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Error(w, http.StatusNotFound, "not_found", "Parent folder not found")
				return
			}
			respondDBError(w, r, err)
			return
		}
		listOwnerID = ownerID
		if ownerID != userID {
			perm, perr := a.folderGrantLevel(r.Context(), userID, ownerID, parentID)
			if perr != nil {
				respondDBError(w, r, perr)
				return
			}
			effPerm = perm
		}
	}
	shared := listOwnerID != userID
	// LEFT JOIN user_favorites so each row carries is_favorited inline. This
	// is the surface the "star a folder" UI reads from — there's no separate
	// is_starred column on folders; favourites is the equivalent concept. The
	// favourites join stays keyed on the REQUESTER so a grantee's own pins
	// surface even while browsing someone else's tree; the WHERE clause is
	// keyed on the resolved owner.
	query := `
		SELECT f.id, f.name, f.parent_id, f.created_at, f.updated_at,
		       CASE WHEN uf.folder_id IS NULL THEN 0 ELSE 1 END AS is_favorited
		FROM folders f
		LEFT JOIN user_favorites uf
		       ON uf.folder_id = f.id AND uf.user_id = ?
		WHERE f.user_id=? AND f.is_deleted=0`
	args := []any{userID, listOwnerID}
	if parentID == "" {
		query += " AND f.parent_id IS NULL"
	} else {
		query += " AND f.parent_id=?"
		args = append(args, parentID)
	}
	query += " ORDER BY f.name ASC"

	rows, err := a.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	defer rows.Close()
	folders := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, createdAt, updatedAt string
		var parent sql.NullString
		var favored int
		if err := rows.Scan(&id, &name, &parent, &createdAt, &updatedAt, &favored); err != nil {
			respondDBError(w, r, err)
			return
		}
		item := map[string]any{
			"id":           id,
			"name":         name,
			"created_at":   createdAt,
			"updated_at":   updatedAt,
			"is_favorited": favored == 1,
		}
		if parent.Valid {
			item["parent_id"] = parent.String
		}
		if shared {
			item["owner_id"] = listOwnerID
			item["permission"] = effPerm
			item["shared"] = true
		}
		folders = append(folders, item)
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"folders": folders}, nil)
}

// listSharedFolders returns folders directly granted to the caller.
// Flat list — backend doesn't yet expose navigation into shared folder trees.
func (a *App) listSharedFolders(w http.ResponseWriter, r *http.Request, userID string) {
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT fo.id, fo.name, fo.parent_id, fo.user_id, fo.created_at, fo.updated_at, g.permission
		FROM folders fo
		JOIN share_grants g ON g.folder_id = fo.id
		WHERE g.grantee_user_id=? AND fo.is_deleted=0
		ORDER BY fo.name ASC`, userID)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, ownerID, createdAt, updatedAt, permission string
		var parent sql.NullString
		if err := rows.Scan(&id, &name, &parent, &ownerID, &createdAt, &updatedAt, &permission); err != nil {
			respondDBError(w, r, err)
			return
		}
		item := map[string]any{
			"id":         id,
			"name":       name,
			"owner_id":   ownerID,
			"created_at": createdAt,
			"updated_at": updatedAt,
			"shared":     true,
			"permission": permission,
		}
		if parent.Valid {
			item["parent_id"] = parent.String
		}
		out = append(out, item)
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"folders": out}, nil)
}

func (a *App) handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Name     string  `json:"name"`
		ParentID *string `json:"parent_id"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.Name == "" {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Folder name is required")
		return
	}
	userID := userIDFrom(r)
	id := uuid.NewString()
	// New folders are owned by the parent's owner, so a collaborator creating
	// inside a shared folder adds into the owner's tree (counts against the
	// owner's quota, survives a grant revoke). A root-level create has no
	// parent and is owned by the actor.
	ownerID := userID
	if payload.ParentID != nil && *payload.ParentID != "" {
		owner, err := a.canAccessFolder(r.Context(), userID, *payload.ParentID, AccessEditor)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Error(w, http.StatusBadRequest, "invalid_parent", "Parent folder not found")
				return
			}
			respondDBError(w, r, err)
			return
		}
		ownerID = owner
		// Depth cap: refuse trees deeper than MaxFolderDepth. Without this,
		// nothing prevents a user from building a 10k-deep chain that DoSes
		// every recursive CTE in the app (folder path, access checks).
		depth, err := a.folderDepth(r.Context(), *payload.ParentID)
		if err != nil {
			respondDBError(w, r, err)
			return
		}
		if depth >= MaxFolderDepth {
			httpapi.Error(w, http.StatusBadRequest, "folder_too_deep",
				"Folder nesting limit reached")
			return
		}
	}
	_, err := a.DB.ExecContext(r.Context(),
		"INSERT INTO folders (id, name, user_id, parent_id, is_deleted) VALUES (?, ?, ?, ?, 0)",
		id, payload.Name, ownerID, payload.ParentID)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "folder.create", "folder", id, clientIP(r), map[string]any{"name": payload.Name})
	httpapi.JSON(w, http.StatusCreated, map[string]any{"id": id, "name": payload.Name, "parent_id": payload.ParentID}, nil)
}

func (a *App) handleGetFolder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	ownerID, err := a.canAccessFolder(r.Context(), userID, id, AccessViewer)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "Folder not found")
			return
		}
		respondDBError(w, r, err)
		return
	}
	var folderID, name, createdAt, updatedAt string
	var parent sql.NullString
	err = a.DB.QueryRowContext(r.Context(), `
		SELECT id, name, parent_id, created_at, updated_at
		FROM folders WHERE id=? AND is_deleted=0`, id).
		Scan(&folderID, &name, &parent, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Folder not found")
		return
	}
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	data := map[string]any{"id": folderID, "name": name, "created_at": createdAt, "updated_at": updatedAt}
	if parent.Valid {
		data["parent_id"] = parent.String
	}
	if ownerID != userID {
		perm, perr := a.folderGrantLevel(r.Context(), userID, ownerID, id)
		if perr != nil {
			respondDBError(w, r, perr)
			return
		}
		data["owner_id"] = ownerID
		data["permission"] = perm
		data["shared"] = true
	}
	httpapi.JSON(w, http.StatusOK, data, nil)
}

func (a *App) handleFolderPath(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	currentID := chi.URLParam(r, "id")

	// Access check resolves the folder's owner and authorizes the caller (owner
	// or grantee). For a grantee we walk the OWNER's tree, then truncate the
	// chain at the share boundary so ancestors above the grant aren't revealed.
	ownerID, err := a.canAccessFolder(r.Context(), userID, currentID, AccessViewer)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "Folder not found")
			return
		}
		respondDBError(w, r, err)
		return
	}
	shared := ownerID != userID

	// Single recursive CTE walks the chain in one round-trip, scoped to the
	// owner. Depth cap protects the planner; the CREATE/UPDATE handlers also
	// reject trees deeper than MaxFolderDepth.
	rows, err := a.DB.QueryContext(r.Context(), `
		WITH RECURSIVE chain AS (
		    SELECT id, name, parent_id, created_at, updated_at, 0 AS depth
		    FROM folders WHERE id = ? AND user_id = ? AND is_deleted = 0
		    UNION ALL
		    SELECT f.id, f.name, f.parent_id, f.created_at, f.updated_at, c.depth + 1
		    FROM folders f JOIN chain c ON f.id = c.parent_id
		    WHERE f.user_id = ? AND f.is_deleted = 0 AND c.depth < 256
		)
		SELECT id, name, parent_id, created_at, updated_at FROM chain ORDER BY depth DESC`,
		currentID, ownerID, ownerID)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	defer rows.Close()
	path := make([]map[string]any, 0, 8)
	for rows.Next() {
		var id, name, createdAt, updatedAt string
		var parent sql.NullString
		if err := rows.Scan(&id, &name, &parent, &createdAt, &updatedAt); err != nil {
			respondDBError(w, r, err)
			return
		}
		item := map[string]any{
			"id":         id,
			"name":       name,
			"created_at": createdAt,
			"updated_at": updatedAt,
		}
		if parent.Valid {
			item["parent_id"] = parent.String
		}
		path = append(path, item)
	}
	if err := rows.Err(); err != nil {
		respondDBError(w, r, err)
		return
	}
	if len(path) == 0 {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Folder not found")
		return
	}

	if shared {
		// Truncate at the share boundary: keep from the HIGHEST directly-granted
		// ancestor (closest to root, i.e. earliest in this root-first ordering)
		// down to currentID, hiding the owner's folders above the share. Every
		// node kept is a descendant of that granted folder, so it's accessible.
		granted, gerr := a.grantedFolderSet(r.Context(), userID)
		if gerr != nil {
			respondDBError(w, r, gerr)
			return
		}
		boundary := -1
		for i, item := range path {
			if granted[item["id"].(string)] {
				boundary = i
				break
			}
		}
		if boundary < 0 {
			// Defensive: folder access is only granted via folder grants, so a
			// boundary must exist on the chain. If not, don't leak it.
			httpapi.Error(w, http.StatusNotFound, "not_found", "Folder not found")
			return
		}
		path = path[boundary:]
		// The new top crumb is the share root; drop its real parent so the UI
		// cannot navigate above it.
		delete(path[0], "parent_id")

		perm, perr := a.folderGrantLevel(r.Context(), userID, ownerID, currentID)
		if perr != nil {
			respondDBError(w, r, perr)
			return
		}
		for _, item := range path {
			item["owner_id"] = ownerID
			item["shared"] = true
			item["permission"] = perm
		}
	}

	httpapi.JSON(w, http.StatusOK, map[string]any{"folders": path}, nil)
}

// MaxFolderDepth caps how deep the folder tree can grow. Tuned to be well
// above any reasonable real-world layout while staying below the CTE
// recursion limits we use in handleFolderPath / access checks.
const MaxFolderDepth = 64

// folderDepth returns the ancestor-count for parentID (0 = root, 1 = parent
// is root, etc.). Used by create/update handlers to reject trees deeper
// than MaxFolderDepth.
func (a *App) folderDepth(ctx context.Context, parentID string) (int, error) {
	if parentID == "" {
		return 0, nil
	}
	var depth sql.NullInt64
	err := a.DB.QueryRowContext(ctx, `
		WITH RECURSIVE chain AS (
		    SELECT id, parent_id, 1 AS depth FROM folders WHERE id = ?
		    UNION ALL
		    SELECT f.id, f.parent_id, c.depth + 1
		    FROM folders f JOIN chain c ON f.id = c.parent_id
		    WHERE c.depth < 256
		)
		SELECT MAX(depth) FROM chain`, parentID).Scan(&depth)
	if err != nil {
		return 0, err
	}
	return int(depth.Int64), nil
}

func (a *App) handleUpdateFolder(w http.ResponseWriter, r *http.Request) {
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
	// Resolve owner + authorize as editor. Shared-folder collaborators act on
	// the OWNER's rows (the mutations below are keyed by id, with user_id
	// dropped now that access is proven).
	ownerID, err := a.canAccessFolder(r.Context(), userID, id, AccessEditor)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "Folder not found")
			return
		}
		respondDBError(w, r, err)
		return
	}
	// Each case writes its own activity entry with a non-nil details payload
	// (rename → {name}, move → {old_parent_id, new_parent_id}). Frontend WS
	// subscribers rely on those fields to invalidate the right SWR caches.
	switch {
	case payload["name"] != nil:
		var name string
		if err := json.Unmarshal(payload["name"], &name); err != nil {
			httpapi.Error(w, http.StatusBadRequest, "validation_error", "name must be a string")
			return
		}
		name = strings.TrimSpace(name)
		if name == "" {
			httpapi.Error(w, http.StatusBadRequest, "validation_error", "Folder name is required")
			return
		}
		res, err := a.DB.ExecContext(r.Context(), "UPDATE folders SET name=? WHERE id=? AND is_deleted=0", name, id)
		if err != nil {
			respondDBError(w, r, err)
			return
		}
		if affected, _ := res.RowsAffected(); affected == 0 {
			httpapi.Error(w, http.StatusNotFound, "not_found", "Folder not found")
			return
		}
		writeActivity(r.Context(), a.DB, &userID, "folder.update", "folder", id, clientIP(r),
			map[string]any{"name": name})
	case payload["parent_id"] != nil:
		// Parse the requested new parent. nil = move to root.
		var parentID *string
		if string(payload["parent_id"]) != "null" {
			var raw string
			if err := json.Unmarshal(payload["parent_id"], &raw); err != nil {
				httpapi.Error(w, http.StatusBadRequest, "validation_error", "parent_id must be a string or null")
				return
			}
			raw = strings.TrimSpace(raw)
			if raw == id {
				httpapi.Error(w, http.StatusBadRequest, "validation_error", "Folder cannot be moved into itself")
				return
			}
			if raw != "" {
				parentID = &raw
			}
		}

		// One transaction so the cycle check and the relocation (or cross-owner
		// subtree reassignment) stay atomic against concurrent moves.
		tx, err := a.DB.BeginTx(r.Context(), nil)
		if err != nil {
			respondDBError(w, r, err)
			return
		}
		defer func() { _ = tx.Rollback() }()

		// Snapshot the old parent for the activity payload (subscribers use
		// old + new to invalidate the affected listings).
		var oldParent sql.NullString
		if err := tx.QueryRowContext(r.Context(),
			"SELECT parent_id FROM folders WHERE id=? AND is_deleted=0", id,
		).Scan(&oldParent); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Error(w, http.StatusNotFound, "not_found", "Folder not found")
			} else {
				respondDBError(w, r, err)
			}
			return
		}

		destOwnerID := userID // a nil parent = the mover's own root
		if parentID != nil {
			// Destination must be writable; its owner may differ from the source
			// owner — cross-owner relocation reassigns the subtree below.
			owner, derr := a.canAccessFolder(r.Context(), userID, *parentID, AccessEditor)
			if derr != nil {
				if errors.Is(derr, sql.ErrNoRows) {
					httpapi.Error(w, http.StatusBadRequest, "invalid_parent", "Parent folder not found")
					return
				}
				respondDBError(w, r, derr)
				return
			}
			destOwnerID = owner
			// Depth cap: refuse moves that would push the destination
			// chain past MaxFolderDepth. Mirrors handleCreateFolder.
			pdepth, derr := a.folderDepth(r.Context(), *parentID)
			if derr != nil {
				respondDBError(w, r, derr)
				return
			}
			if pdepth >= MaxFolderDepth {
				httpapi.Error(w, http.StatusBadRequest, "folder_too_deep",
					"Folder nesting limit reached")
				return
			}
			// Cycle / descendant check: a folder can't be moved into its own
			// subtree. Walks the source owner's subtree from id.
			rows, qerr := tx.QueryContext(r.Context(), `
				WITH RECURSIVE folder_tree AS (
					SELECT id FROM folders WHERE id=? AND user_id=?
					UNION ALL
					SELECT f.id FROM folders f JOIN folder_tree ft ON f.parent_id = ft.id WHERE f.user_id=?
				)
				SELECT id FROM folder_tree`, id, ownerID, ownerID)
			if qerr != nil {
				respondDBError(w, r, qerr)
				return
			}
			cycle := false
			for rows.Next() {
				var descendantID string
				if err := rows.Scan(&descendantID); err != nil {
					rows.Close()
					respondDBError(w, r, err)
					return
				}
				if descendantID == *parentID {
					cycle = true
				}
			}
			rows.Close()
			if cycle {
				httpapi.Error(w, http.StatusBadRequest, "validation_error", "Folder cannot be moved into its descendant")
				return
			}
		}

		if destOwnerID == ownerID {
			// Same owner: a plain re-parent.
			res, err := tx.ExecContext(r.Context(),
				"UPDATE folders SET parent_id=? WHERE id=? AND is_deleted=0",
				nullableStringPtr(parentID), id)
			if err != nil {
				respondDBError(w, r, err)
				return
			}
			if affected, _ := res.RowsAffected(); affected == 0 {
				httpapi.Error(w, http.StatusNotFound, "not_found", "Folder not found")
				return
			}
		} else {
			// Cross-owner: reassign the whole subtree's ownership + quota.
			ids, cerr := a.collectFolderTree(r.Context(), ownerID, id)
			if cerr != nil {
				respondDBError(w, r, cerr)
				return
			}
			if len(ids) == 0 {
				httpapi.Error(w, http.StatusNotFound, "not_found", "Folder not found")
				return
			}
			if rerr := a.reassignFolderTreeTx(r.Context(), tx, id, parentID, ownerID, destOwnerID, ids); rerr != nil {
				if errors.Is(rerr, ErrQuotaExceeded) {
					httpapi.Error(w, http.StatusRequestEntityTooLarge, "quota_exceeded", "Not enough storage at the destination")
					return
				}
				respondDBError(w, r, rerr)
				return
			}
		}
		if err := tx.Commit(); err != nil {
			respondDBError(w, r, err)
			return
		}

		// Activity payload — both fields can be null (moves to/from root).
		// JSON marshals nil interfaces as `null`, which is the shape frontend
		// subscribers branch on.
		var oldParentVal, newParentVal any
		if oldParent.Valid {
			oldParentVal = oldParent.String
		}
		if parentID != nil {
			newParentVal = *parentID
		}
		writeActivity(r.Context(), a.DB, &userID, "folder.update", "folder", id, clientIP(r),
			map[string]any{"old_parent_id": oldParentVal, "new_parent_id": newParentVal})
	default:
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Nothing to update")
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"message": "Updated"}, nil)
}

func (a *App) handleDeleteFolder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	// Authorize as editor; a delete on a shared folder operates on the OWNER's
	// subtree and releases the owner's quota, so pass the owner id into
	// markFolderDeleted. The trashed items land in the owner's trash.
	ownerID, err := a.canAccessFolder(r.Context(), userID, id, AccessEditor)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "Folder not found")
			return
		}
		respondDBError(w, r, err)
		return
	}
	if err := a.markFolderDeleted(r.Context(), ownerID, id); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "delete_failed", err.Error())
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "folder.delete", "folder", id, clientIP(r), nil)
	httpapi.NoContent(w)
}
