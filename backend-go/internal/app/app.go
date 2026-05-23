package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"mycloud/backend-go/internal/auth"
	"mycloud/backend-go/internal/coalesce"
	"mycloud/backend-go/internal/config"
	"mycloud/backend-go/internal/db"
	"mycloud/backend-go/internal/db/cluster"
	"mycloud/backend-go/internal/db/stmtcache"
	"mycloud/backend-go/internal/health"
	"mycloud/backend-go/internal/metrics"
	"mycloud/backend-go/internal/redisc"
	"mycloud/backend-go/internal/wsHub"
)

type App struct {
	Config config.Config
	DB     *sql.DB
	// Stmts memoises prepared statements for hot, static queries (user session
	// lookup on every authenticated request, file/folder fetch by ID, share
	// resolution by token). Dynamic SQL (variable IN-clauses, optional WHERE
	// fragments) must keep using DB.QueryContext directly so we don't blow up
	// the cache with one-shot statements.
	Stmts      *stmtcache.Cache
	// Cluster routes reads to a replica when DB_REPLICA_DSN is set; falls
	// back to the primary otherwise. Handlers should prefer Cluster.Read/
	// Write over DB directly for new code (migration is gradual).
	Cluster    *cluster.Cluster
	Metrics    *metrics.Metrics
	// SearchSF coalesces in-flight identical search queries. Bursts from
	// "user types fast" debounced badly or multiple tabs open the same
	// search land on the same query without N×DB hits.
	SearchSF *coalesce.Group
	// FoldersSF coalesces folder listings — heavy hitters when users
	// switch tabs that all default-render the same root folder.
	FoldersSF *coalesce.Group
	// Hub is the WebSocket fan-out for real-time events. Handlers publish
	// activity-log writes and slow-query events through Hub.Broker().
	Hub *wsHub.Hub
	// Heartbeats track worker liveness for /readyz. Maintenance ticker and
	// the metrics sampler refresh their respective entries.
	Heartbeats map[string]*health.WorkerHeartbeat
	Redis      *redis.Client
	Auth       *auth.Manager
	Router     chi.Router
	Client     *http.Client
	TusHandler http.Handler
	StartUTC   time.Time

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
	if err := db.Migrate(sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, nil, err
	}
	redisClient := redisc.New(cfg.RedisAddr, cfg.RedisPassword)
	_ = redisc.Ping(context.Background(), redisClient)

	// Optional read replica. Build a second pool when DSN is set.
	var replicaDB *sql.DB
	if cfg.DBReplicaDSN != "" {
		var rErr error
		replicaDB, rErr = db.Open(cfg.DBReplicaDSN)
		if rErr != nil {
			// Replica failure shouldn't block startup — we degrade to
			// primary-only and log it.
			replicaDB = nil
		}
	}

	// Surface the mkdir failure at boot rather than at the first upload —
	// silently ignoring this previously turned a permission misconfig into
	// a mystifying upload error hours later.
	if err := os.MkdirAll(filepath.Join(cfg.StoragePath, "tmp"), 0o755); err != nil {
		_ = sqlDB.Close()
		return nil, nil, fmt.Errorf("create storage tmp dir: %w", err)
	}

	app := &App{
		Config:    cfg,
		DB:        sqlDB,
		Stmts:     stmtcache.New(sqlDB),
		Cluster:   cluster.New(sqlDB, replicaDB, redisClient),
		Metrics:   metrics.New(),
		SearchSF:  coalesce.New(),
		FoldersSF: coalesce.New(),
		Heartbeats: map[string]*health.WorkerHeartbeat{
			"maintenance": {},
			"metrics":     {},
			"thumb":       {},
			"extract":     {},
			"hls":         {},
		},
		Redis:    redisClient,
		Auth:     auth.NewManager(cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL, redisClient),
		Client:   &http.Client{Timeout: 20 * time.Second},
		StartUTC: time.Now().UTC(),
	}
	// Install slow-query hook so stmt-cached queries that exceed the
	// threshold land in slow_queries and the admin live tail.
	app.Stmts.SetSlowHook(app.recordSlowQuery)

	// Hub depends on app.Auth, so build it after the App skeleton exists.
	app.Hub = wsHub.New(wsHub.NewInMemoryBroker())
	app.Hub.AuthorizeToken = func(token string) (string, error) {
		claims, err := app.Auth.Parse(token)
		if err != nil || claims.Type != "access" {
			return "", err
		}
		return claims.UserID, nil
	}
	// Mirror connection deltas into the Prometheus gauge so the metrics
	// dashboard reflects live socket counts.
	app.Hub.OnConnect = func() {
		if app.Metrics != nil {
			app.Metrics.WSConnections.Inc()
		}
	}
	app.Hub.OnDisconnect = func() {
		if app.Metrics != nil {
			app.Metrics.WSConnections.Dec()
		}
	}

	// Install the activity publisher so writeActivity fans out to WS
	// topics. We publish two topics per row:
	//   - "user:<id>"        — appears in the user's sidebar feed
	//   - "folder:<id>"      — appears in the per-folder activity panel
	// Errors are swallowed: the DB write already happened, the WS push is a
	// best-effort hint to refresh.
	publishActivity = func(ctx context.Context, _ querier, userID *string, action, resourceType, resourceID string, details []byte) {
		if app.Hub == nil {
			return
		}
		envelope := wsHub.Envelope{
			Type: "event",
			Data: map[string]any{
				"action":        action,
				"resource_type": resourceType,
				"resource_id":   resourceID,
				"details_raw":   string(details),
			},
		}
		if userID != nil && *userID != "" {
			env := envelope
			env.Topic = "user:" + *userID
			_ = app.Hub.Broker().Publish(ctx, env.Topic, env)
		}
		if resourceType == "folder" && resourceID != "" {
			env := envelope
			env.Topic = "folder:" + resourceID
			_ = app.Hub.Broker().Publish(ctx, env.Topic, env)
		}
		// For file actions, walk up to the file's folder to fan out on
		// the folder topic too. Best-effort — the DB lookup may fail
		// (e.g., file already hard-deleted); a failure means the
		// per-folder feed misses one frame, not a critical issue.
		if resourceType == "file" && resourceID != "" && app.DB != nil {
			var folder sql.NullString
			_ = app.DB.QueryRowContext(ctx,
				"SELECT folder_id FROM files WHERE id = ?", resourceID,
			).Scan(&folder)
			if folder.Valid {
				env := envelope
				env.Topic = "folder:" + folder.String
				_ = app.Hub.Broker().Publish(ctx, env.Topic, env)
			}
		}
	}
	tusHandler, err := app.mountTus("/api/v2/files/tus/")
	if err != nil {
		_ = sqlDB.Close()
		_ = redisClient.Close()
		return nil, nil, err
	}
	app.TusHandler = tusHandler
	router := app.routes()
	app.Router = router

	workersCtx, stopWorkers := context.WithCancel(context.Background())
	app.startWorkers(workersCtx)

	cleanup := func() {
		stopWorkers()
		// Close prepared statements before the DB so the driver releases
		// stmt-handle state cleanly. DB.Close() would invalidate them anyway,
		// but doing it explicitly avoids the "stmt closed by db" warning some
		// drivers emit at shutdown.
		_ = app.Stmts.Close()
		_ = sqlDB.Close()
		if replicaDB != nil {
			_ = replicaDB.Close()
		}
		_ = redisClient.Close()
	}
	return router, cleanup, nil
}
