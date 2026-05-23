package app

// POST /api/v2/files:batch — bulk actions on a list of file IDs.
//
// Frontend "Select 50 files, click Move" used to fire 50 sequential
// HTTP requests, each its own auth round-trip, query, and activity_log
// write. This endpoint cuts the chatter to one request and reports
// per-item success/failure so a partial set of permission errors doesn't
// fail the whole batch.
//
// Wire format:
//
//	{ "action": "delete" | "star" | "unstar" | "move" | "trash" | "restore",
//	  "file_ids":   [...],         // optional — files to operate on
//	  "folder_ids": [...],         // optional — folders to operate on (same action)
//	  "params": { ... }            // action-specific extra fields
//	}
//
// For folders, "star"/"unstar" toggles the user_favorites pin (the sidebar
// favourites list) rather than a folder-level is_starred column. There is no
// is_starred on folders — favourites is the equivalent surface.
//
// Response:
//
//	{ "results": [{"id": "...", "ok": true} | {"id": "...", "ok": false, "error": "..."}, ...],
//	  "operation_id": "<uuid>"     // groups all activity_log rows from this batch
//	}

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"mycloud/backend-go/internal/httpapi"
)

type batchRequest struct {
	Action    string          `json:"action"`
	FileIDs   []string        `json:"file_ids"`
	FolderIDs []string        `json:"folder_ids"`
	Params    json.RawMessage `json:"params,omitempty"`
}

type batchItemResult struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// handleFilesBatch dispatches one of the supported actions across the file
// IDs. We deliberately do NOT wrap the whole batch in a single transaction:
// 500 file moves in one tx holds locks too long, and a single failure
// rolling back 499 successes is rarely what the user wants. Each item is
// its own short transaction; results are reported per-item.
func (a *App) handleFilesBatch(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	var req batchRequest
	if err := decodeJSON(r, &req); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if len(req.FileIDs) == 0 && len(req.FolderIDs) == 0 {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "file_ids or folder_ids is required")
		return
	}
	if len(req.FileIDs)+len(req.FolderIDs) > 1000 {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "batch capped at 1000 items")
		return
	}

	// One operation_id for the entire batch so the rows can be grouped + rolled back as a unit.
	opID := uuid.NewString()
	ip := clientIP(r)
	results := make([]batchItemResult, 0, len(req.FileIDs)+len(req.FolderIDs))

	switch req.Action {
	case "star", "unstar":
		want := req.Action == "star"
		for _, id := range req.FileIDs {
			results = append(results, a.batchSetStarred(r.Context(), userID, id, want, ip, opID))
		}
		// For folders, "star" means "pin to sidebar favourites" — there is no
		// folder.is_starred column. Toggle user_favorites instead.
		for _, id := range req.FolderIDs {
			results = append(results, a.batchSetFolderFavorited(r.Context(), userID, id, want, ip, opID))
		}
	case "trash", "delete":
		// "delete" is alias for soft-delete (trash); permanent purge is via /trash.
		for _, id := range req.FileIDs {
			results = append(results, a.batchSoftDelete(r.Context(), userID, id, ip, opID))
		}
		for _, id := range req.FolderIDs {
			results = append(results, a.batchFolderSoftDelete(r.Context(), userID, id, ip, opID))
		}
	case "restore":
		for _, id := range req.FileIDs {
			results = append(results, a.batchRestore(r.Context(), userID, id, ip, opID))
		}
		for _, id := range req.FolderIDs {
			results = append(results, a.batchFolderRestore(r.Context(), userID, id, ip, opID))
		}
	case "move":
		var p struct {
			FolderID *string `json:"folder_id"`
		}
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &p); err != nil {
				httpapi.Error(w, http.StatusBadRequest, "bad_params", err.Error())
				return
			}
		}
		for _, id := range req.FileIDs {
			results = append(results, a.batchMove(r.Context(), userID, id, p.FolderID, ip, opID))
		}
		for _, id := range req.FolderIDs {
			results = append(results, a.batchFolderMove(r.Context(), userID, id, p.FolderID, ip, opID))
		}
	default:
		httpapi.Error(w, http.StatusBadRequest, "bad_action", "unknown action: "+req.Action)
		return
	}

	httpapi.JSON(w, http.StatusOK, map[string]any{
		"results":      results,
		"operation_id": opID,
	}, nil)
}

