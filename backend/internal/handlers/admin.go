package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/iii123iii/mycloud/backend/internal/auth"
	"github.com/iii123iii/mycloud/backend/internal/services"
	"github.com/iii123iii/mycloud/backend/internal/utils"
)

// AdminHandler handles /api/admin/* routes (all require admin role).
type AdminHandler struct {
	db         *sql.DB
	storageSvc *services.StorageService
}

func NewAdminHandler(db *sql.DB, storageSvc *services.StorageService) *AdminHandler {
	return &AdminHandler{db: db, storageSvc: storageSvc}
}

// ListUsers handles GET /api/admin/users
func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(),
		"SELECT id,username,email,role,quota_bytes,used_bytes,is_active,created_at FROM users ORDER BY created_at DESC")
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type userItem struct {
		ID         string `json:"id"`
		Username   string `json:"username"`
		Email      string `json:"email"`
		Role       string `json:"role"`
		QuotaBytes int64  `json:"quota_bytes"`
		UsedBytes  int64  `json:"used_bytes"`
		IsActive   bool   `json:"is_active"`
		CreatedAt  string `json:"created_at"`
	}
	users := []userItem{}
	for rows.Next() {
		var u userItem
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.QuotaBytes, &u.UsedBytes, &u.IsActive, &u.CreatedAt); err != nil {
			utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		users = append(users, u)
	}
	utils.OkJSON(w, map[string]any{"users": users})
}

