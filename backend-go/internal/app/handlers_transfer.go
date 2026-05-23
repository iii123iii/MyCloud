package app

// File ownership transfer.
//
// Owner of a file (or admin) can hand it over to another user on the same
// instance. The recipient must have enough quota headroom; otherwise the
// request rejects early instead of leaving the file in a half-transferred
// state.
//
// POST /api/v2/files/{id}:transfer
//   body: { new_owner_id: string, transfer_grants?: bool }
//
// transfer_grants=true keeps the existing share_grants so the old
// collaborators still have access; false (default) revokes them — the
// recipient typically wants to start fresh.

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"mycloud/backend-go/internal/httpapi"
)

func (a *App) handleTransferFile(w http.ResponseWriter, r *http.Request) {
	callerID := userIDFrom(r)
	fileID := chi.URLParam(r, "id")

	var body struct {
		NewOwnerID     string `json:"new_owner_id"`
		TransferGrants bool   `json:"transfer_grants"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if body.NewOwnerID == "" {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "new_owner_id required")
		return
	}

	// Authorise: caller must be the current owner OR an admin.
	var ownerID string
	var sizeBytes int64
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT user_id, size_bytes FROM files WHERE id = ? AND is_deleted = 0`,
		fileID,
	).Scan(&ownerID, &sizeBytes)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Error(w, http.StatusNotFound, "not_found", "file not found")
		return
	}
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	if ownerID != callerID && userRoleFrom(r) != "admin" {
		httpapi.Error(w, http.StatusForbidden, "forbidden", "only the current owner or an admin may transfer")
		return
	}
	if ownerID == body.NewOwnerID {
		httpapi.Error(w, http.StatusBadRequest, "no_change", "already owned by that user")
		return
	}

	// Pre-flight quota check on recipient. Reject early to avoid a half-
	// applied transfer (file owner flipped but used_bytes blown).
	var used, quota int64
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT used_bytes, quota_bytes FROM users WHERE id = ?`, body.NewOwnerID,
	).Scan(&used, &quota); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "recipient_missing", "recipient user not found")
			return
		}
		respondDBError(w, r, err)
		return
	}
	if used+sizeBytes > quota {
		httpapi.Error(w, http.StatusConflict, "quota_exceeded",
			"recipient quota would be exceeded by this transfer")
		return
	}

	// Transactional swap: decrement old owner's used_bytes, increment
	// recipient's, update files.user_id, optionally revoke grants.
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(r.Context(),
		`UPDATE users SET used_bytes = GREATEST(0, used_bytes - ?) WHERE id = ?`,
		sizeBytes, ownerID); err != nil {
		respondDBError(w, r, err)
		return
	}
	if _, err := tx.ExecContext(r.Context(),
		`UPDATE users SET used_bytes = used_bytes + ? WHERE id = ?`,
		sizeBytes, body.NewOwnerID); err != nil {
		respondDBError(w, r, err)
		return
	}
	if _, err := tx.ExecContext(r.Context(),
		`UPDATE files SET user_id = ? WHERE id = ?`, body.NewOwnerID, fileID); err != nil {
		respondDBError(w, r, err)
		return
	}
	if !body.TransferGrants {
		if _, err := tx.ExecContext(r.Context(),
			`DELETE FROM share_grants WHERE file_id = ?`, fileID); err != nil {
			respondDBError(w, r, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		respondDBError(w, r, err)
		return
	}

	uidPtr := &callerID
	writeActivity(r.Context(), a.DB, uidPtr, "file_transfer_ownership", "file", fileID, clientIP(r),
		map[string]any{
			"old_owner":       ownerID,
			"new_owner":       body.NewOwnerID,
			"size_bytes":      sizeBytes,
			"transfer_grants": body.TransferGrants,
		})

	httpapi.JSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"new_owner":   body.NewOwnerID,
		"transferred": sizeBytes,
	}, nil)
}
