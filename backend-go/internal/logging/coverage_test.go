package logging

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"
)

func testTime() time.Time { return time.Unix(0, 0) }

// teeHandler.WithAttrs returns a new tee with attrs applied to each child.
func TestTeeHandler_WithAttrs(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	tee := &teeHandler{
		handlers: []slog.Handler{
			slog.NewJSONHandler(&buf1, nil),
			slog.NewJSONHandler(&buf2, nil),
		},
	}
	withAttrs := tee.WithAttrs([]slog.Attr{slog.String("request_id", "r-1")})
	rec := slog.NewRecord(testTime(), slog.LevelInfo, "msg", 0)
	_ = withAttrs.Handle(context.Background(), rec)
	if !bytes.Contains(buf1.Bytes(), []byte("r-1")) {
		t.Errorf("first child should have logged request_id, got %s", buf1.String())
	}
	if !bytes.Contains(buf2.Bytes(), []byte("r-1")) {
		t.Errorf("second child should have logged request_id, got %s", buf2.String())
	}
}

// teeHandler.WithGroup propagates the group prefix to all children.
func TestTeeHandler_WithGroup(t *testing.T) {
	var buf bytes.Buffer
	tee := &teeHandler{
		handlers: []slog.Handler{slog.NewJSONHandler(&buf, nil)},
	}
	grouped := tee.WithGroup("req")
	rec := slog.NewRecord(testTime(), slog.LevelInfo, "msg", 0)
	rec.AddAttrs(slog.String("path", "/x"))
	_ = grouped.Handle(context.Background(), rec)
	// The grouped output nests "path" under "req".
	if !bytes.Contains(buf.Bytes(), []byte(`"req"`)) {
		t.Errorf("expected group key in output: %s", buf.String())
	}
}

// FromContext on a nil context returns slog.Default() without panic.
func TestFromContext_NilContext(t *testing.T) {
	l := FromContext(nil) //nolint:staticcheck
	if l == nil {
		t.Errorf("FromContext should never return nil")
	}
}

// IntoContext with nil logger returns ctx unchanged.
func TestIntoContext_NilLogger(t *testing.T) {
	type k struct{}
	parent := context.WithValue(context.Background(), k{}, "sentinel")
	got := IntoContext(parent, nil)
	if got != parent {
		t.Errorf("nil logger should return ctx unchanged")
	}
}

// teeHandler.Handle with zero handlers returns nil (no error).
func TestTeeHandler_HandleNoHandlers(t *testing.T) {
	tee := &teeHandler{}
	rec := slog.NewRecord(testTime(), slog.LevelInfo, "msg", 0)
	if err := tee.Handle(context.Background(), rec); err != nil {
		t.Errorf("zero-handlers tee should return nil, got %v", err)
	}
}

// teeHandler.Enabled returns false when no child enables.
func TestTeeHandler_EnabledFalseAcrossAll(t *testing.T) {
	// Set a default handler that's level=Error so Info-level returns false.
	var buf bytes.Buffer
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})
	tee := &teeHandler{handlers: []slog.Handler{h}}
	if tee.Enabled(context.Background(), slog.LevelDebug) {
		t.Errorf("Debug should not be enabled when all children are at Error level")
	}
}

// parseLevel: cover the default branch (returns Info for unknown).
func TestParseLevel_DefaultsToInfo(t *testing.T) {
	if got := parseLevel("invalid"); got != slog.LevelInfo {
		t.Errorf("unknown level should default to Info, got %v", got)
	}
}
