package app

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"mycloud/backend-go/internal/auth"
	"mycloud/backend-go/internal/httpapi"
)

type authPayload struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type refreshPayload struct {
	RefreshToken string `json:"refresh_token"`
}

// dummyPasswordHash is a precomputed bcrypt hash used to absorb the
// compare time when the supplied identity doesn't exist. Without it,
// handleLogin returns instantly on user-not-found and only spends the
// ~250ms bcrypt cost when a real user row is found — an obvious user-
// enumeration oracle. We init this lazily on first use because BcryptCost
// is set at package level and we'd rather pay the cost once at first
// login than at every process start.
var dummyPasswordHash []byte

func ensureDummyHash() {
	if len(dummyPasswordHash) > 0 {
		return
	}
	h, err := auth.HashPassword("dummy-password-never-matches-real-input")
	if err != nil {
		// If hashing fails the system is unusable anyway — leave the
		// dummy empty and accept a small enumeration window.
		return
	}
	dummyPasswordHash = []byte(h)
}

func (a *App) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	value, err := getSetting(r.Context(), a.DB, "setup_complete", "false")
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"setup_complete": boolSetting(value)}, nil)
}

func (a *App) handleSetupComplete(w http.ResponseWriter, r *http.Request) {
	var payload authPayload
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	payload.Email = strings.TrimSpace(strings.ToLower(payload.Email))
	payload.Username = strings.TrimSpace(payload.Username)
	if payload.Email == "" || payload.Username == "" || len(payload.Password) < 8 {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "username, email, and password are required")
		return
	}

	ctx := r.Context()
	var setupComplete string
	if err := a.DB.QueryRowContext(ctx, "SELECT value FROM settings WHERE key_name='setup_complete'").Scan(&setupComplete); err == nil && boolSetting(setupComplete) {
		httpapi.Error(w, http.StatusConflict, "already_setup", "Setup is already complete")
		return
	}

	hash, err := auth.HashPassword(payload.Password)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "hash_error", err.Error())
		return
	}

	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	defer tx.Rollback()

	id := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO users (id, username, email, password_hash, role, quota_bytes, used_bytes, is_active, must_change_password)
		VALUES (?, ?, ?, ?, 'admin', 10737418240, 0, 1, 0)`,
		id, payload.Username, payload.Email, hash); err != nil {
		httpapi.Error(w, http.StatusConflict, "setup_failed", "Could not create admin user")
		return
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO settings (key_name, value) VALUES ('setup_complete', 'true')
		ON DUPLICATE KEY UPDATE value='true'`); err != nil {
		respondDBError(w, r, err)
		return
	}
	if err := tx.Commit(); err != nil {
		respondDBError(w, r, err)
		return
	}
	writeActivity(ctx, a.DB, &id, "user.setup_complete", "user", id, clientIP(r), map[string]any{"email": payload.Email})
	httpapi.JSON(w, http.StatusCreated, map[string]any{"message": "Setup complete"}, nil)
}

