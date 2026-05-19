package app

import (
	"archive/zip"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path"
	"strings"
	"time"

	"mycloud/backend-go/internal/httpapi"
	"mycloud/backend-go/internal/storage"
)

// handleDownloadArchive streams a zip of the requested files + folder trees.
// Permission per item via canAccessFile / canAccessFolder; missing items are
// skipped silently so a partially-bad request still returns useful data.
func (a *App) handleDownloadArchive(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	var payload struct {
		FileIDs   []string `json:"file_ids"`
		FolderIDs []string `json:"folder_ids"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	if len(payload.FileIDs) == 0 && len(payload.FolderIDs) == 0 {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "No items selected")
		return
	}

	// Validate access for each top-level item.
	for _, id := range payload.FileIDs {
		if _, err := a.canAccessFile(r.Context(), userID, id, AccessViewer); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Error(w, http.StatusNotFound, "not_found", "File not accessible: "+id)
				return
			}
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
	}
	for _, id := range payload.FolderIDs {
		if err := a.canAccessFolder(r.Context(), userID, id, AccessViewer); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Error(w, http.StatusNotFound, "not_found", "Folder not accessible: "+id)
				return
			}
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="mycloud.zip"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	zw := zip.NewWriter(w)
	defer zw.Close()

	// Walk each top-level file. A per-item failure logs and continues so the
	// archive completes with whatever could be read.
	for _, fid := range payload.FileIDs {
		if err := a.writeFileToZip(r, zw, fid, ""); err != nil {
			log.Printf("archive: file %s: %v", fid, err)
		}
	}

	for _, dirID := range payload.FolderIDs {
		if err := a.writeFolderToZip(r, zw, userID, dirID); err != nil {
			log.Printf("archive: folder %s: %v", dirID, err)
		}
	}
}

func (a *App) writeFileToZip(r *http.Request, zw *zip.Writer, fileID, prefix string) error {
	var name, encKey, iv, tag, path string
	if err := a.DB.QueryRowContext(r.Context(), `
		SELECT name, encryption_key_enc, encryption_iv, encryption_tag, storage_path
		FROM files WHERE id=? AND is_deleted=0`, fileID,
	).Scan(&name, &encKey, &iv, &tag, &path); err != nil {
		return err
	}
	header := &zip.FileHeader{
		Name:     joinZip(prefix, name),
		Method:   zip.Store,
		Modified: time.Now(),
	}
	zf, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	fileKey, err := storage.UnwrapKey(a.Config.MasterEncryptionKey, storage.EncryptedKeyBundle{
		IVHex: iv, EncKeyHex: encKey, TagHex: tag,
	})
	if err != nil {
		return err
	}
	return storage.DecryptFileToWriter(path, zf, fileKey)
}

// writeFolderToZip walks the folder subtree rooted at folderID and writes
// every (non-deleted) folder+file into the zip preserving relative paths.
func (a *App) writeFolderToZip(r *http.Request, zw *zip.Writer, userID, folderID string) error {
	ids, err := a.collectFolderTree(r.Context(), userID, folderID)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	ph, args := inClause(ids)

	// Build folder id → relative path map.
	rows, err := a.DB.QueryContext(r.Context(),
		fmt.Sprintf("SELECT id, name, parent_id FROM folders WHERE id IN %s AND is_deleted=0", ph),
		args...)
	if err != nil {
		return err
	}
	type fnode struct {
		name   string
		parent sql.NullString
	}
	nodes := make(map[string]fnode, len(ids))
	for rows.Next() {
		var id, name string
		var parent sql.NullString
		if err := rows.Scan(&id, &name, &parent); err != nil {
			rows.Close()
			return err
		}
		nodes[id] = fnode{name: name, parent: parent}
	}
	rows.Close()

	// Recursive name resolution against the original folderID as the zip root.
	var resolve func(id string) string
	resolve = func(id string) string {
		if id == folderID {
			return nodes[id].name
		}
		n, ok := nodes[id]
		if !ok {
			return ""
		}
		if !n.parent.Valid {
			return n.name
		}
		return path.Join(resolve(n.parent.String), n.name)
	}

	// Write all files under any folder in this subtree.
	fileRows, err := a.DB.QueryContext(r.Context(),
		fmt.Sprintf(`SELECT id, folder_id FROM files WHERE folder_id IN %s AND is_deleted=0`, ph),
		args...)
	if err != nil {
		return err
	}
	defer fileRows.Close()
	for fileRows.Next() {
		var fid string
		var fparent sql.NullString
		if err := fileRows.Scan(&fid, &fparent); err != nil {
			return err
		}
		prefix := ""
		if fparent.Valid {
			prefix = resolve(fparent.String)
		}
		_ = a.writeFileToZip(r, zw, fid, prefix)
	}
	return nil
}

func joinZip(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return strings.TrimRight(prefix, "/") + "/" + name
}
