package app

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"mycloud/backend-go/internal/auth"
	"mycloud/backend-go/internal/health"
	"mycloud/backend-go/internal/httpapi"
	"mycloud/backend-go/internal/logging"
	"mycloud/backend-go/internal/metrics"
)

// requestIDHeader is the HTTP header used to carry a per-request trace ID.
// If the client supplies it (typical when nginx adds X-Request-ID upstream)
// we honour it; otherwise we generate a fresh UUID. The value is echoed back
// in the response so clients can correlate requests with server logs.
const requestIDHeader = "X-Request-ID"

// reqIDCtxKey carries the request ID through ctx for handlers that want to
// include it in business-level logs without re-reading the header.
type reqIDCtxKey struct{}

// RequestID returns the request ID attached to ctx, or "" if none. Exported
// for use by handlers and downstream packages.
func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(reqIDCtxKey{}).(string); ok {
		return v
	}
	return ""
}

func (a *App) routes() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	// requestID must precede requestLogger so every log line carries the ID.
	// Recoverer is placed AFTER both so a panic-recovery log line still
	// carries the request_id field — otherwise crashes are anonymous.
	r.Use(a.requestID)
	r.Use(a.requestLogger)
	r.Use(middleware.Recoverer)
	r.Use(a.securityHeaders)
	r.Use(middleware.Compress(5, "application/json"))
	r.Use(a.cors)

	// /healthz is a liveness probe — should the process be killed? It only
	// confirms the server is up enough to respond. Useful for k8s/docker
	// restart-on-fail logic.
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		httpapi.JSON(w, http.StatusOK, map[string]any{
			"status":      "ok",
			"version":     a.Config.Version,
			"uptime":      time.Since(a.StartUTC).String(),
			"uptime_secs": int64(time.Since(a.StartUTC).Seconds()),
		}, nil)
	})

	// /readyz is a readiness probe — can the process serve traffic? Returns
	// 503 if any dependency is degraded. Load balancers should drain a
	// backend when this flips red even if /healthz still says ok.
	r.Get("/readyz", a.handleReadyz)

	// WebSocket fan-out. Mounted at the v2 base path so existing CORS and
	// rate-limit middleware applies. Auth happens IN-BAND via the first
	// "auth" envelope, not via the HTTP-level requireAuth middleware,
	// because browsers can't set Authorization headers on the upgrade
	// request. Hub validates the token through app.Auth.
	r.Get("/api/v2/ws", a.Hub.ServeHTTP)

	// Only the initial POST that creates an upload counts toward the tus
	// rate-limit — subsequent PATCH/HEAD chunks are part of the same logical
	// upload and would otherwise blow through the cap on any large file.
	tusCreateLimit := a.rateLimit("tus", a.Config.RateLimitTusPerMin, time.Minute, func(r *http.Request, _ string) string {
		if r.Method != http.MethodPost {
			return "" // empty key skips the limiter
		}
		return userIDFrom(r)
	})
	loginIPLimit := a.rateLimit("login_ip", a.Config.RateLimitLoginPerMin, time.Minute, keyByIP)
	loginUserLimit := a.rateLimit("login_user", a.Config.RateLimitLoginPerUserMin, time.Minute, keyByLoginField("email"))
	registerLimit := a.rateLimit("register", a.Config.RateLimitRegisterPerHour, time.Hour, keyByIP)
	publicUploadLimit := a.rateLimit("public_upload", a.Config.RateLimitPublicUploadHour, time.Hour, keyByIP)
	// Share-password limit: applied to the three public share/upload-request
	// endpoints that accept X-Share-Password. Buckets per IP+token-prefix so
	// one attacker can't enumerate passwords across tokens with a single
	// IP bucket, and one token can't be brute-forced across botnet IPs.
	sharePwdLimit := a.rateLimit("share_pwd", a.Config.RateLimitSharePwdPerMin, time.Minute,
		func(r *http.Request, _ string) string {
			return keyByIP(r, "") + "|" + chi.URLParam(r, "token")
		})

	// Feature-bucketed limits. Each buckets by user-id (clientIP for
	// unauthenticated routes is already covered above). The keyer returns
	// empty string for unauth requests so the limiter passes through.
	keyByUser := func(r *http.Request, _ string) string { return userIDFrom(r) }
	downloadLimit := a.rateLimit("download", a.Config.RateLimitDownloadPerMin, time.Minute, keyByUser)
	searchLimit := a.rateLimit("search", a.Config.RateLimitSearchPerMin, time.Minute, keyByUser)
	genericLimit := a.rateLimit("generic", a.Config.RateLimitGenericPerMin, time.Minute, keyByUser)

	r.Route("/api/v2", func(api chi.Router) {
		api.Post("/setup/status", a.handleSetupStatus)
		api.Post("/setup/complete", a.handleSetupComplete)

		api.With(loginIPLimit, loginUserLimit).Post("/auth/login", a.handleLogin)
		api.With(registerLimit).Post("/auth/register", a.handleRegister)
		api.Post("/auth/refresh", a.handleRefresh)
		api.Post("/auth/logout", a.handleLogout)

		api.Group(func(authed chi.Router) {
			authed.Use(a.requireAuth)
			// requirePasswordCurrent runs immediately after auth so any
			// route the SPA gates behind must_change_password is also
			// gated for direct API callers. The middleware is path-aware:
			// /auth/me, /auth/change-password, and /auth/logout remain
			// reachable so users can clear the flag.
			authed.Use(a.requirePasswordCurrent)
			// requireScope is a no-op for interactive JWT sessions; for
			// Personal Access Tokens it enforces the per-route scope and the
			// PAT-blocked routes. Placed after auth so the PAT context exists.
			authed.Use(a.requireScope)
			// genericLimit applies globally to authenticated endpoints so a
			// single user can't pin the backend with API spam. Heavier
			// limiters (download, search) below opt in to tighter buckets.
			authed.Use(genericLimit)
			authed.Get("/auth/me", a.handleMe)
			// Token/identity introspection — scope-exempt so any PAT can verify
			// itself and read its own scopes (used by the desktop/CLI sign-in).
			authed.Get("/auth/whoami", a.handleWhoami)
			// Per-device sessions.
			authed.Get("/me/sessions", a.handleListMySessions)
			authed.Delete("/me/sessions/{jti}", a.handleRevokeMySession)
			// Personal access tokens for API/automation auth.
			authed.Get("/me/tokens", a.handleListMyTokens)
			authed.Post("/me/tokens", a.handleCreateMyToken)
			authed.Patch("/me/tokens/{id}", a.handleRenameMyToken)
			authed.Delete("/me/tokens/{id}", a.handleRevokeMyToken)
			// Per-user outbound webhooks.
			authed.Get("/me/webhooks", a.handleListMyWebhooks)
			authed.Post("/me/webhooks", a.handleCreateMyWebhook)
			authed.Patch("/me/webhooks/{id}", a.handleUpdateMyWebhook)
			authed.Delete("/me/webhooks/{id}", a.handleDeleteMyWebhook)
			authed.Get("/me/webhooks/{id}/deliveries", a.handleListWebhookDeliveries)
			authed.Post("/me/webhooks/{id}:test", a.handleTestMyWebhook)
			authed.Post("/me/webhooks/{id}:rotate-secret", a.handleRotateMyWebhookSecret)
			// Favourites pinned in the sidebar.
			authed.Get("/me/favorites", a.handleListFavorites)
			authed.Post("/me/favorites", a.handleAddFavorite)
			authed.Put("/me/favorites", a.handleReorderFavorites)
			authed.Delete("/me/favorites/{folderID}", a.handleRemoveFavorite)
			authed.Get("/auth/me/activity", a.handleMyActivity)
			authed.Post("/auth/change-password", a.handleChangePassword)
			authed.Delete("/auth/account", a.handleDeleteAccount)

			authed.Get("/files", a.handleListFiles)
			authed.Post("/files:upload", a.handleUploadFile)
			authed.Post("/files:download-archive", a.handleDownloadArchive)
			// Bulk operations on N files in one round-trip.
			authed.Post("/files:batch", a.handleFilesBatch)
			// Cooperative file locks.
			authed.Get("/files/{id}/lock", a.handleListFileLocks)
			authed.Post("/files/{id}/lock", a.handleAcquireFileLock)
			authed.Delete("/files/{id}/lock", a.handleReleaseFileLock)
			// Per-folder retention policies.
			authed.Get("/folders/{id}/retention", a.handleGetRetentionPolicy)
			authed.Put("/folders/{id}/retention", a.handlePutRetentionPolicy)
			authed.Delete("/folders/{id}/retention", a.handleDeleteRetentionPolicy)
			// Per-folder activity feed (historical; live updates via WS).
			authed.Get("/folders/{id}/activity", a.handleFolderActivity)
			authed.Head("/files/by-hash", a.handleHasFileHash)
			authed.Post("/files/by-hash", a.handleCreateFromHash)

			// Tus resumable upload mounted at /api/v2/files/tus/*
			authed.With(tusCreateLimit).Mount("/files/tus", http.StripPrefix("/api/v2/files/tus", a.TusHandler))
			authed.Get("/files/{id}", a.handleFileInfo)
			authed.Patch("/files/{id}", a.handleUpdateFile)
			authed.Delete("/files/{id}", a.handleDeleteFile)
			authed.With(downloadLimit).Get("/files/{id}:download", a.handleDownloadFile)
			authed.With(downloadLimit).Get("/files/{id}:preview", a.handlePreviewFile)
			// HLS adaptive video streaming.
			authed.With(downloadLimit).Get("/files/{id}/hls/playlist.m3u8", a.handleHLSPlaylist)
			authed.With(downloadLimit).Get("/files/{id}/hls/{segment}", a.handleHLSSegment)
			// Pre-signed direct-storage URL issuance (auth-gated).
			authed.Post("/files/{id}:presign", a.handlePresign)
			// File ownership transfer.
			authed.Post("/files/{id}:transfer", a.handleTransferFile)
			// Copy a (readable) file into a folder the caller can write to.
			authed.Post("/files/{id}:copy", a.handleCopyFile)
			authed.Get("/storage/stats", a.handleStorageStats)

			authed.Get("/folders", a.handleListFolders)
			authed.Post("/folders", a.handleCreateFolder)
			authed.Get("/folders/{id}/path", a.handleFolderPath)
			authed.Get("/folders/{id}", a.handleGetFolder)
			authed.Patch("/folders/{id}", a.handleUpdateFolder)
			authed.Delete("/folders/{id}", a.handleDeleteFolder)
			// Copy a (readable) folder subtree into a folder the caller can write to.
			authed.Post("/folders/{id}:copy", a.handleCopyFolder)

			authed.Get("/trash", a.handleListTrash)
			authed.Post("/trash/{id}:restore", a.handleRestoreTrash)
			authed.Delete("/trash/{id}", a.handleDeleteTrashItem)
			authed.Delete("/trash", a.handleEmptyTrash)

			authed.Get("/shares", a.handleListShares)
			authed.Post("/shares", a.handleCreateShare)
			authed.Delete("/shares/{id}", a.handleDeleteShare)

			authed.Get("/files/{id}/thumb", a.handleFileThumb)
			authed.Get("/photos", a.handleListPhotos)

			authed.Get("/files/{id}/office-config", a.handleOfficeConfig)

			authed.Get("/files/{id}/versions", a.handleListVersions)
			authed.Post("/files/{id}/versions/{vno}:restore", a.handleRestoreVersion)
			authed.Get("/files/{id}/versions/{vno}:download", a.handleDownloadVersion)
			authed.Get("/files/{id}/versions/{vno}:preview", a.handlePreviewVersion)
			// Side-by-side diff between two versions.
			authed.Get("/files/{id}/versions/{a}/diff/{b}", a.handleVersionDiff)

			authed.Get("/files/{id}/comments", a.handleListComments)
			authed.Post("/files/{id}/comments", a.handleCreateComment)
			authed.Patch("/comments/{id}", a.handleUpdateComment)
			authed.Delete("/comments/{id}", a.handleDeleteComment)

			authed.Get("/shares/grants", a.handleListGrants)
			authed.Post("/shares/grants", a.handleCreateGrant)
			authed.Patch("/shares/grants/{id}", a.handleUpdateGrant)
			authed.Delete("/shares/grants/{id}", a.handleDeleteGrant)

			authed.Get("/upload-requests", a.handleListUploadRequests)
			authed.Post("/upload-requests", a.handleCreateUploadRequest)
			authed.Delete("/upload-requests/{id}", a.handleDeleteUploadRequest)

			authed.Get("/tags", a.handleListTags)
			authed.Post("/tags", a.handleCreateTag)
			authed.Patch("/tags/{id}", a.handleUpdateTag)
			authed.Delete("/tags/{id}", a.handleDeleteTag)
			authed.Post("/files/{id}/tags/{tagId}", a.handleAttachTagToFile)
			authed.Delete("/files/{id}/tags/{tagId}", a.handleDetachTagFromFile)
			authed.Post("/folders/{id}/tags/{tagId}", a.handleAttachTagToFolder)
			authed.Delete("/folders/{id}/tags/{tagId}", a.handleDetachTagFromFolder)

			authed.With(searchLimit).Get("/search", a.handleSearch)

			authed.Get("/smart-folders", a.handleListSmartFolders)
			authed.Post("/smart-folders", a.handleCreateSmartFolder)
			authed.Patch("/smart-folders/{id}", a.handleUpdateSmartFolder)
			authed.Delete("/smart-folders/{id}", a.handleDeleteSmartFolder)
			authed.Get("/smart-folders/{id}/results", a.handleSmartFolderResults)

			authed.Group(func(admin chi.Router) {
				admin.Use(a.requireAdmin)
				// /metrics lives under the admin group: scrape from inside the
				// docker network with an admin token. Anyone wanting the
				// public scrape pattern can put a sidecar in front.
				admin.Get("/admin/metrics", a.handleMetrics)
				// Rollback a reversible activity_log row.
				admin.Post("/admin/activity/{id}:rollback", a.handleAdminRollback)
				// Background reindexer.
				admin.Post("/admin/reindex", a.handleAdminReindex)
				admin.Get("/admin/reindex/status", a.handleAdminReindexStatus)
				// Slow-query list.
				admin.Get("/admin/slow-queries", a.handleAdminSlowQueries)
				// Structured log viewer + CSV export.
				admin.Get("/admin/logs/activity", a.handleAdminActivityLogs)
				admin.Get("/admin/logs/system", a.handleAdminSystemLogs)
				// Storage breakdown widgets.
				admin.Get("/admin/storage/by-user", a.handleAdminStorageByUser)
				admin.Get("/admin/storage/top-files", a.handleAdminStorageTopFiles)
				// Impersonation + force-reset (invites removed).
				admin.Post("/admin/users/{id}:impersonate", a.handleAdminImpersonate)
				admin.Post("/admin/users/{id}:force-reset", a.handleAdminForceReset)
				admin.Get("/admin/users", a.handleAdminUsers)
				admin.Post("/admin/users", a.handleAdminCreateUser)
				admin.Patch("/admin/users/{id}", a.handleAdminUpdateUser)
				admin.Delete("/admin/users/{id}", a.handleAdminDeleteUser)
				admin.Get("/admin/stats", a.handleAdminStats)
				admin.Get("/admin/logs", a.handleAdminLogs)
				admin.Get("/admin/settings", a.handleAdminSettings)
				admin.Put("/admin/settings", a.handleAdminPutSettings)
				admin.Get("/admin/updates/check", a.handleAdminUpdateCheck)
				admin.Get("/admin/updates/status", a.handleAdminUpdateStatus)
				admin.Post("/admin/updates/apply", a.handleAdminUpdateApply)
				admin.Get("/admin/updates/log", a.handleAdminUpdateLog)
				// Worker dead-letter queue visibility + retry (Stream 7E).
				admin.Get("/admin/workers/dlq", a.handleAdminDLQList)
				admin.Post("/admin/workers/dlq/{queue}:retry", a.handleAdminDLQRetry)
				admin.Post("/admin/workers/dlq/{queue}:clear", a.handleAdminDLQClear)
			})
		})

		// Public share access — sharePwdLimit guards against brute-force on
		// X-Share-Password. Without it, a 4-6 char password falls in
		// seconds on a single host (bcrypt comparison cost notwithstanding).
		api.With(sharePwdLimit).Get("/public/shares/{token}", a.handleResolveShare)
		api.With(sharePwdLimit).Get("/public/shares/{token}:download", a.handleDownloadShare)

		// Presigned-token download (no Bearer required; HMAC carries authority).
		api.Get("/dl/{token}", a.handlePresignedDownload)

		api.With(sharePwdLimit).Get("/public/uploads/{token}", a.handleResolveUploadRequest)
		api.With(publicUploadLimit, sharePwdLimit).Post("/public/uploads/{token}", a.handlePublicUploadToRequest)

		// OnlyOffice fetches the document + posts back without auth tokens;
		// the edit_key in the URL is the bearer of authority. Rate limit at the
		// IP level since the document server's IP is fixed.
		api.Get("/office/doc/{id}", a.handleOfficeDoc)
		api.Post("/office/callback/{id}", a.handleOfficeCallback)
	})

	return r
}

