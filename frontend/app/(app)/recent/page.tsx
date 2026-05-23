"use client";

import { useRef, useEffect, useMemo, useState } from "react";
import useSWRInfinite from "swr/infinite";
import { files as filesApi, tokenStore } from "@/lib/api";
import { FileIcon } from "@/components/file-explorer/FileIcon";
import { FileContextMenu } from "@/components/file-explorer/FileContextMenu";
import { PreviewModal } from "@/components/modals/PreviewModal";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Clock,
  Loader2,
  History,
  Star,
  Download,
  Eye,
  MoreHorizontal,
  FileClock,
  AlertCircle,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  formatBytes,
  formatRelative,
  formatServerDateTime,
  parseServerDate,
} from "@/lib/format";
import {
  isToday,
  isYesterday,
  differenceInCalendarDays,
} from "date-fns";
import { toast } from "sonner";
import { cn } from "@/lib/utils";
import type { FileItem } from "@/lib/types";

const PAGE_SIZE = 50;

type Group = { key: string; label: string; files: FileItem[] };

function groupByDay(files: FileItem[]): Group[] {
  const buckets = new Map<string, Group>();
  const now = new Date();
  for (const f of files) {
    const d = parseServerDate(f.updated_at);
    let key: string;
    let label: string;
    if (isNaN(d.getTime())) {
      key = "unknown";
      label = "Unknown";
    } else if (isToday(d)) {
      key = "today";
      label = "Today";
    } else if (isYesterday(d)) {
      key = "yesterday";
      label = "Yesterday";
    } else {
      const days = differenceInCalendarDays(now, d);
      if (days <= 7) {
        key = "last-week";
        label = "Last week";
      } else if (days <= 30) {
        key = "last-month";
        label = "Last month";
      } else {
        key = "older";
        label = "Older";
      }
    }
    let g = buckets.get(key);
    if (!g) {
      g = { key, label, files: [] };
      buckets.set(key, g);
    }
    g.files.push(f);
  }
  const order = ["today", "yesterday", "last-week", "last-month", "older", "unknown"];
  return order.map((k) => buckets.get(k)).filter((g): g is Group => !!g);
}

