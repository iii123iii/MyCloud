"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import useSWR from "swr";
import { AlertCircle, Camera, Calendar as CalendarIcon, ImageIcon, X } from "lucide-react";

import { photos as photosApi, fetchAuthed, type Photo } from "@/lib/api";
import { Lightbox } from "@/components/lightbox/Lightbox";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Badge } from "@/components/ui/badge";
import { parseServerDate } from "@/lib/format";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Grouping
// ---------------------------------------------------------------------------

type PhotoGroup = { key: string; label: string; items: Photo[] };

// Groups photos by month. If a month is very dense (>120 shots), we split that
// section into per-day sub-groups so the gallery doesn't become an undifferentiated
// wall — gives it that Google Photos "Tuesday, May 12" feel.
function groupPhotos(photos: Photo[]): PhotoGroup[] {
  const monthMap = new Map<string, Photo[]>();
  for (const p of photos) {
    const dt = parseServerDate(p.shot_at || p.created_at);
    const key = `${dt.getUTCFullYear()}-${String(dt.getUTCMonth() + 1).padStart(2, "0")}`;
    const bucket = monthMap.get(key) ?? [];
    bucket.push(p);
    monthMap.set(key, bucket);
  }

  const sortedMonths = Array.from(monthMap.entries()).sort((a, b) => (a[0] < b[0] ? 1 : -1));

  const out: PhotoGroup[] = [];
  for (const [monthKey, items] of sortedMonths) {
    // Stable sort within the month: newest first.
    items.sort((a, b) => {
      const ta = parseServerDate(a.shot_at || a.created_at).getTime();
      const tb = parseServerDate(b.shot_at || b.created_at).getTime();
      return tb - ta;
    });

    if (items.length <= 120) {
      const [y, m] = monthKey.split("-");
      const label = new Date(Number(y), Number(m) - 1).toLocaleString(undefined, {
        year: "numeric",
        month: "long",
      });
      out.push({ key: monthKey, label, items });
      continue;
    }

    // Dense month → split per day.
    const dayMap = new Map<string, Photo[]>();
    for (const p of items) {
      const dt = parseServerDate(p.shot_at || p.created_at);
      const dayKey = `${monthKey}-${String(dt.getUTCDate()).padStart(2, "0")}`;
      const bucket = dayMap.get(dayKey) ?? [];
      bucket.push(p);
      dayMap.set(dayKey, bucket);
    }
    const sortedDays = Array.from(dayMap.entries()).sort((a, b) => (a[0] < b[0] ? 1 : -1));
    for (const [dayKey, dayItems] of sortedDays) {
      const [y, m, d] = dayKey.split("-");
      const label = new Date(Number(y), Number(m) - 1, Number(d)).toLocaleString(undefined, {
        weekday: "long",
        month: "long",
        day: "numeric",
        year: "numeric",
      });
      out.push({ key: dayKey, label, items: dayItems });
    }
  }
  return out;
}

// ---------------------------------------------------------------------------
// Date range filter UI
// ---------------------------------------------------------------------------