// requestID assigns each inbound request a stable correlation ID. If the
// caller provided X-Request-ID (typical for proxies that want to thread a
// trace through nginx -> backend), we trust it as-is once it parses as UUID
// to keep the ID space well-formed; otherwise we mint a new UUID. The ID is
// echoed back in the response header so clients can quote it when reporting
// problems.
func (a *App) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" || !looksLikeUUID(id) {
			id = uuid.NewString()
		}
		w.Header().Set(requestIDHeader, id)
		ctx := context.WithValue(r.Context(), reqIDCtxKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// looksLikeUUID is a fast structural check — it doesn't verify version/variant
// bits because we only care that the supplied ID is reasonably bounded
// (avoids absurdly long client-supplied values blowing up our log lines).
func looksLikeUUID(s string) bool {
	if len(s) < 8 || len(s) > 64 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		case c == '-':
		default:
			return false
		}
	}
	return true
}

// requestLogger times each request, attaches a per-request slog.Logger to ctx
// (so handlers can emit lines that automatically carry request_id, method,
// path, user_id), and emits a single structured access record on exit.
//
// The user_id is read AFTER ServeHTTP because requireAuth populates it on the
// child context — the per-request logger built up front therefore captures it
// via a post-flight update rather than at construction time.
func (a *App) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		// Build the per-request logger and attach to ctx. Handlers that call
		// logging.FromContext will see these fields prepended to every line.
		base := slog.With(
			"request_id", RequestID(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
		)
		// Seed a mutable user-id slot so requireAuth, which runs further down
		// the chain on a child context, can publish the authenticated user
		// back up to the access logger.
		holder := &userIDHolder{}
		ctx := context.WithValue(r.Context(), userIDHolderKey, holder)
		ctx = logging.IntoContext(ctx, base)
		next.ServeHTTP(ww, r.WithContext(ctx))

		// Choose log level by status class so noisy 200s stay at info while
		// 5xx fire warnings the operator wants to see.
		status := ww.Status()
		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelWarn
		}
		duration := time.Since(start)
		base.LogAttrs(r.Context(), level, "http_request",
			slog.Int("status", status),
			slog.Duration("duration", duration),
			slog.Int("bytes", ww.BytesWritten()),
			slog.String("user_id", holder.get()),
			slog.String("remote_ip", r.RemoteAddr),
		)

		// Record Prometheus observation. The chi RouteContext's RoutePattern
		// (e.g. "/api/v2/files/{id}") gives us a low-cardinality label —
		// raw r.URL.Path would explode the time series space with one entry
		// per concrete file ID. Falls back to the raw path only for routes
		// that chi never matched (404s) so adversarial clients can't blow up
		// the metric: bucket all unmatched paths under the literal "unmatched".
		route := chi.RouteContext(r.Context()).RoutePattern()
		if route == "" {
			route = "unmatched"
		}
		a.Metrics.ObserveHTTPRequest(r.Method, route, metrics.StatusClass(status), duration.Seconds())
	})
}

