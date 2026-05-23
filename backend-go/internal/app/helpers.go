package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mycloud/backend-go/internal/httpapi"
	"mycloud/backend-go/internal/logging"
	"mycloud/backend-go/internal/search"
)

// (searchNormaliseName indirection removed — see normaliseName below, which
// delegates directly to search.NormaliseName.)

func decodeJSON(r *http.Request, dst interface{}) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func strPtr(ns sql.NullString) *string {
	if ns.Valid {
		v := ns.String
		return &v
	}
	return nil
}

func ts(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func qInt(r *http.Request, key string, def, minVal, maxVal int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	if n < minVal {
		return minVal
	}
	if n > maxVal {
		return maxVal
	}
	return n
}

func clientIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// querier abstracts *sql.DB and *stmtcache.Cache so hot, static-SQL helpers
// transparently get prepared-statement caching once the app passes its Stmts
// cache instead of the raw DB. Callers that still pass *sql.DB keep working
// unchanged — the cache is purely an optimisation.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// normaliseName mirrors search.NormaliseName so callers in the app package
// can keep `files.name_normalised` aligned with `files.name` on every
// INSERT/UPDATE without importing the search package at every call site.
// Kept as a thin re-export — the canonical implementation lives in
// internal/search/fuzzy.go.
func normaliseName(name string) string {
	return search.NormaliseName(name)
}

// parseUserDatetime accepts the variety of datetime strings clients send and
// returns a *time.Time the MySQL driver can serialise into a DATETIME column.
//
// JavaScript's `Date#toISOString()` yields `2026-05-27T14:46:49.577Z` which
// MariaDB rejects with `Incorrect datetime value` because of the fractional
// seconds + trailing Z. The frontend already uses this format for share +
// upload-request expiries, so the server has to be the one that normalises.
//
// Accepted formats (tried in order):
//   - RFC 3339 with millis + Z (`2026-05-27T14:46:49.577Z`)
//   - RFC 3339 without millis    (`2026-05-27T14:46:49Z`)
//   - MySQL DATETIME             (`2026-05-27 14:46:49`)
//   - Date only                  (`2026-05-27`)  → midnight UTC
//
// Returns (nil, nil) when s is "" or nil — the column is nullable, an empty
// value means "no expiry". Returns (nil, error) for malformed input so the
// caller can surface a 400.
func parseUserDatetime(s *string) (*time.Time, error) {
	if s == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil, nil
	}
	layouts := []string{
		time.RFC3339Nano,           // 2026-05-27T14:46:49.577Z
		time.RFC3339,               // 2026-05-27T14:46:49Z
		"2006-01-02T15:04:05",      // local-time variant without zone
		"2006-01-02 15:04:05",      // MySQL DATETIME
		"2006-01-02",               // date-only
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, v); err == nil {
			t = t.UTC()
			return &t, nil
		}
	}
	return nil, fmt.Errorf("invalid datetime: %q", v)
}

func getSetting(ctx context.Context, q querier, key, def string) (string, error) {
	var value sql.NullString
	err := q.QueryRowContext(ctx, "SELECT value FROM settings WHERE key_name=?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return def, nil
	}
	if err != nil {
		return "", err
	}
	if !value.Valid {
		return def, nil
	}
	return value.String, nil
}

// respondDBError centralises how internal DB errors leave the handler.
// Logs the raw error with request_id at warn level so operators can grep
// for failures, and surfaces a stable generic envelope to the client so
// schema/driver text never reaches the wire. Replaces ~200 sites of
// httpapi.Error(w, 500, "db_error", err.Error()) — those leaked table
// names, mysql duplicate-key text, and driver versions to every API
// caller. After this helper, the audit-flagged information disclosure
// only exists in handlers that haven't been swept; new code should use
// this helper unconditionally for unexpected DB failures.
func respondDBError(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		return
	}
	log := logging.FromContext(r.Context())
	log.Warn("db_error",
		"error", err.Error(),
		"path", r.URL.Path,
		"method", r.Method)
	httpapi.Error(w, http.StatusInternalServerError, "internal_error",
		"An internal error occurred. Reference: "+RequestID(r.Context()))
}

