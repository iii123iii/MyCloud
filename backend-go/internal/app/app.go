package app

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"mycloud/backend-go/internal/auth"
	"mycloud/backend-go/internal/config"
	"mycloud/backend-go/internal/db"
	"mycloud/backend-go/internal/redisc"
)

type App struct {
	Config   config.Config
	DB       *sql.DB
	Redis    *redis.Client
	Auth     *auth.Manager
	Router   chi.Router
	Client   *http.Client
	StartUTC time.Time

	// updateMu guards the fields below, which track activity-log writes for updates.
	updateMu              sync.Mutex
	updateStarted         bool   // true once an apply was triggered in this session
	updateCompletionLogged bool  // true once a succeeded/failed log entry was written
}

func New(cfg config.Config) (http.Handler, func(), error) {
	sqlDB, err := db.Open(cfg.DBDSN)
	if err != nil {
		return nil, nil, err
	}
	redisClient := redisc.New(cfg.RedisAddr, cfg.RedisPassword)
	_ = redisc.Ping(context.Background(), redisClient)

	_ = os.MkdirAll(filepath.Join(cfg.StoragePath, "tmp"), 0o755)

	app := &App{
		Config:   cfg,
		DB:       sqlDB,
		Redis:    redisClient,
		Auth:     auth.NewManager(cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL, redisClient),
		Client:   &http.Client{Timeout: 20 * time.Second},
		StartUTC: time.Now().UTC(),
	}
	router := app.routes()
	app.Router = router

	cleanup := func() {
		_ = sqlDB.Close()
		_ = redisClient.Close()
	}
	return router, cleanup, nil
}