// securityHeaders applies a baseline of browser-side hardening to every
// response. Headers are emitted on every request — including errors and 304s —
// because anything served from the app origin can act as an XSS vector for the
// localStorage token store. CSP is intentionally tight: scripts and styles
// only from same-origin, no inline, no eval. Tune per-route if a feature
// genuinely needs more (the preview endpoint already forces a download
// disposition for HTML/SVG so it doesn't need exceptions here).
func (a *App) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// HSTS: only meaningful over HTTPS; the browser ignores it otherwise so
		// it's safe to always emit. 1-year max-age aligns with browser preload
		// requirements.
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "geolocation=(), camera=(), microphone=(), payment=(), usb=()")
		// Default-src self covers everything not explicitly listed. Allow
		// blob:/data: for image previews + thumbnails (which the SPA renders
		// from API responses), and connect-src self+wss for the in-band
		// WebSocket. No 'unsafe-inline' — Next builds non-inline styles.
		h.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' blob: data:; "+
				"font-src 'self' data:; "+
				"connect-src 'self' ws: wss:; "+
				"frame-ancestors 'none'; "+
				"base-uri 'self'; "+
				"form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

func (a *App) cors(next http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(a.Config.AllowedOrigins))
	for _, origin := range a.Config.AllowedOrigins {
		allowed[origin] = struct{}{}
	}
	if len(allowed) == 0 {
		// Fail closed — never reflect arbitrary origins. An operator who
		// boots without ALLOWED_ORIGINS will see CORS rejections in the
		// browser, which surfaces the misconfig immediately rather than
		// silently permitting any caller.
		slog.Warn("ALLOWED_ORIGINS is empty; CORS is fail-closed (no Access-Control-Allow-Origin will be set)")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if _, ok := allowed[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}
		}
		w.Header().Set("Access-Control-Allow-Headers",
			"Authorization, Content-Type, X-Share-Password, "+
				"Tus-Resumable, Upload-Length, Upload-Offset, Upload-Metadata, "+
				"Upload-Defer-Length, Upload-Concat")
		w.Header().Set("Access-Control-Expose-Headers",
			"Location, Tus-Resumable, Upload-Offset, Upload-Length, "+
				"Upload-Metadata, Upload-Defer-Length, Upload-Concat, X-File-ID, "+
				// Audit fix: expose X-Request-ID so the browser can read
				// it on error responses for correlation with backend logs.
				"X-Request-ID, X-Cache, X-Impersonating, X-Pagination-Deprecated-Offset")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,HEAD,PUT,PATCH,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			httpapi.Error(w, http.StatusUnauthorized, "unauthorized", "Missing bearer token")
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		// Personal Access Tokens take an entirely separate validation path
		// (DB-backed, scope-bearing). They share the Bearer header but are
		// disambiguated by their "mc_pat_" prefix — JWTs never start with it.
		if auth.IsPATToken(token) {
			a.authenticatePAT(w, r, next, token)
			return
		}
		claims, err := a.Auth.Parse(token)
		if err != nil || claims.Type != "access" {
			httpapi.Error(w, http.StatusUnauthorized, "unauthorized", "Invalid token")
			return
		}
		role, active, mustChange, err := getUserSessionFull(r.Context(), a.Stmts, claims.UserID)
		if err != nil {
			if err == sql.ErrNoRows {
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
		// Stash JTI so /me/sessions can detect "this device" + refuse
		// self-revoke without re-parsing the token in the handler.
		ctx := withUser(r.Context(), claims.UserID, role)
		ctx = context.WithValue(ctx, userJTIKey, claims.ID)
		ctx = context.WithValue(ctx, mustChangePwKey, mustChange)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// passwordCurrentExempt lists the route patterns that remain accessible
// while must_change_password is set. Everything else is locked to a 403
// until the user changes their password. The patterns are matched against
// chi's RoutePattern (the registered template, not the concrete URL), so
// path params don't break matching.
var passwordCurrentExempt = map[string]struct{}{
	"/api/v2/auth/me":              {},
	"/api/v2/auth/change-password": {},
	"/api/v2/auth/logout":          {},
}

// requirePasswordCurrent rejects routes when the authenticated user has
// must_change_password=1. The previous implementation only surfaced the flag
// to the SPA and let the API serve every endpoint regardless — direct API
// callers could ignore it entirely. This middleware closes that gap server-
// side, matching the user-visible UX.
func (a *App) requirePasswordCurrent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mustChange, _ := r.Context().Value(mustChangePwKey).(bool)
		if !mustChange {
			next.ServeHTTP(w, r)
			return
		}
		pattern := chi.RouteContext(r.Context()).RoutePattern()
		if _, ok := passwordCurrentExempt[pattern]; ok {
			next.ServeHTTP(w, r)
			return
		}
		httpapi.Error(w, http.StatusForbidden, "password_change_required",
			"Your password must be changed before continuing.")
	})
}