// CreateUser handles POST /api/admin/users
func (h *AdminHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username   string `json:"username"`
		Email      string `json:"email"`
		Password   string `json:"password"`
		Role       string `json:"role"`
		QuotaBytes int64  `json:"quota_bytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.ErrorJSON(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if body.Username == "" || body.Email == "" || len(body.Password) < 8 {
		utils.ErrorJSON(w, http.StatusBadRequest, "username, email, password(min 8) required")
		return
	}
	if body.Role != "admin" && body.Role != "user" {
		body.Role = "user"
	}
	if body.QuotaBytes <= 0 {
		body.QuotaBytes = 10737418240
	}

	hash, err := utils.HashPassword(body.Password)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	id := utils.GenerateUUID()

	_, err = h.db.ExecContext(r.Context(),
		"INSERT INTO users (id,username,email,password_hash,role,quota_bytes) VALUES (?,?,?,?,?,?)",
		id, body.Username, body.Email, hash, body.Role, body.QuotaBytes)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") || strings.Contains(err.Error(), "duplicate") {
			utils.ErrorJSON(w, http.StatusConflict, "Username or email already taken")
			return
		}
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	utils.CreatedJSON(w, map[string]any{"id": id, "username": body.Username})
}

// UpdateUser handles PATCH /api/admin/users/{id}
func (h *AdminHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")

	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.ErrorJSON(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	setClauses := []string{}
	args := []any{}

	if raw, ok := body["is_active"]; ok {
		var v bool
		if err := json.Unmarshal(raw, &v); err == nil {
			setClauses = append(setClauses, "is_active=?")
			val := 0
			if v {
				val = 1
			}
			args = append(args, val)
		}
	}
	if raw, ok := body["role"]; ok {
		var v string
		if err := json.Unmarshal(raw, &v); err == nil && (v == "admin" || v == "user") {
			setClauses = append(setClauses, "role=?")
			args = append(args, v)
		}
	}
	if raw, ok := body["quota_bytes"]; ok {
		var v int64
		if err := json.Unmarshal(raw, &v); err == nil {
			setClauses = append(setClauses, "quota_bytes=?")
			args = append(args, v)
		}
	}
	if raw, ok := body["password"]; ok {
		var v string
		if err := json.Unmarshal(raw, &v); err == nil && len(v) >= 8 {
			hash, err := utils.HashPassword(v)
			if err == nil {
				setClauses = append(setClauses, "password_hash=?")
				args = append(args, hash)
				setClauses = append(setClauses, "must_change_password=?")
				args = append(args, 1)
			}
		}
	}
	if raw, ok := body["must_change_password"]; ok {
		var v bool
		if err := json.Unmarshal(raw, &v); err == nil {
			setClauses = append(setClauses, "must_change_password=?")
			val := 0
			if v {
				val = 1
			}
			args = append(args, val)
		}
	}

	if len(setClauses) == 0 {
		utils.ErrorJSON(w, http.StatusBadRequest, "Nothing to update")
		return
	}

	// #nosec G201 — setClauses are built from a hardcoded allowlist above
	query := "UPDATE users SET " + strings.Join(setClauses, ",") + " WHERE id=?"
	args = append(args, targetID)

	if _, err := h.db.ExecContext(r.Context(), query, args...); err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	utils.OkJSON(w, map[string]string{"message": "User updated"})
}

// DeleteUser handles DELETE /api/admin/users/{id}
func (h *AdminHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	selfID := auth.UserIDFromCtx(r.Context())
	targetID := chi.URLParam(r, "id")

	if targetID == selfID {
		utils.ErrorJSON(w, http.StatusBadRequest, "Cannot delete your own account")
		return
	}

	if err := deleteUser(r.Context(), h.db, h.storageSvc, targetID); err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	utils.NoContent(w)
}

// GetStats handles GET /api/admin/stats
func (h *AdminHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	var totalUsers, totalFiles int64
	var totalStorageUsed, totalQuota int64

	err := h.db.QueryRowContext(r.Context(),
		"SELECT (SELECT COUNT(*) FROM users) AS total_users,"+
			"(SELECT COUNT(*) FROM files WHERE is_deleted=0) AS total_files,"+
			"(SELECT COALESCE(SUM(size_bytes),0) FROM files WHERE is_deleted=0) AS total_storage_used,"+
			"(SELECT COALESCE(SUM(quota_bytes),0) FROM users) AS total_quota").
		Scan(&totalUsers, &totalFiles, &totalStorageUsed, &totalQuota)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	utils.OkJSON(w, map[string]any{
		"total_users":         totalUsers,
		"total_files":         totalFiles,
		"total_storage_used":  totalStorageUsed,
		"total_quota":         totalQuota,
	})
}

// GetLogs handles GET /api/admin/logs
func (h *AdminHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	limit := safeInt(r.URL.Query().Get("limit"), 100, 1, 10000)

	query := `SELECT a.id,a.action,a.resource_type,a.resource_id,a.ip_address,a.created_at,u.username
FROM activity_log a LEFT JOIN users u ON a.user_id=u.id
ORDER BY a.created_at DESC LIMIT ?`

	rows, err := h.db.QueryContext(r.Context(), query, limit)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type logItem struct {
		ID           int64   `json:"id"`
		Action       string  `json:"action"`
		ResourceType *string `json:"resource_type,omitempty"`
		ResourceID   *string `json:"resource_id,omitempty"`
		IPAddress    string  `json:"ip_address"`
		CreatedAt    string  `json:"created_at"`
		Username     *string `json:"username,omitempty"`
	}
	logs := []logItem{}
	for rows.Next() {
		var l logItem
		var resourceType, resourceID, username sql.NullString
		if err := rows.Scan(&l.ID, &l.Action, &resourceType, &resourceID, &l.IPAddress, &l.CreatedAt, &username); err != nil {
			continue
		}
		if resourceType.Valid {
			l.ResourceType = &resourceType.String
		}
		if resourceID.Valid {
			l.ResourceID = &resourceID.String
		}
		if username.Valid {
			l.Username = &username.String
		}
		logs = append(logs, l)
	}
	utils.OkJSON(w, map[string]any{"logs": logs})
}

// GetSettings handles GET /api/admin/settings
func (h *AdminHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(), "SELECT key_name, value FROM settings")
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	settings := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err == nil {
			settings[k] = v
		}
	}
	utils.OkJSON(w, settings)
}

// PutSettings handles PUT /api/admin/settings
func (h *AdminHandler) PutSettings(w http.ResponseWriter, r *http.Request) {
	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.ErrorJSON(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	allowed := map[string]bool{
		"registration_enabled": true,
		"default_quota_bytes":  true,
	}

	for key := range allowed {
		rawVal, ok := body[key]
		if !ok {
			continue
		}
		var val string
		// Accept both string and numeric values
		var strVal string
		if err := json.Unmarshal(rawVal, &strVal); err == nil {
			val = strVal
		} else {
			val = string(rawVal)
		}
		_, err := h.db.ExecContext(r.Context(),
			"INSERT INTO settings (key_name,value) VALUES (?,?) ON DUPLICATE KEY UPDATE value=?",
			key, val, val)
		if err != nil {
			utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	utils.OkJSON(w, map[string]string{"message": "Settings updated"})
}
