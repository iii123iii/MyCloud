"use client";

// Image lightbox.
//
// Renders a modal that:
//   - shows one image at a time with crossfade between images
//   - arrow keys navigate previous/next
//   - Escape closes
//   - scroll wheel zooms (no modifier needed)
//   - double-click toggles fit-to-screen vs. fill (2x)
//   - pinch-zoom on touch devices (browser default), single-finger swipe paginates
//   - chrome (controls + filmstrip) auto-hides after 2s of mouse idle
//   - filmstrip thumbnail row highlights the current image
//   - shows file name, "X of Y" top-left and a Download button top-right
//
// Parents pass a list of image entries (already filtered to image MIME)
// plus a starting index. The component owns its own visible-image cursor.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  X,
  ChevronLeft,
  ChevronRight,
  ZoomIn,
  ZoomOut,
  RotateCcw,
  Download,
  Loader2,
} from "lucide-react";

import { Dialog, DialogContent } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

type Image = {
  id: string;
  name: string;
  // Direct streaming URL (e.g. /api/v2/files/{id}:preview or a presigned URL).
  src: string;
};

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  images: Image[];
  startIndex: number;
};

const MIN_ZOOM = 0.25;
const MAX_ZOOM = 8;
const FILL_ZOOM = 2;
const IDLE_MS = 2000;
// Anything past this is treated as a horizontal swipe, not a tap.
const SWIPE_THRESHOLD_PX = 60;

