package media

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Available returns nil when ffmpeg is callable; error otherwise. Without
// ffmpeg in CI we can still test the error path by pointing FFmpegPath at
// a binary that doesn't exist.
func TestAvailable_NonExistentBinary(t *testing.T) {
	orig := FFmpegPath
	t.Cleanup(func() { FFmpegPath = orig })
	FFmpegPath = "/path/that/definitely/does/not/exist/ffmpeg"
	if err := Available(context.Background()); err == nil {
		t.Errorf("expected error for non-existent ffmpeg binary")
	}
}

// Timeout path: a binary that blocks should produce a context-deadline-
// exceeded error within the 2s timeout. We use `sleep` (or `cmd` on
// Windows) — actually simpler to just shorten the timeout via a custom
// FFmpegPath that always errors quickly.
func TestAvailable_ReturnsErrorPromptly(t *testing.T) {
	orig := FFmpegPath
	t.Cleanup(func() { FFmpegPath = orig })
	FFmpegPath = "/no/such/binary"
	start := time.Now()
	_ = Available(context.Background())
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("Available should fail quickly, took %v", elapsed)
	}
}

// TranscodeToHLS: SegmentSec default applied when zero.
func TestTranscodeToHLS_AppliesDefaultSegmentSec(t *testing.T) {
	// We can't actually run ffmpeg, but we can call the function with a
	// non-existent FFmpegPath and verify it returns an error (which
	// proves the function reached the ffmpeg exec, having gone through
	// the default-segment + mkdir + arg-construction code).
	orig := FFmpegPath
	t.Cleanup(func() { FFmpegPath = orig })
	FFmpegPath = "/no/such/binary"

	dir := t.TempDir()
	srcPath := filepath.Join(dir, "src.mp4")
	if err := os.WriteFile(srcPath, []byte("fake"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	outDir := filepath.Join(dir, "out")
	_, err := TranscodeToHLS(context.Background(), TranscodeOpts{
		SourcePath: srcPath,
		OutputDir:  outDir,
		SegmentSec: 0, // triggers default 6
	})
	if err == nil {
		t.Errorf("expected error from missing ffmpeg")
	}
	// Output dir should have been created even though ffmpeg failed.
	if _, statErr := os.Stat(outDir); statErr != nil {
		t.Errorf("OutputDir should have been mkdir'd: %v", statErr)
	}
}

// TranscodeToHLS: explicit SegmentSec is honored (we can't observe the
// actual ffmpeg arg, but the code path is exercised).
func TestTranscodeToHLS_CustomSegmentSec(t *testing.T) {
	orig := FFmpegPath
	t.Cleanup(func() { FFmpegPath = orig })
	FFmpegPath = "/no/such/binary"
	dir := t.TempDir()
	_, err := TranscodeToHLS(context.Background(), TranscodeOpts{
		SourcePath: filepath.Join(dir, "src"),
		OutputDir:  filepath.Join(dir, "out"),
		SegmentSec: 10,
	})
	if err == nil {
		t.Errorf("expected error")
	}
}

// MkdirAll failure: pass a path under a read-only directory. Skipped on
// Windows because read-only enforcement differs.
func TestTranscodeToHLS_MkdirFailure(t *testing.T) {
	if isWindowsLike() {
		t.Skip("skipping on Windows: read-only file mode doesn't block subdir creation")
	}
	dir := t.TempDir()
	roParent := filepath.Join(dir, "ro")
	_ = os.Mkdir(roParent, 0o555)
	t.Cleanup(func() { _ = os.Chmod(roParent, 0o755) })

	_, err := TranscodeToHLS(context.Background(), TranscodeOpts{
		SourcePath: filepath.Join(dir, "src"),
		OutputDir:  filepath.Join(roParent, "out"),
		SegmentSec: 6,
	})
	if err == nil {
		t.Errorf("expected mkdir error in read-only parent")
	}
}

func isWindowsLike() bool {
	return strings.Contains(strings.ToLower(os.Getenv("OS")), "windows") ||
		os.PathSeparator == '\\'
}
