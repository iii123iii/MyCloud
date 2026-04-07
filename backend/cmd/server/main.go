package main

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/iii123iii/mycloud/backend/internal/auth"
	"github.com/iii123iii/mycloud/backend/internal/config"
	dbpkg "github.com/iii123iii/mycloud/backend/internal/db"
	"github.com/iii123iii/mycloud/backend/internal/handlers"
	"github.com/iii123iii/mycloud/backend/internal/redisclient"
	"github.com/iii123iii/mycloud/backend/internal/services"
)

// Version is embedded at build time via -ldflags="-X main.Version=..."
var Version = "v0.0.0-dev"

func main() {
	cfg := config.Load()

	// ── Database ──────────────────────────────────────────────────────────────
	db, err := dbpkg.Open(cfg.DBHost, cfg.DBPort, cfg.DBName, cfg.DBUser, cfg.DBPassword)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("db ping: %v", err)
	}

	// ── Redis ─────────────────────────────────────────────────────────────────
	rdb := redisclient.New(cfg.RedisHost, cfg.RedisPort)

	// ── Services ──────────────────────────────────────────────────────────────
	authSvc := services.NewAuthService(rdb, cfg.JWTSecret)
	storageSvc := services.NewStorageService(cfg.StoragePath)

	// ── Handlers ─────────────────────────────────────────────────────────────
	authH := handlers.NewAuthHandler(db, authSvc, storageSvc)
	filesH := handlers.NewFilesHandler(db, storageSvc)
	foldersH := handlers.NewFoldersHandler(db)
	sharesH := handlers.NewSharesHandler(db, storageSvc)
	trashH := handlers.NewTrashHandler(db, storageSvc)
	searchH := handlers.NewSearchHandler(db)
	setupH := handlers.NewSetupHandler(db)
	adminH := handlers.NewAdminHandler(db, storageSvc)
	updatesH := handlers.NewUpdatesHandler(Version, cfg.MycloudGithubRepo)

	// ── Router ────────────────────────────────────────────────────────────────
	r := chi.NewRouter()
	r.Use(corsMiddleware(cfg.AllowedOrigins))

	// Public routes
	r.Post("/api/auth/login", authH.Login)
	r.Post("/api/auth/register", authH.Register)
	r.Post("/api/auth/refresh", authH.Refresh)
	r.Post("/api/auth/logout", authH.Logout)
	r.Get("/api/setup/status", setupH.Status)
	r.Post("/api/setup/complete", setupH.Complete)
	// Public share routes
	r.Get("/api/s/{token}", sharesH.ResolveShare)
	r.Get("/api/s/{token}/download", sharesH.DownloadShare)

	// Protected routes (require JWT)
	jwtMW := auth.Middleware(cfg.JWTSecret)
	r.Group(func(r chi.Router) {
		r.Use(jwtMW)

		// Auth
		r.Get("/api/auth/me", authH.Me)
		r.Post("/api/auth/change-password", authH.ChangePassword)
		r.Delete("/api/auth/delete-account", authH.DeleteAccount)

		// Files
		r.Get("/api/files", filesH.ListFiles)
		r.Post("/api/files/upload", filesH.Upload)
		r.Get("/api/storage/stats", filesH.StorageStats)
		r.Get("/api/files/{id}/download", filesH.Download)
		r.Get("/api/files/{id}/preview", filesH.Preview)
		r.Get("/api/files/{id}", filesH.GetInfo)
		r.Patch("/api/files/{id}", filesH.UpdateFile)
		r.Delete("/api/files/{id}", filesH.DeleteFile)

		// Folders
		r.Get("/api/folders", foldersH.ListFolders)
		r.Post("/api/folders", foldersH.CreateFolder)
		r.Get("/api/folders/{id}", foldersH.GetFolder)
		r.Patch("/api/folders/{id}", foldersH.UpdateFolder)
		r.Delete("/api/folders/{id}", foldersH.DeleteFolder)

		// Shares (authenticated)
		r.Get("/api/shares", sharesH.ListShares)
		r.Post("/api/shares", sharesH.CreateShare)
		r.Delete("/api/shares/{id}", sharesH.DeleteShare)

		// Trash
		r.Get("/api/trash", trashH.ListTrash)
		r.Post("/api/trash/{id}/restore", trashH.RestoreItem)
		r.Delete("/api/trash/empty", trashH.EmptyTrash)
		r.Delete("/api/trash/{id}", trashH.DeleteItem)

		// Search
		r.Get("/api/search", searchH.Search)

		// Admin routes (additionally require admin role)
		r.Group(func(r chi.Router) {
			r.Use(auth.AdminMiddleware)

			r.Get("/api/admin/users", adminH.ListUsers)
			r.Post("/api/admin/users", adminH.CreateUser)
			r.Patch("/api/admin/users/{id}", adminH.UpdateUser)
			r.Delete("/api/admin/users/{id}", adminH.DeleteUser)
			r.Get("/api/admin/stats", adminH.GetStats)
			r.Get("/api/admin/logs", adminH.GetLogs)
			r.Get("/api/admin/settings", adminH.GetSettings)
			r.Put("/api/admin/settings", adminH.PutSettings)
			r.Get("/api/admin/updates/check", updatesH.CheckUpdate)
			r.Post("/api/admin/updates/apply", updatesH.ApplyUpdate)
		})
	})

	log.Printf("MyCloud backend %s starting on :8080", Version)
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// corsMiddleware returns a middleware that handles CORS.
func corsMiddleware(allowedOrigins string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigins)
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Share-Password")
				w.Header().Set("Access-Control-Max-Age", "86400")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
