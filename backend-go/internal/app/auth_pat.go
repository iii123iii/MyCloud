package app

// Personal Access Token authentication + scope enforcement.
//
// requireAuth (router.go) routes any "mc_pat_…" bearer here instead of JWT
// parsing. A PAT authenticates the same user the token belongs to and produces
// the SAME request context as a JWT session (user_id + role), plus the token's
// scopes. requireScope then gates each route: it is a no-op for interactive
// JWT sessions and enforces the per-route scope only for PAT requests.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"mycloud/backend-go/internal/auth"
	"mycloud/backend-go/internal/httpapi"
)

// patTouchKey is the Redis set into which each PAT auth records a
// "<patID>|<ip>|<unixMillis>" member; the maintenance ticker drains it with
// SPOP and batches last_used_at/last_used_ip updates.
const patTouchKey = "pat:touch"

// allowedPATScopes is the closed set of scopes a token may carry. "*" is full
// access. Adding a scope here without also mapping the routes it guards (see
// patRouteScopes) leaves those routes "*"-only, which is the safe direction.
var allowedPATScopes = map[string]bool{
	"files:read":   true,
	"files:write":  true,
	"shares:read":  true,
	"shares:write": true,
	"tags:read":    true,
	"tags:write":   true,
	"account:read": true,
	"*":            true,
}

// patBlockedRoutes are route patterns a PAT may NEVER reach, regardless of
// scope — account/security management and token/webhook administration require
// an interactive session so a leaked token can't escalate privileges, lock the
// owner out, or mint/hide further credentials. Keyed by chi RoutePattern (the
// registered template incl. the /api/v2 prefix, matching requirePasswordCurrent).
// Everything under /api/v2/admin/* is blocked separately by prefix.
var patBlockedRoutes = map[string]bool{
	"/api/v2/auth/change-password":           true,
	"/api/v2/auth/account":                   true,
	"/api/v2/me/sessions":                    true,
	"/api/v2/me/sessions/{jti}":              true,
	"/api/v2/me/favorites":                   true,
	"/api/v2/me/favorites/{folderID}":        true,
	"/api/v2/me/tokens":                      true,
	"/api/v2/me/tokens/{id}":                 true,
	"/api/v2/me/webhooks":                    true,
	"/api/v2/me/webhooks/{id}":               true,
	"/api/v2/me/webhooks/{id}/deliveries":    true,
	"/api/v2/me/webhooks/{id}:test":          true,
	"/api/v2/me/webhooks/{id}:rotate-secret": true,
}

// patScopeExempt lists routes a PAT may call regardless of its scopes, so any
// token can verify itself and introspect what it's allowed to do. Keyed by chi
// RoutePattern. Kept tiny and read-only on purpose — only self-introspection
// belongs here, never anything that returns or mutates user data.
var patScopeExempt = map[string]bool{
	"/api/v2/auth/whoami": true,
}

