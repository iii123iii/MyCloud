package app

// Diff two historical versions of a file.
//
// For text MIME types, returns a structured LCS diff that the frontend can
// render as side-by-side or inline. For other types (images, binary), we
// return the metadata (size + mtime) of both sides so the UI can show
// "Version 3 (1.2 MiB) vs Version 5 (1.5 MiB)" and embed the preview
// endpoints for visual side-by-side.
//
// GET /api/v2/files/{id}/versions/{a}/diff/{b}

import (
	"bytes"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sergi/go-diff/diffmatchpatch"

	"mycloud/backend-go/internal/httpapi"
	"mycloud/backend-go/internal/storage"
)

// versionMetadata is loaded once per side of the diff.
type versionMetadata struct {
	versionNo  int
	encKey     string
	iv         string
	tag        string
	storagePath string
	size       int64
	mimeType   string
	createdAt  string
}

func (a *App) handleVersionDiff(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	fileID := chi.URLParam(r, "id")
	vaStr := chi.URLParam(r, "a")
	vbStr := chi.URLParam(r, "b")
	va, err := strconv.Atoi(vaStr)
	if err != nil || va < 1 {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "invalid version a")
		return
	}
	vb, err := strconv.Atoi(vbStr)
	if err != nil || vb < 1 {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "invalid version b")
		return
	}
	if _, err := a.canAccessFile(r.Context(), userID, fileID, AccessViewer); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "file not accessible")
			return
		}
		respondDBError(w, r, err)
		return
	}

	left, err := a.loadVersionMetadata(r, fileID, va)
	if err != nil {
		httpapi.Error(w, http.StatusNotFound, "not_found", "version a not found: "+err.Error())
		return
	}
	right, err := a.loadVersionMetadata(r, fileID, vb)
	if err != nil {
		httpapi.Error(w, http.StatusNotFound, "not_found", "version b not found: "+err.Error())
		return
	}

	// Only generate a text diff when both sides claim a text MIME type AND
	// total combined size is under 2 MiB. Anything larger gets a metadata
	// stub that the UI uses to render previews instead of trying to diff.
	const textDiffCap = 2 * 1024 * 1024
	if isTextMime(left.mimeType) && isTextMime(right.mimeType) &&
		left.size+right.size <= textDiffCap {
		leftText, err := a.decryptVersionToString(left)
		if err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "decrypt_error", err.Error())
			return
		}
		rightText, err := a.decryptVersionToString(right)
		if err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "decrypt_error", err.Error())
			return
		}
		dmp := diffmatchpatch.New()
		// LineMode so the frontend can render line-by-line side-by-side
		// without re-splitting on its own.
		diffs := dmp.DiffMain(leftText, rightText, true)
		diffs = dmp.DiffCleanupSemantic(diffs)
		out := make([]map[string]any, 0, len(diffs))
		for _, d := range diffs {
			op := "equal"
			switch d.Type {
			case diffmatchpatch.DiffInsert:
				op = "insert"
			case diffmatchpatch.DiffDelete:
				op = "delete"
			}
			out = append(out, map[string]any{"op": op, "text": d.Text})
		}
		httpapi.JSON(w, http.StatusOK, map[string]any{
			"kind":  "text",
			"diff":  out,
			"left":  map[string]any{"version_no": left.versionNo, "size": left.size, "mime_type": left.mimeType, "created_at": left.createdAt},
			"right": map[string]any{"version_no": right.versionNo, "size": right.size, "mime_type": right.mimeType, "created_at": right.createdAt},
		}, nil)
		return
	}

	// Metadata-only stub for non-text or oversized content. The frontend
	// uses /files/{id}/versions/{n}:preview for visual side-by-side.
	httpapi.JSON(w, http.StatusOK, map[string]any{
		"kind":  "metadata",
		"left":  map[string]any{"version_no": left.versionNo, "size": left.size, "mime_type": left.mimeType, "created_at": left.createdAt},
		"right": map[string]any{"version_no": right.versionNo, "size": right.size, "mime_type": right.mimeType, "created_at": right.createdAt},
	}, nil)
}

func (a *App) loadVersionMetadata(r *http.Request, fileID string, versionNo int) (*versionMetadata, error) {
	m := &versionMetadata{versionNo: versionNo}
	err := a.DB.QueryRowContext(r.Context(), `
		SELECT v.encryption_key_enc, v.encryption_iv, v.encryption_tag, v.storage_path,
		       v.size_bytes, v.mime_type, v.created_at
		FROM file_versions v
		WHERE v.file_id = ? AND v.version_no = ?`, fileID, versionNo,
	).Scan(&m.encKey, &m.iv, &m.tag, &m.storagePath, &m.size, &m.mimeType, &m.createdAt)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (a *App) decryptVersionToString(m *versionMetadata) (string, error) {
	key, err := storage.UnwrapKey(a.Config.MasterEncryptionKey, storage.EncryptedKeyBundle{
		IVHex: m.iv, EncKeyHex: m.encKey, TagHex: m.tag,
	})
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := storage.DecryptFileToWriter(m.storagePath, &buf, key); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// isTextMime catches the common text-ish content types. Conservative on
// purpose — we don't want to attempt a text diff against a JPEG just
// because it starts with a few printable bytes.
func isTextMime(mt string) bool {
	mt = strings.ToLower(mt)
	if strings.HasPrefix(mt, "text/") {
		return true
	}
	switch mt {
	case "application/json", "application/xml", "application/yaml", "application/x-yaml",
		"application/javascript", "application/typescript":
		return true
	}
	return false
}

// reader-cast placeholder; the diff code uses bytes.Buffer directly.
var _ io.Reader = (*bytes.Buffer)(nil)
