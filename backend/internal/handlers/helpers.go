package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/iii123iii/mycloud/backend/internal/services"
)

// parseInt64 parses a string to int64.
func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
}

// sanitizeFilename strips characters dangerous in Content-Disposition headers.
func sanitizeFilename(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, c := range name {
		if c == '"' || c == '\\' || c == '\r' || c == '\n' || c == 0 {
			continue
		}
		b.WriteRune(c)
	}
	if b.Len() == 0 {
		return "download"
	}
	return b.String()
}

// mimeFromFilename returns a MIME type based on the file extension.
func mimeFromFilename(name string) string {
	dot := strings.LastIndex(name, ".")
	if dot == -1 {
		return "application/octet-stream"
	}
	ext := strings.ToLower(name[dot+1:])
	mimeMap := map[string]string{
		"jpg": "image/jpeg", "jpeg": "image/jpeg", "png": "image/png",
		"gif": "image/gif", "webp": "image/webp", "svg": "image/svg+xml",
		"pdf": "application/pdf",
		"mp4": "video/mp4", "webm": "video/webm", "mov": "video/quicktime",
		"avi": "video/x-msvideo", "mkv": "video/x-matroska",
		"mp3": "audio/mpeg", "wav": "audio/wav", "ogg": "audio/ogg",
		"flac": "audio/flac", "aac": "audio/aac",
		"txt": "text/plain", "md": "text/markdown", "html": "text/html",
		"css": "text/css", "js": "text/javascript", "ts": "text/plain",
		"json": "application/json", "xml": "application/xml",
		"zip": "application/zip", "tar": "application/x-tar",
		"gz": "application/gzip", "7z": "application/x-7z-compressed",
		"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		"doc":  "application/msword",
		"xls":  "application/vnd.ms-excel",
	}
	if m, ok := mimeMap[ext]; ok {
		return m
	}
	return "application/octet-stream"
}

// safeInt parses a string to int, clamped to [minVal, maxVal].
func safeInt(s string, def, minVal, maxVal int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	if v < minVal {
		return minVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}

// deleteUser hard-deletes all data owned by the user.
func deleteUser(ctx context.Context, db *sql.DB, storageSvc *services.StorageService, userID string) error {
	// Collect and delete all files from disk
	rows, err := db.QueryContext(ctx, "SELECT id FROM files WHERE user_id=?", userID)
	if err != nil {
		return fmt.Errorf("list files: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var fileID string
		if err := rows.Scan(&fileID); err == nil {
			storageSvc.DeleteFile(userID, fileID)
		}
	}
	rows.Close()

	// Delete DB rows
	if _, err := db.ExecContext(ctx, "DELETE FROM files WHERE user_id=?", userID); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM folders WHERE user_id=?", userID); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM shares WHERE created_by=?", userID); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM activity_log WHERE user_id=?", userID); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM users WHERE id=?", userID); err != nil {
		return err
	}
	return nil
}

// nullableString returns nil if s is empty, else &s.
func nullableString(s sql.NullString) any {
	if !s.Valid {
		return nil
	}
	return s.String
}