// patRouteScopes maps "METHOD /api/v2/route/pattern" → required scope. Routes
// absent from this map fail closed (require "*") via requiredScopeFor, so a new
// endpoint is never accidentally exposed to a narrowly-scoped token until
// someone deliberately maps it. The tus upload mount is handled by prefix.
var patRouteScopes = map[string]string{
	// account
	"GET /api/v2/auth/me":          "account:read",
	"GET /api/v2/auth/me/activity": "account:read",

	// files / folders — reads
	"GET /api/v2/files":                              "files:read",
	"POST /api/v2/files:download-archive":            "files:read",
	"GET /api/v2/files/{id}":                         "files:read",
	"GET /api/v2/files/{id}:download":                "files:read",
	"GET /api/v2/files/{id}:preview":                 "files:read",
	"GET /api/v2/files/{id}/hls/playlist.m3u8":       "files:read",
	"GET /api/v2/files/{id}/hls/{segment}":           "files:read",
	"HEAD /api/v2/files/by-hash":                     "files:read",
	"GET /api/v2/files/{id}/lock":                    "files:read",
	"GET /api/v2/files/{id}/thumb":                   "files:read",
	"GET /api/v2/files/{id}/office-config":           "files:read",
	"GET /api/v2/files/{id}/versions":                "files:read",
	"GET /api/v2/files/{id}/versions/{vno}:download": "files:read",
	"GET /api/v2/files/{id}/versions/{vno}:preview":  "files:read",
	"GET /api/v2/files/{id}/versions/{a}/diff/{b}":   "files:read",
	"GET /api/v2/files/{id}/comments":                "files:read",
	"GET /api/v2/folders":                            "files:read",
	"GET /api/v2/folders/{id}":                       "files:read",
	"GET /api/v2/folders/{id}/path":                  "files:read",
	"GET /api/v2/folders/{id}/retention":             "files:read",
	"GET /api/v2/folders/{id}/activity":              "files:read",
	"GET /api/v2/storage/stats":                      "files:read",
	"GET /api/v2/photos":                             "files:read",
	"GET /api/v2/trash":                              "files:read",
	"GET /api/v2/search":                             "files:read",
	"GET /api/v2/smart-folders":                      "files:read",
	"GET /api/v2/smart-folders/{id}/results":         "files:read",

	// files / folders — writes
	"POST /api/v2/files:upload":                      "files:write",
	"POST /api/v2/files:batch":                       "files:write",
	"POST /api/v2/files/{id}/lock":                   "files:write",
	"DELETE /api/v2/files/{id}/lock":                 "files:write",
	"PUT /api/v2/folders/{id}/retention":             "files:write",
	"DELETE /api/v2/folders/{id}/retention":          "files:write",
	"POST /api/v2/files/by-hash":                     "files:write",
	"PATCH /api/v2/files/{id}":                       "files:write",
	"DELETE /api/v2/files/{id}":                      "files:write",
	"POST /api/v2/files/{id}:presign":                "files:write",
	"POST /api/v2/files/{id}:transfer":               "files:write",
	"POST /api/v2/files/{id}:copy":                   "files:write",
	"POST /api/v2/files/{id}/versions/{vno}:restore": "files:write",
	"POST /api/v2/files/{id}/comments":               "files:write",
	"PATCH /api/v2/comments/{id}":                    "files:write",
	"DELETE /api/v2/comments/{id}":                   "files:write",
	"POST /api/v2/folders":                           "files:write",
	"PATCH /api/v2/folders/{id}":                     "files:write",
	"DELETE /api/v2/folders/{id}":                    "files:write",
	"POST /api/v2/folders/{id}:copy":                 "files:write",
	"POST /api/v2/trash/{id}:restore":                "files:write",
	"DELETE /api/v2/trash/{id}":                      "files:write",
	"DELETE /api/v2/trash":                           "files:write",
	"POST /api/v2/smart-folders":                     "files:write",
	"PATCH /api/v2/smart-folders/{id}":               "files:write",
	"DELETE /api/v2/smart-folders/{id}":              "files:write",

	// shares + grants + upload-requests
	"GET /api/v2/shares":                  "shares:read",
	"POST /api/v2/shares":                 "shares:write",
	"DELETE /api/v2/shares/{id}":          "shares:write",
	"GET /api/v2/shares/grants":           "shares:read",
	"POST /api/v2/shares/grants":          "shares:write",
	"PATCH /api/v2/shares/grants/{id}":    "shares:write",
	"DELETE /api/v2/shares/grants/{id}":   "shares:write",
	"GET /api/v2/upload-requests":         "shares:read",
	"POST /api/v2/upload-requests":        "shares:write",
	"DELETE /api/v2/upload-requests/{id}": "shares:write",

	// tags
	"GET /api/v2/tags":                         "tags:read",
	"POST /api/v2/tags":                        "tags:write",
	"PATCH /api/v2/tags/{id}":                  "tags:write",
	"DELETE /api/v2/tags/{id}":                 "tags:write",
	"POST /api/v2/files/{id}/tags/{tagId}":     "tags:write",
	"DELETE /api/v2/files/{id}/tags/{tagId}":   "tags:write",
	"POST /api/v2/folders/{id}/tags/{tagId}":   "tags:write",
	"DELETE /api/v2/folders/{id}/tags/{tagId}": "tags:write",
}

// requiredScopeFor resolves the scope a PAT must hold for (method, pattern).
// Unmapped routes fail closed to "*" so they are reachable only by a
// full-access token. The tus resumable-upload mount uses several methods under
// one prefix, so it's matched by prefix rather than enumerated.
func requiredScopeFor(method, pattern string) string {
	if strings.HasPrefix(pattern, "/api/v2/files/tus") {
		return "files:write"
	}
	if s, ok := patRouteScopes[method+" "+pattern]; ok {
		return s
	}
	return "*"
}

func hasScope(scopes []string, want string) bool {
	for _, s := range scopes {
		if s == want || s == "*" {
			return true
		}
	}
	return false
}

// authenticatePAT validates a Personal Access Token bearer and, on success,
// continues the chain with a context identical to a JWT session plus the
// token's scopes. Mirrors requireAuth's failure envelopes so clients can't
// distinguish auth schemes from error shape.
func (a *App) authenticatePAT(w http.ResponseWriter, r *http.Request, next http.Handler, token string) {
	hash := auth.HashPATToken(token)
	var (
		id, userID, scopesCSV string
		expiresAt, revokedAt  sql.NullTime
	)
	err := a.Stmts.QueryRowContext(r.Context(),
		`SELECT id, user_id, scopes, expires_at, revoked_at
		   FROM personal_access_tokens WHERE token_hash = ?`, hash).
		Scan(&id, &userID, &scopesCSV, &expiresAt, &revokedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			a.Metrics.IncPATAuth("invalid")
			httpapi.Error(w, http.StatusUnauthorized, "unauthorized", "Invalid token")
			return
		}
		respondDBError(w, r, err)
		return
	}
	if revokedAt.Valid {
		a.Metrics.IncPATAuth("revoked")
		httpapi.Error(w, http.StatusUnauthorized, "unauthorized", "Token revoked")
		return
	}
	if expiresAt.Valid && !expiresAt.Time.After(time.Now()) {
		a.Metrics.IncPATAuth("expired")
		httpapi.Error(w, http.StatusUnauthorized, "unauthorized", "Token expired")
		return
	}
	role, active, _, err := getUserSessionFull(r.Context(), a.Stmts, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusUnauthorized, "unauthorized", "User not found")
			return
		}
		respondDBError(w, r, err)
		return
	}
	if !active {
		httpapi.Error(w, http.StatusUnauthorized, "unauthorized", "Account is disabled")
		return
	}
	a.Metrics.IncPATAuth("ok")
	a.recordPATUse(r.Context(), id, clientIP(r))

	ctx := withUser(r.Context(), userID, role)
	ctx = withPAT(ctx, id, splitScopes(scopesCSV))
	// Deliberately do not set mustChangePwKey: PATs are pre-authorised
	// credentials, not interactive logins, so the password-change gate
	// (requirePasswordCurrent) treats them as unaffected.
	next.ServeHTTP(w, r.WithContext(ctx))
}