// batchSetStarred toggles is_starred. Reversible? No — starring is trivial
// to undo from the UI itself, so we use plain writeActivity to keep noise
// out of the rollback view.
func (a *App) batchSetStarred(ctx context.Context, userID, fileID string, starred bool, ip, opID string) batchItemResult {
	// is_starred is an owner-global column on the file row, so only the owner
	// may toggle it — a grantee starring would flip it for everyone.
	ownerID, err := a.canAccessFile(ctx, userID, fileID, AccessViewer)
	if err != nil {
		return batchItemResult{ID: fileID, OK: false, Error: "no access"}
	}
	if ownerID != userID {
		return batchItemResult{ID: fileID, OK: false, Error: "only the owner can star"}
	}
	res, err := a.DB.ExecContext(ctx,
		`UPDATE files SET is_starred = ? WHERE id = ? AND is_deleted = 0`,
		starred, fileID)
	if err != nil {
		return batchItemResult{ID: fileID, OK: false, Error: err.Error()}
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return batchItemResult{ID: fileID, OK: false, Error: "no rows updated"}
	}
	action := "file_unstar"
	if starred {
		action = "file_star"
	}
	uidPtr := &userID
	writeActivity(ctx, a.DB, uidPtr, action, "file", fileID, ip, map[string]any{"operation_id": opID})
	return batchItemResult{ID: fileID, OK: true}
}

// batchSoftDelete moves a file to trash. Reversible via the audit log
// (rollback will restore is_deleted=0). No old/new payload needed —
// the action's existence is enough; rollback infers the prior state.
func (a *App) batchSoftDelete(ctx context.Context, userID, fileID, ip, opID string) batchItemResult {
	if _, err := a.canAccessFile(ctx, userID, fileID, AccessEditor); err != nil {
		return batchItemResult{ID: fileID, OK: false, Error: "no access"}
	}
	res, err := a.DB.ExecContext(ctx,
		`UPDATE files SET is_deleted = 1, deleted_at = NOW() WHERE id = ? AND is_deleted = 0`,
		fileID)
	if err != nil {
		return batchItemResult{ID: fileID, OK: false, Error: err.Error()}
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return batchItemResult{ID: fileID, OK: false, Error: "already deleted or missing"}
	}
	uidPtr := &userID
	writeActivityWithDiff(ctx, a.DB, uidPtr, "file_soft_delete", "file", fileID, ip, opID,
		map[string]any{"is_deleted": false},
		map[string]any{"is_deleted": true},
		nil,
	)
	return batchItemResult{ID: fileID, OK: true}
}

// batchRestore moves a file out of trash.
func (a *App) batchRestore(ctx context.Context, userID, fileID, ip, opID string) batchItemResult {
	if _, err := a.canAccessFile(ctx, userID, fileID, AccessEditor); err != nil {
		return batchItemResult{ID: fileID, OK: false, Error: "no access"}
	}
	res, err := a.DB.ExecContext(ctx,
		`UPDATE files SET is_deleted = 0, deleted_at = NULL WHERE id = ? AND is_deleted = 1`,
		fileID)
	if err != nil {
		return batchItemResult{ID: fileID, OK: false, Error: err.Error()}
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return batchItemResult{ID: fileID, OK: false, Error: "not in trash"}
	}
	uidPtr := &userID
	writeActivity(ctx, a.DB, uidPtr, "file_restore", "file", fileID, ip, map[string]any{"operation_id": opID})
	return batchItemResult{ID: fileID, OK: true}
}

// batchMove changes folder_id. Captures old/new for rollback.
func (a *App) batchMove(ctx context.Context, userID, fileID string, newFolderID *string, ip, opID string) batchItemResult {
	srcOwnerID, err := a.canAccessFile(ctx, userID, fileID, AccessEditor)
	if err != nil {
		return batchItemResult{ID: fileID, OK: false, Error: "no access"}
	}
	// Resolve the destination owner. A non-root destination must be writable;
	// its owner may differ from the source owner (cross-owner relocation is
	// allowed and reassigns ownership + quota in moveFile). nil/"" = own root.
	var destFolder *string
	destOwnerID := userID
	if newFolderID != nil && *newFolderID != "" {
		owner, derr := a.canAccessFolder(ctx, userID, *newFolderID, AccessEditor)
		if derr != nil {
			if errors.Is(derr, sql.ErrNoRows) {
				return batchItemResult{ID: fileID, OK: false, Error: "destination folder not found"}
			}
			return batchItemResult{ID: fileID, OK: false, Error: "no destination access"}
		}
		destFolder = newFolderID
		destOwnerID = owner
	}
	// Capture current folder for the diff.
	var oldFolder sql.NullString
	if err := a.DB.QueryRowContext(ctx,
		`SELECT folder_id FROM files WHERE id = ? AND is_deleted = 0`, fileID,
	).Scan(&oldFolder); err != nil {
		return batchItemResult{ID: fileID, OK: false, Error: err.Error()}
	}
	if err := a.moveFile(ctx, fileID, destFolder, srcOwnerID, destOwnerID); err != nil {
		if errors.Is(err, ErrQuotaExceeded) {
			return batchItemResult{ID: fileID, OK: false, Error: "not enough storage at the destination"}
		}
		return batchItemResult{ID: fileID, OK: false, Error: err.Error()}
	}
	uidPtr := &userID
	oldVal := map[string]any{"folder_id": nil}
	if oldFolder.Valid {
		oldVal["folder_id"] = oldFolder.String
	}
	newVal := map[string]any{"folder_id": newFolderID}
	writeActivityWithDiff(ctx, a.DB, uidPtr, "file_move", "file", fileID, ip, opID, oldVal, newVal, nil)
	return batchItemResult{ID: fileID, OK: true}
}