function isoDate(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

function DateRangeFilter({
  from,
  to,
  onApply,
  onClear,
}: {
  from: string;
  to: string;
  onApply: (from: string, to: string) => void;
  onClear: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [draftFrom, setDraftFrom] = useState(from);
  const [draftTo, setDraftTo] = useState(to);

  useEffect(() => {
    if (open) {
      // Re-seed the picker drafts whenever it's re-opened so it reflects the
      // currently committed filter instead of the user's last unsubmitted edit.
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setDraftFrom(from);
      setDraftTo(to);
    }
  }, [open, from, to]);

  const activeLabel = useMemo(() => {
    if (!from && !to) return "Any date";
    if (from && to) return `${from} → ${to}`;
    if (from) return `After ${from}`;
    return `Before ${to}`;
  }, [from, to]);

  const applyPreset = (days: number) => {
    const end = new Date();
    const start = new Date();
    start.setDate(start.getDate() - days);
    setDraftFrom(isoDate(start));
    setDraftTo(isoDate(end));
  };

  const applyMonth = (offset: number) => {
    const now = new Date();
    const start = new Date(now.getFullYear(), now.getMonth() + offset, 1);
    const end = new Date(now.getFullYear(), now.getMonth() + offset + 1, 0);
    setDraftFrom(isoDate(start));
    setDraftTo(isoDate(end));
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        aria-label={`Filter photos by date. Current: ${activeLabel}`}
        className={cn(
          buttonVariants({ variant: from || to ? "default" : "outline", size: "sm" }),
          "gap-2 transition-colors",
        )}
      >
        <CalendarIcon className="h-4 w-4" aria-hidden="true" />
        <span className="hidden sm:inline">{activeLabel}</span>
        <span className="sm:hidden">Date</span>
        {(from || to) && (
          <span
            role="button"
            tabIndex={0}
            aria-label="Clear date filter"
            onClick={(e) => {
              e.stopPropagation();
              onClear();
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                e.stopPropagation();
                onClear();
              }
            }}
            className="ml-1 -mr-1 rounded-full p-0.5 transition-colors hover:bg-background/20 focus:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1"
          >
            <X className="h-3 w-3" aria-hidden="true" />
          </span>
        )}
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 space-y-4">
        <div className="space-y-2">
          <Label className="text-xs font-medium text-muted-foreground">Quick ranges</Label>
          <div className="flex flex-wrap gap-1.5">
            <Button size="sm" variant="outline" onClick={() => applyPreset(7)}>Last 7 days</Button>
            <Button size="sm" variant="outline" onClick={() => applyPreset(30)}>Last 30 days</Button>
            <Button size="sm" variant="outline" onClick={() => applyMonth(0)}>This month</Button>
            <Button size="sm" variant="outline" onClick={() => applyMonth(-1)}>Last month</Button>
            <Button size="sm" variant="outline" onClick={() => applyPreset(365)}>Last year</Button>
          </div>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div className="space-y-1.5">
            <Label htmlFor="date-from" className="text-xs">From</Label>
            <Input
              id="date-from"
              type="date"
              value={draftFrom}
              max={draftTo || undefined}
              onChange={(e) => setDraftFrom(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="date-to" className="text-xs">To</Label>
            <Input
              id="date-to"
              type="date"
              value={draftTo}
              min={draftFrom || undefined}
              onChange={(e) => setDraftTo(e.target.value)}
            />
          </div>
        </div>
        <div className="flex justify-between gap-2 pt-1">
          <Button
            size="sm"
            variant="ghost"
            onClick={() => {
              setDraftFrom("");
              setDraftTo("");
              onClear();
              setOpen(false);
            }}
          >
            Clear
          </Button>
          <Button
            size="sm"
            onClick={() => {
              onApply(draftFrom, draftTo);
              setOpen(false);
            }}
          >
            Apply
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function PhotosPage() {
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");

  const { data, isLoading, error, mutate } = useSWR(
    ["photos", from, to],
    () => photosApi.list(from || undefined, to || undefined),
    { keepPreviousData: true },
  );

  const photos = data?.photos ?? [];
  const groups = useMemo(() => groupPhotos(photos), [photos]);

  // Flat list across all groups, in display order — what the lightbox navigates.
  const flatList = useMemo(() => groups.flatMap((g) => g.items), [groups]);

  const [lightboxIndex, setLightboxIndex] = useState<number | null>(null);
  const open = lightboxIndex !== null;

  // Fetch the full-resolution preview as a blob URL for the active image only.
  // The Lightbox component takes a static `images` array, so we hydrate the
  // entries lazily and cache the resulting blob URLs across image changes.
  const blobCacheRef = useRef<Map<string, string>>(new Map());
  const [, force] = useState(0);

  useEffect(() => {
    if (lightboxIndex === null) return;
    const target = flatList[lightboxIndex];
    if (!target) return;
    if (blobCacheRef.current.has(target.id)) return;

    let cancelled = false;
    (async () => {
      try {
        const res = await fetchAuthed(`/api/v2/files/${target.id}:preview`);
        if (!res.ok) return;
        const blob = await res.blob();
        if (cancelled) return;
        const objUrl = URL.createObjectURL(blob);
        blobCacheRef.current.set(target.id, objUrl);
        force((n) => n + 1);
      } catch {
        /* swallow — Lightbox will render the broken-image placeholder */
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [lightboxIndex, flatList]);

  // Revoke all blob URLs on unmount.
  useEffect(() => {
    const cache = blobCacheRef.current;
    return () => {
      for (const u of cache.values()) URL.revokeObjectURL(u);
      cache.clear();
    };
  }, []);

  const lightboxImages = useMemo(
    () =>
      flatList.map((p) => ({
        id: p.id,
        name: p.name,
        src: blobCacheRef.current.get(p.id) ?? "",
      })),
    // Re-derive when flatList changes or the forced-update token bumps.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [flatList, blobCacheRef.current.size, open, lightboxIndex],
  );

  const totalCount = photos.length;
  const hasFilter = !!(from || to);

  return (
    <div className="p-4 sm:p-6 lg:p-8 space-y-6 sm:space-y-8">
      {/* Header */}
      <header className="flex flex-wrap items-center justify-between gap-3 sm:gap-4">
        <div className="flex items-center gap-2.5 min-w-0">
          <Camera className="h-5 w-5 shrink-0 text-muted-foreground" aria-hidden="true" />
          <h1 className="text-xl sm:text-2xl font-semibold tracking-tight">Photos</h1>
          {!isLoading && totalCount > 0 && (
            <Badge
              variant="secondary"
              className="ml-1 tabular-nums"
              aria-label={`${totalCount} ${totalCount === 1 ? "photo" : "photos"}`}
            >
              {totalCount}
            </Badge>
          )}
        </div>
        <DateRangeFilter
          from={from}
          to={to}
          onApply={(f, t) => {
            setFrom(f);
            setTo(t);
          }}
          onClear={() => {
            setFrom("");
            setTo("");
          }}
        />
      </header>

      {/* Loading skeletons */}
      {isLoading && (
        <div
          className="space-y-8 animate-in fade-in duration-300"
          aria-busy="true"
          aria-live="polite"
          aria-label="Loading photos"
        >
          {[0, 1].map((s) => (
            <section key={s} className="space-y-3">
              <Skeleton className="h-4 w-40" />
              <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-2 sm:gap-3">
                {Array.from({ length: 12 }).map((_, i) => (
                  <Skeleton
                    key={i}
                    className="aspect-square rounded-lg"
                    style={{ animationDelay: `${(i % 6) * 60}ms` }}
                  />
                ))}
              </div>
            </section>
          ))}
        </div>
      )}

      {/* Error state */}
      {!isLoading && error && (
        <Alert
          variant="destructive"
          className="animate-in fade-in slide-in-from-top-1 duration-300"
        >
          <AlertCircle className="h-4 w-4" aria-hidden="true" />
          <AlertTitle>Couldn&apos;t load photos</AlertTitle>
          <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
            <span>
              {error instanceof Error && error.message
                ? error.message
                : "Something went wrong while fetching your gallery."}
            </span>
            <Button
              size="sm"
              variant="outline"
              onClick={() => mutate()}
              className="transition-colors"
            >
              Try again
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {/* Empty state */}
      {!isLoading && !error && groups.length === 0 && (
        <div
          className="flex flex-col items-center justify-center text-center py-16 sm:py-20 px-6 rounded-xl border border-dashed bg-muted/30 animate-in fade-in zoom-in-95 duration-300"
          role="status"
        >
          <div className="rounded-full bg-muted p-4 mb-4 ring-1 ring-border/50">
            <ImageIcon className="h-8 w-8 text-muted-foreground" aria-hidden="true" />
          </div>
          <h2 className="text-lg font-semibold mb-1.5 tracking-tight">
            {hasFilter ? "No photos match this range" : "No photos yet"}
          </h2>
          <p className="text-sm text-muted-foreground max-w-sm leading-relaxed">
            {hasFilter
              ? "Try widening the date range or clear the filter to see everything."
              : "Upload some images from the dashboard and they'll show up here."}
          </p>
          {hasFilter && (
            <Button
              variant="outline"
              size="sm"
              className="mt-5 transition-colors"
              onClick={() => {
                setFrom("");
                setTo("");
              }}
            >
              Clear filter
            </Button>
          )}
        </div>
      )}

      {/* Groups */}
      {!isLoading && !error && groups.map((g) => (
        <section
          key={g.key}
          className="space-y-3 animate-in fade-in duration-300"
          aria-labelledby={`photos-group-${g.key}`}
        >
          <h2
            id={`photos-group-${g.key}`}
            className="text-sm font-semibold tracking-tight text-foreground/80 sticky top-0 z-10 bg-background/85 backdrop-blur supports-[backdrop-filter]:bg-background/70 py-2 -mx-1 px-1 flex items-baseline gap-2"
          >
            <span>{g.label}</span>
            <span
              className="text-xs font-normal text-muted-foreground tabular-nums"
              aria-hidden="true"
            >
              · {g.items.length} {g.items.length === 1 ? "photo" : "photos"}
            </span>
          </h2>
          <div
            role="grid"
            aria-label={`Photos from ${g.label}`}
            className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-2 sm:gap-3"
          >
            {g.items.map((p) => {
              const idx = flatList.findIndex((f) => f.id === p.id);
              return (
                <PhotoTile
                  key={p.id}
                  photo={p}
                  onClick={() => setLightboxIndex(idx)}
                />
              );
            })}
          </div>
        </section>
      ))}

      {/* Lightbox */}
      {open && lightboxImages.length > 0 && (
        <Lightbox
          open={open}
          onOpenChange={(o) => {
            if (!o) setLightboxIndex(null);
          }}
          images={lightboxImages}
          startIndex={lightboxIndex ?? 0}
        />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Tile
// ---------------------------------------------------------------------------

function PhotoTile({ photo, onClick }: { photo: Photo; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      role="gridcell"
      className={cn(
        "group relative aspect-square overflow-hidden rounded-lg bg-muted shadow-sm",
        "transition-[transform,box-shadow,outline] duration-200 ease-out",
        "hover:shadow-md hover:-translate-y-0.5",
        "focus:outline-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background",
        "active:translate-y-0 active:shadow-sm",
        "animate-in fade-in zoom-in-95 duration-300",
      )}
      aria-label={`Open photo: ${photo.name}`}
    >
      <ThumbImage photoId={photo.id} alt={photo.name} />
      {/* Hover gradient + filename */}
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-x-0 bottom-0 h-1/3 bg-gradient-to-t from-black/65 via-black/20 to-transparent opacity-0 transition-opacity duration-200 group-hover:opacity-100 group-focus-visible:opacity-100"
      />
      <div className="pointer-events-none absolute inset-x-0 bottom-0 px-2 py-1.5 text-[11px] font-medium text-white truncate opacity-0 transition-opacity duration-200 group-hover:opacity-100 group-focus-visible:opacity-100">
        {photo.name}
      </div>
    </button>
  );
}

// ThumbImage fetches the encrypted thumb URL with the auth header and exposes
// it as a blob URL. While loading it occupies the same square so the grid
// doesn't reflow when the image lands.
function ThumbImage({ photoId, alt }: { photoId: string; alt: string }) {
  const { data } = useSWR(["thumb", photoId], async () => {
    const res = await fetchAuthed(`/api/v2/files/${photoId}/thumb`);
    if (!res.ok) return null;
    const blob = await res.blob();
    return URL.createObjectURL(blob);
  });

  if (data === undefined) {
    return (
      <div
        className="absolute inset-0 animate-pulse bg-gradient-to-br from-muted-foreground/5 via-muted-foreground/10 to-muted-foreground/5"
        aria-hidden="true"
      />
    );
  }
  if (!data) {
    return (
      <div
        className="absolute inset-0 flex flex-col items-center justify-center gap-1 bg-muted/40"
        role="img"
        aria-label={`Preview unavailable for ${alt}`}
      >
        <ImageIcon className="h-6 w-6 text-muted-foreground/40" aria-hidden="true" />
        <span className="text-[10px] text-muted-foreground/60">Unavailable</span>
      </div>
    );
  }
  // eslint-disable-next-line @next/next/no-img-element
  return (
    <img
      src={data}
      alt={alt}
      loading="lazy"
      draggable={false}
      className="absolute inset-0 w-full h-full object-cover transition-transform duration-500 ease-out group-hover:scale-[1.06] group-focus-visible:scale-[1.06] motion-reduce:transition-none motion-reduce:group-hover:scale-100"
    />
  );
}