// requireScope gates the authed chain. For interactive JWT sessions it is a
// pure pass-through; for PAT requests it enforces patBlockedRoutes, the admin
// prefix block, and the per-route scope (fail-closed for unmapped routes).
func (a *App) requireScope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scopes, isPAT := patScopesFrom(r)
		if !isPAT {
			next.ServeHTTP(w, r)
			return
		}
		pattern := chi.RouteContext(r.Context()).RoutePattern()
		if patBlockedRoutes[pattern] || strings.HasPrefix(pattern, "/api/v2/admin/") {
			httpapi.Error(w, http.StatusForbidden, "insufficient_scope",
				"This endpoint requires an interactive session and is not accessible with a personal access token.")
			return
		}
		// Scope-exempt endpoints (e.g. /auth/whoami) are reachable by any token
		// regardless of scope so a credential can always verify itself.
		if patScopeExempt[pattern] {
			next.ServeHTTP(w, r)
			return
		}
		required := requiredScopeFor(r.Method, pattern)
		if hasScope(scopes, required) {
			next.ServeHTTP(w, r)
			return
		}
		httpapi.Error(w, http.StatusForbidden, "insufficient_scope",
			"This token is missing the required scope: "+required)
	})
}

// recordPATUse queues a best-effort last-used update. Encoding the IP and
// timestamp into the set member keeps everything in one atomic structure (the
// flush SPOPs members and keeps the newest per token) — no per-request DB write
// and no companion hash to race across replicas.
func (a *App) recordPATUse(ctx context.Context, patID, ip string) {
	if a.Redis == nil || patID == "" {
		return
	}
	member := patID + "|" + ip + "|" + strconv.FormatInt(time.Now().UnixMilli(), 10)
	_ = a.Redis.SAdd(ctx, patTouchKey, member).Err()
}

// splitScopes parses a stored CSV scope string into a slice, dropping blanks.
func splitScopes(csv string) []string {
	if strings.TrimSpace(csv) == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// canonicalizeScopes validates that every requested scope is known, dedupes and
// sorts them, and collapses to ["*"] only when the caller EXPLICITLY asked for
// "*". A fully-checked set is NOT auto-collapsed to "*" — that would silently
// grant any scope added in a future release. Returns the CSV to persist.
func canonicalizeScopes(requested []string) (string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(requested))
	hasStar := false
	for _, s := range requested {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !allowedPATScopes[s] {
			return "", fmt.Errorf("unknown scope: %s", s)
		}
		if s == "*" {
			hasStar = true
			continue
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	if hasStar {
		return "*", nil
	}
	if len(out) == 0 {
		return "", fmt.Errorf("at least one scope is required")
	}
	sort.Strings(out)
	return strings.Join(out, ","), nil
}

// flushPATTouches drains the pat:touch set and batches last_used updates. SPOP
// (atomic read+remove) is used rather than SMEMBERS+DEL so a touch added by
// another replica between read and delete isn't lost. Per token only the newest
// observation is written. Best-effort: a drop on error costs at most an
// approximate timestamp.
func (a *App) flushPATTouches(ctx context.Context) {
	if a.Redis == nil {
		return
	}
	type seen struct {
		ip string
		ms int64
	}
	latest := map[string]seen{}
	for {
		members, err := a.Redis.SPopN(ctx, patTouchKey, 500).Result()
		if err != nil || len(members) == 0 {
			break
		}
		for _, m := range members {
			parts := strings.SplitN(m, "|", 3)
			if len(parts) != 3 {
				continue
			}
			ms, _ := strconv.ParseInt(parts[2], 10, 64)
			if cur, ok := latest[parts[0]]; !ok || ms > cur.ms {
				latest[parts[0]] = seen{ip: parts[1], ms: ms}
			}
		}
		if len(members) < 500 {
			break
		}
	}
	for patID, s := range latest {
		_, _ = a.DB.ExecContext(ctx,
			"UPDATE personal_access_tokens SET last_used_at = ?, last_used_ip = ? WHERE id = ?",
			time.UnixMilli(s.ms).UTC(), s.ip, patID)
	}
}
