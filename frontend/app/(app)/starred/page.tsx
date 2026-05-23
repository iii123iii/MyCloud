"use client";

import { useRef, useEffect, useState, useCallback } from "react";
import useSWRInfinite from "swr/infinite";
import { toast } from "sonner";
import { files as filesApi } from "@/lib/api";
import { FileRow } from "@/components/file-explorer/FileRow";
import { FileIcon } from "@/components/file-explorer/FileIcon";
import { PreviewModal } from "@/components/modals/PreviewModal";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  AlertCircle,
  LayoutGrid,
  List,
  Loader2,
  Star,
  StarOff,
} from "lucide-react";
import { formatBytes, formatRelative } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { FileItem, PaginatedFiles } from "@/lib/types";

const PAGE_SIZE = 50;
const VIEW_STORAGE_KEY = "mc_starred_view";

type ViewMode = "grid" | "list";

export default function StarredPage() {
  const {
    data, error, setSize, isValidating, mutate,
  } = useSWRInfinite<PaginatedFiles>(
    (i) => `starred-p${i}`,
    (key) => {
      const page = parseInt(key.split("-p").pop()!) + 1;
      return filesApi.list({
        sort: "updated_at",
        order: "desc",
        all: true,
        starred_only: true,
        page,
        page_size: PAGE_SIZE,
      });
    },
  );

  // View toggle (persisted to localStorage so it survives reloads).
  const [view, setView] = useState<ViewMode>("list");
  useEffect(() => {
    const saved = typeof window !== "undefined" ? localStorage.getItem(VIEW_STORAGE_KEY) : null;
    // Hydrating the persisted view mode after mount. A lazy useState initializer
    // would call localStorage during SSR which Next.js disallows.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (saved === "grid" || saved === "list") setView(saved);
  }, []);
  const setViewPersisted = (v: ViewMode) => {
    setView(v);
    try { localStorage.setItem(VIEW_STORAGE_KEY, v); } catch { /* ignore quota */ }
  };

  const allFiles = data ? data.flatMap((d) => d.files) : [];
  const total    = data?.[0]?.total ?? allFiles.length;
  const hasMore  = data ? data[data.length - 1]?.has_more ?? false : false;
  const loading  = !data && !error && isValidating;

  const sentinelRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!sentinelRef.current || !hasMore) return;
    const obs = new IntersectionObserver(
      ([e]) => { if (e.isIntersecting && !isValidating) setSize((s) => s + 1); },
      { rootMargin: "200px" }
    );
    obs.observe(sentinelRef.current);
    return () => obs.disconnect();
  }, [hasMore, isValidating, setSize]);

  // Optimistic unstar: drop the row immediately, then PATCH; on failure
  // we revalidate to restore the true state and surface a toast.
  const handleUnstar = useCallback(async (file: FileItem) => {
    const prev = data;
    if (prev) {
      const next = prev.map((page) => ({
        ...page,
        files: page.files.filter((f) => f.id !== file.id),
        total: Math.max(0, page.total - 1),
      }));
      mutate(next, false);
    }
    try {
      await filesApi.update(file.id, { is_starred: false });
      toast.success("Removed from starred");
      mutate();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Could not unstar");
      mutate();
    }
  }, [data, mutate]);

  return (
    <TooltipProvider delay={300}>
      <div
        className="p-4 sm:p-6 space-y-6 max-w-5xl mx-auto"
        aria-busy={loading || undefined}
      >
        {/* Hero header */}
        <header className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
          <div className="flex items-center gap-3 min-w-0">
            <div
              className="flex h-10 w-10 sm:h-12 sm:w-12 shrink-0 items-center justify-center rounded-xl bg-yellow-400/10 ring-1 ring-yellow-400/30"
              aria-hidden="true"
            >
              <Star className="h-5 w-5 sm:h-6 sm:w-6 fill-yellow-400 text-yellow-400" />
            </div>
            <div className="min-w-0">
              <h1 className="text-xl sm:text-2xl font-semibold leading-tight tracking-tight">
                Your starred items
              </h1>
              <p
                className="text-xs sm:text-sm text-muted-foreground mt-0.5"
                aria-live="polite"
              >
                {loading
                  ? "Loading…"
                  : error
                    ? "Couldn’t load starred items"
                    : total === 0
                      ? "Nothing starred yet"
                      : `${total} starred item${total === 1 ? "" : "s"}`}
              </p>
            </div>
          </div>

          {/* View toggle — hidden until we know there's something to show */}
          {!loading && !error && allFiles.length > 0 && (
            <div
              role="group"
              aria-label="View mode"
              className="inline-flex items-center rounded-lg border bg-background p-0.5 self-start sm:self-auto shadow-xs"
            >
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      variant={view === "list" ? "secondary" : "ghost"}
                      size="sm"
                      className="h-7 px-2 transition-colors"
                      aria-label="List view"
                      aria-pressed={view === "list"}
                      onClick={() => setViewPersisted("list")}
                    >
                      <List className="h-4 w-4" aria-hidden="true" />
                    </Button>
                  }
                />
                <TooltipContent>List view</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      variant={view === "grid" ? "secondary" : "ghost"}
                      size="sm"
                      className="h-7 px-2 transition-colors"
                      aria-label="Grid view"
                      aria-pressed={view === "grid"}
                      onClick={() => setViewPersisted("grid")}
                    >
                      <LayoutGrid className="h-4 w-4" aria-hidden="true" />
                    </Button>
                  }
                />
                <TooltipContent>Grid view</TooltipContent>
              </Tooltip>
            </div>
          )}
        </header>

        {/* Error fallback */}
        {error && !loading && (
          <Alert
            variant="destructive"
            className="animate-in fade-in slide-in-from-top-1 duration-300"
          >
            <AlertCircle className="h-4 w-4" aria-hidden="true" />
            <AlertTitle>Couldn&apos;t load starred items</AlertTitle>
            <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
              <span>
                {error instanceof Error && error.message
                  ? error.message
                  : "Something went wrong while fetching your starred items."}
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

        {/* Loading skeleton — mirrors the active view */}
        {loading && (
          view === "grid" ? (
            <div
              className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-3"
              aria-hidden="true"
            >
              {Array.from({ length: 10 }).map((_, i) => (
                <div
                  key={i}
                  className="flex flex-col rounded-lg border bg-card overflow-hidden"
                >
                  <Skeleton className="aspect-[4/3] rounded-none" />
                  <div className="p-2.5 space-y-1.5">
                    <Skeleton className="h-3.5 w-3/4" />
                    <div className="flex items-center justify-between">
                      <Skeleton className="h-2.5 w-12" />
                      <Skeleton className="h-2.5 w-16" />
                    </div>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div
              className="border rounded-lg divide-y overflow-hidden bg-card"
              aria-hidden="true"
            >
              {Array.from({ length: 6 }).map((_, i) => (
                <div key={i} className="flex items-center gap-3 px-4 py-2.5">
                  <Skeleton className="h-4 w-4 rounded shrink-0" />
                  <Skeleton className="h-3.5 flex-1 max-w-[260px]" />
                  <Skeleton className="hidden sm:block h-3 w-16" />
                  <Skeleton className="hidden sm:block h-3 w-24" />
                </div>
              ))}
            </div>
          )
        )}

        {/* Empty state */}
        {!loading && !error && allFiles.length === 0 && (
          <div
            role="status"
            className="flex flex-col items-center justify-center text-center py-16 px-6 rounded-xl border border-dashed bg-card/40"
          >
            <div className="relative mb-4">
              <div
                className="absolute inset-0 bg-yellow-400/10 blur-2xl rounded-full"
                aria-hidden="true"
              />
              <div
                className="relative flex h-14 w-14 items-center justify-center rounded-full bg-yellow-400/10 ring-1 ring-yellow-400/20"
                aria-hidden="true"
              >
                <Star className="h-7 w-7 text-yellow-400" />
              </div>
            </div>
            <h2 className="text-base font-semibold">No starred items yet</h2>
            <p className="mt-1.5 text-sm text-muted-foreground max-w-sm leading-relaxed">
              Star items to find them quickly. Right-click any file and choose
              {" "}
              <span className="font-medium text-foreground">Star</span>
              {" "}
              to pin it here.
            </p>
          </div>
        )}

        {/* Content */}
        {!error && allFiles.length > 0 && (
          view === "grid" ? (
            <ul
              className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-3"
              aria-label="Starred items"
            >
              {allFiles.map((file) => (
                <li key={file.id}>
                  <StarredCard file={file} onUnstar={handleUnstar} onMutate={mutate} />
                </li>
              ))}
            </ul>
          ) : (
            <ul
              className="border rounded-lg divide-y overflow-hidden bg-card"
              aria-label="Starred items"
            >
              {allFiles.map((file) => (
                <li
                  key={file.id}
                  className="group relative transition-colors focus-within:bg-accent/40"
                >
                  <FileRow file={file} onMutate={mutate} />
                  <span className="sr-only">Starred.</span>
                  {/* Quick unstar action — hover on desktop, always visible on touch */}
                  <Tooltip>
                    <TooltipTrigger
                      render={
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          className={cn(
                            "absolute right-2 top-1/2 -translate-y-1/2",
                            "opacity-100 sm:opacity-0",
                            "group-hover:opacity-100 group-focus-within:opacity-100",
                            "focus-visible:opacity-100",
                            "transition-opacity",
                          )}
                          aria-label={`Remove ${file.name} from starred`}
                          onClick={(e) => {
                            e.stopPropagation();
                            handleUnstar(file);
                          }}
                        >
                          <StarOff className="h-4 w-4" aria-hidden="true" />
                        </Button>
                      }
                    />
                    <TooltipContent>Unstar</TooltipContent>
                  </Tooltip>
                </li>
              ))}
            </ul>
          )
        )}

        {/* Infinite-scroll sentinel */}
        {hasMore && (
          <div
            ref={sentinelRef}
            className="flex justify-center py-4"
            aria-hidden={isValidating ? "true" : undefined}
          >
            {isValidating ? (
              <span className="inline-flex items-center gap-2 text-xs text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
                <span>Loading more…</span>
              </span>
            ) : (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setSize((s) => s + 1)}
              >
                Load more
              </Button>
            )}
          </div>
        )}
      </div>
    </TooltipProvider>
  );
}

// StarredCard — grid-view tile. Uses the same colored FileIcon as the rest
// of the explorer so the visual language stays consistent. Double-click opens
// the preview, the floating button unstars without ever opening the preview.
function StarredCard({
  file,
  onUnstar,
  onMutate,
}: {
  file: FileItem;
  onUnstar: (f: FileItem) => void;
  onMutate: () => void;
}) {
  const [previewOpen, setPreviewOpen] = useState(false);
  return (
    <>
      <div
        className={cn(
          "group relative flex flex-col rounded-lg border bg-card overflow-hidden cursor-pointer",
          "transition-all duration-150 ease-out",
          "hover:bg-accent hover:border-accent-foreground/10 hover:shadow-sm",
          "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:border-ring",
        )}
        onDoubleClick={() => setPreviewOpen(true)}
        onClick={() => setPreviewOpen(true)}
        role="button"
        tabIndex={0}
        aria-label={`Open ${file.name}`}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            setPreviewOpen(true);
          }
        }}
      >
        <div className="aspect-[4/3] flex items-center justify-center bg-muted/40 transition-colors group-hover:bg-muted/60">
          <FileIcon
            mime={file.mime_type}
            className="h-10 w-10 transition-transform duration-200 group-hover:scale-105"
          />
        </div>
        <div className="p-2.5 min-w-0">
          <div className="flex items-center gap-1">
            <div className="text-sm font-medium truncate flex-1" title={file.name}>
              {file.name}
            </div>
            <Star
              className="h-3 w-3 shrink-0 fill-yellow-400 text-yellow-400"
              aria-hidden="true"
            />
            <span className="sr-only">Starred.</span>
          </div>
          <div className="mt-0.5 flex items-center justify-between text-[11px] text-muted-foreground tabular-nums">
            <span className="truncate">{formatBytes(file.size_bytes)}</span>
            <span className="shrink-0 ml-2 truncate">{formatRelative(file.updated_at)}</span>
          </div>
        </div>

        <Button
          variant="secondary"
          size="icon-sm"
          className={cn(
            "absolute right-1.5 top-1.5 shadow-sm",
            "opacity-100 sm:opacity-0",
            "group-hover:opacity-100 group-focus-within:opacity-100",
            "focus-visible:opacity-100",
            "transition-opacity",
          )}
          aria-label={`Remove ${file.name} from starred`}
          onClick={(e) => {
            e.stopPropagation();
            onUnstar(file);
          }}
        >
          <StarOff className="h-3.5 w-3.5" aria-hidden="true" />
        </Button>
      </div>

      {previewOpen && (
        <PreviewModal
          file={file}
          open={previewOpen}
          onOpenChange={(o) => {
            setPreviewOpen(o);
            // PreviewModal may invoke mutations (rename, etc) — refresh on close.
            if (!o) onMutate();
          }}
        />
      )}
    </>
  );
}
