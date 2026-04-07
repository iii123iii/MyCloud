package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/iii123iii/mycloud/backend/internal/auth"
	"github.com/iii123iii/mycloud/backend/internal/services"
	"github.com/iii123iii/mycloud/backend/internal/utils"
)

// FilesHandler handles /api/files/* and /api/storage/stats.
type FilesHandler struct {
	db         *sql.DB
	storageSvc *services.StorageService
}

func NewFilesHandler(db *sql.DB, storageSvc *services.StorageService) *FilesHandler {
	return &FilesHandler{db: db, storageSvc: storageSvc}
}

// ListFiles handles GET /api/files
func (h *FilesHandler) ListFiles(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	q := r.URL.Query()

	folderId := q.Get("folder_id")
	all := q.Get("all") == "1"
	starredOnly := q.Get("starred_only") == "1"
	sortField := q.Get("sort")
	order := q.Get("order")
	page := safeInt(q.Get("page"), 1, 1, 100000)
	pageSize := safeInt(q.Get("page_size"), 50, 1, 200)
	offset := (page - 1) * pageSize

	if sortField == "" {
		sortField = "name"
	}
	if order == "" {
		order = "asc"
	}
	validSort := map[string]bool{"name": true, "size_bytes": true, "created_at": true, "updated_at": true}
	if !validSort[sortField] {
		sortField = "name"
	}
	if order != "asc" && order != "desc" {
		order = "asc"
	}

	where := "WHERE user_id=? AND is_deleted=0"
	args := []any{userID}

	if starredOnly {
		where += " AND is_starred=1"
	}
	if !all {
		if folderId == "" {
			where += " AND folder_id IS NULL"
		} else {
			where += " AND folder_id=?"
			args = append(args, folderId)
		}
	}

	countSQL := "SELECT COUNT(*) AS total FROM files " + where
	// #nosec G201 — sortField and order are validated against allowlists above
	dataSQL := fmt.Sprintf(
		"SELECT id,name,size_bytes,mime_type,folder_id,is_starred,created_at,updated_at "+
			"FROM files %s ORDER BY %s %s LIMIT %d OFFSET %d",
		where, sortField, order, pageSize, offset)

	var total int64
	if err := h.db.QueryRowContext(r.Context(), countSQL, args...).Scan(&total); err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	rows, err := h.db.QueryContext(r.Context(), dataSQL, args...)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type fileItem struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		SizeBytes int64   `json:"size_bytes"`
		MimeType  string  `json:"mime_type"`
		FolderID  *string `json:"folder_id,omitempty"`
		IsStarred bool    `json:"is_starred"`
		CreatedAt string  `json:"created_at"`
		UpdatedAt string  `json:"updated_at"`
	}
	files := []fileItem{}
	for rows.Next() {
		var f fileItem
		var folderID sql.NullString
		if err := rows.Scan(&f.ID, &f.Name, &f.SizeBytes, &f.MimeType, &folderID, &f.IsStarred, &f.CreatedAt, &f.UpdatedAt); err != nil {
			utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		if folderID.Valid {
			f.FolderID = &folderID.String
		}
		files = append(files, f)
	}

	utils.OkJSON(w, map[string]any{
		"files":     files,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"has_more":  int64(page*pageSize) < total,
	})
}

