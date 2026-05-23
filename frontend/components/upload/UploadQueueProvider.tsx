"use client";

// Persistent upload queue panel.
//
// Wraps the app shell. Exposes useUploadQueue() that callers (FileExplorer
// drop handler) use to enqueue files. The panel shows per-file progress,
// status, and per-file actions (cancel, retry). State persists to
// sessionStorage so a page navigation doesn't lose the queue — items
// already in-flight pick up where they left off because tus-js-client
// stores its own offset in localStorage.

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  CheckCircle2,
  ChevronDown,
  ChevronUp,
  FileText,
  Loader2,
  RefreshCw,
  Upload,
  X,
  XCircle,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { cn } from "@/lib/utils";

type Status = "queued" | "uploading" | "done" | "error" | "canceled";

export type QueueItem = {
  id: string;
  name: string;
  size: number;
  status: Status;
  progress: number; // 0..1
  error?: string;
};

type Ctx = {
  items: QueueItem[];
  enqueue: (item: Omit<QueueItem, "status" | "progress">) => string;
  update: (id: string, patch: Partial<QueueItem>) => void;
  remove: (id: string) => void;
  clearDone: () => void;
};

const UploadQueueContext = createContext<Ctx | null>(null);
const STORAGE_KEY = "mc_upload_queue_v1";

export function UploadQueueProvider({ children }: { children: ReactNode }) {
  // Lazy initializer reads sessionStorage exactly once during the first
  // render. Doing this in a useEffect instead causes a setState-in-effect
  // lint error and a visible flash of empty state on hydration.
  const [items, setItems] = useState<QueueItem[]>(() => {
    if (typeof window === "undefined") return [];
    try {
      const raw = sessionStorage.getItem(STORAGE_KEY);
      return raw ? (JSON.parse(raw) as QueueItem[]) : [];
    } catch {
      return [];
    }
  });

  // Persist on change. sessionStorage so a fresh tab starts clean.
  useEffect(() => {
    if (typeof window === "undefined") return;
    try {
      sessionStorage.setItem(STORAGE_KEY, JSON.stringify(items));
    } catch {
      // QuotaExceeded — fail silently; queue still works in-memory.
    }
  }, [items]);

  const enqueue: Ctx["enqueue"] = useCallback((item) => {
    setItems((prev) => [...prev, { ...item, status: "queued", progress: 0 }]);
    return item.id;
  }, []);
  const update: Ctx["update"] = useCallback((id, patch) => {
    setItems((prev) => prev.map((i) => (i.id === id ? { ...i, ...patch } : i)));
  }, []);
  const remove: Ctx["remove"] = useCallback((id) => {
    setItems((prev) => prev.filter((i) => i.id !== id));
  }, []);
  const clearDone: Ctx["clearDone"] = useCallback(() => {
    setItems((prev) => prev.filter((i) => i.status !== "done" && i.status !== "canceled"));
  }, []);

  const value = useMemo<Ctx>(
    () => ({ items, enqueue, update, remove, clearDone }),
    [items, enqueue, update, remove, clearDone],
  );

  return (
    <UploadQueueContext.Provider value={value}>
      {children}
      <UploadQueuePanel />
    </UploadQueueContext.Provider>
  );
}

export function useUploadQueue(): Ctx {
  const ctx = useContext(UploadQueueContext);
  if (!ctx) throw new Error("useUploadQueue must be used within UploadQueueProvider");
  return ctx;
}

// ──────────────────────────────────────────────────────────────────────
// Helpers

function extOf(name: string): string {
  const idx = name.lastIndexOf(".");
  if (idx <= 0 || idx === name.length - 1) return "";
  return name.slice(idx + 1).toUpperCase();
}