// batchSetFolderFavorited toggles a folder's presence in user_favorites —
// the "star" equivalent for folders. INSERT-on-not-exists / DELETE-on-exists
// so a no-op (star a folder that's already a favourite) reports ok=true
// without surprising the caller.
func (a *App) batchSetFolderFavorited(ctx context.Context, userID, folderID string, want bool, ip, opID string) batchItemResult {
	if _, err := a.canAccessFolder(ctx, userID, folderID, AccessViewer); err != nil {
		return batchItemResult{ID: folderID, OK: false, Error: "no access"}
	}
	if want {
		// Position = MAX + 1 so newly-pinned folders appear at the end of the
		// sidebar list, matching handleAddFavorite's behaviour.
		var maxPos sql.NullInt64
		_ = a.DB.QueryRowContext(ctx,
			`SELECT MAX(position) FROM user_favorites WHERE user_id = ?`, userID,
		).Scan(&maxPos)
		pos := int64(0)
		if maxPos.Valid {
			pos = maxPos.Int64 + 1
		}
		if _, err := a.DB.ExecContext(ctx, `
			INSERT INTO user_favorites (user_id, folder_id, position)
			VALUES (?, ?, ?)
			ON DUPLICATE KEY UPDATE position = position`,
			userID, folderID, pos); err != nil {
			return batchItemResult{ID: folderID, OK: false, Error: err.Error()}
		}
	} else {
		if _, err := a.DB.ExecContext(ctx,
			`DELETE FROM user_favorites WHERE user_id = ? AND folder_id = ?`,
			userID, folderID); err != nil {
			return batchItemResult{ID: folderID, OK: false, Error: err.Error()}
		}
	}
	action := "folder_unfavorite"
	if want {
		action = "folder_favorite"
	}
	uidPtr := &userID
	writeActivity(ctx, a.DB, uidPtr, action, "folder", folderID, ip, map[string]any{"operation_id": opID})
	return batchItemResult{ID: folderID, OK: true}
}

// batchFolderSoftDelete moves a folder (and its full subtree of files +
// subfolders) to trash. Reuses markFolderDeleted so the quota release and
// recursive descent stay in one place.
func (a *App) batchFolderSoftDelete(ctx context.Context, userID, folderID, ip, opID string) batchItemResult {
	ownerID, err := a.canAccessFolder(ctx, userID, folderID, AccessEditor)
	if err != nil {
		return batchItemResult{ID: folderID, OK: false, Error: "no access"}
	}
	if err := a.markFolderDeleted(ctx, ownerID, folderID); err != nil {
		return batchItemResult{ID: folderID, OK: false, Error: err.Error()}
	}
	uidPtr := &userID
	writeActivityWithDiff(ctx, a.DB, uidPtr, "folder_soft_delete", "folder", folderID, ip, opID,
		map[string]any{"is_deleted": false},
		map[string]any{"is_deleted": true},
		nil,
	)
	return batchItemResult{ID: folderID, OK: true}
}

// batchFolderRestore pulls a folder subtree back out of trash. Mirrors
// batchRestore but routes through restoreFolder for the quota re-debit.
func (a *App) batchFolderRestore(ctx context.Context, userID, folderID, ip, opID string) batchItemResult {
	ownerID, err := a.canAccessFolder(ctx, userID, folderID, AccessEditor)
	if err != nil {
		return batchItemResult{ID: folderID, OK: false, Error: "no access"}
	}
	if err := a.restoreFolder(ctx, ownerID, folderID); err != nil {
		return batchItemResult{ID: folderID, OK: false, Error: err.Error()}
	}
	uidPtr := &userID
	writeActivity(ctx, a.DB, uidPtr, "folder_restore", "folder", folderID, ip, map[string]any{"operation_id": opID})
	return batchItemResult{ID: folderID, OK: true}
}