// Upload handles POST /api/files/upload (multipart)
func (h *FilesHandler) Upload(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())

	// Parse multipart (limit 10 GiB)
	if err := r.ParseMultipartForm(10 << 30); err != nil {
		utils.ErrorJSON(w, http.StatusBadRequest, "Multipart parse error: "+err.Error())
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		utils.ErrorJSON(w, http.StatusBadRequest, "No file in request")
		return
	}
	defer file.Close()

	filename := fileHeader.Filename
	if filename == "" {
		filename = "untitled"
	}
	folderId := r.FormValue("folder_id")

	// Read entire file into memory (consistent with C++ behaviour)
	data, err := io.ReadAll(file)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, "Failed to read uploaded file")
		return
	}
	size := int64(len(data))

	// Check quota
	var quotaBytes, usedBytes int64
	if err := h.db.QueryRowContext(r.Context(),
		"SELECT quota_bytes, used_bytes FROM users WHERE id=?", userID).
		Scan(&quotaBytes, &usedBytes); err == sql.ErrNoRows {
		utils.ErrorJSON(w, http.StatusNotFound, "User not found")
		return
	} else if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if usedBytes+size > quotaBytes {
		utils.ErrorJSON(w, http.StatusRequestEntityTooLarge, "Storage quota exceeded")
		return
	}

	fileID := utils.GenerateUUID()
	mime := mimeFromFilename(filename)

	bundle, err := h.storageSvc.StoreFile(userID, fileID, data)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, "Encryption failed: "+err.Error())
		return
	}
	storagePath := h.storageSvc.FilePath(userID, fileID)

	var folderIDArg any
	if folderId == "" {
		folderIDArg = nil
	} else {
		folderIDArg = folderId
	}

	_, err = h.db.ExecContext(r.Context(),
		"INSERT INTO files (id,name,storage_path,size_bytes,mime_type,user_id,folder_id,"+
			"encryption_key_enc,encryption_iv,encryption_tag) VALUES (?,?,?,?,?,?,?,?,?,?)",
		fileID, filename, storagePath, size, mime, userID, folderIDArg,
		bundle.EncKeyHex, bundle.IVHex, bundle.TagHex)
	if err != nil {
		h.storageSvc.DeleteFile(userID, fileID)
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	if _, err := h.db.ExecContext(r.Context(),
		"UPDATE users SET used_bytes=used_bytes+? WHERE id=?", size, userID); err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	var folderIDResp any
	if folderId == "" {
		folderIDResp = nil
	} else {
		folderIDResp = folderId
	}
	utils.CreatedJSON(w, map[string]any{
		"message":   "File uploaded",
		"id":        fileID,
		"name":      filename,
		"folder_id": folderIDResp,
	})
}

// StorageStats handles GET /api/storage/stats
func (h *FilesHandler) StorageStats(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())

	var usedBytes, quotaBytes int64
	if err := h.db.QueryRowContext(r.Context(),
		"SELECT used_bytes, quota_bytes FROM users WHERE id=?", userID).
		Scan(&usedBytes, &quotaBytes); err == sql.ErrNoRows {
		utils.ErrorJSON(w, http.StatusNotFound, "User not found")
		return
	} else if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	var fileCount int64
	if err := h.db.QueryRowContext(r.Context(),
		"SELECT COUNT(*) FROM files WHERE user_id=? AND is_deleted=0", userID).
		Scan(&fileCount); err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	var folderCount int64
	if err := h.db.QueryRowContext(r.Context(),
		"SELECT COUNT(*) FROM folders WHERE user_id=? AND is_deleted=0", userID).
		Scan(&folderCount); err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.OkJSON(w, map[string]any{
		"used_bytes":   usedBytes,
		"quota_bytes":  quotaBytes,
		"file_count":   fileCount,
		"folder_count": folderCount,
	})
}

// Download handles GET /api/files/{id}/download
func (h *FilesHandler) Download(w http.ResponseWriter, r *http.Request) {
	h.serveFile(w, r, chi.URLParam(r, "id"), "attachment")
}

// Preview handles GET /api/files/{id}/preview
func (h *FilesHandler) Preview(w http.ResponseWriter, r *http.Request) {
	h.serveFile(w, r, chi.URLParam(r, "id"), "inline")
}

func (h *FilesHandler) serveFile(w http.ResponseWriter, r *http.Request, fileID, disposition string) {
	userID := auth.UserIDFromCtx(r.Context())

	var (
		ownerID       string
		fileName      string
		mimeType      string
		encKeyEncHex  string
		encIVHex      string
		encTagHex     string
	)
	err := h.db.QueryRowContext(r.Context(),
		"SELECT user_id,name,mime_type,encryption_key_enc,encryption_iv,encryption_tag "+
			"FROM files WHERE id=? AND user_id=? AND is_deleted=0",
		fileID, userID).
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
		fmt.Sprintf(`%s; filename="%s"`, disposition, sanitizeFilename(fileName)))

	filePath := h.storageSvc.FilePath(ownerID, fileID)
	if err := services.DecryptFileStream(filePath, fileKey, func(chunk []byte) error {
		_, err := w.Write(chunk)
		return err
	}); err != nil {
		// Headers may already be sent; best effort
		_ = err
	}
}

