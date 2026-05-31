package app

// GET /api/v2/auth/whoami — identity + (for PATs) scope introspection.
//
// Intentionally SCOPE-EXEMPT (see patScopeExempt in auth_pat.go): any valid
// principal — including a narrowly-scoped token that lacks account:read — can
// call it to (a) confirm the credential works and (b) discover what it's
// allowed to do. This is the "verify credentials" endpoint headless clients
// (desktop sync, CLI) hit right after a user supplies a token, so they can show
// an identity and warn about missing scopes without burning account:read.

import (
	"database/sql"
	"errors"
	"net/http"

	"mycloud/backend-go/internal/httpapi"
)

func (a *App) handleWhoami(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	var user struct {
		ID       string
		Username string
		Email    string
		Role     string
	}
	err := a.DB.QueryRowContext(r.Context(),
		"SELECT id, username, email, role FROM users WHERE id=?", userID).
		Scan(&user.ID, &user.Username, &user.Email, &user.Role)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Error(w, http.StatusNotFound, "not_found", "User not found")
		return
	}
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	resp := map[string]any{
		"id":       user.ID,
		"username": user.Username,
		"email":    user.Email,
		"role":     user.Role,
		"auth":     "session",
	}
	// patScopesFrom is true only on PAT-authenticated requests; for those,
	// surface the token's scopes (always an array, never null) so clients can
	// warn the user when a required scope is missing.
	if scopes, ok := patScopesFrom(r); ok {
		if scopes == nil {
			scopes = []string{}
		}
		resp["auth"] = "pat"
		resp["scopes"] = scopes
	}
	httpapi.JSON(w, http.StatusOK, resp, nil)
}
