package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/iii123iii/mycloud/backend/internal/utils"
)

// SetupHandler handles /api/setup/* routes.
type SetupHandler struct {
	db *sql.DB
}

func NewSetupHandler(db *sql.DB) *SetupHandler {
	return &SetupHandler{db: db}
}

// Status handles GET /api/setup/status  (public)
func (h *SetupHandler) Status(w http.ResponseWriter, r *http.Request) {
	var value string
	err := h.db.QueryRowContext(r.Context(),
		"SELECT value FROM settings WHERE key_name='setup_complete'").Scan(&value)
	complete := err == nil && value == "true"
	utils.OkJSON(w, map[string]bool{"setup_complete": complete})
}

// Complete handles POST /api/setup/complete  (public)
func (h *SetupHandler) Complete(w http.ResponseWriter, r *http.Request) {
	// Check if already complete
	var value string
	err := h.db.QueryRowContext(r.Context(),
		"SELECT value FROM settings WHERE key_name='setup_complete'").Scan(&value)
	if err == nil && value == "true" {
		utils.ErrorJSON(w, http.StatusConflict, "Setup already completed")
		return
	}

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
		utils.ErrorJSON(w, http.StatusBadRequest, "username (min 3), email, and password (min 8 chars) required")
		return
	}

	hash, err := utils.HashPassword(body.Password)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	id := utils.GenerateUUID()

	_, err = h.db.ExecContext(r.Context(),
		"INSERT INTO users (id,username,email,password_hash,role,quota_bytes) VALUES (?,?,?,?,?,?)",
		id, body.Username, body.Email, hash, "admin", 107374182400)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	_, _ = h.db.ExecContext(r.Context(),
		"INSERT INTO settings (key_name,value) VALUES ('setup_complete','true') "+
			"ON DUPLICATE KEY UPDATE value='true'")

	utils.CreatedJSON(w, map[string]string{"message": "Setup complete", "user_id": id})
}