// GetInfo handles GET /api/files/{id}
func (h *FilesHandler) GetInfo(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	fileID := chi.URLParam(r, "id")

	var (
		id        string
		name      string
		sizeBytes int64
		mimeType  string
		folderID  sql.NullString
		isStarred bool
		isDeleted bool
		createdAt string
		updatedAt string
	)
	err := h.db.QueryRowContext(r.Context(),
		"SELECT id,name,size_bytes,mime_type,folder_id,is_starred,is_deleted,created_at,updated_at "+
			"FROM files WHERE id=? AND user_id=?",
		fileID, userID).
		Scan(&id, &name, &sizeBytes, &mimeType, &folderID, &isStarred, &isDeleted, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		utils.ErrorJSON(w, http.StatusNotFound, "File not found")
		return
	}
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]any{
		"id":         id,
		"name":       name,
		"size_bytes": sizeBytes,
		"mime_type":  mimeType,
		"is_starred": isStarred,
		"is_deleted": isDeleted,
		"created_at": createdAt,
		"updated_at": updatedAt,
	}
	if folderID.Valid {
		resp["folder_id"] = folderID.String
	}
	utils.OkJSON(w, resp)
}

// UpdateFile handles PATCH /api/files/{id}
func (h *FilesHandler) UpdateFile(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	fileID := chi.URLParam(r, "id")

	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.ErrorJSON(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if rawStarred, ok := body["is_starred"]; ok && len(body) == 1 {
		var starred bool
		if err := json.Unmarshal(rawStarred, &starred); err != nil {
			utils.ErrorJSON(w, http.StatusBadRequest, "Invalid is_starred value")
			return
		}
		val := 0
		if starred {
			val = 1
		}
		_, err := h.db.ExecContext(r.Context(),
			"UPDATE files SET is_starred=? WHERE id=? AND user_id=? AND is_deleted=0",
			val, fileID, userID)
		if err != nil {
			utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		utils.OkJSON(w, map[string]string{"message": "Updated"})
		return
	}

	if rawName, ok := body["name"]; ok {
		var name string
		if err := json.Unmarshal(rawName, &name); err != nil || name == "" {
			utils.ErrorJSON(w, http.StatusBadRequest, "Invalid name")
			return
		}
		_, err := h.db.ExecContext(r.Context(),
			"UPDATE files SET name=? WHERE id=? AND user_id=? AND is_deleted=0",
			name, fileID, userID)
		if err != nil {
			utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		utils.OkJSON(w, map[string]string{"message": "Updated"})
		return
	}

	if rawFolderID, ok := body["folder_id"]; ok {
		if string(rawFolderID) == "null" {
			_, err := h.db.ExecContext(r.Context(),
				"UPDATE files SET folder_id=NULL WHERE id=? AND user_id=? AND is_deleted=0",
				fileID, userID)
			if err != nil {
				utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
				return
			}
		} else {
			var folderID string
			if err := json.Unmarshal(rawFolderID, &folderID); err != nil {
				utils.ErrorJSON(w, http.StatusBadRequest, "Invalid folder_id")
				return
			}
			_, err := h.db.ExecContext(r.Context(),
				"UPDATE files SET folder_id=? WHERE id=? AND user_id=? AND is_deleted=0",
				folderID, fileID, userID)
			if err != nil {
				utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		utils.OkJSON(w, map[string]string{"message": "Updated"})
		return
	}

	utils.ErrorJSON(w, http.StatusBadRequest, "Nothing to update")
}

// DeleteFile handles DELETE /api/files/{id}
func (h *FilesHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	fileID := chi.URLParam(r, "id")

	res, err := h.db.ExecContext(r.Context(),
		"UPDATE files SET is_deleted=1, deleted_at=NOW() WHERE id=? AND user_id=? AND is_deleted=0",
		fileID, userID)
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		utils.ErrorJSON(w, http.StatusNotFound, "File not found")
		return
	}
	utils.NoContent(w)
}
