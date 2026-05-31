package app

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"mycloud/backend-go/internal/httpapi"
	"mycloud/backend-go/internal/search"
	"mycloud/backend-go/internal/search/dsl"
)

// handleSearch supports searching file/folder NAMES (default) and optionally
// file CONTENT via the FULLTEXT index on file_text.
// Query params:
//   q     — search term (required)
//   scope — "name" | "content" | "both" (default "both")
//   folder_id — optional; when set, constrains every search path to that
//               folder's recursive subtree (the folder + all descendants),
//               owner-scoped. Omitted ⇒ global, whole-drive search.
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

	// Optional folder scope. When folder_id is supplied, constrain every search
	// path to that folder's recursive subtree (the folder itself plus all
	// nested descendants). collectFolderTree is user-scoped, so an unknown or
	// foreign folder yields an empty subtree → no results. A nil subtree (no
	// folder_id) preserves the global, whole-drive search.
	folderID := strings.TrimSpace(r.URL.Query().Get("folder_id"))
	var subtreeIDs []string
	if folderID != "" {
		ids, err := a.collectFolderTree(r.Context(), userID, folderID)
		if err != nil {
			respondDBError(w, r, err)
			return
		}
		if len(ids) == 0 {
			httpapi.JSON(w, http.StatusOK, map[string]any{"results": []map[string]any{}}, nil)
			return
		}
		subtreeIDs = ids
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

	// If the query contains a field operator (e.g. "name:foo" or
	// "tag:invoice"), route through the DSL composer. Plain-text queries
	// continue to use the legacy FULLTEXT+LIKE path so existing UX doesn't
	// regress.
	if strings.ContainsRune(q, ':') {
		parsed, err := dsl.Parse(q)
		if err != nil {
			httpapi.Error(w, http.StatusBadRequest, "bad_query", err.Error())
			return
		}
		frag, args := dsl.Compile(parsed, userID)
		// User-scoped base query. file_tags / tags joins live inside the
		// fragment as EXISTS subqueries so the SELECT shape stays uniform.
		query := `SELECT 'file' AS item_type, id, name, size_bytes, mime_type, is_starred, updated_at
		          FROM files WHERE user_id = ? AND is_deleted = 0` + frag
		queryArgs := append([]any{userID}, args...)
		// Constrain to the folder subtree when scoped. Must precede LIMIT.
		if subtreeIDs != nil {
			ph, idArgs := inClause(subtreeIDs)
			query += " AND folder_id IN " + ph
			queryArgs = append(queryArgs, idArgs...)
		}
		query += " LIMIT 100"
		rows, err := a.DB.QueryContext(r.Context(), query, queryArgs...)
		if err != nil {
			respondDBError(w, r, err)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var itemType, id, name, mimeType, updatedAt string
			var sizeBytes sql.NullInt64
			var starred sql.NullBool
			if err := rows.Scan(&itemType, &id, &name, &sizeBytes, &mimeType, &starred, &updatedAt); err != nil {
				respondDBError(w, r, err)
				return
			}
			add(map[string]any{
				"type": itemType, "id": id, "name": name,
				"size_bytes": sizeBytes.Int64, "mime_type": mimeType,
				"is_starred": starred.Bool, "updated_at": updatedAt,
			})
		}
		httpapi.JSON(w, http.StatusOK, map[string]any{"results": results}, nil)
		return
	}

	// ── Name matches across files + folders. Prefer FULLTEXT when the term is
	// long enough; fall back to LIKE for short queries so single-character
	// searches still work (MariaDB's default minimum token length is 3).
	if scope == "name" || scope == "both" {
		nameRows, err := a.queryNameMatches(r, userID, q, subtreeIDs)
		if err != nil {
			respondDBError(w, r, err)
			return
		}
		for _, item := range nameRows {
			add(item)
		}
	}

	// ── Content matches via file_text FULLTEXT.
	if scope == "content" || scope == "both" {
		contentRows, err := a.queryContentMatches(r, userID, q, subtreeIDs)
		if err != nil {
			respondDBError(w, r, err)
			return
		}
		for _, item := range contentRows {
			add(item)
		}
	}

	httpapi.JSON(w, http.StatusOK, map[string]any{"results": results}, nil)
}