// readLimitOffset parses ?limit= and ?offset= from a request with sensible
// caps. Used by list endpoints to add cheap pagination without rewriting
// each query to use the cursor model. defaultLimit applies when the param
// is missing or unparseable; maxLimit caps deliberate or accidental large
// requests.
func readLimitOffset(r *http.Request, defaultLimit, maxLimit int) (int, int) {
	limit := defaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if limit < 1 {
		limit = 1
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}

func getUserSession(ctx context.Context, q querier, userID string) (string, bool, error) {
	role, active, _, err := getUserSessionFull(ctx, q, userID)
	return role, active, err
}

// getUserSessionFull returns role, is_active, and must_change_password in one
// query. Used by requireAuth to populate ctx so downstream middleware can
// gate routes on must_change_password without a second DB hit.
func getUserSessionFull(ctx context.Context, q querier, userID string) (string, bool, bool, error) {
	var (
		role       string
		isActive   bool
		mustChange bool
	)
	err := q.QueryRowContext(ctx,
		"SELECT role, is_active, must_change_password FROM users WHERE id=?", userID).
		Scan(&role, &isActive, &mustChange)
	if err != nil {
		return "", false, false, err
	}
	return role, isActive, mustChange, nil
}

func boolSetting(raw string) bool {
	return strings.EqualFold(raw, "true") || raw == "1"
}

func writeActivity(ctx context.Context, q querier, userID *string, action, resourceType, resourceID, ip string, details interface{}) {
	payload, _ := json.Marshal(details)
	_, _ = q.ExecContext(ctx, `
		INSERT INTO activity_log (user_id, action, resource_type, resource_id, details, ip_address)
		VALUES (?, ?, ?, ?, ?, ?)`,
		userID, action, nullableString(resourceType), nullableString(resourceID), nullableJSON(payload), nullableString(ip))
	// Publish to the per-resource WS topic so subscribed clients (the
	// per-folder activity panel, the user's recent-activity sidebar) get a
	// live event. We don't block on the publish — the broker's in-memory
	// path is synchronous-but-fast; a future Redis broker would benefit
	// from being async too.
	publishActivity(ctx, q, userID, action, resourceType, resourceID, payload)
}

// publishActivity is set at app startup to the hub publisher. Stays nil in
// tests / detached helpers so non-app callers don't drag in the hub dep.
var publishActivity = func(ctx context.Context, q querier, userID *string, action, resourceType, resourceID string, details []byte) {
	// no-op until App.New replaces it
}

// writeActivityWithDiff records a reversible action by snapshotting the
// row state both before and after the change. The admin rollback endpoint
// uses these blobs to restore the prior state via a registered RollbackFn.
//
// operationID groups multi-step batches: pass uuid.NewString() once and pass
// the same value for every per-item activity row so the admin can "rollback
// the whole batch" later. Leave empty for single-item operations.
//
// reversible defaults true here — only call this helper for actions that
// have a registered RollbackFn in rollback.go. Non-reversible actions
// should keep using plain writeActivity.
func writeActivityWithDiff(
	ctx context.Context, q querier, userID *string, action, resourceType, resourceID, ip, operationID string,
	oldValue, newValue, details any,
) {
	detailsPayload, _ := json.Marshal(details)
	oldPayload, _ := json.Marshal(oldValue)
	newPayload, _ := json.Marshal(newValue)
	_, _ = q.ExecContext(ctx, `
		INSERT INTO activity_log
		    (user_id, action, resource_type, resource_id, details, old_value, new_value, operation_id, reversible, ip_address)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?)`,
		userID,
		action,
		nullableString(resourceType),
		nullableString(resourceID),
		nullableJSON(detailsPayload),
		nullableJSON(oldPayload),
		nullableJSON(newPayload),
		nullableString(operationID),
		nullableString(ip),
	)
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullableStringPtr(s *string) interface{} {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

func nullableJSON(raw []byte) interface{} {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return string(raw)
}
