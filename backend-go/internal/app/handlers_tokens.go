package app

// Personal Access Token management endpoints (interactive-session only — these
// routes are in patBlockedRoutes so a PAT can't mint or revoke other PATs).
//
//   GET    /api/v2/me/tokens        → list active tokens (no secret)
//   POST   /api/v2/me/tokens        → create; returns the plaintext token ONCE
//   PATCH  /api/v2/me/tokens/{id}   → rename
//   DELETE /api/v2/me/tokens/{id}   → revoke (soft; preserves audit trail)

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"mycloud/backend-go/internal/auth"
	"mycloud/backend-go/internal/httpapi"
)

func (a *App) handleListMyTokens(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT id, name, token_prefix, last_four, scopes, last_used_at, last_used_ip, expires_at, created_at
		FROM personal_access_tokens
		WHERE user_id = ? AND revoked_at IS NULL
		ORDER BY created_at DESC`, userID)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, prefix, lastFour, scopes string
		var lastUsedAt, expiresAt sql.NullTime
		var lastUsedIP sql.NullString
		var createdAt time.Time
		if err := rows.Scan(&id, &name, &prefix, &lastFour, &scopes,
			&lastUsedAt, &lastUsedIP, &expiresAt, &createdAt); err != nil {
			respondDBError(w, r, err)
			return
		}
		row := map[string]any{
			"id":           id,
			"name":         name,
			"token_prefix": prefix,
			"last_four":    lastFour,
			"scopes":       splitScopes(scopes),
			"created_at":   createdAt,
			"last_used_at": nullTimeJSON(lastUsedAt),
			"last_used_ip": nullStringJSON(lastUsedIP),
			"expires_at":   nullTimeJSON(expiresAt),
		}
		out = append(out, row)
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"tokens": out}, nil)
}

func (a *App) handleCreateMyToken(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	var payload struct {
		Name      string   `json:"name"`
		Scopes    []string `json:"scopes"`
		ExpiresAt *string  `json:"expires_at"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.Name == "" {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Token name is required")
		return
	}
	if len(payload.Name) > 100 {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Token name is too long")
		return
	}
	scopesCSV, err := canonicalizeScopes(payload.Scopes)
	if err != nil {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	expiresAt, err := parseUserDatetime(payload.ExpiresAt)
	if err != nil {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Invalid expires_at")
		return
	}
	if expiresAt == nil && a.Config.PATDefaultExpiryDays > 0 {
		t := time.Now().UTC().AddDate(0, 0, a.Config.PATDefaultExpiryDays)
		expiresAt = &t
	}
	if expiresAt != nil && !expiresAt.After(time.Now()) {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Expiry must be in the future")
		return
	}
	if a.Config.PATMaxPerUser > 0 {
		var count int
		if err := a.DB.QueryRowContext(r.Context(),
			"SELECT COUNT(*) FROM personal_access_tokens WHERE user_id = ? AND revoked_at IS NULL",
			userID).Scan(&count); err != nil {
			respondDBError(w, r, err)
			return
		}
		if count >= a.Config.PATMaxPerUser {
			httpapi.Error(w, http.StatusConflict, "limit_reached",
				"You have reached the maximum number of personal access tokens")
			return
		}
	}
	full, prefix, lastFour, err := auth.GeneratePATToken()
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	id := uuid.NewString()
	if _, err := a.DB.ExecContext(r.Context(), `
		INSERT INTO personal_access_tokens
		    (id, user_id, name, token_hash, token_prefix, last_four, scopes, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, userID, payload.Name, auth.HashPATToken(full), prefix, lastFour, scopesCSV, expiresAt); err != nil {
		if IsDuplicate(err) {
			httpapi.Error(w, http.StatusConflict, "duplicate", "Token already exists, please retry")
			return
		}
		respondDBError(w, r, err)
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "token.create", "token", id, clientIP(r),
		map[string]any{"name": payload.Name, "scopes": scopesCSV})
	resp := map[string]any{
		"id":           id,
		"name":         payload.Name,
		"token_prefix": prefix,
		"last_four":    lastFour,
		"scopes":       splitScopes(scopesCSV),
		"created_at":   time.Now().UTC(),
		// The plaintext token is returned exactly once, here, and never stored.
		"token": full,
	}
	if expiresAt != nil {
		resp["expires_at"] = *expiresAt
	}
	httpapi.JSON(w, http.StatusCreated, resp, nil)
}

func (a *App) handleRenameMyToken(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	var payload struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.Name == "" || len(payload.Name) > 100 {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Invalid token name")
		return
	}
	res, err := a.DB.ExecContext(r.Context(),
		"UPDATE personal_access_tokens SET name = ? WHERE id = ? AND user_id = ? AND revoked_at IS NULL",
		payload.Name, id, userID)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Token not found")
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"message": "Updated"}, nil)
}

func (a *App) handleRevokeMyToken(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	// Soft revoke (not DELETE) so the activity trail attributed to this token
	// stays interpretable after it's gone.
	res, err := a.DB.ExecContext(r.Context(),
		"UPDATE personal_access_tokens SET revoked_at = NOW() WHERE id = ? AND user_id = ? AND revoked_at IS NULL",
		id, userID)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Token not found")
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "token.revoke", "token", id, clientIP(r), nil)
	httpapi.NoContent(w)
}

// nullTimeJSON / nullStringJSON render nullable columns as a value or JSON null
// rather than omitting the key, so the frontend can rely on the field existing.
func nullTimeJSON(t sql.NullTime) any {
	if t.Valid {
		return t.Time
	}
	return nil
}

func nullStringJSON(s sql.NullString) any {
	if s.Valid {
		return s.String
	}
	return nil
}
