"use client";

// HLS-aware <video> wrapper.
//
// Used by PreviewModal when the file's mime starts with "video/". Behaviour:
//   - Safari plays HLS natively — set src directly to the playlist URL.
//   - Other browsers use hls.js, which buffers segments and feeds the
//     <video> element via MediaSource Extensions.
//   - If the backend returns 202 Accepted (transcode in progress), we
//     show a "Preparing video…" overlay and poll every 5 s until ready.
//   - On any hard failure, fall back to streaming the raw bytes from
//     /files/{id}:download so MP4/WebM still play even without HLS.

import { useEffect, useRef, useState } from "react";
import { tokenStore } from "@/lib/api";

type Props = {
  fileId: string;
  mimeType: string;
  // Direct-stream URL used as the fallback when HLS fails / isn't ready.
  fallbackUrl: string;
};

const BASE = process.env.NEXT_PUBLIC_API_URL ?? "";

export function VideoPlayer({ fileId, mimeType, fallbackUrl }: Props) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const [status, setStatus] = useState<"preparing" | "ready" | "fallback">("preparing");
  const [pollAttempt, setPollAttempt] = useState(0);

  const playlistUrl = `${BASE}/api/v2/files/${encodeURIComponent(fileId)}/hls/playlist.m3u8`;

  // Probe the playlist endpoint. 200 = ready, 202 = wait, anything else = fallback.
  useEffect(() => {
    let cancelled = false;
    const probe = async () => {
      try {
        const token = tokenStore.getAccess();
        const res = await fetch(playlistUrl, {
          method: "HEAD",
          headers: token ? { Authorization: `Bearer ${token}` } : {},
          credentials: "same-origin",
        });
        if (cancelled) return;
        if (res.status === 200) {
          setStatus("ready");
        } else if (res.status === 202) {
          // Try again after Retry-After (default 5 s) — bounded retries so a
          // permanently failing transcode falls back gracefully.
          if (pollAttempt < 24) {
            setTimeout(() => setPollAttempt((n) => n + 1), 5000);
          } else {
            setStatus("fallback");
          }
        } else {
          setStatus("fallback");
        }
      } catch {
        if (!cancelled) setStatus("fallback");
      }
    };
    probe();
    return () => {
      cancelled = true;
    };
  }, [playlistUrl, pollAttempt]);

  // Wire up hls.js once the playlist is ready (non-Safari browsers).
  useEffect(() => {
    if (status !== "ready" || !videoRef.current) return;
    const video = videoRef.current;

    // Safari + iOS handle HLS natively — assigning src is enough.
    if (video.canPlayType("application/vnd.apple.mpegurl")) {
      video.src = playlistUrl;
      return;
    }

    // Lazy-load hls.js so the chunk doesn't ship for pages that never play video.
    let hls: { destroy(): void } | null = null;
    let cancelled = false;
    (async () => {
      const Hls = (await import("hls.js")).default;
      if (cancelled || !Hls.isSupported()) {
        setStatus("fallback");
        return;
      }
      const instance = new Hls({
        // Backend already streams chunks behind Bearer auth via fetch; we
        // tell hls.js to include credentials so it forwards the cookie if
        // we ever switch to cookie auth.
        xhrSetup: (xhr) => {
          const token = tokenStore.getAccess();
          if (token) xhr.setRequestHeader("Authorization", `Bearer ${token}`);
        },
      });
      hls = instance;
      instance.loadSource(playlistUrl);
      instance.attachMedia(video);
    })();
    return () => {
      cancelled = true;
      hls?.destroy();
    };
  }, [status, playlistUrl]);

  if (status === "preparing") {
    return (
      <div className="flex items-center justify-center bg-black text-white aspect-video">
        <div className="text-center space-y-2">
          <div className="animate-pulse">Preparing video…</div>
          <div className="text-xs text-muted-foreground">
            First playback transcodes to HLS. Subsequent plays are instant.
          </div>
        </div>
      </div>
    );
  }

  if (status === "fallback") {
    // Direct stream — works for MP4/WebM; degrades gracefully for codecs the
    // browser can't decode (the user sees a "no playable source" message).
    return (
      <video
        controls
        className="w-full max-h-[80vh] bg-black"
        src={fallbackUrl}
      >
        Your browser cannot play this {mimeType} file directly.
      </video>
    );
  }

  return (
    <video
      ref={videoRef}
      controls
      playsInline
      className="w-full max-h-[80vh] bg-black"
    />
  );
}