// batchFolderMove changes parent_id. Captures old/new for rollback and runs
// the same cycle / depth checks as handleUpdateFolder so a batch can't slip
// past validation a single-folder move would have rejected.
func (a *App) batchFolderMove(ctx context.Context, userID, folderID string, newParentID *string, ip, opID string) batchItemResult {
	ownerID, err := a.canAccessFolder(ctx, userID, folderID, AccessEditor)
	if err != nil {
		return batchItemResult{ID: folderID, OK: false, Error: "no access"}
	}
	// Same-folder no-op guard: refuse to move a folder into itself.
	if newParentID != nil && *newParentID == folderID {
		return batchItemResult{ID: folderID, OK: false, Error: "cannot move folder into itself"}
	}
	// Snapshot the old parent for the activity diff.
	var oldParent sql.NullString
	if err := a.DB.QueryRowContext(ctx,
		"SELECT parent_id FROM folders WHERE id=? AND is_deleted=0", folderID,
	).Scan(&oldParent); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return batchItemResult{ID: folderID, OK: false, Error: "folder not found"}
		}
		return batchItemResult{ID: folderID, OK: false, Error: err.Error()}
	}

	// Resolve the destination owner. Cross-owner relocation is allowed and
	// reassigns the subtree's ownership + quota in moveFolder. nil/"" = own root.
	var destParent *string
	destOwnerID := userID
	if newParentID != nil && *newParentID != "" {
		owner, derr := a.canAccessFolder(ctx, userID, *newParentID, AccessEditor)
		if derr != nil {
			if errors.Is(derr, sql.ErrNoRows) {
				return batchItemResult{ID: folderID, OK: false, Error: "destination folder not found"}
			}
			return batchItemResult{ID: folderID, OK: false, Error: "no destination access"}
		}
		destParent = newParentID
		destOwnerID = owner
		// Depth cap.
		pdepth, derr := a.folderDepth(ctx, *newParentID)
		if derr != nil {
			return batchItemResult{ID: folderID, OK: false, Error: derr.Error()}
		}
		if pdepth >= MaxFolderDepth {
			return batchItemResult{ID: folderID, OK: false, Error: "folder nesting limit reached"}
		}
		// Cycle check — refuse to move a folder under one of its own descendants.
		rows, qerr := a.DB.QueryContext(ctx, `
			WITH RECURSIVE folder_tree AS (
				SELECT id FROM folders WHERE id=? AND user_id=?
				UNION ALL
				SELECT f.id FROM folders f JOIN folder_tree ft ON f.parent_id = ft.id WHERE f.user_id=?
			)
			SELECT id FROM folder_tree`, folderID, ownerID, ownerID)
		if qerr != nil {
			return batchItemResult{ID: folderID, OK: false, Error: qerr.Error()}
		}
		cycle := false
		for rows.Next() {
			var descendantID string
			if err := rows.Scan(&descendantID); err != nil {
				rows.Close()
				return batchItemResult{ID: folderID, OK: false, Error: err.Error()}
			}
			if descendantID == *newParentID {
				cycle = true
			}
		}
		rows.Close()
		if cycle {
			return batchItemResult{ID: folderID, OK: false, Error: "cannot move folder into its descendant"}
		}
	}

	if err := a.moveFolder(ctx, folderID, destParent, ownerID, destOwnerID); err != nil {
		if errors.Is(err, ErrQuotaExceeded) {
			return batchItemResult{ID: folderID, OK: false, Error: "not enough storage at the destination"}
		}
		return batchItemResult{ID: folderID, OK: false, Error: err.Error()}
	}

	uidPtr := &userID
	var oldParentVal, newParentVal any
	if oldParent.Valid {
		oldParentVal = oldParent.String
	}
	if newParentID != nil {
		newParentVal = *newParentID
	}
	writeActivityWithDiff(ctx, a.DB, uidPtr, "folder_move", "folder", folderID, ip, opID,
		map[string]any{"old_parent_id": oldParentVal},
		map[string]any{"new_parent_id": newParentVal},
		nil,
	)
	return batchItemResult{ID: folderID, OK: true}
}

// _unusedGuard silences "fmt imported but not used" if any branch above
// stops using it. fmt is currently unused after the refactor; keep this so
// future edits can re-import without churn.
var _ = fmt.Errorf
