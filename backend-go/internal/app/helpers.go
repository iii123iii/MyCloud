package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

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

func getSetting(ctx context.Context, db *sql.DB, key, def string) (string, error) {
	var value sql.NullString
	err := db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key_name=?", key).Scan(&value)
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

func boolSetting(raw string) bool {
	return strings.EqualFold(raw, "true") || raw == "1"
}

func writeActivity(ctx context.Context, db *sql.DB, userID *string, action, resourceType, resourceID, ip string, details interface{}) {
	payload, _ := json.Marshal(details)
	_, _ = db.ExecContext(ctx, `
		INSERT INTO activity_log (user_id, action, resource_type, resource_id, details, ip_address)
		VALUES (?, ?, ?, ?, ?, ?)`,
		userID, action, nullableString(resourceType), nullableString(resourceID), nullableJSON(payload), nullableString(ip))
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullableJSON(raw []byte) interface{} {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return string(raw)
}