function RecentRow({ file, onMutate }: { file: FileItem; onMutate: () => void }) {
  const [previewOpen, setPreviewOpen] = useState(false);
  const [starring, setStarring] = useState(false);
  const absolute = useMemo(() => formatServerDateTime(file.updated_at), [file.updated_at]);
  const relative = useMemo(() => formatRelative(file.updated_at), [file.updated_at]);
  const isoDate = useMemo(() => {
    const d = parseServerDate(file.updated_at);
    return isNaN(d.getTime()) ? undefined : d.toISOString();
  }, [file.updated_at]);

  const handleStar = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (starring) return;
    setStarring(true);
    try {
      await filesApi.update(file.id, { is_starred: !file.is_starred });
      onMutate();
    } catch {
      toast.error("Could not update star");
    } finally {
      setStarring(false);
    }
  };

  const handleDownload = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      const token = tokenStore.getAccess();
      const res = await fetch(filesApi.downloadUrl(file.id), {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });
      if (!res.ok) throw new Error();
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = file.name;
      a.click();
      URL.revokeObjectURL(url);
    } catch {
      toast.error("Download failed");
    }
  };

  const handlePreview = (e: React.MouseEvent) => {
    e.stopPropagation();
    setPreviewOpen(true);
  };

  return (
    <>
      <FileContextMenu file={file} onPreview={() => setPreviewOpen(true)} onMutate={onMutate}>
        <div
          role="button"
          tabIndex={0}
          aria-label={`${file.name}, modified ${relative}, ${formatBytes(file.size_bytes)}${file.is_starred ? ", starred" : ""}`}
          className={cn(
            "group relative flex items-center gap-3 px-3 sm:px-4 py-2.5 cursor-pointer outline-none",
            "transition-colors duration-150",
            "hover:bg-accent/60 active:bg-accent",
            "focus-visible:bg-accent/60 focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:ring-inset",
          )}
          onDoubleClick={() => setPreviewOpen(true)}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === " ") {
              e.preventDefault();
              setPreviewOpen(true);
            }
          }}
        >
          <FileIcon mime={file.mime_type} className="h-4 w-4 shrink-0" />
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-1.5">
              <span className="text-sm font-medium truncate">{file.name}</span>
              {file.is_starred && (
                <Star
                  className="h-3 w-3 fill-yellow-400 text-yellow-400 shrink-0"
                  aria-label="Starred"
                />
              )}
            </div>
            <div className="flex items-center gap-2 text-[11px] text-muted-foreground sm:hidden mt-0.5">
              <span className="tabular-nums">{formatBytes(file.size_bytes)}</span>
              <span aria-hidden>&middot;</span>
              <time dateTime={isoDate} title={absolute}>
                {relative}
              </time>
            </div>
          </div>

          <span className="hidden sm:inline text-xs text-muted-foreground shrink-0 w-20 text-right tabular-nums">
            {formatBytes(file.size_bytes)}
          </span>

          <Tooltip>
            <TooltipTrigger
              render={
                <time
                  dateTime={isoDate}
                  className="hidden sm:inline text-xs text-muted-foreground shrink-0 w-28 text-right cursor-default"
                >
                  {relative}
                </time>
              }
            />
            <TooltipContent>{absolute}</TooltipContent>
          </Tooltip>

          <div
            className={cn(
              "hidden sm:flex items-center gap-0.5 shrink-0",
              "opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 focus-within:opacity-100",
              "transition-opacity duration-150",
            )}
          >
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7"
                    onClick={handlePreview}
                    aria-label={`Preview ${file.name}`}
                  >
                    <Eye className="h-3.5 w-3.5" />
                  </Button>
                }
              />
              <TooltipContent>Preview</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7"
                    onClick={handleStar}
                    aria-label={file.is_starred ? `Unstar ${file.name}` : `Star ${file.name}`}
                    aria-pressed={file.is_starred}
                    disabled={starring}
                  >
                    <Star
                      className={cn(
                        "h-3.5 w-3.5 transition-transform",
                        file.is_starred && "fill-yellow-400 text-yellow-400",
                        starring && "scale-90",
                      )}
                    />
                  </Button>
                }
              />
              <TooltipContent>{file.is_starred ? "Unstar" : "Star"}</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7"
                    onClick={handleDownload}
                    aria-label={`Download ${file.name}`}
                  >
                    <Download className="h-3.5 w-3.5" />
                  </Button>
                }
              />
              <TooltipContent>Download</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7"
                    aria-label={`More actions for ${file.name}`}
                    aria-haspopup="menu"
                    onClick={(e) => {
                      e.stopPropagation();
                      const target = e.currentTarget as HTMLElement;
                      const evt = new MouseEvent("contextmenu", {
                        bubbles: true,
                        clientX: target.getBoundingClientRect().left,
                        clientY: target.getBoundingClientRect().bottom,
                      });
                      target.dispatchEvent(evt);
                    }}
                  >
                    <MoreHorizontal className="h-3.5 w-3.5" />
                  </Button>
                }
              />
              <TooltipContent>More</TooltipContent>
            </Tooltip>
          </div>
        </div>
      </FileContextMenu>

      {previewOpen && (
        <PreviewModal file={file} open={previewOpen} onOpenChange={setPreviewOpen} />
      )}
    </>
  );
}

function RowSkeleton() {
  return (
    <div
      className="flex items-center gap-3 px-3 sm:px-4 py-2.5"
      aria-hidden="true"
    >
      <Skeleton className="h-4 w-4 rounded shrink-0" />
      <div className="flex-1 min-w-0 space-y-1.5">
        <Skeleton className="h-3.5 w-1/2 max-w-[280px]" />
        <Skeleton className="h-2.5 w-24 sm:hidden" />
      </div>
      <Skeleton className="hidden sm:block h-3 w-16" />
      <Skeleton className="hidden sm:block h-3 w-24" />
    </div>
  );
}

