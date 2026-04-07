package app

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"mycloud/backend-go/internal/auth"
	"mycloud/backend-go/internal/httpapi"
)

func (a *App) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT id, username, email, role, quota_bytes, used_bytes, is_active, must_change_password, created_at
		FROM users ORDER BY created_at DESC`)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()
	users := make([]map[string]any, 0)
	for rows.Next() {
		var id, username, email, role, createdAt string
		var quota, used int64
		var active, mustChange bool
		if err := rows.Scan(&id, &username, &email, &role, &quota, &used, &active, &mustChange, &createdAt); err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		users = append(users, map[string]any{
			"id":                   id,
			"username":             username,
			"email":                email,
			"role":                 role,
			"quota_bytes":          quota,
			"used_bytes":           used,
			"is_active":            active,
			"must_change_password": mustChange,
			"created_at":           createdAt,
		})
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"users": users}, nil)
}

func (a *App) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Username           string `json:"username"`
		Email              string `json:"email"`
		Password           string `json:"password"`
		Role               string `json:"role"`
		QuotaBytes         int64  `json:"quota_bytes"`
		IsActive           *bool  `json:"is_active"`
		MustChangePassword *bool  `json:"must_change_password"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	if payload.Role == "" {
		payload.Role = "user"
	}
	if payload.QuotaBytes <= 0 {
		payload.QuotaBytes = 10737418240
	}
	active := true
	if payload.IsActive != nil {
		active = *payload.IsActive
	}
	mustChange := false
	if payload.MustChangePassword != nil {
		mustChange = *payload.MustChangePassword
	}
	hash, err := auth.HashPassword(payload.Password)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "hash_error", err.Error())
		return
	}
	id := uuid.NewString()
	if _, err := a.DB.ExecContext(r.Context(), `
		INSERT INTO users (id, username, email, password_hash, role, quota_bytes, used_bytes, is_active, must_change_password)
		VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		id, payload.Username, strings.ToLower(strings.TrimSpace(payload.Email)), hash, payload.Role, payload.QuotaBytes, active, mustChange); err != nil {
		httpapi.Error(w, http.StatusConflict, "user_exists", "User with this email or username already exists")
		return
	}
	httpapi.JSON(w, http.StatusCreated, map[string]any{"id": id}, nil)
}

func (a *App) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var payload map[string]any
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	if password, ok := payload["password"].(string); ok && password != "" {
		hash, err := auth.HashPassword(password)
		if err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "hash_error", err.Error())
			return
		}
		if _, err := a.DB.ExecContext(r.Context(), "UPDATE users SET password_hash=? WHERE id=?", hash, id); err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
	}
	if role, ok := payload["role"].(string); ok && role != "" {
		_, _ = a.DB.ExecContext(r.Context(), "UPDATE users SET role=? WHERE id=?", role, id)
	}
	if quota, ok := payload["quota_bytes"].(float64); ok {
		_, _ = a.DB.ExecContext(r.Context(), "UPDATE users SET quota_bytes=? WHERE id=?", int64(quota), id)
	}
	if active, ok := payload["is_active"].(bool); ok {
		_, _ = a.DB.ExecContext(r.Context(), "UPDATE users SET is_active=? WHERE id=?", active, id)
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"message": "Updated"}, nil)
}

func (a *App) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := a.deleteUserResources(r.Context(), id); err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpapi.Error(w, http.StatusInternalServerError, "delete_failed", err.Error())
		return
	}
	if _, err := a.DB.ExecContext(r.Context(), "DELETE FROM users WHERE id=?", id); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	httpapi.NoContent(w)
}

func (a *App) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	var totalUsers, totalFiles, totalStorage, totalQuota int64
	_ = a.DB.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM users").Scan(&totalUsers)
	_ = a.DB.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM files WHERE is_deleted=0").Scan(&totalFiles)
	_ = a.DB.QueryRowContext(r.Context(), "SELECT COALESCE(SUM(used_bytes), 0), COALESCE(SUM(quota_bytes), 0) FROM users").Scan(&totalStorage, &totalQuota)
	httpapi.JSON(w, http.StatusOK, map[string]any{
		"total_users":        totalUsers,
		"total_files":        totalFiles,
		"total_storage_used": totalStorage,
		"total_quota":        totalQuota,
	}, nil)
}

func (a *App) handleAdminLogs(w http.ResponseWriter, r *http.Request) {
	limit := qInt(r, "limit", 100, 1, 500)
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT al.id, al.action, u.username, al.resource_type, al.ip_address, al.created_at
		FROM activity_log al
		LEFT JOIN users u ON u.id = al.user_id
		ORDER BY al.created_at DESC LIMIT ?`, limit)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()
	logs := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var action, createdAt string
		var username, resourceType, ip sql.NullString
		if err := rows.Scan(&id, &action, &username, &resourceType, &ip, &createdAt); err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		item := map[string]any{"id": id, "action": action, "created_at": createdAt}
		if username.Valid {
			item["username"] = username.String
		}
		if resourceType.Valid {
			item["resource_type"] = resourceType.String
		}
		if ip.Valid {
			item["ip_address"] = ip.String
		}
		logs = append(logs, item)
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"logs": logs}, nil)
}