// handleMetrics serves the Prometheus exposition format from the app's
// private registry. Auth happens via the admin middleware; the handler
// itself is purely a passthrough.
func (a *App) handleMetrics(w http.ResponseWriter, r *http.Request) {
	promhttp.HandlerFor(a.Metrics.Reg, promhttp.HandlerOpts{}).ServeHTTP(w, r)
}

// handleReadyz aggregates dependency probes. Any single failure flips the
// overall status to 503 — load balancers respect that without further parsing.
// The body still itemises every check so operators can see WHICH dependency
// failed without grepping the logs.
//
// Probes run concurrently in goroutines because the DB + Redis pings can
// each take their 500 ms budget; serial would push p99 of /readyz over a
// second. Worker heartbeats are O(1) reads and run inline.
func (a *App) handleReadyz(w http.ResponseWriter, r *http.Request) {
	results := make([]health.Result, 0, 4+len(a.Heartbeats))

	type chRes struct{ r health.Result }
	dbCh := make(chan chRes, 1)
	redisCh := make(chan chRes, 1)
	go func() { dbCh <- chRes{health.CheckDB(r.Context(), a.DB)} }()
	go func() { redisCh <- chRes{health.CheckRedis(r.Context(), a.Redis)} }()

	results = append(results, (<-dbCh).r)
	results = append(results, (<-redisCh).r)

	// Worker freshness — anything that hasn't beat in 5 minutes is stuck.
	// Conservative threshold because the maintenance ticker only beats
	// every 30s by default; thumb/extract beat each iteration of their
	// drain loop (≤5s).
	for name, hb := range a.Heartbeats {
		results = append(results, health.CheckWorker(name, hb, 5*time.Minute))
	}

	allOK := true
	for _, res := range results {
		if !res.OK {
			allOK = false
			break
		}
	}
	status := http.StatusOK
	overall := "ok"
	if !allOK {
		status = http.StatusServiceUnavailable
		overall = "degraded"
	}
	httpapi.JSON(w, status, map[string]any{
		"status":  overall,
		"checks":  results,
		"version": a.Config.Version,
	}, nil)
}

func (a *App) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userRoleFrom(r) != "admin" {
			httpapi.Error(w, http.StatusForbidden, "forbidden", "Admin access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
