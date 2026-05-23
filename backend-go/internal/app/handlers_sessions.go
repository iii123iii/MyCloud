package app

// Session management endpoints.
//
//   GET    /api/v2/me/sessions          → list active sessions for caller
//   DELETE /api/v2/me/sessions/{jti}    → revoke one (current-device-safe)
//
// Token issuance writes a row here; the existing blacklist (bl:<jti> in
// Redis) revokes by jti so the auth middleware doesn't need a DB hit on
// the hot path.
//
// NB: the existing JWT manager already issues a jti claim — see auth/token.go.
// This handler just consumes it.

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"mycloud/backend-go/internal/httpapi"
	"mycloud/backend-go/internal/wsHub"
)

func (a *App) handleListMySessions(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	// Caller's own JTI — used to tag the matching row as is_current so the
	// FE can grey out the Revoke button (you logout instead).
	currentJTI := jtiFrom(r)
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT jti, COALESCE(device_label, ''), COALESCE(user_agent, ''), COALESCE(ip_address, ''),
		       created_at, last_seen_at, expires_at
		FROM sessions
		WHERE user_id = ? AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY last_seen_at DESC`, userID)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var jti, label, ua, ip string
		var created, seen time.Time
		var expires sql.NullTime
		if err := rows.Scan(&jti, &label, &ua, &ip, &created, &seen, &expires); err != nil {
			respondDBError(w, r, err)
			return
		}
		row := map[string]any{
			"jti":          jti,
			"device_label": label,
			"user_agent":   ua,
			"ip_address":   ip,
			"created_at":   created,
			"last_seen_at": seen,
			"is_current":   jti == currentJTI,
		}
		if expires.Valid {
			row["expires_at"] = expires.Time
		}
		out = append(out, row)
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"sessions": out}, nil)
}

func (a *App) handleRevokeMySession(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	jti := chi.URLParam(r, "jti")
	if jti == "" {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "missing jti")
		return
	}
	// Refuse self-revoke. Logging yourself out via the sessions panel is a
	// footgun: you'd kill the very token that's authorising the request,
	// leaving you stranded without confirmation. The "Sign out" button is
	// the right path for that.
	if jti == jtiFrom(r) {
		httpapi.Error(w, http.StatusBadRequest, "self_revoke",
			"Use Sign out to end your current session — Revoke is for other devices.")
		return
	}
	res, err := a.DB.ExecContext(r.Context(),
		`DELETE FROM sessions WHERE jti = ? AND user_id = ?`, jti, userID)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpapi.Error(w, http.StatusNotFound, "not_found", "session not found")
		return
	}
	// Blacklist the JTI so any in-flight access tokens stop working
	// immediately. TTL set to the access-token TTL — past that the token
	// has expired anyway.
	if a.Redis != nil {
		ttl := time.Duration(a.Config.AccessTokenTTL) * time.Second
		if ttl <= 0 {
			ttl = 15 * time.Minute
		}
		_ = a.Redis.Set(r.Context(), "bl:"+jti, 1, ttl).Err()
	}
	// Publish to the user's WS topic so the revoked device hears about it
	// in real time and can boot itself to /login without waiting for its
	// next API call to fail. The frontend WS provider matches the JTI
	// against its locally-stored access_jti and kicks itself out on match.
	if a.Hub != nil {
		_ = a.Hub.Broker().Publish(r.Context(), "user:"+userID, wsHub.Envelope{
			Type:  "session_revoked",
			Topic: "user:" + userID,
			Data:  map[string]any{"jti": jti},
		})
	}
	w.WriteHeader(http.StatusNoContent)
}

// recordSessionFromTokens unpacks the access_jti + access_expires_at that
// auth.IssuePair returns and writes a sessions row. Keeps the call site in
// handlers_auth.go a one-liner. Silently no-ops when the token map lacks
// the JTI (e.g., during test fakes) so calling code can stay unconditional.
func (a *App) recordSessionFromTokens(r *http.Request, userID string, tokens map[string]any) {
	jti, _ := tokens["access_jti"].(string)
	if jti == "" {
		return
	}
	expires, _ := tokens["access_expires_at"].(time.Time)
	if expires.IsZero() {
		expires = time.Now().Add(time.Duration(a.Config.AccessTokenTTL) * time.Second)
	}
	a.recordSession(r, jti, userID, expires)
}

// recordSession upserts a session row on login/refresh. Called from the
// auth handlers after a successful token issuance. The UA string is
// truncated to 500 chars so we don't blow up on bot UAs that include
// kilobyte-long version trails.
func (a *App) recordSession(r *http.Request, jti, userID string, expires time.Time) {
	ua := r.UserAgent()
	if len(ua) > 500 {
		ua = ua[:500]
	}
	label := deviceLabelFromUA(ua)
	ip := clientIP(r)
	_, _ = a.DB.ExecContext(r.Context(), `
		INSERT INTO sessions (jti, user_id, device_label, user_agent, ip_address, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		    last_seen_at = NOW(),
		    user_agent   = VALUES(user_agent),
		    ip_address   = VALUES(ip_address),
		    expires_at   = VALUES(expires_at)`,
		jti, userID, label, ua, ip, expires)
}

// deviceLabelFromUA picks a short human label from a User-Agent string.
// Avoids dragging in a full UA-parsing dep — the goal is "Chrome on Mac"
// not "Chrome/121.0.6167.85 Safari/537.36".
func deviceLabelFromUA(ua string) string {
	ua = strings.ToLower(ua)
	browser := "Unknown browser"
	switch {
	case strings.Contains(ua, "edg/"):
		browser = "Edge"
	case strings.Contains(ua, "chrome/"):
		browser = "Chrome"
	case strings.Contains(ua, "firefox/"):
		browser = "Firefox"
	case strings.Contains(ua, "safari/") && !strings.Contains(ua, "chrome"):
		browser = "Safari"
	case strings.Contains(ua, "electron/") || strings.Contains(ua, "mycloud-desktop"):
		browser = "Desktop sync"
	case strings.Contains(ua, "curl/"):
		browser = "curl"
	}
	platform := "Unknown OS"
	switch {
	case strings.Contains(ua, "windows"):
		platform = "Windows"
	// iOS devices must be matched BEFORE mac. iPhone/iPad UAs include
	// "like Mac OS X" verbatim — checking macOS first misclassifies every
	// mobile Safari user as desktop Safari, which broke the session
	// device label.
	case strings.Contains(ua, "iphone"):
		platform = "iPhone"
	case strings.Contains(ua, "ipad"):
		platform = "iPad"
	case strings.Contains(ua, "mac os") || strings.Contains(ua, "macintosh"):
		platform = "macOS"
	case strings.Contains(ua, "android"):
		platform = "Android"
	case strings.Contains(ua, "linux"):
		platform = "Linux"
	}
	return browser + " on " + platform
}
