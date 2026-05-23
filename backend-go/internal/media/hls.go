// Package media wraps ffmpeg for HLS adaptive streaming.
//
// HLS pipeline:
//
//   1. GET /files/{id}/hls/playlist.m3u8 arrives. Handler looks up the
//      hls_jobs row.
//        - missing/failed: enqueue a job on q:hls, return 202 + Retry-After.
//        - running:        return 202 + Retry-After.
//        - ready:          serve playlist.m3u8 from the cache directory.
//   2. q:hls worker decrypts the source into a temp dir, runs ffmpeg to
//      produce a single-renditon HLS stream (the design doc has room for
//      multi-bitrate later), moves the result into the cache dir, marks
//      status='ready'.
//   3. GET /files/{id}/hls/<segment.ts> streams a segment from cache.
//
// Multi-bitrate: deliberately deferred. The job table is shaped to support
// it; the transcoder produces a single 720p H.264 stream for v1.
package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// FFmpegPath is the binary location. Production sets this to "ffmpeg" so
// $PATH lookup finds the Debian-installed binary. Tests can override.
var FFmpegPath = "ffmpeg"

// Available probes the install. Returns nil if ffmpeg is callable; an error
// otherwise. The HLS handler uses this on startup so the endpoint can return
// 503 with a useful body when ffmpeg is missing rather than 500'ing later.
func Available(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, FFmpegPath, "-version")
	return cmd.Run()
}

// TranscodeOpts holds the source + destination + format knobs. KeepSource=
// true is the usual mode: callers prepare a decrypted source file in a temp
// path and don't want it removed (the cleanup is owned by the caller).
type TranscodeOpts struct {
	SourcePath string // decrypted input
	OutputDir  string // empty directory; playlist.m3u8 + .ts segments land here
	SegmentSec int    // segment length, default 6
}

// TranscodeToHLS runs ffmpeg with HLS muxer options. Returns the resulting
// playlist path. The output directory MUST be writable and empty; ffmpeg
// will populate it with segNNN.ts files plus playlist.m3u8.
//
// We use:
//
//   -c:v libx264 -preset veryfast -crf 23  →  decent quality, low CPU
//   -c:a aac -b:a 128k                       →  near-universal browser support
//   -hls_time 6 -hls_playlist_type vod        →  6 s segments, full VOD list
//   -hls_segment_filename ...                  →  numbered segments
//
// Errors include the ffmpeg stderr tail to make debugging tractable.
func TranscodeToHLS(ctx context.Context, opts TranscodeOpts) (string, error) {
	if opts.SegmentSec <= 0 {
		opts.SegmentSec = 6
	}
	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return "", err
	}
	playlist := filepath.Join(opts.OutputDir, "playlist.m3u8")
	segPattern := filepath.Join(opts.OutputDir, "seg%03d.ts")

	args := []string{
		"-y",
		"-i", opts.SourcePath,
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "23",
		"-c:a", "aac", "-b:a", "128k",
		"-hls_time", fmt.Sprintf("%d", opts.SegmentSec),
		"-hls_playlist_type", "vod",
		"-hls_segment_filename", segPattern,
		// Force keyframe alignment to segment boundary so seeking works
		// without artefacts. Without this ffmpeg uses scene detection
		// which produces uneven segments.
		"-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", opts.SegmentSec),
		playlist,
	}
	// 10 min hard cap — anything longer than this hosts a streaming video
	// the OS will rotate in/out of the cache; we don't transcode movies.
	tctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(tctx, FFmpegPath, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	// Drain stderr concurrently so the pipe doesn't fill and block ffmpeg.
	// We keep only the last 4 KiB because the full log is megabytes.
	tailCh := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 0, 4096)
		tmp := make([]byte, 1024)
		for {
			n, rerr := stderr.Read(tmp)
			if n > 0 {
				buf = append(buf, tmp[:n]...)
				if len(buf) > 4096 {
					buf = buf[len(buf)-4096:]
				}
			}
			if rerr != nil {
				tailCh <- buf
				return
			}
		}
	}()
	if err := cmd.Wait(); err != nil {
		tail := <-tailCh
		return "", fmt.Errorf("ffmpeg failed: %w; stderr tail: %s", err, string(tail))
	}
	<-tailCh
	if _, err := os.Stat(playlist); err != nil {
		return "", errors.New("ffmpeg succeeded but no playlist produced")
	}
	return playlist, nil
}
