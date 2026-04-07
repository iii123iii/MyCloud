package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/iii123iii/mycloud/backend/internal/auth"
	"github.com/iii123iii/mycloud/backend/internal/services"
	"github.com/iii123iii/mycloud/backend/internal/utils"
)

// AuthHandler handles /api/auth/* routes.
type AuthHandler struct {
	db          *sql.DB
	authService *services.AuthService
	storageSvc  *services.StorageService
}

func NewAuthHandler(db *sql.DB, authService *services.AuthService, storageSvc *services.StorageService) *AuthHandler {
	return &AuthHandler{db: db, authService: authService, storageSvc: storageSvc}
}

// Login handles POST /api/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	// Rate limit by IP
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	if !h.authService.CheckRateLimit(r.Context(), ip) {
		utils.ErrorJSON(w, http.StatusTooManyRequests, "Too many login attempts. Try again in 15 minutes.")
		return
	}

	var body struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.ErrorJSON(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	loginField := body.Email
	if loginField == "" {
		loginField = body.Username
	}
	if loginField == "" || body.Password == "" {
		utils.ErrorJSON(w, http.StatusBadRequest, "email/username and password required")
		return
	}

	var (
		id                string
		username          string
		email             string
		passwordHash      string
		role              string
		isActive          bool
		mustChangePassword bool
	)
	err := h.db.QueryRowContext(r.Context(),
		"SELECT id,username,email,password_hash,role,is_active,must_change_password "+
			"FROM users WHERE email=? OR username=? LIMIT 1",
		loginField, loginField,
	).Scan(&id, &username, &email, &passwordHash, &role, &isActive, &mustChangePassword)
	if err == sql.ErrNoRows {
		utils.ErrorJSON(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !isActive {
		utils.ErrorJSON(w, http.StatusForbidden, "Account is disabled")
		return
	}
	if !utils.VerifyPassword(body.Password, passwordHash) {
		utils.ErrorJSON(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	accessToken, refreshToken, err := h.authService.IssueTokenPair(id, role)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	utils.OkJSON(w, map[string]any{
		"access_token":        accessToken,
		"refresh_token":       refreshToken,
		"token_type":          "Bearer",
		"user_id":             id,
		"username":            username,
		"email":               email,
		"role":                role,
		"must_change_password": mustChangePassword,
	})
}

// Register handles POST /api/auth/register
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.ErrorJSON(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if len(body.Username) < 3 || body.Email == "" || len(body.Password) < 8 {
		utils.ErrorJSON(w, http.StatusBadRequest, "username (min 3), email, and password (min 8 chars) are required")
		return
	}

	// Check registration enabled
	var regEnabled string
	err := h.db.QueryRowContext(r.Context(),
		"SELECT value FROM settings WHERE key_name='registration_enabled'").Scan(&regEnabled)
	if err != nil && err != sql.ErrNoRows {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if regEnabled == "false" {
		utils.ErrorJSON(w, http.StatusForbidden, "Registration is disabled by administrator")
		return
	}

	// Get default quota
	var quotaStr string
	var quota int64 = 10737418240 // 10 GiB default
	err = h.db.QueryRowContext(r.Context(),
		"SELECT value FROM settings WHERE key_name='default_quota_bytes'").Scan(&quotaStr)
	if err == nil {
		if q, err2 := parseInt64(quotaStr); err2 == nil {
			quota = q
		}
	}

	id := utils.GenerateUUID()
	hash, err := utils.HashPassword(body.Password)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	_, err = h.db.ExecContext(r.Context(),
		"INSERT INTO users (id,username,email,password_hash,quota_bytes) VALUES (?,?,?,?,?)",
		id, body.Username, body.Email, hash, quota)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") || strings.Contains(err.Error(), "duplicate") {
			utils.ErrorJSON(w, http.StatusConflict, "Username or email already taken")
			return
		}
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	accessToken, refreshToken, err := h.authService.IssueTokenPair(id, "user")
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	utils.CreatedJSON(w, map[string]any{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"token_type":    "Bearer",
		"user_id":       id,
		"username":      body.Username,
		"role":          "user",
	})
}

// Refresh handles POST /api/auth/refresh
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.ErrorJSON(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if body.RefreshToken == "" {
		utils.ErrorJSON(w, http.StatusBadRequest, "refresh_token required")
		return
	}
	if h.authService.IsBlacklisted(r.Context(), body.RefreshToken) {
		utils.ErrorJSON(w, http.StatusUnauthorized, "Token revoked")
		return
	}
	claims, err := h.authService.VerifyToken(body.RefreshToken)
	if err != nil {
		utils.ErrorJSON(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	if claims.Type != "refresh" {
		utils.ErrorJSON(w, http.StatusUnauthorized, "Expected refresh token")
		return
	}

	var role string
	err = h.db.QueryRowContext(r.Context(),
		"SELECT role FROM users WHERE id=? AND is_active=1", claims.UserID).Scan(&role)
	if err == sql.ErrNoRows {
		utils.ErrorJSON(w, http.StatusUnauthorized, "User not found")
		return
	}
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	accessToken, refreshToken, err := h.authService.IssueTokenPair(claims.UserID, role)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	utils.OkJSON(w, map[string]any{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"token_type":    "Bearer",
	})
}

// Logout handles POST /api/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.RefreshToken != "" {
		h.authService.BlacklistToken(r.Context(), body.RefreshToken)
	}
	utils.NoContent(w)
}

// Me handles GET /api/auth/me
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	row := h.db.QueryRowContext(r.Context(),
		"SELECT id,username,email,role,quota_bytes,used_bytes,is_active,must_change_password,created_at "+
			"FROM users WHERE id=?", userID)

	var (
		id                 string
		username           string
		email              string
		role               string
		quotaBytes         int64
		usedBytes          int64
		isActive           bool
		mustChangePassword bool
		createdAt          string
	)
	if err := row.Scan(&id, &username, &email, &role, &quotaBytes, &usedBytes, &isActive, &mustChangePassword, &createdAt); err == sql.ErrNoRows {
		utils.ErrorJSON(w, http.StatusNotFound, "User not found")
		return
	} else if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	utils.OkJSON(w, map[string]any{
		"id":                   id,
		"username":             username,
		"email":                email,
		"role":                 role,
		"quota_bytes":          quotaBytes,
		"used_bytes":           usedBytes,
		"is_active":            isActive,
		"must_change_password": mustChangePassword,
		"created_at":           createdAt,
	})
}

// ChangePassword handles POST /api/auth/change-password
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	var body struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.ErrorJSON(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if len(body.NewPassword) < 8 {
		utils.ErrorJSON(w, http.StatusBadRequest, "New password must be at least 8 characters")
		return
	}

	var hash string
	if err := h.db.QueryRowContext(r.Context(),
		"SELECT password_hash FROM users WHERE id=?", userID).Scan(&hash); err == sql.ErrNoRows {
		utils.ErrorJSON(w, http.StatusNotFound, "User not found")
		return
	} else if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !utils.VerifyPassword(body.OldPassword, hash) {
		utils.ErrorJSON(w, http.StatusUnauthorized, "Current password is incorrect")
		return
	}

	newHash, err := utils.HashPassword(body.NewPassword)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := h.db.ExecContext(r.Context(),
		"UPDATE users SET password_hash=?, must_change_password=0 WHERE id=?",
		newHash, userID); err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	utils.OkJSON(w, map[string]string{"message": "Password changed successfully"})
}

// DeleteAccount handles DELETE /api/auth/delete-account
func (h *AuthHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.ErrorJSON(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if body.Password == "" {
		utils.ErrorJSON(w, http.StatusBadRequest, "Current password required")
		return
	}

	var hash string
	if err := h.db.QueryRowContext(r.Context(),
		"SELECT password_hash FROM users WHERE id=?", userID).Scan(&hash); err == sql.ErrNoRows {
		utils.ErrorJSON(w, http.StatusNotFound, "User not found")
		return
	} else if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !utils.VerifyPassword(body.Password, hash) {
		utils.ErrorJSON(w, http.StatusUnauthorized, "Current password is incorrect")
		return
	}

	if err := deleteUser(r.Context(), h.db, h.storageSvc, userID); err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	utils.NoContent(w)
}
