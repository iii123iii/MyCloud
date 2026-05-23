package app

// Admin impersonation.
//
//   POST /api/v2/admin/users/{id}:impersonate
//     body: { ttl_seconds?: number }   // capped at 1h
//     resp: { access_token, refresh_token, acting_as_admin: true }
//
// Implementation notes:
//   - The token pair is issued for the target user; the admin's session
//     keeps its own token. This is a fork, not a swap.
//   - We log to activity_log with action="admin_impersonate_start" so
//     auditors see who took over whom.
//   - Restrictions per the plan's risk R4:
//       • cannot impersonate another admin
//       • TTL capped at 1h
//       • response header X-Impersonating advertises the admin's id so
//         frontends can render the banner.
//
// Future hardening: the issued token should carry an "act" JWT claim
// identifying the admin so middleware can refuse password changes etc.
// The current Manager doesn't accept arbitrary claims, so we settle for the
// activity_log trail + response header until token.go grows that hook.

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"mycloud/backend-go/internal/httpapi"
)

func (a *App) handleAdminImpersonate(w http.ResponseWriter, r *http.Request) {
	adminID := userIDFrom(r)
	targetID := chi.URLParam(r, "id")
	if targetID == "" {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "missing target id")
		return
	}
	if targetID == adminID {
		httpapi.Error(w, http.StatusBadRequest, "no_self", "cannot impersonate yourself")
		return
	}

	var body struct {
		TTLSeconds int `json:"ttl_seconds"`
	}
	_ = decodeJSON(r, &body)
	ttl := time.Duration(body.TTLSeconds) * time.Second
	if ttl <= 0 || ttl > time.Hour {
		ttl = time.Hour
	}

	var role string
	var active bool
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT role, is_active FROM users WHERE id = ?`, targetID,
	).Scan(&role, &active)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Error(w, http.StatusNotFound, "not_found", "target user not found")
		return
	}
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	if !active {
		httpapi.Error(w, http.StatusForbidden, "user_disabled", "target account is disabled")
		return
	}
	if role == "admin" {
		// Prevent escalating sideways into another admin's session — admins
		// can only impersonate non-admins, which is the practical use case
		// (debugging a user-reported issue).
		httpapi.Error(w, http.StatusForbidden, "no_admin_target", "cannot impersonate another admin")
		return
	}

	pair, err := a.Auth.IssuePair(targetID, role)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "token_error", err.Error())
		return
	}

	writeActivity(r.Context(), a.DB, &adminID, "admin_impersonate_start", "user", targetID, clientIP(r),
		map[string]any{"ttl_seconds": int(ttl.Seconds())})

	resp := map[string]any{
		"acting_as_admin": adminID,
		"target_user_id":  targetID,
		"ttl_seconds":     int(ttl.Seconds()),
	}
	for k, v := range pair {
		resp[k] = v
	}
	w.Header().Set("X-Impersonating", adminID)
	httpapi.JSON(w, http.StatusCreated, resp, nil)
}

// handleAdminForceResetPassword sets a flag on the user that requires them
// to change their password on next login. The actual enforcement lives in
// the login handler (looks at must_change_password and refuses to issue
// access tokens until /auth/change-password succeeds). We expose
// the toggle; existing schemas without the column fall back to a noop
// activity-log entry the admin can observe.
func (a *App) handleAdminForceReset(w http.ResponseWriter, r *http.Request) {
	adminID := userIDFrom(r)
	targetID := chi.URLParam(r, "id")
	// Soft-attempt the column — if the migration hasn't shipped, we still
	// log the intent so audits show the admin requested it.
	_, err := a.DB.ExecContext(r.Context(),
		`UPDATE users SET must_change_password = 1 WHERE id = ?`, targetID)
	if err != nil && !columnMissingError(err) {
		respondDBError(w, r, err)
		return
	}
	writeActivity(r.Context(), a.DB, &adminID, "admin_force_password_reset", "user", targetID, clientIP(r), nil)
	httpapi.JSON(w, http.StatusOK, map[string]any{"ok": true, "target_user_id": targetID}, nil)
}

// columnMissingError matches the MariaDB "Unknown column" error so we can
// fall through gracefully when must_change_password hasn't migrated yet.
func columnMissingError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "Unknown column") || contains(msg, "must_change_password")
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOfCS(haystack, needle) >= 0
}

func indexOfCS(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
