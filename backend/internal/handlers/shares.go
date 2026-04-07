package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/iii123iii/mycloud/backend/internal/auth"
	"github.com/iii123iii/mycloud/backend/internal/services"
	"github.com/iii123iii/mycloud/backend/internal/utils"
)

// SharesHandler handles /api/shares/* and /api/s/* routes.
type SharesHandler struct {
	db         *sql.DB
	storageSvc *services.StorageService
}

func NewSharesHandler(db *sql.DB, storageSvc *services.StorageService) *SharesHandler {
	return &SharesHandler{db: db, storageSvc: storageSvc}
}

// ListShares handles GET /api/shares
func (h *SharesHandler) ListShares(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	rows, err := h.db.QueryContext(r.Context(),
		"SELECT s.id,s.token,s.permission,s.expires_at,s.created_at,"+
			"s.file_id,s.folder_id,f.name AS file_name "+
			"FROM shares s LEFT JOIN files f ON s.file_id=f.id "+
			"WHERE s.created_by=? ORDER BY s.created_at DESC",
		userID)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type shareItem struct {
		ID         string  `json:"id"`
		Token      string  `json:"token"`
		Permission string  `json:"permission"`
		CreatedAt  string  `json:"created_at"`
		ExpiresAt  *string `json:"expires_at,omitempty"`
		FileID     *string `json:"file_id,omitempty"`
		FolderID   *string `json:"folder_id,omitempty"`
		FileName   *string `json:"file_name,omitempty"`
	}
	shares := []shareItem{}
	for rows.Next() {
		var s shareItem
		var expiresAt, fileID, folderID, fileName sql.NullString
		if err := rows.Scan(&s.ID, &s.Token, &s.Permission, &expiresAt, &s.CreatedAt,
			&fileID, &folderID, &fileName); err != nil {
			utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		if expiresAt.Valid {
			s.ExpiresAt = &expiresAt.String
		}
		if fileID.Valid {
			s.FileID = &fileID.String
		}
		if folderID.Valid {
			s.FolderID = &folderID.String
		}
		if fileName.Valid {
			s.FileName = &fileName.String
		}
		shares = append(shares, s)
	}
	utils.OkJSON(w, map[string]any{"shares": shares})
}

// CreateShare handles POST /api/shares
func (h *SharesHandler) CreateShare(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	var body struct {
		FileID     string `json:"file_id"`
		FolderID   string `json:"folder_id"`
		Permission string `json:"permission"`
		Password   string `json:"password"`
		ExpiresAt  string `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.ErrorJSON(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if body.FileID == "" && body.FolderID == "" {
		utils.ErrorJSON(w, http.StatusBadRequest, "file_id or folder_id required")
		return
	}
	if body.Permission != "read" && body.Permission != "write" {
		body.Permission = "read"
	}

	id := utils.GenerateUUID()
	token, err := services.GenerateShareToken()
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	var pwHash string
	if body.Password != "" {
		pwHash, err = services.HashSharePassword(body.Password)
		if err != nil {
			utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	var fileIDArg, folderIDArg, expiresAtArg, pwHashArg any
	if body.FileID != "" {
		fileIDArg = body.FileID
	}
	if body.FolderID != "" {
		folderIDArg = body.FolderID
	}
	if body.ExpiresAt != "" {
		expiresAtArg = body.ExpiresAt
	}
	if pwHash != "" {
		pwHashArg = pwHash
	}

	_, err = h.db.ExecContext(r.Context(),
		"INSERT INTO shares (id,token,file_id,folder_id,permission,password_hash,expires_at,created_by) "+
			"VALUES (?,?,?,?,?,?,?,?)",
		id, token, fileIDArg, folderIDArg, body.Permission, pwHashArg, expiresAtArg, userID)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	utils.CreatedJSON(w, map[string]any{"id": id, "token": token})
}

// DeleteShare handles DELETE /api/shares/{id}
func (h *SharesHandler) DeleteShare(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	shareID := chi.URLParam(r, "id")

	res, err := h.db.ExecContext(r.Context(),
		"DELETE FROM shares WHERE id=? AND created_by=?", shareID, userID)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		utils.ErrorJSON(w, http.StatusNotFound, "Share not found")
		return
	}
	utils.NoContent(w)
}

// ResolveShare handles GET /api/s/{token}  (public)
func (h *SharesHandler) ResolveShare(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	sharePassword := r.Header.Get("X-Share-Password")

	var (
		id          string
		fileID      sql.NullString
		folderID    sql.NullString
		permission  string
		pwHash      sql.NullString
		expiresAt   sql.NullString
	)
	err := h.db.QueryRowContext(r.Context(),
		"SELECT id,file_id,folder_id,permission,password_hash,expires_at "+
			"FROM shares WHERE token=?", token).
		Scan(&id, &fileID, &folderID, &permission, &pwHash, &expiresAt)
	if err == sql.ErrNoRows {
		utils.ErrorJSON(w, http.StatusNotFound, "Share not found")
		return
	}
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	if pwHash.Valid && pwHash.String != "" {
		if !services.VerifySharePassword(sharePassword, pwHash.String) {
			utils.ErrorJSON(w, http.StatusUnauthorized, "Password required or incorrect")
			return
		}
	}

	resp := map[string]any{
		"id":         id,
		"permission": permission,
	}
	if fileID.Valid {
		resp["file_id"] = fileID.String
		// Fetch file info
		var name string
		var size int64
		var mime string
		err := h.db.QueryRowContext(r.Context(),
			"SELECT name,size_bytes,mime_type FROM files WHERE id=? AND is_deleted=0", fileID.String).
			Scan(&name, &size, &mime)
		if err == nil {
			resp["file_name"] = name
			resp["file_size"] = size
			resp["file_mime"] = mime
		}
	}
	if folderID.Valid {
		resp["folder_id"] = folderID.String
	}
	if expiresAt.Valid {
		resp["expires_at"] = expiresAt.String
	}
	utils.OkJSON(w, resp)
}

// DownloadShare handles GET /api/s/{token}/download  (public)
func (h *SharesHandler) DownloadShare(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	sharePassword := r.Header.Get("X-Share-Password")

	var (
		fileID   sql.NullString
		pwHash   sql.NullString
	)
	err := h.db.QueryRowContext(r.Context(),
		"SELECT file_id,password_hash FROM shares WHERE token=?", token).
		Scan(&fileID, &pwHash)
	if err == sql.ErrNoRows {
		utils.ErrorJSON(w, http.StatusNotFound, "Share not found")
		return
	}
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	if pwHash.Valid && pwHash.String != "" {
		if !services.VerifySharePassword(sharePassword, pwHash.String) {
			utils.ErrorJSON(w, http.StatusUnauthorized, "Password required or incorrect")
			return
		}
	}
	if !fileID.Valid {
		utils.ErrorJSON(w, http.StatusBadRequest, "This share does not link to a file")
		return
	}

	var (
		ownerID      string
		fileName     string
		mimeType     string
		encKeyEncHex string
		encIVHex     string
		encTagHex    string
	)
	err = h.db.QueryRowContext(r.Context(),
		"SELECT user_id,name,mime_type,encryption_key_enc,encryption_iv,encryption_tag "+
			"FROM files WHERE id=? AND is_deleted=0", fileID.String).
		Scan(&ownerID, &fileName, &mimeType, &encKeyEncHex, &encIVHex, &encTagHex)
	if err == sql.ErrNoRows {
		utils.ErrorJSON(w, http.StatusNotFound, "File not found")
		return
	}
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	bundle := &services.EncryptedKeyBundle{
		IVHex:     encIVHex,
		EncKeyHex: encKeyEncHex,
		TagHex:    encTagHex,
	}
	fileKey, err := services.UnwrapKey(bundle)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, "Key unwrap failed: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"`, sanitizeFilename(fileName)))

	filePath := h.storageSvc.FilePath(ownerID, fileID.String)
	_ = services.DecryptFileStream(filePath, fileKey, func(chunk []byte) error {
		_, err := w.Write(chunk)
		return err
	})
}
