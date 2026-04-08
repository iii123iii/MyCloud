package app

import (
	"database/sql"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"mycloud/backend-go/internal/httpapi"
)

func (a *App) routes() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(a.requestLogger)
	r.Use(middleware.Compress(5, "application/json"))
	r.Use(a.cors)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		httpapi.JSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"version": a.Config.Version,
			"uptime":  time.Since(a.StartUTC).String(),
		}, nil)
	})

	r.Route("/api/v2", func(api chi.Router) {
		api.Post("/setup/status", a.handleSetupStatus)
		api.Post("/setup/complete", a.handleSetupComplete)

		api.Post("/auth/login", a.handleLogin)
		api.Post("/auth/register", a.handleRegister)
		api.Post("/auth/refresh", a.handleRefresh)
		api.Post("/auth/logout", a.handleLogout)

		api.Group(func(authed chi.Router) {
			authed.Use(a.requireAuth)
			authed.Get("/auth/me", a.handleMe)
			authed.Post("/auth/change-password", a.handleChangePassword)
			authed.Delete("/auth/account", a.handleDeleteAccount)

			authed.Get("/files", a.handleListFiles)
			authed.Post("/files:upload", a.handleUploadFile)
			authed.Get("/files/{id}", a.handleFileInfo)
			authed.Patch("/files/{id}", a.handleUpdateFile)
			authed.Delete("/files/{id}", a.handleDeleteFile)
			authed.Get("/files/{id}:download", a.handleDownloadFile)
			authed.Get("/files/{id}:preview", a.handlePreviewFile)
			authed.Get("/storage/stats", a.handleStorageStats)

			authed.Get("/folders", a.handleListFolders)
			authed.Post("/folders", a.handleCreateFolder)
			authed.Get("/folders/{id}/path", a.handleFolderPath)
			authed.Get("/folders/{id}", a.handleGetFolder)
			authed.Patch("/folders/{id}", a.handleUpdateFolder)
			authed.Delete("/folders/{id}", a.handleDeleteFolder)

			authed.Get("/trash", a.handleListTrash)
			authed.Post("/trash/{id}:restore", a.handleRestoreTrash)
			authed.Delete("/trash/{id}", a.handleDeleteTrashItem)
			authed.Delete("/trash", a.handleEmptyTrash)

			authed.Get("/shares", a.handleListShares)
			authed.Post("/shares", a.handleCreateShare)
			authed.Delete("/shares/{id}", a.handleDeleteShare)

			authed.Get("/search", a.handleSearch)

			authed.Group(func(admin chi.Router) {
				admin.Use(a.requireAdmin)
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
			})
		})

		api.Get("/public/shares/{token}", a.handleResolveShare)
		api.Get("/public/shares/{token}:download", a.handleDownloadShare)
	})

	return r
}

func (a *App) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		log.Printf("method=%s path=%s status=%d duration=%s bytes=%d user_id=%s", r.Method, r.URL.Path, ww.Status(), time.Since(start), ww.BytesWritten(), userIDFrom(r))
	})
}

func (a *App) cors(next http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(a.Config.AllowedOrigins))
	for _, origin := range a.Config.AllowedOrigins {
		allowed[origin] = struct{}{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if _, ok := allowed[origin]; ok || len(allowed) == 0 {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}
		}
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Share-Password")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
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
		claims, err := a.Auth.Parse(strings.TrimPrefix(authHeader, "Bearer "))
		if err != nil || claims.Type != "access" {
			httpapi.Error(w, http.StatusUnauthorized, "unauthorized", "Invalid token")
			return
		}
		role, active, err := getUserSession(r.Context(), a.DB, claims.UserID)
		if err != nil {
			if err == sql.ErrNoRows {
				httpapi.Error(w, http.StatusUnauthorized, "unauthorized", "User not found")
				return
			}
			httpapi.Error(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		if !active {
			httpapi.Error(w, http.StatusUnauthorized, "unauthorized", "Account is disabled")
			return
		}
		next.ServeHTTP(w, r.WithContext(withUser(r.Context(), claims.UserID, role)))
	})
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
