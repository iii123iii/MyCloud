"use client";

// Video player with two strategies, direct-first:
//
//   1. Presign a short-lived, Range-capable URL and let the browser stream it
//      natively (<video src>). Instant start + seeking for web-native codecs
//      (MP4/H.264, WebM/VP9) at any size — no full-file download, no blob.
//   2. If native playback fails (a container/codec the browser can't decode),
//      fall back to HLS: the backend transcodes on demand and we play through
//      hls.js (or native HLS on Safari).
//
// A presigned URL is short-lived; if it expires mid-playback we re-presign once
// — restoring the playback position — before giving up on the direct path.

import { useCallback, useEffect, useRef, useState } from "react";
import { files as filesApi, tokenStore } from "@/lib/api";

type Props = {
  fileId: string;
  mimeType: string;
};

const BASE = process.env.NEXT_PUBLIC_API_URL ?? "";
const PRESIGN_TTL_SECONDS = 3600; // backend clamps to this max

export function VideoPlayer({ fileId, mimeType }: Props) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const [src, setSrc] = useState<string | null>(null);
  const [useHls, setUseHls] = useState(false);
  const [hlsStatus, setHlsStatus] = useState<"preparing" | "ready" | "unplayable">("preparing");
  const [pollAttempt, setPollAttempt] = useState(0);
  const directRetries = useRef(0);

  const playlistUrl = `${BASE}/api/v2/files/${encodeURIComponent(fileId)}/hls/playlist.m3u8`;

  const getStreamUrl = useCallback(
    () => filesApi.presign(fileId, PRESIGN_TTL_SECONDS).then((r) => r.url),
    [fileId],
  );

  // Primary path: presign a direct streaming URL for native playback.
  useEffect(() => {
    let cancelled = false;
    getStreamUrl()
      .then((url) => {
        if (!cancelled) setSrc(url);
      })
      .catch(() => {
        if (!cancelled) {
          setUseHls(true);
          setHlsStatus("preparing");
        }
      });
    return () => {
      cancelled = true;
    };
  }, [getStreamUrl]);

  // Native playback failed. Re-presign once (covers an expired token, keeping
  // the current position); if it still fails, switch to HLS transcoding.
  const handleDirectError = useCallback(() => {
    const video = videoRef.current;
    if (directRetries.current < 1 && video) {
      directRetries.current += 1;
      const resumeAt = video.currentTime || 0;
      getStreamUrl()
        .then((url) => {
          setSrc(url);
          const restore = () => {
            try {
              if (resumeAt > 0) video.currentTime = resumeAt;
            } catch {
              /* seeking momentarily unavailable; ignore */
            }
            video.removeEventListener("loadedmetadata", restore);
          };
          video.addEventListener("loadedmetadata", restore);
        })
        .catch(() => {
          setUseHls(true);
          setHlsStatus("preparing");
        });
      return;
    }
    setUseHls(true);
    setHlsStatus("preparing");
  }, [getStreamUrl]);

  // HLS fallback: probe the playlist (200 ready / 202 transcoding / else stop).
  useEffect(() => {
    if (!useHls) return;
    let cancelled = false;
    (async () => {
      try {
        const token = tokenStore.getAccess();
        const res = await fetch(playlistUrl, {
          method: "HEAD",
          headers: token ? { Authorization: `Bearer ${token}` } : {},
          credentials: "same-origin",
        });
        if (cancelled) return;
        if (res.status === 200) {
          setHlsStatus("ready");
        } else if (res.status === 202) {
          if (pollAttempt < 24) setTimeout(() => setPollAttempt((n) => n + 1), 5000);
          else setHlsStatus("unplayable");
        } else {
          setHlsStatus("unplayable");
        }
      } catch {
        if (!cancelled) setHlsStatus("unplayable");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [useHls, playlistUrl, pollAttempt]);

  // Attach hls.js (or native HLS) once the playlist is ready.
  useEffect(() => {
    if (!useHls || hlsStatus !== "ready" || !videoRef.current) return;
    const video = videoRef.current;

    // Safari + iOS play HLS natively — assigning src is enough.
    if (video.canPlayType("application/vnd.apple.mpegurl")) {
      video.src = playlistUrl;
      return;
    }

    let hls: { destroy(): void } | null = null;
    let cancelled = false;
    (async () => {
      const Hls = (await import("hls.js")).default;
      if (cancelled || !Hls.isSupported()) {
        setHlsStatus("unplayable");
        return;
      }
      const instance = new Hls({
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
  }, [useHls, hlsStatus, playlistUrl]);

  if (useHls) {
    if (hlsStatus === "preparing") {
      return (
        <div className="flex items-center justify-center bg-black text-white aspect-video">
          <div className="text-center space-y-2">
            <div className="animate-pulse">Preparing video…</div>
            <div className="text-xs text-muted-foreground">
              This format needs transcoding for in-browser playback. The first play prepares it; later plays are instant.
            </div>
          </div>
        </div>
      );
    }
    if (hlsStatus === "unplayable") {
      return (
        <div className="flex items-center justify-center bg-black text-white aspect-video">
          <div className="text-center text-sm px-6">
            This {mimeType} video cannot be played in the browser. Download it to watch in a desktop player.
          </div>
        </div>
      );
    }
    // ready → hls.js / native HLS drives this element.
    return <video ref={videoRef} controls playsInline className="w-full max-h-[80vh] bg-black" />;
  }

  if (!src) {
    return (
      <div className="flex items-center justify-center bg-black text-white aspect-video">
        <div className="animate-pulse">Loading…</div>
      </div>
    );
  }

  return (
    <video
      ref={videoRef}
      controls
      playsInline
      className="w-full max-h-[80vh] bg-black"
      src={src}
      onError={handleDirectError}
    >
      Your browser cannot play this {mimeType} file.
    </video>
  );
}