export default function RecentPage() {
  const {
    data, error, setSize, isValidating, mutate,
  } = useSWRInfinite(
    (i) => `recent-p${i}`,
    (key) => {
      const page = parseInt(key.split("-p").pop()!) + 1;
      return filesApi.list({ sort: "updated_at", order: "desc", all: true, page, page_size: PAGE_SIZE });
    },
  );

  const allFiles = data ? data.flatMap((d) => d.files) : [];
  const hasMore  = data ? data[data.length - 1]?.has_more ?? false : false;
  const loading  = !data && isValidating;
  const showError = !data && !!error;

  const groups = useMemo(() => groupByDay(allFiles), [allFiles]);

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

  return (
    <TooltipProvider delay={300}>
      <div
        className="p-4 sm:p-6 space-y-5 sm:space-y-6 max-w-5xl mx-auto"
        aria-busy={loading}
      >
        <header className="flex items-center gap-2.5">
          <Clock className="h-5 w-5 text-muted-foreground" aria-hidden />
          <h1 className="text-xl sm:text-2xl font-semibold tracking-tight">Recent</h1>
          {data?.[0] && (
            <span
              className="text-xs text-muted-foreground ml-1 tabular-nums"
              aria-label={`${data[0].total} ${data[0].total === 1 ? "file" : "files"} total`}
            >
              {data[0].total} {data[0].total === 1 ? "file" : "files"}
            </span>
          )}
        </header>

        {loading && (
          <div
            className="space-y-6"
            role="status"
            aria-live="polite"
            aria-label="Loading recent files"
          >
            <span className="sr-only">Loading recent files…</span>
            {[6, 4].map((n, gi) => (
              <div key={gi} className="space-y-2">
                <Skeleton className="h-3.5 w-24" aria-hidden="true" />
                <div className="border rounded-lg divide-y overflow-hidden bg-card">
                  {Array.from({ length: n }).map((_, i) => <RowSkeleton key={i} />)}
                </div>
              </div>
            ))}
          </div>
        )}

        {showError && (
          <Alert variant="destructive" role="alert">
            <AlertCircle className="h-4 w-4" aria-hidden />
            <AlertTitle>Could not load recent files</AlertTitle>
            <AlertDescription className="flex flex-col gap-2">
              <span>Something went wrong while fetching your activity. Please try again.</span>
              <Button
                variant="outline"
                size="sm"
                className="self-start"
                onClick={() => mutate()}
              >
                Try again
              </Button>
            </AlertDescription>
          </Alert>
        )}

        {!loading && !showError && allFiles.length === 0 && (
          <div
            className="flex flex-col items-center justify-center text-center py-16 px-4 border border-dashed rounded-lg"
            role="status"
          >
            <div className="relative mb-4">
              <div
                className="absolute inset-0 bg-primary/10 blur-2xl rounded-full"
                aria-hidden
              />
              <div className="relative flex items-center justify-center h-16 w-16 rounded-full bg-muted ring-1 ring-border/50">
                <FileClock className="h-7 w-7 text-muted-foreground" aria-hidden />
              </div>
            </div>
            <h2 className="text-sm font-medium">No recent activity</h2>
            <p className="text-xs text-muted-foreground mt-1 max-w-xs leading-relaxed">
              Files you upload or modify will show up here, grouped by when you last touched them.
            </p>
          </div>
        )}

        {!loading && !showError && groups.length > 0 && (
          <div className="space-y-6">
            {groups.map((group, gi) => (
              <section
                key={group.key}
                aria-labelledby={`recent-${group.key}`}
                className="scroll-mt-4"
              >
                <div className="flex items-center gap-2 mb-2">
                  <History
                    className="h-3.5 w-3.5 text-muted-foreground shrink-0"
                    aria-hidden
                  />
                  <h2
                    id={`recent-${group.key}`}
                    className="text-xs font-semibold uppercase tracking-wider text-muted-foreground"
                  >
                    {group.label}
                  </h2>
                  <span
                    className="text-[11px] tabular-nums text-muted-foreground/70"
                    aria-label={`${group.files.length} ${group.files.length === 1 ? "file" : "files"}`}
                  >
                    {group.files.length}
                  </span>
                  <div className="flex-1 h-px bg-border ml-2" aria-hidden />
                </div>
                <div className="border rounded-lg divide-y overflow-hidden bg-card shadow-sm">
                  {group.files.map((file) => (
                    <RecentRow key={file.id} file={file} onMutate={mutate} />
                  ))}
                </div>
                {gi < groups.length - 1 && (
                  <div className="sr-only">End of {group.label}</div>
                )}
              </section>
            ))}
          </div>
        )}

        {hasMore && (
          <div ref={sentinelRef} className="flex justify-center py-4">
            {isValidating ? (
              <span
                className="inline-flex items-center gap-2 text-xs text-muted-foreground"
                role="status"
                aria-live="polite"
              >
                <Loader2 className="h-4 w-4 animate-spin" aria-hidden />
                <span>Loading more…</span>
              </span>
            ) : (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setSize((s) => s + 1)}
                aria-label="Load more recent files"
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
