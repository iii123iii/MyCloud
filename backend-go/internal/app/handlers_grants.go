package app

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"mycloud/backend-go/internal/httpapi"
)

// handleCreateGrant grants viewer/editor/owner access to another MyCloud user
// on one of the caller's files OR folders. Owner of the resource only.
func (a *App) handleCreateGrant(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	var payload struct {
		FileID     *string `json:"file_id"`
		FolderID   *string `json:"folder_id"`
		Grantee    string  `json:"grantee"`
		Permission string  `json:"permission"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	if (payload.FileID == nil) == (payload.FolderID == nil) {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Specify exactly one of file_id or folder_id")
		return
	}
	payload.Grantee = strings.TrimSpace(payload.Grantee)
	if payload.Grantee == "" {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "grantee is required")
		return
	}
	if payload.Permission == "" {
		payload.Permission = "viewer"
	}
	if payload.Permission != "viewer" && payload.Permission != "editor" && payload.Permission != "owner" {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "invalid permission")
		return
	}

	// Caller must own the resource (no chained grants).
	if payload.FileID != nil {
		var owner string
		if err := a.DB.QueryRowContext(r.Context(),
			"SELECT user_id FROM files WHERE id=? AND is_deleted=0", *payload.FileID,
		).Scan(&owner); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
				return
			}
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		if owner != userID {
			httpapi.Error(w, http.StatusForbidden, "forbidden", "Only the owner can share")
			return
		}
	} else {
		var owner string
		if err := a.DB.QueryRowContext(r.Context(),
			"SELECT user_id FROM folders WHERE id=? AND is_deleted=0", *payload.FolderID,
		).Scan(&owner); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Error(w, http.StatusNotFound, "not_found", "Folder not found")
				return
			}
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		if owner != userID {
			httpapi.Error(w, http.StatusForbidden, "forbidden", "Only the owner can share")
			return
		}
	}

	// Resolve grantee by username OR email.
	var granteeID string
	err := a.DB.QueryRowContext(r.Context(),
		"SELECT id FROM users WHERE (username=? OR email=?) AND is_active=1", payload.Grantee, payload.Grantee,
	).Scan(&granteeID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Error(w, http.StatusNotFound, "user_not_found", "Recipient not found")
		return
	}
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if granteeID == userID {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Cannot share with yourself")
		return
	}

	id := uuid.NewString()
	if _, err := a.DB.ExecContext(r.Context(), `
		INSERT INTO share_grants (id, file_id, folder_id, grantee_user_id, granted_by, permission)
		VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE permission=VALUES(permission)`,
		id, payload.FileID, payload.FolderID, granteeID, userID, payload.Permission,
	); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	resourceID := ""
	resourceType := ""
	if payload.FileID != nil {
		resourceID = *payload.FileID
		resourceType = "file"
	} else {
		resourceID = *payload.FolderID
		resourceType = "folder"
	}
	writeActivity(r.Context(), a.DB, &userID, "grant.create", resourceType, resourceID, clientIP(r), map[string]any{
		"grantee":    granteeID,
		"permission": payload.Permission,
	})
	httpapi.JSON(w, http.StatusCreated, map[string]any{
		"id":         id,
		"permission": payload.Permission,
		"grantee_id": granteeID,
	}, nil)
}

// handleListGrants returns grants where the caller is either the granter (outgoing)
// or the grantee (incoming). Direction is required via ?direction=incoming|outgoing.
func (a *App) handleListGrants(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	direction := r.URL.Query().Get("direction")
	if direction != "incoming" && direction != "outgoing" {
		direction = "incoming"
	}

	var where string
	if direction == "incoming" {
		where = "g.grantee_user_id = ?"
	} else {
		where = "g.granted_by = ?"
	}

	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT g.id, g.file_id, g.folder_id, g.grantee_user_id, g.granted_by,
		       g.permission, g.created_at,
		       f.name, fo.name,
		       ug.username, ub.username
		FROM share_grants g
		LEFT JOIN files   f  ON f.id  = g.file_id   AND f.is_deleted = 0
		LEFT JOIN folders fo ON fo.id = g.folder_id AND fo.is_deleted = 0
		LEFT JOIN users ug ON ug.id = g.grantee_user_id
		LEFT JOIN users ub ON ub.id = g.granted_by
		WHERE `+where+`
		ORDER BY g.created_at DESC`, userID)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, granteeID, grantedBy, permission, createdAt string
		var fileID, folderID, fileName, folderName, granteeName, granterName sql.NullString
		if err := rows.Scan(&id, &fileID, &folderID, &granteeID, &grantedBy, &permission, &createdAt,
			&fileName, &folderName, &granteeName, &granterName); err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		// Hide grants where the target was hard-deleted (LEFT JOIN returned null).
		if !fileID.Valid && !folderID.Valid {
			continue
		}
		if fileID.Valid && !fileName.Valid {
			continue
		}
		if folderID.Valid && !folderName.Valid {
			continue
		}
		item := map[string]any{
			"id":           id,
			"permission":   permission,
			"created_at":   createdAt,
			"grantee_id":   granteeID,
			"granted_by":   grantedBy,
			"grantee_name": granteeName.String,
			"granter_name": granterName.String,
		}
		if fileID.Valid {
			item["file_id"] = fileID.String
			item["file_name"] = fileName.String
		}
		if folderID.Valid {
			item["folder_id"] = folderID.String
			item["folder_name"] = folderName.String
		}
		out = append(out, item)
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"grants": out}, nil)
}

// handleUpdateGrant changes the permission level on an existing grant.
// Only the granter (resource owner) may modify it.
func (a *App) handleUpdateGrant(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	var payload struct {
		Permission string `json:"permission"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	if payload.Permission != "viewer" && payload.Permission != "editor" && payload.Permission != "owner" {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "invalid permission")
		return
	}
	res, err := a.DB.ExecContext(r.Context(),
		"UPDATE share_grants SET permission=? WHERE id=? AND granted_by=?",
		payload.Permission, id, userID)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Grant not found")
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"message": "Updated"}, nil)
}

// handleDeleteGrant removes a grant. Either the granter or the grantee may revoke.
func (a *App) handleDeleteGrant(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	res, err := a.DB.ExecContext(r.Context(),
		"DELETE FROM share_grants WHERE id=? AND (granted_by=? OR grantee_user_id=?)",
		id, userID, userID)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Grant not found")
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "grant.revoke", "grant", id, clientIP(r), nil)
	httpapi.NoContent(w)
}
