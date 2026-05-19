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

const maxCommentBody = 4000

func (a *App) handleListComments(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	fileID := chi.URLParam(r, "id")
	if _, err := a.canAccessFile(r.Context(), userID, fileID, AccessViewer); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
			return
		}
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT c.id, c.user_id, u.username, c.body, c.created_at, c.updated_at
		FROM comments c
		JOIN users u ON u.id = c.user_id
		WHERE c.file_id = ? AND c.deleted_at IS NULL
		ORDER BY c.created_at ASC`, fileID)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, uid, username, body, createdAt, updatedAt string
		if err := rows.Scan(&id, &uid, &username, &body, &createdAt, &updatedAt); err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		out = append(out, map[string]any{
			"id":         id,
			"user_id":    uid,
			"username":   username,
			"body":       body,
			"created_at": createdAt,
			"updated_at": updatedAt,
			"editable":   uid == userID,
		})
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"comments": out}, nil)
}

func (a *App) handleCreateComment(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	fileID := chi.URLParam(r, "id")
	var payload struct {
		Body string `json:"body"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	payload.Body = strings.TrimSpace(payload.Body)
	if payload.Body == "" {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Comment body is required")
		return
	}
	if len(payload.Body) > maxCommentBody {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Comment is too long")
		return
	}
	if _, err := a.canAccessFile(r.Context(), userID, fileID, AccessViewer); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "File not found")
			return
		}
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	id := uuid.NewString()
	if _, err := a.DB.ExecContext(r.Context(),
		"INSERT INTO comments (id, file_id, user_id, body) VALUES (?, ?, ?, ?)",
		id, fileID, userID, payload.Body,
	); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "comment.post", "file", fileID, clientIP(r), nil)
	httpapi.JSON(w, http.StatusCreated, map[string]any{"id": id}, nil)
}

func (a *App) handleUpdateComment(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	var payload struct {
		Body string `json:"body"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	payload.Body = strings.TrimSpace(payload.Body)
	if payload.Body == "" {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Comment body is required")
		return
	}
	if len(payload.Body) > maxCommentBody {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Comment is too long")
		return
	}
	res, err := a.DB.ExecContext(r.Context(),
		"UPDATE comments SET body=? WHERE id=? AND user_id=? AND deleted_at IS NULL",
		payload.Body, id, userID)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Comment not found")
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "comment.edit", "comment", id, clientIP(r), nil)
	httpapi.JSON(w, http.StatusOK, map[string]any{"message": "Updated"}, nil)
}

func (a *App) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	// Author OR file owner may delete.
	var authorID, fileID string
	err := a.DB.QueryRowContext(r.Context(),
		"SELECT user_id, file_id FROM comments WHERE id=? AND deleted_at IS NULL", id,
	).Scan(&authorID, &fileID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Comment not found")
		return
	}
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if authorID != userID {
		// Check if caller owns the file.
		var fileOwner string
		if err := a.DB.QueryRowContext(r.Context(),
			"SELECT user_id FROM files WHERE id=?", fileID,
		).Scan(&fileOwner); err != nil || fileOwner != userID {
			httpapi.Error(w, http.StatusForbidden, "forbidden", "Cannot delete this comment")
			return
		}
	}
	if _, err := a.DB.ExecContext(r.Context(),
		"UPDATE comments SET deleted_at=NOW() WHERE id=?", id,
	); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "comment.delete", "comment", id, clientIP(r), nil)
	httpapi.NoContent(w)
}
