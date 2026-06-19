package app

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"mycloud/backend-go/internal/httpapi"
	"mycloud/backend-go/internal/storage"
)

// supportedOfficeMimes maps the MIME types OnlyOffice can edit to the file
// extension OnlyOffice expects in its document config.
var supportedOfficeMimes = map[string]string{
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   "docx",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         "xlsx",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": "pptx",
}

// handleOfficeConfig returns a signed config object for the OnlyOffice
// document editor. The returned URL pointing at the document is short-lived;
// the callback URL is internal-network (Docker DNS) so OnlyOffice can reach
// the backend.
func (a *App) handleOfficeConfig(w http.ResponseWriter, r *http.Request) {
	if a.Config.OfficeURL == "" || a.Config.OfficeJWTSecret == "" {
		httpapi.Error(w, http.StatusNotImplemented, "office_disabled", "Office editing is not configured")
		return
	}
	userID := userIDFrom(r)
	fileID := chi.URLParam(r, "id")
	if _, err := a.canAccessFile(r.Context(), userID, fileID, AccessViewer); err != nil {
		httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
		return
	}

	var fileName, mimeType string
	if err := a.DB.QueryRowContext(r.Context(),
		"SELECT name, mime_type FROM files WHERE id=?", fileID,
	).Scan(&fileName, &mimeType); err != nil {
		respondDBError(w, r, err)
		return
	}
	ext, ok := supportedOfficeMimes[mimeType]
	if !ok {
		// Fall back to the file extension.
		ext = strings.ToLower(strings.TrimPrefix(filepath.Ext(fileName), "."))
	}

	// Issue a short-lived edit key for this user/file.
	editKey := uuid.NewString() + "-" + randomHex(8)
	sessionID := uuid.NewString()
	if _, err := a.DB.ExecContext(r.Context(), `
		INSERT INTO office_sessions (id, file_id, user_id, edit_key)
		VALUES (?, ?, ?, ?)`,
		sessionID, fileID, userID, editKey); err != nil {
		respondDBError(w, r, err)
		return
	}

	publicBase := a.publicBackendURL(r)
	docURL := fmt.Sprintf("%s/api/v2/office/doc/%s?key=%s", publicBase, fileID, editKey)
	callbackURL := fmt.Sprintf("%s/api/v2/office/callback/%s?key=%s", publicBase, fileID, editKey)

	user, _ := a.lookupUsername(r.Context(), userID)

	config := map[string]any{
		"document": map[string]any{
			"fileType": ext,
			"key":      editKey, // version identifier; changes invalidate cached edits
			"title":    fileName,
			"url":      docURL,
		},
		"documentType": officeDocumentType(ext),
		"editorConfig": map[string]any{
			"callbackUrl": callbackURL,
			"user": map[string]any{
				"id":   userID,
				"name": user,
			},
			"mode": "edit",
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims(config)).
		SignedString([]byte(a.Config.OfficeJWTSecret))
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "jwt_error", err.Error())
		return
	}
	config["token"] = token
	httpapi.JSON(w, http.StatusOK, map[string]any{
		"office_url": a.Config.OfficeURL,
		"config":     config,
	}, nil)
}

// handleOfficeDoc serves the file content to OnlyOffice's document server.
// Authenticated by the edit_key issued in handleOfficeConfig.
func (a *App) handleOfficeDoc(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "id")
	editKey := r.URL.Query().Get("key")
	if editKey == "" {
		http.Error(w, "missing key", http.StatusUnauthorized)
		return
	}
	var ownerID string
	if err := a.DB.QueryRowContext(r.Context(),
		"SELECT user_id FROM office_sessions WHERE edit_key=? AND file_id=?",
		editKey, fileID,
	).Scan(&ownerID); err != nil {
		http.Error(w, "invalid key", http.StatusUnauthorized)
		return
	}

	var name, mimeType, encKey, iv, tag, blobPath string
	if err := a.DB.QueryRowContext(r.Context(),
		"SELECT name, mime_type, encryption_key_enc, encryption_iv, encryption_tag, storage_path FROM files WHERE id=?",
		fileID,
	).Scan(&name, &mimeType, &encKey, &iv, &tag, &blobPath); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	key, err := storage.UnwrapKey(a.Config.MasterEncryptionKey, storage.EncryptedKeyBundle{
		IVHex: iv, EncKeyHex: encKey, TagHex: tag,
	})
	if err != nil {
		http.Error(w, "key", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+sanitizeFilename(name)+"\"")
	_ = storage.DecryptFileToWriter(blobPath, w, key)
}