export function Lightbox({ open, onOpenChange, images, startIndex }: Props) {
  const [index, setIndex] = useState(startIndex);
  const [zoom, setZoom] = useState(1);
  // Tracks whether the current image's <img> has finished loading. We flip
  // this back to false on every index/src change so the spinner shows while
  // the browser fetches the next preview.
  const [loaded, setLoaded] = useState(false);
  const [chromeVisible, setChromeVisible] = useState(true);

  const idleTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const filmstripRef = useRef<HTMLDivElement | null>(null);
  // Touch state for single-finger horizontal swipe paging. Pinch (2 fingers)
  // falls through to the browser's native zoom.
  const touchStartRef = useRef<{ x: number; y: number; id: number } | null>(
    null,
  );

  useEffect(() => {
    if (open) {
      // Reset transient view state whenever the lightbox is (re)opened with
      // a new starting image. React 19's hook lint rule flags setState in an
      // effect, but the alternative (keying the component on `open`) would
      // throw away the keydown / idle listeners on every open.
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setIndex(startIndex);
      setZoom(1);
      setLoaded(false);
      setChromeVisible(true);
    }
  }, [open, startIndex]);

  const goPrev = useCallback(() => {
    setIndex((i) => (i - 1 + images.length) % images.length);
    setZoom(1);
    setLoaded(false);
  }, [images.length]);

  const goNext = useCallback(() => {
    setIndex((i) => (i + 1) % images.length);
    setZoom(1);
    setLoaded(false);
  }, [images.length]);

  // Auto-hide chrome after IDLE_MS of no mouse movement. Resets whenever the
  // user wiggles the mouse. We always show chrome when zoomed != 1 so the
  // user has a way to reset.
  const bumpIdle = useCallback(() => {
    setChromeVisible(true);
    if (idleTimerRef.current) clearTimeout(idleTimerRef.current);
    idleTimerRef.current = setTimeout(() => {
      setChromeVisible(false);
    }, IDLE_MS);
  }, []);

  useEffect(() => {
    if (!open) return;
    // bumpIdle() flips chromeVisible + starts a timer; running it once on
    // open is intentional so the chrome is visible the moment we appear.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    bumpIdle();
    return () => {
      if (idleTimerRef.current) clearTimeout(idleTimerRef.current);
    };
  }, [open, bumpIdle]);

  // Keyboard navigation. We listen at the document level because Dialog's
  // own focus trap consumes most events.
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "ArrowLeft") goPrev();
      else if (e.key === "ArrowRight") goNext();
      else if (e.key === "+" || e.key === "=")
        setZoom((z) => Math.min(MAX_ZOOM, z * 1.2));
      else if (e.key === "-") setZoom((z) => Math.max(MIN_ZOOM, z / 1.2));
      else if (e.key === "0") setZoom(1);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open, goPrev, goNext]);

  // Keep the active filmstrip thumbnail visible. We scroll the strip
  // horizontally so the highlighted item is centered (best-effort — the
  // browser will clamp at the strip edges).
  useEffect(() => {
    if (!open) return;
    const strip = filmstripRef.current;
    if (!strip) return;
    const el = strip.querySelector<HTMLElement>(`[data-thumb-index="${index}"]`);
    if (el) {
      el.scrollIntoView({
        behavior: "smooth",
        block: "nearest",
        inline: "center",
      });
    }
  }, [open, index]);

  const current = useMemo(() => images[index], [images, index]);

  if (!open || images.length === 0 || !current) return null;

  const handleDownload = () => {
    // Anchor-with-download attribute respects same-origin; for cross-origin
    // presigned URLs the browser falls back to navigation, which is still
    // the correct behavior (the resource is meant to be saved).
    const a = document.createElement("a");
    a.href = current.src;
    a.download = current.name;
    a.rel = "noopener";
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
  };

  const handleWheel: React.WheelEventHandler<HTMLDivElement> = (e) => {
    // Scroll wheel always zooms — no shift required. We can't preventDefault
    // here because React attaches the wheel listener passively; instead we
    // rely on the fact that the inner image isn't scrollable and the dialog
    // overlay swallows page scroll.
    setZoom((z) =>
      Math.max(MIN_ZOOM, Math.min(MAX_ZOOM, z * (e.deltaY > 0 ? 0.9 : 1.1))),
    );
  };

  const handleDoubleClick = () => {
    // Toggle between fit (1x) and fill (2x). Anything in between snaps to fill.
    setZoom((z) => (z === 1 ? FILL_ZOOM : 1));
  };

  const handleTouchStart: React.TouchEventHandler<HTMLDivElement> = (e) => {
    if (e.touches.length !== 1) {
      // Pinch — defer to browser native zoom, cancel any pending swipe.
      touchStartRef.current = null;
      return;
    }
    const t = e.touches[0];
    touchStartRef.current = { x: t.clientX, y: t.clientY, id: t.identifier };
  };

  const handleTouchEnd: React.TouchEventHandler<HTMLDivElement> = (e) => {
    const start = touchStartRef.current;
    touchStartRef.current = null;
    if (!start) return;
    // Find the changed touch matching our recorded identifier. React's
    // synthetic Touch type differs structurally from the DOM Touch (no
    // force/radiusX/radiusY/rotationAngle), so we use React.Touch here.
    let end: React.Touch | null = null;
    for (let i = 0; i < e.changedTouches.length; i++) {
      const t = e.changedTouches[i];
      if (t.identifier === start.id) {
        end = t;
        break;
      }
    }
    if (!end) return;
    const dx = end.clientX - start.x;
    const dy = end.clientY - start.y;
    // Only treat predominantly-horizontal motion past threshold as a swipe;
    // ignore when the user is zoomed in (they probably want to pan).
    if (zoom !== 1) return;
    if (Math.abs(dx) > SWIPE_THRESHOLD_PX && Math.abs(dx) > Math.abs(dy)) {
      if (dx < 0) goNext();
      else goPrev();
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        className="w-screen h-screen max-w-none max-h-none p-0 bg-black/95 border-0 rounded-none data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95"
        data-no-hotkeys="true"
        onMouseMove={bumpIdle}
      >
        <div
          className="relative w-full h-screen flex items-center justify-center overflow-hidden touch-pan-y"
          onWheel={handleWheel}
          onDoubleClick={handleDoubleClick}
          onTouchStart={handleTouchStart}
          onTouchEnd={handleTouchEnd}
        >
          {/* The image itself. key={current.id} forces React to swap the
              <img> element on navigation so onLoad fires reliably, and so
              the crossfade animation re-plays. */}
          <img
            key={current.id}
            src={current.src}
            alt={current.name}
            onLoad={() => setLoaded(true)}
            onError={() => setLoaded(true)}
            className={cn(
              "max-w-full max-h-full select-none transition-opacity duration-300 ease-out",
              loaded ? "opacity-100" : "opacity-0",
            )}
            style={{
              transform: `scale(${zoom})`,
              transition: "transform 120ms ease-out, opacity 300ms ease-out",
              cursor: zoom === 1 ? "zoom-in" : "zoom-out",
            }}
            draggable={false}
          />

          {/* Loading spinner — only shown while the current image is still
              decoding. Positioned absolutely so it sits on top of the (still
              transparent) <img>. */}
          {!loaded && (
            <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
              <Loader2 className="size-10 text-white/80 animate-spin" />
            </div>
          )}

          {/* All chrome (top bar, side arrows, zoom controls, filmstrip)
              shares one container that fades together based on mouse idle. */}
          <div
            className={cn(
              "absolute inset-0 pointer-events-none transition-opacity duration-300 ease-out",
              chromeVisible || zoom !== 1
                ? "opacity-100"
                : "opacity-0",
            )}
          >
            {/* Top-left: filename + counter */}
            <div className="pointer-events-auto absolute top-3 left-3 max-w-[60vw] px-3 py-1.5 bg-black/55 backdrop-blur-sm rounded-full text-white text-xs flex items-center gap-2">
              <span className="opacity-70 tabular-nums">
                {index + 1} of {images.length}
              </span>
              <span className="opacity-30">·</span>
              <span className="truncate font-medium">{current.name}</span>
            </div>

            {/* Top-right: download + zoom + close */}
            <div className="pointer-events-auto absolute top-3 right-3 flex items-center gap-1 bg-black/55 backdrop-blur-sm rounded-full px-1.5 py-1">
              <Button
                size="icon-sm"
                variant="ghost"
                onClick={handleDownload}
                aria-label="Download"
                className="text-white hover:bg-white/15"
              >
                <Download className="size-4" />
              </Button>
              <span className="w-px h-4 bg-white/20 mx-0.5" />
              <Button
                size="icon-sm"
                variant="ghost"
                onClick={() => setZoom((z) => Math.max(MIN_ZOOM, z / 1.2))}
                aria-label="Zoom out"
                className="text-white hover:bg-white/15"
              >
                <ZoomOut className="size-4" />
              </Button>
              <Button
                size="icon-sm"
                variant="ghost"
                onClick={() => setZoom(1)}
                aria-label="Reset zoom"
                className="text-white hover:bg-white/15"
              >
                <RotateCcw className="size-4" />
              </Button>
              <Button
                size="icon-sm"
                variant="ghost"
                onClick={() => setZoom((z) => Math.min(MAX_ZOOM, z * 1.2))}
                aria-label="Zoom in"
                className="text-white hover:bg-white/15"
              >
                <ZoomIn className="size-4" />
              </Button>
              <span className="w-px h-4 bg-white/20 mx-0.5" />
              <Button
                size="icon-sm"
                variant="ghost"
                onClick={() => onOpenChange(false)}
                aria-label="Close"
                className="text-white hover:bg-white/15"
              >
                <X className="size-4" />
              </Button>
            </div>

            {/* Side arrows. Hidden when there's only one image. */}
            {images.length > 1 && (
              <>
                <button
                  type="button"
                  onClick={goPrev}
                  className="pointer-events-auto absolute left-3 top-1/2 -translate-y-1/2 p-2.5 bg-black/55 hover:bg-black/75 backdrop-blur-sm rounded-full text-white transition-colors"
                  aria-label="Previous"
                >
                  <ChevronLeft className="size-6" />
                </button>
                <button
                  type="button"
                  onClick={goNext}
                  className="pointer-events-auto absolute right-3 top-1/2 -translate-y-1/2 p-2.5 bg-black/55 hover:bg-black/75 backdrop-blur-sm rounded-full text-white transition-colors"
                  aria-label="Next"
                >
                  <ChevronRight className="size-6" />
                </button>
              </>
            )}

            {/* Bottom filmstrip. We render an <img> per entry — the same
                `src` the lightbox uses, since callers already pass a
                thumbnail-suitable URL (preview blob or signed URL). The row
                scrolls horizontally and the active item is highlighted. */}
            {images.length > 1 && (
              <div
                ref={filmstripRef}
                className="pointer-events-auto absolute bottom-3 left-1/2 -translate-x-1/2 max-w-[92vw] flex items-center gap-1.5 px-2 py-1.5 bg-black/55 backdrop-blur-sm rounded-xl overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
              >
                {images.map((img, i) => (
                  <button
                    key={img.id}
                    type="button"
                    data-thumb-index={i}
                    onClick={() => {
                      if (i !== index) {
                        setIndex(i);
                        setZoom(1);
                        setLoaded(false);
                      }
                    }}
                    aria-label={`Show image ${i + 1}`}
                    aria-current={i === index ? "true" : undefined}
                    className={cn(
                      "relative shrink-0 size-12 rounded-md overflow-hidden border-2 transition-all",
                      i === index
                        ? "border-white scale-105"
                        : "border-transparent opacity-60 hover:opacity-100",
                    )}
                  >
                    <img
                      src={img.src}
                      alt=""
                      className="w-full h-full object-cover"
                      draggable={false}
                      loading="lazy"
                    />
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
