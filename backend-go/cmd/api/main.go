package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mycloud/backend-go/internal/app"
	"mycloud/backend-go/internal/config"
	"mycloud/backend-go/internal/logging"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		// Logger isn't built yet — config errors go to stderr verbatim.
		// We can't use slog.Default until logging.New runs, but the default
		// handler still writes JSON-ish records to stderr if invoked, so this
		// covers either path.
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	// Install the root logger. Every subsequent slog.X call inherits this
	// configuration; ctx-attached loggers in handlers/workers add per-request
	// fields on top. closeLog releases the JSONL file sink on shutdown.
	logger, closeLog := logging.New(logging.Config{
		Format:   cfg.LogFormat,
		Level:    cfg.LogLevel,
		FilePath: cfg.LogFilePath,
	})
	slog.SetDefault(logger)
	defer closeLog()

	srv, cleanup, err := app.New(cfg)
	if err != nil {
		slog.Error("create app", "error", err)
		os.Exit(1)
	}
	defer cleanup()

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	slog.Info("server starting",
		"listen", cfg.ListenAddr,
		"version", cfg.Version,
		"log_format", cfg.LogFormat,
		"log_level", cfg.LogLevel,
	)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("listen", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}