// handleOfficeCallback is invoked by OnlyOffice when document state changes.
// status codes (from OnlyOffice docs):
//
//	1 = being edited
//	2 = ready for saving (download from .url)
//	3 = saving error
//	4 = closed without changes
//	6 = being edited but saved
//	7 = error while force-save
func (a *App) handleOfficeCallback(w http.ResponseWriter, r *http.Request) {
	if a.Config.OfficeJWTSecret == "" {
		http.Error(w, "office disabled", http.StatusNotImplemented)
		return
	}
	fileID := chi.URLParam(r, "id")
	editKey := r.URL.Query().Get("key")
	if editKey == "" {
		http.Error(w, "missing key", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 4*1024*1024))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	var payload struct {
		Status int    `json:"status"`
		URL    string `json:"url"`
		Token  string `json:"token"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "json", http.StatusBadRequest)
		return
	}
	// Verify JWT — required when JWT_ENABLED=true on OnlyOffice. Pin the
	// algorithm to HS256 so a forged token claiming "none" or RS256 is rejected.
	if payload.Token != "" {
		if _, err := jwt.Parse(payload.Token, func(t *jwt.Token) (interface{}, error) {
			return []byte(a.Config.OfficeJWTSecret), nil
		}, jwt.WithValidMethods([]string{"HS256"})); err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
	}

	// Validate session. The session user is the acting editor.
	var editorID string
	if err := a.DB.QueryRowContext(r.Context(),
		"SELECT user_id FROM office_sessions WHERE edit_key=? AND file_id=?",
		editKey, fileID,
	).Scan(&editorID); err != nil {
		http.Error(w, "invalid key", http.StatusUnauthorized)
		return
	}

	if payload.Status == 2 || payload.Status == 6 {
		// Document ready for saving. Fetch and store as a new version.
		if payload.URL == "" {
			http.Error(w, "missing url", http.StatusBadRequest)
			return
		}
		resp, err := a.Client.Get(payload.URL)
		if err != nil {
			http.Error(w, "fetch", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// Get file metadata to know its name + mime.
		var name, mimeType, folderID string
		var folder sql.NullString
		if err := a.DB.QueryRowContext(r.Context(),
			"SELECT name, mime_type, folder_id FROM files WHERE id=?", fileID,
		).Scan(&name, &mimeType, &folder); err != nil {
			http.Error(w, "db", http.StatusInternalServerError)
			return
		}
		if folder.Valid {
			folderID = folder.String
		}

		// The editor must hold write access; the resulting version is attributed
		// to the file's owner so a shared-file edit versions the owner's file
		// rather than spawning a duplicate owned by the editor.
		fileOwnerID, aerr := a.canAccessFile(r.Context(), editorID, fileID, AccessEditor)
		if aerr != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		// Save via the same path uploads go through.
		if err := a.saveOfficeEdit(r, editorID, fileOwnerID, name, folderID, mimeType, resp.Body); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Mark the session closed.
		_, _ = a.DB.ExecContext(r.Context(),
			"DELETE FROM office_sessions WHERE edit_key=?", editKey)
	}

	// OnlyOffice expects {"error": 0} JSON for success.
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"error":0}`))
}