func (a *App) handleRegister(w http.ResponseWriter, r *http.Request) {
	var payload authPayload
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	payload.Email = strings.TrimSpace(strings.ToLower(payload.Email))
	payload.Username = strings.TrimSpace(payload.Username)

	setupComplete, err := getSetting(r.Context(), a.DB, "setup_complete", "false")
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	if !boolSetting(setupComplete) {
		httpapi.Error(w, http.StatusForbidden, "setup_required", "Initial setup is not complete")
		return
	}
	regEnabled, err := getSetting(r.Context(), a.DB, "registration_enabled", "true")
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	if !boolSetting(regEnabled) {
		httpapi.Error(w, http.StatusForbidden, "registration_disabled", "Registration is disabled")
		return
	}
	if payload.Email == "" || payload.Username == "" || len(payload.Password) < 8 {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "username, email, and password are required")
		return
	}

	hash, err := auth.HashPassword(payload.Password)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "hash_error", err.Error())
		return
	}
	defaultQuota, _ := getSetting(r.Context(), a.DB, "default_quota_bytes", "10737418240")
	id := uuid.NewString()
	if _, err := a.DB.ExecContext(r.Context(), `
		INSERT INTO users (id, username, email, password_hash, role, quota_bytes, used_bytes, is_active, must_change_password)
		VALUES (?, ?, ?, ?, 'user', ?, 0, 1, 0)`,
		id, payload.Username, payload.Email, hash, defaultQuota); err != nil {
		httpapi.Error(w, http.StatusConflict, "user_exists", "User with this email or username already exists")
		return
	}
	tokens, err := a.Auth.IssuePair(id, "user")
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "token_error", err.Error())
		return
	}
	tokens["username"] = payload.Username
	tokens["email"] = payload.Email
	tokens["must_change_password"] = false
	// Persist the access-token JTI so /me/sessions can list this device.
	a.recordSessionFromTokens(r, id, tokens)
	writeActivity(r.Context(), a.DB, &id, "user.register", "user", id, clientIP(r), nil)
	httpapi.JSON(w, http.StatusCreated, tokens, nil)
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var payload authPayload
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	payload.Email = strings.TrimSpace(strings.ToLower(payload.Email))

	var (
		id, username, email, passwordHash, role string
		quotaBytes, usedBytes                   int64
		isActive, mustChange                    bool
	)
	// Usernames are stored lowercased at register-time, so an equality match
	// on the indexed column hits the uq_users_username unique index. The
	// previous `LOWER(username)=?` predicate was unsargable and forced a
	// full table scan on every login attempt, which compounds badly when
	// rate-limiting only kicks in after several attempts.
	err := a.DB.QueryRowContext(r.Context(), `
		SELECT id, username, email, password_hash, role, quota_bytes, used_bytes, is_active, must_change_password
		FROM users WHERE email=? OR username=?`, payload.Email, payload.Email).
		Scan(&id, &username, &email, &passwordHash, &role, &quotaBytes, &usedBytes, &isActive, &mustChange)
	if errors.Is(err, sql.ErrNoRows) {
		// Mitigate user-enumeration by spending the same bcrypt budget we
		// would for a real user. Without this, response time leaks
		// existence (~10ms vs ~250ms).
		ensureDummyHash()
		if len(dummyPasswordHash) > 0 {
			_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(payload.Password))
		}
		httpapi.Error(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
		return
	}
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	if !isActive || auth.ComparePassword(passwordHash, payload.Password) != nil {
		httpapi.Error(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
		return
	}
	if auth.NeedsRehash(passwordHash) {
		if newHash, err := auth.HashPassword(payload.Password); err == nil {
			_, _ = a.DB.ExecContext(r.Context(), "UPDATE users SET password_hash=? WHERE id=?", newHash, id)
		}
	}

	tokens, err := a.Auth.IssuePair(id, role)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "token_error", err.Error())
		return
	}
	tokens["username"] = username
	tokens["email"] = email
	tokens["must_change_password"] = mustChange
	// Persist the access-token JTI so /me/sessions can list this device.
	a.recordSessionFromTokens(r, id, tokens)
	writeActivity(r.Context(), a.DB, &id, "user.login", "user", id, clientIP(r), nil)
	httpapi.JSON(w, http.StatusOK, tokens, nil)
}

