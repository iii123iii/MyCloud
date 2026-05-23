// Package logging owns the application's slog setup.
//
// The root logger is built once in main and installed as slog.Default so any
// code path that wants to log without threading a logger through ctx can still
// emit structured records. Handlers and workers should prefer FromContext to
// pick up per-request fields (request_id, user_id) attached by middleware.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Config controls how the root logger is built. Zero values are reasonable
// defaults for local dev (text format, info level, no file sink).
type Config struct {
	// Format is "json" or "text". Anything else falls back to text.
	Format string
	// Level is "debug", "info", "warn", "error". Anything else falls back to info.
	Level string
	// FilePath, when non-empty, also writes records to this file in JSON format
	// regardless of console Format. The directory is created if missing.
	// Errors creating the file are swallowed — stdout is always the primary sink.
	FilePath string
}

// New builds the root logger per cfg and returns it along with a Close
// function that releases the file sink (no-op when FilePath is empty).
// Callers should defer the returned Close at process shutdown so tests and
// graceful exits don't leak open file descriptors.
func New(cfg Config) (*slog.Logger, func()) {
	level := parseLevel(cfg.Level)
	opts := &slog.HandlerOptions{Level: level}

	consoleWriter := io.Writer(os.Stdout)

	var fileWriter io.Writer
	var fileCloser io.Closer
	if cfg.FilePath != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.FilePath), 0o755); err == nil {
			// O_APPEND so concurrent writes from goroutines don't tear; the OS
			// guarantees atomic writes up to PIPE_BUF (4 KiB on Linux) which is
			// more than enough for a single JSON record.
			f, err := os.OpenFile(cfg.FilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
			if err == nil {
				fileWriter = f
				fileCloser = f
			}
		}
	}

	var consoleHandler slog.Handler
	if strings.EqualFold(cfg.Format, "json") {
		consoleHandler = slog.NewJSONHandler(consoleWriter, opts)
	} else {
		consoleHandler = slog.NewTextHandler(consoleWriter, opts)
	}

	closer := func() {
		if fileCloser != nil {
			_ = fileCloser.Close()
		}
	}

	if fileWriter == nil {
		return slog.New(consoleHandler), closer
	}

	fileHandler := slog.NewJSONHandler(fileWriter, opts)
	return slog.New(&teeHandler{handlers: []slog.Handler{consoleHandler, fileHandler}}), closer
}

// parseLevel maps strings to slog.Level. Unknown values fall back to info.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// teeHandler fans out records to multiple underlying handlers. Used so a single
// logger writes to both stdout (captured by Docker) and a JSONL file (read by
// the admin log viewer). Errors from individual handlers are joined and
// returned — slog itself swallows handler errors so callers rarely see them,
// but Handle's contract requires the error path.
type teeHandler struct {
	handlers []slog.Handler
}

func (t *teeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range t.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (t *teeHandler) Handle(ctx context.Context, r slog.Record) error {
	var firstErr error
	for _, h := range t.handlers {
		if err := h.Handle(ctx, r.Clone()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (t *teeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clones := make([]slog.Handler, len(t.handlers))
	for i, h := range t.handlers {
		clones[i] = h.WithAttrs(attrs)
	}
	return &teeHandler{handlers: clones}
}

func (t *teeHandler) WithGroup(name string) slog.Handler {
	clones := make([]slog.Handler, len(t.handlers))
	for i, h := range t.handlers {
		clones[i] = h.WithGroup(name)
	}
	return &teeHandler{handlers: clones}
}

// ctxKey is the unexported type used to stash the request logger in a context.
// A typed empty struct prevents collisions with any string-keyed context value.
type ctxKey struct{}

// IntoContext returns a child context carrying logger. Middleware uses this to
// attach per-request fields (request_id, user_id, path, method) so deeper
// handlers can log without re-deriving them.
func IntoContext(ctx context.Context, logger *slog.Logger) context.Context {
	if logger == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, logger)
}

// FromContext returns the logger attached to ctx, or slog.Default() when none
// was attached. Always returns a non-nil logger so callers can chain `.Info(...)`
// without a guard.
func FromContext(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return slog.Default()
	}
	if v, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && v != nil {
		return v
	}
	return slog.Default()
}

// Note: the file sink is intentionally never closed. The OS reclaims the FD
// on process exit; we accept up to one buffered record loss in exchange for a
// simpler API. A future Close() hook can wrap os.OpenFile's result if graceful
// shutdown flushing becomes a hard requirement.
