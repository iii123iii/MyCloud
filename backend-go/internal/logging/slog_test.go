package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNewWritesToFileSink builds a logger pointed at a temp file and verifies
// the file sink receives a JSON record. Stdout is not asserted because the
// test framework hijacks it; the tee handler's behaviour is covered by the
// dedicated test below.
func TestNewWritesToFileSink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.jsonl")

	logger, closeLog := New(Config{Format: "json", Level: "info", FilePath: path})
	logger.Info("hello", "k", "v")
	// Close before reading + before TempDir cleanup so the file isn't held
	// open (a no-op on Linux/macOS but required on Windows where open files
	// can't be removed).
	closeLog()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected log file to contain at least one record")
	}
	// Validate the record is well-formed JSON with the expected message + key.
	var record map[string]any
	line := strings.SplitN(string(data), "\n", 2)[0]
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		t.Fatalf("parse log line as JSON: %v\nline: %s", err, line)
	}
	if record["msg"] != "hello" {
		t.Errorf("msg = %v, want hello", record["msg"])
	}
	if record["k"] != "v" {
		t.Errorf("k = %v, want v", record["k"])
	}
}

// TestTeeHandlerFansOut feeds a logger that writes to two in-memory buffers
// and asserts each receives an independent copy of the record. Catches
// regressions where record.Clone() is dropped from Handle.
func TestTeeHandlerFansOut(t *testing.T) {
	var a, b bytes.Buffer
	tee := &teeHandler{
		handlers: []slog.Handler{
			slog.NewJSONHandler(&a, nil),
			slog.NewJSONHandler(&b, nil),
		},
	}
	logger := slog.New(tee)
	logger.Info("ping", "n", 7)

	for name, buf := range map[string]*bytes.Buffer{"a": &a, "b": &b} {
		var rec map[string]any
		if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
			t.Fatalf("handler %s: parse JSON: %v", name, err)
		}
		if rec["msg"] != "ping" {
			t.Errorf("handler %s: msg = %v, want ping", name, rec["msg"])
		}
		// JSON unmarshals numbers as float64.
		if rec["n"] != float64(7) {
			t.Errorf("handler %s: n = %v, want 7", name, rec["n"])
		}
	}
}

// TestFromContextFallsBackToDefault confirms callers can safely use
// FromContext on a bare context without nil-checks.
func TestFromContextFallsBackToDefault(t *testing.T) {
	got := FromContext(context.Background())
	if got == nil {
		t.Fatal("FromContext returned nil for empty ctx")
	}
}

// TestIntoContextRoundTrip exercises the basic put/get path.
func TestIntoContextRoundTrip(t *testing.T) {
	want := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	ctx := IntoContext(context.Background(), want)
	got := FromContext(ctx)
	if got != want {
		t.Fatalf("FromContext returned a different logger than IntoContext stored")
	}
}

// TestParseLevelDefaults sanity-checks the level mapper.
func TestParseLevelDefaults(t *testing.T) {
	cases := map[string]slog.Level{
		"":        slog.LevelInfo,
		"bogus":   slog.LevelInfo,
		"debug":   slog.LevelDebug,
		"INFO":    slog.LevelInfo,
		"warn":    slog.LevelWarn,
		"WARNING": slog.LevelWarn,
		"error":   slog.LevelError,
	}
	for in, want := range cases {
		if got := parseLevel(in); got != want {
			t.Errorf("parseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}
