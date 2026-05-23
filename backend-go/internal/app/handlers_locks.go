package app

// Cooperative file locks.
//
// HTTP shape:
//   POST   /api/v2/files/{id}/lock     { "kind": "edit", "ttl_seconds": 60 }  →  { "lock": {...} }
//   POST   /api/v2/files/{id}/lock:refresh                                     →  { "lock": {...} }
//   DELETE /api/v2/files/{id}/lock?kind=edit                                   →  204
//   GET    /api/v2/files/{id}/lock                                             →  { "locks": [...] }
//
// The desktop sync client takes a 'sync' lock for the duration of an upload
// so concurrent web edits know to wait. OnlyOffice continues using the
// dedicated 'office' kind through office_sessions; this generic table is
// for everyone else.

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"mycloud/backend-go/internal/httpapi"
)

type lockDTO struct {
	ID        string    `json:"id"`
	FileID    string    `json:"file_id"`
	UserID    string    `json:"user_id"`
	Kind      string    `json:"kind"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

const (
	maxLockTTL     = 10 * time.Minute
	defaultLockTTL = 60 * time.Second
)

// handleAcquireFileLock acquires (or refreshes if same user/kind) a lock.
// Returns 409 if another user already holds an unexpired lock of this kind.
func (a *App) handleAcquireFileLock(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	fileID := chi.URLParam(r, "id")
	if _, err := a.canAccessFile(r.Context(), userID, fileID, AccessEditor); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "file not accessible")
			return
		}
		respondDBError(w, r, err)
		return
	}

	var body struct {
		Kind       string `json:"kind"`
		TTLSeconds int    `json:"ttl_seconds"`
	}
	_ = decodeJSON(r, &body)
	if body.Kind == "" {
		body.Kind = "edit"
	}
	if !validLockKind(body.Kind) {
		httpapi.Error(w, http.StatusBadRequest, "bad_kind", "kind must be one of edit, sync, office")
		return
	}
	ttl := time.Duration(body.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = defaultLockTTL
	}
	if ttl > maxLockTTL {
		ttl = maxLockTTL
	}
	expires := time.Now().Add(ttl)

	// Atomic upsert: refresh if same user holds it OR existing lock is
	// expired; reject (409) if a different user has it unexpired.
	// We do this in two steps so the conflict path can return a useful
	// error body identifying the holder.
	var holderID string
	var existingExpires time.Time
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT user_id, expires_at FROM file_locks WHERE file_id = ? AND kind = ?`,
		fileID, body.Kind,
	).Scan(&holderID, &existingExpires)
	if err != nil && err != sql.ErrNoRows {
		respondDBError(w, r, err)
		return
	}
	now := time.Now()
	if err == nil && existingExpires.After(now) && holderID != userID {
		httpapi.Error(w, http.StatusConflict, "locked",
			"file is locked by another user; retry after "+existingExpires.Sub(now).Truncate(time.Second).String())
		return
	}

	id := uuid.NewString()
	if _, err := a.DB.ExecContext(r.Context(), `
		INSERT INTO file_locks (id, file_id, user_id, kind, expires_at)
		VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		    user_id    = VALUES(user_id),
		    expires_at = VALUES(expires_at),
		    id         = IF(VALUES(user_id) = file_locks.user_id, file_locks.id, VALUES(id))`,
		id, fileID, userID, body.Kind, expires); err != nil {
		respondDBError(w, r, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, lockDTO{
		ID: id, FileID: fileID, UserID: userID, Kind: body.Kind,
		ExpiresAt: expires, CreatedAt: time.Now(),
	}, nil)
}

// handleReleaseFileLock removes the lock if it belongs to the caller.
func (a *App) handleReleaseFileLock(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	fileID := chi.URLParam(r, "id")
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "edit"
	}
	res, err := a.DB.ExecContext(r.Context(),
		`DELETE FROM file_locks WHERE file_id = ? AND kind = ? AND user_id = ?`,
		fileID, kind, userID)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Idempotent: releasing a lock you don't own is a no-op, not an error.
		// (Avoids a thundering 404 from a desktop client retrying after a crash.)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListFileLocks reports all active (non-expired) locks for a file.
// Used by the editor UI to show "Alice is editing" before opening.
func (a *App) handleListFileLocks(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	fileID := chi.URLParam(r, "id")
	if _, err := a.canAccessFile(r.Context(), userID, fileID, AccessViewer); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "file not accessible")
			return
		}
		respondDBError(w, r, err)
		return
	}
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT id, file_id, user_id, kind, expires_at, created_at
		FROM file_locks
		WHERE file_id = ? AND expires_at > NOW()`, fileID)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	defer rows.Close()
	out := make([]lockDTO, 0)
	for rows.Next() {
		var l lockDTO
		if err := rows.Scan(&l.ID, &l.FileID, &l.UserID, &l.Kind, &l.ExpiresAt, &l.CreatedAt); err != nil {
			respondDBError(w, r, err)
			return
		}
		out = append(out, l)
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"locks": out}, nil)
}

func validLockKind(k string) bool {
	switch strings.ToLower(k) {
	case "edit", "sync", "office":
		return true
	}
	return false
}