// saveOfficeEdit stores an OnlyOffice edit as a new version. uploaderID is the
// acting editor (recorded as the version's created_by); ownerID is the file's
// owner the bytes are attributed to (quota, storage path, same-name scope).
func (a *App) saveOfficeEdit(r *http.Request, uploaderID, ownerID, filename, folderID, mimeType string, body io.Reader) error {
	fileKey, err := storage.GenerateFileKey()
	if err != nil {
		return err
	}
	bundle, err := storage.WrapKey(a.Config.MasterEncryptionKey, fileKey)
	if err != nil {
		return err
	}
	tmpPath := storage.TempPath(a.Config.StoragePath, "office-"+uuid.NewString())
	if err := os.MkdirAll(filepath.Dir(tmpPath), 0o755); err != nil {
		return err
	}
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	hasher := newSHA256()
	teed := io.TeeReader(body, hasher)
	size, encErr := storage.EncryptStream(teed, tmpFile, fileKey)
	closeErr := tmpFile.Close()
	if encErr != nil {
		_ = os.Remove(tmpPath)
		return encErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}
	if _, err := storage.EnsureUserDir(a.Config.StoragePath, ownerID); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	contentHash := hasher.HexSum()
	if _, err := a.commitUploadWithVersioning(r.Context(), uploaderID, ownerID, filename, folderID, mimeType,
		contentHash, size, bundle, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func (a *App) publicBackendURL(r *http.Request) string {
	if a.Config.PublicBackendURL != "" {
		return strings.TrimRight(a.Config.PublicBackendURL, "/")
	}
	scheme := firstHeaderValue(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		scheme = forwardedParam(r.Header.Get("Forwarded"), "proto")
	}
	if scheme == "" {
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
	}
	host := firstHeaderValue(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = forwardedParam(r.Header.Get("Forwarded"), "host")
	}
	if host == "" {
		host = r.Host
	}
	if isPrivateHost(host) {
		scheme = "http"
		host = hostWithoutPort(host)
	}
	return strings.ToLower(scheme) + "://" + host
}

func firstHeaderValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.TrimSpace(strings.Split(value, ",")[0])
}

func forwardedParam(header, key string) string {
	key = strings.ToLower(key)
	for _, part := range strings.Split(header, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || strings.ToLower(strings.TrimSpace(name)) != key {
			continue
		}
		return strings.Trim(strings.TrimSpace(value), `"`)
	}
	return ""
}

func isPrivateHost(value string) bool {
	host := strings.ToLower(strings.Trim(hostWithoutPort(value), "[]"))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

func hostWithoutPort(value string) string {
	host := strings.TrimSpace(value)
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	if strings.Count(host, ":") > 1 {
		return strings.Trim(host, "[]")
	}
	return strings.Split(host, ":")[0]
}

func (a *App) lookupUsername(ctx interface {
	Deadline() (time.Time, bool)
}, userID string) (string, error) {
	_ = ctx
	var u string
	if err := a.DB.QueryRow("SELECT username FROM users WHERE id=?", userID).Scan(&u); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return u, nil
}

func officeDocumentType(ext string) string {
	switch ext {
	case "docx", "doc", "odt", "rtf", "txt":
		return "word"
	case "xlsx", "xls", "csv", "ods":
		return "cell"
	case "pptx", "ppt", "odp":
		return "slide"
	}
	return "word"
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// newSHA256 is a tiny hasher wrapper that hex-encodes its digest on Sum.
// Defined here so handlers_office.go is self-contained.
type sha256Wrapper struct{ h hashWriter }

type hashWriter interface {
	io.Writer
	Sum(b []byte) []byte
}

func newSHA256() *sha256Wrapper {
	return &sha256Wrapper{h: newSHA256Hasher()}
}

func (s *sha256Wrapper) Write(p []byte) (int, error) { return s.h.Write(p) }
func (s *sha256Wrapper) HexSum() string              { return hex.EncodeToString(s.h.Sum(nil)) }
