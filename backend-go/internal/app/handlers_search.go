package app

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"mycloud/backend-go/internal/httpapi"
)

// handleSearch supports searching file/folder NAMES (default) and optionally
// file CONTENT via the FULLTEXT index on file_text.
// Query params:
//   q     — search term (required)
//   scope — "name" | "content" | "both" (default "both")
func (a *App) handleSearch(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		httpapi.JSON(w, http.StatusOK, map[string]any{"results": []map[string]any{}}, nil)
		return
	}
	scope := r.URL.Query().Get("scope")
	if scope == "" {
		scope = "both"
	}

	results := make([]map[string]any, 0, 100)
	seen := map[string]bool{}
	add := func(item map[string]any) {
		key := item["type"].(string) + ":" + item["id"].(string)
		if seen[key] {
			return
		}
		seen[key] = true
		results = append(results, item)
	}

	// ── Name matches across files + folders. Prefer FULLTEXT when the term is
	// long enough; fall back to LIKE for short queries so single-character
	// searches still work (MariaDB's default minimum token length is 3).
	if scope == "name" || scope == "both" {
		nameRows, err := a.queryNameMatches(r, userID, q)
		if err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		for _, item := range nameRows {
			add(item)
		}
	}

	// ── Content matches via file_text FULLTEXT.
	if scope == "content" || scope == "both" {
		contentRows, err := a.queryContentMatches(r, userID, q)
		if err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		for _, item := range contentRows {
			add(item)
		}
	}

	httpapi.JSON(w, http.StatusOK, map[string]any{"results": results}, nil)
}

func (a *App) queryNameMatches(r *http.Request, userID, q string) ([]map[string]any, error) {
	useFulltext := len(q) >= 3
	var rows *sql.Rows
	var err error
	if useFulltext {
		rows, err = a.DB.QueryContext(r.Context(), `
			SELECT 'file' AS item_type, id, name, size_bytes, mime_type, is_starred, updated_at
			FROM files
			WHERE user_id=? AND is_deleted=0
			  AND MATCH(name) AGAINST(? IN NATURAL LANGUAGE MODE)
			UNION ALL
			SELECT 'folder', id, name, NULL, NULL, NULL, updated_at
			FROM folders
			WHERE user_id=? AND is_deleted=0 AND name LIKE ?
			ORDER BY updated_at DESC LIMIT 100`, userID, q, userID, "%"+q+"%")
	} else {
		like := "%" + q + "%"
		rows, err = a.DB.QueryContext(r.Context(), `
			SELECT 'file' AS item_type, id, name, size_bytes, mime_type, is_starred, updated_at
			FROM files WHERE user_id=? AND is_deleted=0 AND name LIKE ?
			UNION ALL
			SELECT 'folder', id, name, NULL, NULL, NULL, updated_at
			FROM folders WHERE user_id=? AND is_deleted=0 AND name LIKE ?
			ORDER BY updated_at DESC LIMIT 100`, userID, like, userID, like)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSearchRows(rows)
}

func (a *App) queryContentMatches(r *http.Request, userID, q string) ([]map[string]any, error) {
	if len(q) < 3 {
		return nil, nil // FULLTEXT needs at least 3 chars
	}
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT 'file' AS item_type, f.id, f.name, f.size_bytes, f.mime_type, f.is_starred, f.updated_at
		FROM files f
		JOIN file_text t ON t.file_id = f.id
		WHERE f.user_id=? AND f.is_deleted=0
		  AND MATCH(t.content) AGAINST(? IN NATURAL LANGUAGE MODE)
		ORDER BY MATCH(t.content) AGAINST(? IN NATURAL LANGUAGE MODE) DESC
		LIMIT 100`, userID, q, q)
	if err != nil {
		return nil, fmt.Errorf("content search: %w", err)
	}
	defer rows.Close()
	return scanSearchRows(rows)
}

func scanSearchRows(rows *sql.Rows) ([]map[string]any, error) {
	out := make([]map[string]any, 0, 100)
	for rows.Next() {
		var itemType, id, name, updatedAt string
		var size sql.NullInt64
		var mime sql.NullString
		var starred sql.NullBool
		if err := rows.Scan(&itemType, &id, &name, &size, &mime, &starred, &updatedAt); err != nil {
			return nil, err
		}
		item := map[string]any{"type": itemType, "id": id, "name": name, "updated_at": updatedAt}
		if size.Valid {
			item["size_bytes"] = size.Int64
		}
		if mime.Valid {
			item["mime_type"] = mime.String
		}
		if starred.Valid {
			item["is_starred"] = starred.Bool
		}
		out = append(out, item)
	}
	return out, nil
}
