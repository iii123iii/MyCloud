package app

// Per-folder retention policy CRUD + the maintenance sweep that
// enforces them.

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"mycloud/backend-go/internal/httpapi"
)

type retentionPolicy struct {
	FolderID      string    `json:"folder_id"`
	RetentionDays int       `json:"retention_days"`
	Action        string    `json:"action"`
	CreatedBy     string    `json:"created_by"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// handleGetRetentionPolicy returns the policy for a folder, or 404 if none.
func (a *App) handleGetRetentionPolicy(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	folderID := chi.URLParam(r, "id")
	if _, err := a.canAccessFolder(r.Context(), userID, folderID, AccessEditor); err != nil {
		httpapi.Error(w, http.StatusForbidden, "forbidden", "no folder access")
		return
	}
	var p retentionPolicy
	err := a.DB.QueryRowContext(r.Context(), `
		SELECT folder_id, retention_days, action, created_by, updated_at
		FROM retention_policies WHERE folder_id = ?`, folderID,
	).Scan(&p.FolderID, &p.RetentionDays, &p.Action, &p.CreatedBy, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Error(w, http.StatusNotFound, "not_found", "no policy set")
		return
	}
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, p, nil)
}

// handlePutRetentionPolicy upserts a policy. Body: { retention_days, action }.
func (a *App) handlePutRetentionPolicy(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	folderID := chi.URLParam(r, "id")
	if _, err := a.canAccessFolder(r.Context(), userID, folderID, AccessEditor); err != nil {
		httpapi.Error(w, http.StatusForbidden, "forbidden", "no folder access")
		return
	}
	var body struct {
		RetentionDays int    `json:"retention_days"`
		Action        string `json:"action"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	body.Action = strings.ToLower(strings.TrimSpace(body.Action))
	if body.RetentionDays < 1 || body.RetentionDays > 3650 {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "retention_days must be 1..3650")
		return
	}
	if body.Action != "soft_delete" && body.Action != "hard_delete" && body.Action != "notify" {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "action must be soft_delete|hard_delete|notify")
		return
	}
	if _, err := a.DB.ExecContext(r.Context(), `
		INSERT INTO retention_policies (folder_id, retention_days, action, created_by)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE retention_days = VALUES(retention_days),
		                        action         = VALUES(action),
		                        created_by     = VALUES(created_by)`,
		folderID, body.RetentionDays, body.Action, userID); err != nil {
		respondDBError(w, r, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{
		"folder_id":      folderID,
		"retention_days": body.RetentionDays,
		"action":         body.Action,
	}, nil)
}

// handleDeleteRetentionPolicy removes the policy.
func (a *App) handleDeleteRetentionPolicy(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	folderID := chi.URLParam(r, "id")
	if _, err := a.canAccessFolder(r.Context(), userID, folderID, AccessEditor); err != nil {
		httpapi.Error(w, http.StatusForbidden, "forbidden", "no folder access")
		return
	}
	if _, err := a.DB.ExecContext(r.Context(),
		`DELETE FROM retention_policies WHERE folder_id = ?`, folderID); err != nil {
		respondDBError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// runRetentionSweep applies every policy. Called from the maintenance ticker
// once an hour. Per-policy errors are logged and skipped so one bad policy
// doesn't stall the others.
func (a *App) runRetentionSweep(ctx context.Context) {
	lg := slog.With("worker", "retention")
	rows, err := a.DB.QueryContext(ctx, `
		SELECT folder_id, retention_days, action FROM retention_policies`)
	if err != nil {
		lg.Error("load policies", "error", err)
		return
	}
	type policy struct {
		folderID string
		days     int
		action   string
	}
	var policies []policy
	for rows.Next() {
		var p policy
		if err := rows.Scan(&p.folderID, &p.days, &p.action); err != nil {
			lg.Error("scan policy", "error", err)
			continue
		}
		policies = append(policies, p)
	}
	rows.Close()

	for _, p := range policies {
		var query string
		switch p.action {
		case "soft_delete":
			query = `UPDATE files
			         SET is_deleted = 1, deleted_at = NOW()
			         WHERE folder_id = ? AND is_deleted = 0
			           AND created_at < NOW() - INTERVAL ? DAY`
		case "hard_delete":
			// Soft-delete first so the maintenance/storage purge picks it up
			// like any other deletion. Avoids special-casing the encrypted
			// blob cleanup path.
			query = `UPDATE files
			         SET is_deleted = 1, deleted_at = NOW()
			         WHERE folder_id = ? AND created_at < NOW() - INTERVAL ? DAY`
		case "notify":
			// Count matching files; write a single activity row per sweep.
			var count int
			if err := a.DB.QueryRowContext(ctx, `
				SELECT COUNT(*) FROM files
				WHERE folder_id = ? AND is_deleted = 0
				  AND created_at < NOW() - INTERVAL ? DAY`,
				p.folderID, p.days).Scan(&count); err != nil {
				lg.Error("notify count", "folder", p.folderID, "error", err)
				continue
			}
			if count > 0 {
				writeActivity(ctx, a.DB, nil, "retention_notify", "folder", p.folderID, "",
					map[string]any{"matching_files": count, "days": p.days})
			}
			continue
		default:
			continue
		}
		res, err := a.DB.ExecContext(ctx, query, p.folderID, p.days)
		if err != nil {
			lg.Error("retention exec", "folder", p.folderID, "action", p.action, "error", err)
			continue
		}
		if n, _ := res.RowsAffected(); n > 0 {
			lg.Info("retention applied",
				"folder", p.folderID, "action", p.action, "days", p.days, "affected", n)
			writeActivity(ctx, a.DB, nil, "retention_"+p.action, "folder", p.folderID, "",
				map[string]any{"affected": n, "days": p.days})
		}
	}
}