func (a *App) handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.QueryContext(r.Context(), "SELECT key_name, value FROM settings")
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()
	settings := make(map[string]string)
	for rows.Next() {
		var key string
		var value sql.NullString
		if err := rows.Scan(&key, &value); err == nil {
			settings[key] = value.String
		}
	}
	httpapi.JSON(w, http.StatusOK, settings, nil)
}

func (a *App) handleAdminPutSettings(w http.ResponseWriter, r *http.Request) {
	var payload map[string]string
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer tx.Rollback()
	for key, value := range payload {
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO settings (key_name, value) VALUES (?, ?)
			ON DUPLICATE KEY UPDATE value=VALUES(value)`, key, value); err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
	}
	if err := tx.Commit(); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"message": "Settings updated"}, nil)
}

func (a *App) handleAdminUpdateCheck(w http.ResponseWriter, r *http.Request) {
	info := map[string]any{
		"current":               a.Config.Version,
		"latest":                a.Config.Version,
		"update_available":      false,
		"release_url":           "",
		"release_name":          a.Config.Version,
		"published_at":          a.StartUTC.Format("2006-01-02T15:04:05Z"),
		"release_notes":         "",
		"apply_supported":       a.Config.UpdaterURL != "",
		"apply_message":         "Updater service not configured",
		"update_in_progress":    false,
		"update_status":         "idle",
		"update_status_message": "Idle",
		"update_log_path":       a.Config.UpdateLogPath,
		"last_started_target":   "",
	}
	if a.Config.UpdaterURL != "" {
		info["apply_message"] = "Updater service available"
		resp, err := a.Client.Get(strings.TrimRight(a.Config.UpdaterURL, "/") + "/status")
		if err == nil {
			defer resp.Body.Close()
			var status map[string]any
			if json.NewDecoder(resp.Body).Decode(&status) == nil {
				info["update_in_progress"] = status["in_progress"]
				if v, ok := status["status"].(string); ok {
					info["update_status"] = v
				}
				if v, ok := status["message"].(string); ok && v != "" {
					info["update_status_message"] = v
				}
				if v, ok := status["target_version"].(string); ok {
					info["last_started_target"] = v
				}
			}
		}
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", a.Config.GitHubRepo)
	resp, err := a.Client.Get(url)
	if err == nil {
		defer resp.Body.Close()
		var gh struct {
			TagName     string `json:"tag_name"`
			Name        string `json:"name"`
			HTMLURL     string `json:"html_url"`
			PublishedAt string `json:"published_at"`
			Body        string `json:"body"`
		}
		if json.NewDecoder(resp.Body).Decode(&gh) == nil && gh.TagName != "" {
			info["latest"] = gh.TagName
			info["release_name"] = gh.Name
			info["release_url"] = gh.HTMLURL
			info["published_at"] = gh.PublishedAt
			info["release_notes"] = gh.Body
			info["update_available"] = gh.TagName != a.Config.Version
		}
	}
	httpapi.JSON(w, http.StatusOK, info, nil)
}

// handleAdminUpdateStatus returns only the updater's in-progress status.
// The frontend polls this during an update — it never touches the GitHub API.
func (a *App) handleAdminUpdateStatus(w http.ResponseWriter, r *http.Request) {
	info := map[string]any{
		"update_in_progress":    false,
		"update_status":         "idle",
		"update_status_message": "Idle",
	}
	if a.Config.UpdaterURL != "" {
		resp, err := a.Client.Get(strings.TrimRight(a.Config.UpdaterURL, "/") + "/status")
		if err == nil {
			defer resp.Body.Close()
			var status map[string]any
			if json.NewDecoder(resp.Body).Decode(&status) == nil {
				info["update_in_progress"] = status["in_progress"]
				statusStr, _ := status["status"].(string)
				if statusStr != "" {
					info["update_status"] = statusStr
				}
				if v, ok := status["message"].(string); ok && v != "" {
					info["update_status_message"] = v
				}

				// Write a one-time activity log entry when the update finishes.
				if (statusStr == "succeeded" || statusStr == "failed") {
					a.updateMu.Lock()
					shouldLog := a.updateStarted && !a.updateCompletionLogged
					if shouldLog {
						a.updateCompletionLogged = true
					}
					a.updateMu.Unlock()
					if shouldLog {
						action := "update_succeeded"
						if statusStr == "failed" {
							action = "update_failed"
						}
						uid := userIDFrom(r)
						msg, _ := status["message"].(string)
						writeActivity(r.Context(), a.DB, &uid, action, "system", "", clientIP(r), map[string]any{"message": msg})
					}
				}
			}
		}
	}
	httpapi.JSON(w, http.StatusOK, info, nil)
}

func (a *App) handleAdminUpdateApply(w http.ResponseWriter, r *http.Request) {
	if a.Config.UpdaterURL == "" {
		httpapi.Error(w, http.StatusNotImplemented, "unsupported", "Updater service not configured")
		return
	}
	checkReq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, strings.TrimRight(a.Config.UpdaterURL, "/")+"/status", nil)
	statusResp, err := a.Client.Do(checkReq)
	if err != nil {
		httpapi.Error(w, http.StatusBadGateway, "updater_error", err.Error())
		return
	}
	_ = statusResp.Body.Close()
	payload := map[string]string{
		"target_version":  r.URL.Query().Get("target"),
		"current_version": a.Config.Version,
	}
	if payload["target_version"] == "" {
		payload["target_version"] = r.URL.Query().Get("target")
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, strings.TrimRight(a.Config.UpdaterURL, "/")+"/update", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.Client.Do(req)
	if err != nil {
		httpapi.Error(w, http.StatusBadGateway, "updater_error", err.Error())
		return
	}
	defer resp.Body.Close()
	var data map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&data)
	message, _ := data["message"].(string)
	if resp.StatusCode >= 300 {
		httpapi.Error(w, resp.StatusCode, "updater_error", message)
		return
	}
	// Record the update start in the activity log and mark this session as having started one.
	uid := userIDFrom(r)
	writeActivity(r.Context(), a.DB, &uid, "update_started", "system", payload["target_version"], clientIP(r), map[string]any{
		"target_version":  payload["target_version"],
		"current_version": payload["current_version"],
	})
	a.updateMu.Lock()
	a.updateStarted = true
	a.updateCompletionLogged = false
	a.updateMu.Unlock()

	httpapi.JSON(w, http.StatusAccepted, map[string]any{"message": message}, nil)
}

func (a *App) handleAdminUpdateLog(w http.ResponseWriter, r *http.Request) {
	if a.Config.UpdaterURL == "" {
		httpapi.JSON(w, http.StatusOK, map[string]any{"lines": []string{}}, nil)
		return
	}
	resp, err := a.Client.Get(strings.TrimRight(a.Config.UpdaterURL, "/") + "/log")
	if err != nil {
		httpapi.Error(w, http.StatusBadGateway, "updater_error", err.Error())
		return
	}
	defer resp.Body.Close()
	var data map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&data)
	httpapi.JSON(w, http.StatusOK, data, nil)
}
