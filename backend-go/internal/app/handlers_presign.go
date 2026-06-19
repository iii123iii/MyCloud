package app

// HMAC-signed temporary download URLs.
//
// Use case: an embedded <video src="..."> or <img src="..."> can't easily
// carry an Authorization header. Instead the SPA asks for a presigned URL
// once, gets back a short-lived token-bearing URL, and the browser fetches
// it as a normal GET.
//
// Token shape (URL-safe base64 of):
//   {file_id}|{expires_unix}|{purpose}|hex(HMAC_SHA256(secret, file_id|expires|purpose))
//
// The same HMAC secret is reused from the JWT secret to avoid managing yet
// another rotation knob. We salt the HMAC with a fixed purpose tag so a
// leaked file download token can't be re-purposed for, say, a thumbnail
// request that may have different access semantics.

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"mycloud/backend-go/internal/httpapi"
	"mycloud/backend-go/internal/storage"
)

const presignPurposeDownload = "download"
const presignDefaultTTL = 5 * time.Minute
const presignMaxTTL = 1 * time.Hour

// handlePresign issues a token. Body: {ttl_seconds?: number}.
func (a *App) handlePresign(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	fileID := chi.URLParam(r, "id")
	if _, err := a.canAccessFile(r.Context(), userID, fileID, AccessViewer); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "file not accessible")
			return
		}
		respondDBError(w, r, err)
		return
	}
	var body struct {
		TTLSeconds int `json:"ttl_seconds"`
	}
	_ = decodeJSON(r, &body)
	ttl := time.Duration(body.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = presignDefaultTTL
	}
	if ttl > presignMaxTTL {
		ttl = presignMaxTTL
	}
	expires := time.Now().Add(ttl)
	token := signPresignToken(a.Config.JWTSecret, fileID, expires.Unix(), presignPurposeDownload)

	auditID := uuid.NewString()
	if _, err := a.DB.ExecContext(r.Context(), `
		INSERT INTO presign_audit (id, file_id, issued_by, purpose, expires_at)
		VALUES (?, ?, ?, ?, ?)`,
		auditID, fileID, userID, presignPurposeDownload, expires); err != nil {
		// Audit failure shouldn't fail the issuance — log and continue.
	}

	httpapi.JSON(w, http.StatusOK, map[string]any{
		"url":        "/api/v2/dl/" + token,
		"expires_at": expires,
	}, nil)
}

// handlePresignedDownload validates the token then streams the decrypted
// file. Auth happens entirely via the token; this endpoint is OUTSIDE
// requireAuth so embedded media tags can fetch it directly.
func (a *App) handlePresignedDownload(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	fileID, expires, purpose, ok := verifyPresignToken(a.Config.JWTSecret, token)
	if !ok {
		httpapi.Error(w, http.StatusUnauthorized, "invalid_token", "bad signature")
		return
	}
	if time.Now().Unix() > expires {
		httpapi.Error(w, http.StatusGone, "expired", "token has expired")
		return
	}
	if purpose != presignPurposeDownload {
		httpapi.Error(w, http.StatusForbidden, "wrong_purpose", "token purpose does not match endpoint")
		return
	}
	// Mark used in the audit log (best effort).
	_, _ = a.DB.ExecContext(r.Context(),
		`UPDATE presign_audit SET used_at = NOW() WHERE file_id = ? AND purpose = ? AND used_at IS NULL ORDER BY issued_at DESC LIMIT 1`,
		fileID, purpose)

	var name, mimeType, encKey, iv, tag, path string
	var sizeBytes int64
	err := a.DB.QueryRowContext(r.Context(), `
		SELECT name, mime_type, encryption_key_enc, encryption_iv, encryption_tag, storage_path, size_bytes
		FROM files WHERE id=? AND is_deleted=0`, fileID,
	).Scan(&name, &mimeType, &encKey, &iv, &tag, &path, &sizeBytes)
	if err == sql.ErrNoRows {
		httpapi.Error(w, http.StatusNotFound, "not_found", "file not found")
		return
	}
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	fileKey, err := storage.UnwrapKey(a.Config.MasterEncryptionKey, storage.EncryptedKeyBundle{
		IVHex: iv, EncKeyHex: encKey, TagHex: tag,
	})
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "crypto_error", err.Error())
		return
	}
	// Inline disposition + Range support: this is the endpoint embedded media
	// tags (<video>/<audio>/<img>) hit directly, so seekable streaming of large
	// videos flows through here.
	serveDecryptedBlob(w, r, path, fileKey, name, mimeType, sizeBytes, "inline")
}

// signPresignToken builds the token described in the package comment.
func signPresignToken(secret, fileID string, expires int64, purpose string) string {
	payload := fileID + "|" + strconv.FormatInt(expires, 10) + "|" + purpose
	h := hmac.New(sha256.New, []byte("presign:"+secret))
	h.Write([]byte(payload))
	sig := hex.EncodeToString(h.Sum(nil))
	full := payload + "|" + sig
	return base64.RawURLEncoding.EncodeToString([]byte(full))
}

// verifyPresignToken returns (fileID, expires, purpose, ok). ok=false on
// any decoding or HMAC mismatch.
func verifyPresignToken(secret, token string) (string, int64, string, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", 0, "", false
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 4 {
		return "", 0, "", false
	}
	fileID, expiresStr, purpose, gotSig := parts[0], parts[1], parts[2], parts[3]
	expires, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil {
		return "", 0, "", false
	}
	expected := signPresignToken(secret, fileID, expires, purpose)
	// Re-decode our own to compare structurally — easier than recomputing
	// the inner string. hmac.Equal is constant-time.
	expectedRaw, _ := base64.RawURLEncoding.DecodeString(expected)
	if !hmac.Equal([]byte(string(raw)), expectedRaw) {
		_ = gotSig // shut up linter
		return "", 0, "", false
	}
	return fileID, expires, purpose, true
}