func (a *App) queryNameMatches(r *http.Request, userID, q string, subtreeIDs []string) ([]map[string]any, error) {
	// Fuzzy path uses the ngram FULLTEXT on name_normalised so even
	// 2-char prefixes and typo'd queries return candidates. The composer
	// builds two candidate sets:
	//   1) ngram FULLTEXT against name_normalised (substring tolerant)
	//   2) LIKE fallback against name (in case the migration hasn't run
	//      on this deployment yet — keeps the endpoint resilient)
	// Results are scored client-side by Damerau-Levenshtein and ordered.
	normQ := search.NormaliseName(q)
	like := "%" + normQ + "%"
	// Optional folder-subtree scope. Files filter on folder_id; folders filter
	// on parent_id, which yields exactly the root's descendant folders (and
	// naturally excludes the root you're already inside). Args are assembled in
	// the same left-to-right order as the placeholders.
	filesScope, foldersScope := "", ""
	queryArgs := []any{userID, like, q}
	if subtreeIDs != nil {
		ph, idArgs := inClause(subtreeIDs)
		filesScope = " AND folder_id IN " + ph
		queryArgs = append(queryArgs, idArgs...)
	}
	queryArgs = append(queryArgs, userID, like)
	if subtreeIDs != nil {
		ph, idArgs := inClause(subtreeIDs)
		foldersScope = " AND parent_id IN " + ph
		queryArgs = append(queryArgs, idArgs...)
	}
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT 'file' AS item_type, id, name, size_bytes, mime_type, is_starred, updated_at
		FROM files
		WHERE user_id=? AND is_deleted=0
		  AND (COALESCE(name_normalised, LOWER(name)) LIKE ?
		    OR MATCH(name) AGAINST(? IN NATURAL LANGUAGE MODE))`+filesScope+`
		UNION ALL
		SELECT 'folder', id, name, NULL, NULL, NULL, updated_at
		FROM folders WHERE user_id=? AND is_deleted=0 AND LOWER(name) LIKE ?`+foldersScope+`
		ORDER BY updated_at DESC LIMIT 200`,
		queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanSearchRows(rows)
	if err != nil {
		return nil, err
	}
	// Re-rank candidates by edit distance — closer matches first. Falls
	// back to updated_at order if everything ties at distance 0 (substring
	// hit on the original normalised value).
	type scored struct {
		item     map[string]any
		distance int
	}
	ranked := make([]scored, 0, len(items))
	for _, it := range items {
		name, _ := it["name"].(string)
		dist := search.DamerauLevenshtein(search.NormaliseName(name), normQ)
		// Substring matches get a bonus (distance 0) so "rep" still scores
		// "report.pdf" above "rope.pdf".
		if normName := search.NormaliseName(name); len(normName) > 0 && normQ != "" && containsLite(normName, normQ) {
			dist = 0
		}
		ranked = append(ranked, scored{item: it, distance: dist})
	}
	// Stable selection-sort up to top 100; the candidate set is bounded
	// to 200 so this is O(n²)≤40k comparisons — fine inline.
	if len(ranked) > 100 {
		ranked = ranked[:100]
	}
	for i := 0; i < len(ranked); i++ {
		minIdx := i
		for j := i + 1; j < len(ranked); j++ {
			if ranked[j].distance < ranked[minIdx].distance {
				minIdx = j
			}
		}
		ranked[i], ranked[minIdx] = ranked[minIdx], ranked[i]
	}
	out := make([]map[string]any, len(ranked))
	for i, r := range ranked {
		out[i] = r.item
	}
	return out, nil
}

// containsLite is a stdlib-strings.Contains shim that keeps this file's
// import tidy when other call sites might be reaching for a different
// search alphabet later (case-insensitive, etc).
func containsLite(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(s, sub string) int {
	// strings.Index is fine; alias for clarity at call site.
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func (a *App) queryContentMatches(r *http.Request, userID, q string, subtreeIDs []string) ([]map[string]any, error) {
	if len(q) < 3 {
		return nil, nil // FULLTEXT needs at least 3 chars
	}
	// Optional folder-subtree scope on the file's containing folder.
	folderScope := ""
	queryArgs := []any{q, userID, q}
	if subtreeIDs != nil {
		ph, idArgs := inClause(subtreeIDs)
		folderScope = " AND f.folder_id IN " + ph
		queryArgs = append(queryArgs, idArgs...)
	}
	// Alias the MATCH expression in a subquery so the FT scan only runs
	// once, while the outer SELECT keeps the column shape scanSearchRows
	// expects. The previous version invoked MATCH twice (WHERE + ORDER
	// BY) — InnoDB caches the per-row score but the index read still
	// happens twice, doubling FT cost on a wide corpus.
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT item_type, id, name, size_bytes, mime_type, is_starred, updated_at
		FROM (
		    SELECT 'file' AS item_type, f.id, f.name, f.size_bytes, f.mime_type, f.is_starred, f.updated_at,
		           MATCH(t.content) AGAINST(? IN NATURAL LANGUAGE MODE) AS relevance
		    FROM files f
		    JOIN file_text t ON t.file_id = f.id
		    WHERE f.user_id=? AND f.is_deleted=0
		      AND MATCH(t.content) AGAINST(? IN NATURAL LANGUAGE MODE)`+folderScope+`
		    ORDER BY relevance DESC
		    LIMIT 100
		) ranked`, queryArgs...)
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