func (a *App) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var payload refreshPayload
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	// Reuse-detection: if this exact JTI is already on the blacklist, an
	// attacker is replaying a refresh token we already rotated. The
	// legitimate device's next refresh will also fail and force a re-login —
	// but that's the correct response: we don't know which holder is the
	// thief, so both lose the session and the user must authenticate fresh.
	// We do this BEFORE Parse so even an already-revoked token is treated
	// as reuse (Parse would otherwise refuse and we'd never reach the
	// family-revocation branch).
	preClaims, _ := a.Auth.ParseUnverifiedForReuse(payload.RefreshToken)
	if preClaims != nil && preClaims.ID != "" && a.Auth.IsJTIRevoked(preClaims.ID) {
		if preClaims.Family != "" {
			a.Auth.RevokeFamily(preClaims.Family)
		}
		httpapi.Error(w, http.StatusUnauthorized, "refresh_reuse",
			"Refresh token has already been used. Please sign in again.")
		return
	}
	claims, err := a.Auth.Parse(payload.RefreshToken)
	if err != nil || claims.Type != "refresh" {
		httpapi.Error(w, http.StatusUnauthorized, "unauthorized", "Invalid refresh token")
		return
	}
	role, active, err := getUserSession(r.Context(), a.Stmts, claims.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Error(w, http.StatusUnauthorized, "unauthorized", "User not found")
		return
	}
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	if !active {
		httpapi.Error(w, http.StatusUnauthorized, "unauthorized", "Account is disabled")
		return
	}
	// Rotate: blacklist the presented refresh JTI before issuing the new
	// pair. The new pair inherits the family so the chain stays linked for
	// reuse-detection — but this specific JTI is now single-use.
	a.Auth.RevokeJTI(claims.ID)
	tokens, err := a.Auth.IssuePairForRefresh(claims.UserID, role, claims.Family)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "token_error", err.Error())
		return
	}
	// Refresh issues a new JTI; record it. The OLD session row keeps
	// its expiry for revocation visibility; the new one represents this
	// device's current access token.
	a.recordSessionFromTokens(r, claims.UserID, tokens)
	httpapi.JSON(w, http.StatusOK, tokens, nil)
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	var payload refreshPayload
	_ = decodeJSON(r, &payload)
	// Best-effort revoke the supplied refresh token (clients may call logout
	// without a refresh payload from token-expired states — that's fine).
	// We also revoke the bearer access token if present so the in-flight
	// access JTI dies immediately, not only on its next ~15min expiry.
	a.Auth.Revoke(payload.RefreshToken)
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		bearer := strings.TrimPrefix(authHeader, "Bearer ")
		a.Auth.Revoke(bearer)
		// If we can read the family from the bearer, kill the whole session
		// family — that includes any refresh tokens we didn't see in this
		// call. Logout should be a clean break.
		if claims, _ := a.Auth.ParseUnverifiedForReuse(bearer); claims != nil && claims.Family != "" {
			a.Auth.RevokeFamily(claims.Family)
		}
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"message": "Logged out"}, nil)
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	var user struct {
		ID                 string
		Username           string
		Email              string
		Role               string
		QuotaBytes         int64
		UsedBytes          int64
		IsActive           bool
		MustChangePassword bool
		CreatedAt          string
	}
	err := a.DB.QueryRowContext(r.Context(), `
		SELECT id, username, email, role, quota_bytes, used_bytes, is_active, must_change_password, created_at
		FROM users WHERE id=?`, userID).
		Scan(&user.ID, &user.Username, &user.Email, &user.Role, &user.QuotaBytes, &user.UsedBytes, &user.IsActive, &user.MustChangePassword, &user.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Error(w, http.StatusNotFound, "not_found", "User not found")
		return
	}
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{
		"id":                   user.ID,
		"username":             user.Username,
		"email":                user.Email,
		"role":                 user.Role,
		"quota_bytes":          user.QuotaBytes,
		"used_bytes":           user.UsedBytes,
		"is_active":            user.IsActive,
		"must_change_password": user.MustChangePassword,
		"created_at":           user.CreatedAt,
	}, nil)
}

func (a *App) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	if len(payload.NewPassword) < 8 {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "New password must be at least 8 characters")
		return
	}
	userID := userIDFrom(r)
	var currentHash string
	if err := a.DB.QueryRowContext(r.Context(), "SELECT password_hash FROM users WHERE id=?", userID).Scan(&currentHash); err != nil {
		httpapi.Error(w, http.StatusNotFound, "not_found", "User not found")
		return
	}
	if auth.ComparePassword(currentHash, payload.OldPassword) != nil {
		httpapi.Error(w, http.StatusUnauthorized, "invalid_credentials", "Current password is incorrect")
		return
	}
	hash, err := auth.HashPassword(payload.NewPassword)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "hash_error", err.Error())
		return
	}
	if _, err := a.DB.ExecContext(r.Context(), "UPDATE users SET password_hash=?, must_change_password=0 WHERE id=?", hash, userID); err != nil {
		respondDBError(w, r, err)
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "user.change_password", "user", userID, clientIP(r), nil)
	httpapi.JSON(w, http.StatusOK, map[string]any{"message": "Password updated"}, nil)
}

func (a *App) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	userID := userIDFrom(r)
	var hash string
	if err := a.DB.QueryRowContext(r.Context(), "SELECT password_hash FROM users WHERE id=?", userID).Scan(&hash); err != nil {
		httpapi.Error(w, http.StatusNotFound, "not_found", "User not found")
		return
	}
	if auth.ComparePassword(hash, payload.Password) != nil {
		httpapi.Error(w, http.StatusUnauthorized, "invalid_credentials", "Current password is incorrect")
		return
	}
	if err := a.deleteUserResources(r.Context(), userID); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "delete_failed", err.Error())
		return
	}
	if _, err := a.DB.ExecContext(r.Context(), "DELETE FROM users WHERE id=?", userID); err != nil {
		respondDBError(w, r, err)
		return
	}
	httpapi.NoContent(w)
}