function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(units.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
  return `${(n / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

// ──────────────────────────────────────────────────────────────────────
// Panel

function UploadQueuePanel() {
  const ctx = useContext(UploadQueueContext);
  const [minimized, setMinimized] = useState(false);
  const [expandedErrors, setExpandedErrors] = useState<Record<string, boolean>>({});
  // Trigger slide-in only on the first transition from 0 → >0 items.
  const [mounted, setMounted] = useState(false);
  const wasEmpty = useRef(true);
  const autoTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const items = ctx?.items ?? [];
  const count = items.length;

  useEffect(() => {
    // Mount / unmount the slide-in animation based on whether the queue has
    // any items. The setState calls below are synchronising animation state
    // with a derived count — necessary because we need the unmount delay to
    // play before the panel actually leaves the DOM.
    if (count > 0 && wasEmpty.current) {
      wasEmpty.current = false;
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setMounted(true);
    } else if (count === 0) {
      wasEmpty.current = true;
      setMounted(false);
      setMinimized(false);
      setExpandedErrors({});
    }
  }, [count]);

  // Counts (computed before the early-return so hooks order stays stable).
  const finishedCount = items.filter((i) => i.status === "done" || i.status === "canceled").length;
  const erroredCount = items.filter((i) => i.status === "error").length;
  const activeCount = items.filter((i) => i.status === "queued" || i.status === "uploading").length;
  const totalProgress =
    count === 0
      ? 0
      : Math.round(
          (items.reduce(
            (sum, i) => sum + (i.status === "done" ? 1 : i.status === "uploading" ? i.progress : 0),
            0,
          ) /
            count) *
            100,
        );
  const allFinished = count > 0 && activeCount === 0 && erroredCount === 0;

  // Auto-collapse + auto-clear when everything finished cleanly.
  useEffect(() => {
    if (autoTimer.current) {
      clearTimeout(autoTimer.current);
      autoTimer.current = null;
    }
    if (allFinished && ctx) {
      // Leave the panel visible for a comfortable window after everything
      // finishes so users have time to actually read the completion state
      // before it disappears. The user can still dismiss manually via the
      // X button on each row or "Clear all finished".
      autoTimer.current = setTimeout(() => {
        setMinimized(true);
        ctx.clearDone();
      }, 30000);
    }
    return () => {
      if (autoTimer.current) {
        clearTimeout(autoTimer.current);
        autoTimer.current = null;
      }
    };
  }, [allFinished, ctx]);

  if (!ctx) return null;
  if (count === 0) return null;
  const { remove, clearDone } = ctx;

  const containerBase =
    "fixed z-40 bottom-0 inset-x-0 sm:inset-x-auto sm:bottom-4 sm:right-4 sm:w-96";
  const slideIn = mounted ? "animate-in slide-in-from-bottom-4 fade-in-0 duration-300" : "";

  // Minimised chip
  if (minimized) {
    return (
      <div className={cn(containerBase, slideIn)}>
        <button
          type="button"
          onClick={() => setMinimized(false)}
          aria-label="Expand upload queue"
          className={cn(
            "mx-auto sm:mx-0 sm:ml-auto flex items-center gap-2 rounded-t-lg sm:rounded-full",
            "bg-background border shadow-lg px-3 py-2 text-sm",
            "hover:bg-muted transition-colors w-full sm:w-auto justify-center sm:justify-start",
          )}
        >
          {activeCount > 0 ? (
            <Loader2 className="size-4 animate-spin text-primary" />
          ) : erroredCount > 0 ? (
            <XCircle className="size-4 text-destructive" />
          ) : (
            <CheckCircle2 className="size-4 text-green-600" />
          )}
          <span className="font-medium">
            {activeCount > 0
              ? `${activeCount} uploading`
              : erroredCount > 0
                ? `${erroredCount} failed`
                : `${finishedCount} done`}
          </span>
          {activeCount > 0 && (
            <span className="text-xs text-muted-foreground tabular-nums">{totalProgress}%</span>
          )}
          <ChevronUp className="size-4 text-muted-foreground" />
        </button>
      </div>
    );
  }

  return (
    <div
      className={cn(
        containerBase,
        slideIn,
        "bg-background border sm:rounded-lg shadow-lg text-sm overflow-hidden",
      )}
      role="region"
      aria-label="Upload queue"
    >
      {/* Header */}
      <header className="px-3 py-2 flex items-center gap-2">
        <Upload className="size-4 text-muted-foreground" />
        <div className="flex-1 min-w-0">
          <div className="font-medium leading-tight">Uploads</div>
          <div className="text-xs text-muted-foreground leading-tight">
            {activeCount > 0
              ? `${count - finishedCount - erroredCount} of ${count} files`
              : erroredCount > 0
                ? `${erroredCount} failed, ${finishedCount} done`
                : `${finishedCount} of ${count} complete`}
          </div>
        </div>
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={() => setMinimized(true)}
          aria-label="Minimize upload queue"
        >
          <ChevronDown className="size-4" />
        </Button>
      </header>

      {/* Total progress.
          Bare <div> bar instead of the shadcn <Progress> primitive — that
          primitive's Root auto-renders its own Track+Indicator AFTER
          children, so passing explicit ProgressTrack/Indicator children
          would stack two bars on top of each other. Matches the per-row
          bar pattern below for visual consistency. */}
      <div className="px-3 pb-2">
        <div className="flex items-center justify-between text-xs text-muted-foreground mb-1">
          <span className="tabular-nums">{totalProgress}% complete</span>
          <span className="tabular-nums">
            {count - activeCount} / {count}
          </span>
        </div>
        <div
          role="progressbar"
          aria-label="Overall upload progress"
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={totalProgress}
          className="h-1.5 bg-muted rounded overflow-hidden"
        >
          <div
            className="h-full bg-primary transition-[width] duration-300 ease-out"
            style={{ width: `${totalProgress}%` }}
          />
        </div>
      </div>

      <Separator />

      {/* List */}
      <ul className="max-h-[50vh] sm:max-h-[45vh] overflow-y-auto divide-y">
        {items.map((it) => {
          const pct = Math.round((it.status === "done" ? 1 : it.progress) * 100);
          const ext = extOf(it.name);
          const isErrorExpanded = !!expandedErrors[it.id];
          return (
            <li key={it.id} className="px-3 py-2">
              <div className="flex items-center gap-2">
                {/* Status / file icon */}
                <div className="relative shrink-0">
                  <FileText className="size-7 text-muted-foreground/70" />
                  <span className="absolute -bottom-0.5 -right-0.5">
                    {it.status === "uploading" && (
                      <Loader2 className="size-3.5 animate-spin text-primary bg-background rounded-full" />
                    )}
                    {it.status === "done" && (
                      <CheckCircle2 className="size-3.5 text-green-600 bg-background rounded-full" />
                    )}
                    {it.status === "error" && (
                      <XCircle className="size-3.5 text-destructive bg-background rounded-full" />
                    )}
                    {it.status === "queued" && (
                      <span className="size-3.5 inline-block rounded-full bg-muted ring-2 ring-background" />
                    )}
                    {it.status === "canceled" && (
                      <X className="size-3.5 text-muted-foreground bg-background rounded-full" />
                    )}
                  </span>
                </div>

                {/* Name + meta */}
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-1.5 min-w-0">
                    <span className="truncate font-medium">{it.name}</span>
                    {ext && (
                      <Badge variant="secondary" className="shrink-0 text-[10px] uppercase">
                        {ext}
                      </Badge>
                    )}
                  </div>
                  <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
                    <span className="tabular-nums">{formatBytes(it.size)}</span>
                    {it.status === "uploading" && (
                      <span className="tabular-nums">{pct}%</span>
                    )}
                    {it.status === "queued" && <span>Queued</span>}
                    {it.status === "done" && <span className="text-green-600">Complete</span>}
                    {it.status === "canceled" && <span>Canceled</span>}
                    {it.status === "error" && (
                      <button
                        type="button"
                        onClick={() =>
                          setExpandedErrors((s) => ({ ...s, [it.id]: !s[it.id] }))
                        }
                        className="text-destructive hover:underline inline-flex items-center gap-1"
                        aria-expanded={isErrorExpanded}
                      >
                        Failed
                        {isErrorExpanded ? (
                          <ChevronUp className="size-3" />
                        ) : (
                          <ChevronDown className="size-3" />
                        )}
                      </button>
                    )}
                  </div>
                  {(it.status === "uploading" || it.status === "queued") && (
                    <div className="h-1 bg-muted rounded mt-1 overflow-hidden">
                      <div
                        className="h-full bg-primary transition-[width] duration-300 ease-out"
                        style={{ width: `${it.status === "uploading" ? pct : 0}%` }}
                      />
                    </div>
                  )}
                </div>

                <Button
                  variant="ghost"
                  size="icon-xs"
                  onClick={() => remove(it.id)}
                  aria-label={`Remove ${it.name}`}
                  className="shrink-0"
                >
                  <X className="size-3.5" />
                </Button>
              </div>

              {/* Error detail row */}
              {it.status === "error" && isErrorExpanded && (
                <div className="mt-2 ml-9 rounded-md bg-destructive/10 px-2 py-1.5 text-xs text-destructive">
                  <div className="break-words">{it.error || "Upload failed."}</div>
                  <div className="mt-1.5 flex justify-end">
                    <Button
                      variant="outline"
                      size="xs"
                      onClick={() => {
                        // The actual retry is owned by the uploader; we just
                        // reset the item state so the uploader picks it up.
                        ctx.update(it.id, { status: "queued", progress: 0, error: undefined });
                        setExpandedErrors((s) => ({ ...s, [it.id]: false }));
                      }}
                    >
                      <RefreshCw className="size-3" />
                      Retry
                    </Button>
                  </div>
                </div>
              )}
            </li>
          );
        })}
      </ul>

      {/* Footer (only when there's something to clear) */}
      {finishedCount > 0 && (
        <>
          <Separator />
          <div className="px-3 py-2 flex justify-end">
            <Button variant="ghost" size="xs" onClick={clearDone}>
              Clear all finished
            </Button>
          </div>
        </>
      )}
    </div>
  );
}
