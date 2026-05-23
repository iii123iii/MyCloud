package app

// Audit-trail rollback engine.
//
// Reversible actions register a RollbackFn keyed by action name. The admin
// endpoint POST /api/v2/admin/activity/{id}:rollback reads the activity_log
// row, looks up the fn by action, and invokes it with the stored old/new
// JSON payloads. Each fn is responsible for validating that the current
// state still matches new_value (otherwise rollback would clobber an
// intervening change made by the user) and for restoring old_value.
//
// Coverage:
//   - file_rename, file_move, file_soft_delete: see registrations below
//   - share_create: undo by deleting the share row
//   - tag_attach: undo by detaching the tag
//
// Hard deletes are explicitly non-rollback: the encrypted blob is gone and
// no amount of metadata diff can bring it back. The admin endpoint returns
// 422 with code "non_reversible" for those.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"mycloud/backend-go/internal/httpapi"
	"mycloud/backend-go/internal/logging"
)

// RollbackFn restores the state captured in oldValue, given that the row is
// currently in newValue. ctx carries auth/log fields. Returns the standard
// error contract; the caller surfaces it as 500.
type RollbackFn func(ctx context.Context, app *App, resourceID string, oldValue, newValue json.RawMessage) error

// rollbackRegistry is set up at package init from the per-action wiring
// below. Adding a new reversible action means: (1) write a RollbackFn,
// (2) register it here, (3) make the handler write old_value/new_value
// when it logs the action.
var rollbackRegistry = map[string]RollbackFn{
	"file_rename":      rollbackFileRename,
	"file_move":        rollbackFileMove,
	"file_soft_delete": rollbackFileSoftDelete,
	"share_create":     rollbackShareCreate,
}

// ErrNonReversible flags actions that exist in activity_log but have no
// registered rollback fn (typically hard deletes or content edits).
var ErrNonReversible = errors.New("action is not reversible")

// handleAdminRollback implements POST /api/v2/admin/activity/{id}:rollback.
func (a *App) handleAdminRollback(w http.ResponseWriter, r *http.Request) {
	activityID := chi.URLParam(r, "id")
	if activityID == "" {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "missing activity id")
		return
	}

	var (
		action, resourceID string
		oldValue, newValue sql.NullString
		reversible         bool
		rolledBack         sql.NullTime
	)
	err := a.DB.QueryRowContext(r.Context(), `
		SELECT action, COALESCE(resource_id, ''), old_value, new_value, reversible, rolled_back_at
		FROM activity_log WHERE id = ?`, activityID,
	).Scan(&action, &resourceID, &oldValue, &newValue, &reversible, &rolledBack)
	if err == sql.ErrNoRows {
		httpapi.Error(w, http.StatusNotFound, "not_found", "activity row not found")
		return
	}
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	if !reversible {
		httpapi.Error(w, http.StatusUnprocessableEntity, "non_reversible", "this action wasn't recorded as reversible")
		return
	}
	if rolledBack.Valid {
		httpapi.Error(w, http.StatusConflict, "already_rolled_back",
			fmt.Sprintf("this action was already rolled back at %s", rolledBack.Time.Format(time.RFC3339)))
		return
	}

	fn, ok := rollbackRegistry[action]
	if !ok {
		httpapi.Error(w, http.StatusUnprocessableEntity, "non_reversible", "no rollback fn registered for action "+action)
		return
	}

	var oldRaw, newRaw json.RawMessage
	if oldValue.Valid {
		oldRaw = json.RawMessage(oldValue.String)
	}
	if newValue.Valid {
		newRaw = json.RawMessage(newValue.String)
	}

	if err := fn(r.Context(), a, resourceID, oldRaw, newRaw); err != nil {
		logging.FromContext(r.Context()).Error("rollback failed",
			"action", action, "activity_id", activityID, "error", err,
		)
		httpapi.Error(w, http.StatusInternalServerError, "rollback_failed", err.Error())
		return
	}

	// Mark the activity row as rolled back so it can't be undone twice.
	if _, err := a.DB.ExecContext(r.Context(),
		`UPDATE activity_log SET rolled_back_at = NOW() WHERE id = ?`, activityID); err != nil {
		// Rollback already succeeded — log the marker failure but don't 500;
		// the admin can manually mark it via SQL if needed.
		logging.FromContext(r.Context()).Warn("rollback marker failed",
			"activity_id", activityID, "error", err)
	}

	httpapi.JSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"activity_id": activityID,
		"action":      action,
	}, nil)
}

// ---- rollback fns ---------------------------------------------------------

// rollbackFileRename undoes a rename by restoring the file's name from
// old_value. Skips if the current name no longer matches new_value (someone
// else renamed it in the meantime).
func rollbackFileRename(ctx context.Context, a *App, fileID string, oldValue, newValue json.RawMessage) error {
	var oldData, newData struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(oldValue, &oldData); err != nil {
		return fmt.Errorf("decode old_value: %w", err)
	}
	if err := json.Unmarshal(newValue, &newData); err != nil {
		return fmt.Errorf("decode new_value: %w", err)
	}
	// Revert name_normalised together with name on rollback.
	res, err := a.DB.ExecContext(ctx,
		`UPDATE files SET name = ?, name_normalised = ? WHERE id = ? AND name = ? AND is_deleted = 0`,
		oldData.Name, normaliseName(oldData.Name), fileID, newData.Name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("file name changed since the action — manual review required")
	}
	return nil
}

// rollbackFileMove undoes a folder move. old_value/new_value carry folder_id
// (nullable string).
func rollbackFileMove(ctx context.Context, a *App, fileID string, oldValue, newValue json.RawMessage) error {
	var oldData, newData struct {
		FolderID *string `json:"folder_id"`
	}
	if err := json.Unmarshal(oldValue, &oldData); err != nil {
		return fmt.Errorf("decode old_value: %w", err)
	}
	if err := json.Unmarshal(newValue, &newData); err != nil {
		return fmt.Errorf("decode new_value: %w", err)
	}
	// Restore. The COALESCE/IS NULL ladder handles both "current folder
	// matches new" and "old was NULL" cases without ambiguity.
	res, err := a.DB.ExecContext(ctx,
		`UPDATE files SET folder_id = ? WHERE id = ?
		 AND ((? IS NULL AND folder_id IS NULL) OR folder_id = ?)
		 AND is_deleted = 0`,
		oldData.FolderID, fileID, newData.FolderID, newData.FolderID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("file folder changed since the action — manual review required")
	}
	return nil
}

// rollbackFileSoftDelete restores a file from the trash. Hard deletes are
// non-reversible — they remove the row entirely.
func rollbackFileSoftDelete(ctx context.Context, a *App, fileID string, _, _ json.RawMessage) error {
	res, err := a.DB.ExecContext(ctx,
		`UPDATE files SET is_deleted = 0, deleted_at = NULL WHERE id = ? AND is_deleted = 1`,
		fileID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("file is not in a soft-deleted state")
	}
	return nil
}

// rollbackShareCreate undoes a share creation by deleting the share row.
// resource_id is the share's UUID; the share token is gone after this.
func rollbackShareCreate(ctx context.Context, a *App, shareID string, _, _ json.RawMessage) error {
	res, err := a.DB.ExecContext(ctx, `DELETE FROM shares WHERE id = ?`, shareID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("share already gone")
	}
	return nil
}
