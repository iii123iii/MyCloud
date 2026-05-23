"use client";

import useSWR from "swr";
import { trash as trashApi } from "@/lib/api";
import { Trash2, ArchiveRestore, AlertTriangle, X, AlertCircle } from "lucide-react";
import { toast } from "sonner";
import { AlertDialog as AlertDialogPrimitive } from "@base-ui/react/alert-dialog";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { formatBytes, formatRelative } from "@/lib/format";
import { cn } from "@/lib/utils";

const RETENTION_DAYS = 30;

function ConfirmDialog({
  trigger,
  title,
  description,
  confirmLabel,
  onConfirm,
}: {
  trigger: React.ReactNode;
  title: string;
  description: React.ReactNode;
  confirmLabel: string;
  onConfirm: () => void | Promise<void>;
}) {
  return (
    <AlertDialogPrimitive.Root>
      <AlertDialogPrimitive.Trigger render={trigger as React.ReactElement} />
      <AlertDialogPrimitive.Portal>
        <AlertDialogPrimitive.Backdrop
          className="fixed inset-0 isolate z-50 bg-black/10 duration-100 supports-backdrop-filter:backdrop-blur-xs data-open:animate-in data-open:fade-in-0 data-closed:animate-out data-closed:fade-out-0"
        />
        <AlertDialogPrimitive.Popup
          className={cn(
            "fixed top-1/2 left-1/2 z-50 grid w-full max-w-[calc(100%-2rem)] -translate-x-1/2 -translate-y-1/2 gap-4 rounded-xl bg-popover p-4 text-sm text-popover-foreground ring-1 ring-foreground/10 duration-100 outline-none sm:max-w-sm",
            "data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95"
          )}
        >
          <div className="flex flex-col gap-2 pr-2">
            <AlertDialogPrimitive.Title className="font-heading text-base leading-none font-medium">
              {title}
            </AlertDialogPrimitive.Title>
            <AlertDialogPrimitive.Description className="text-sm text-muted-foreground">
              {description}
            </AlertDialogPrimitive.Description>
          </div>
          <div className="-mx-4 -mb-4 flex flex-col-reverse gap-2 rounded-b-xl border-t bg-muted/50 p-4 sm:flex-row sm:justify-end">
            <AlertDialogPrimitive.Close render={<Button variant="outline" />}>
              Cancel
            </AlertDialogPrimitive.Close>
            <AlertDialogPrimitive.Close
              render={<Button variant="destructive" />}
              onClick={() => void onConfirm()}
            >
              {confirmLabel}
            </AlertDialogPrimitive.Close>
          </div>
        </AlertDialogPrimitive.Popup>
      </AlertDialogPrimitive.Portal>
    </AlertDialogPrimitive.Root>
  );
}

export default function TrashPage() {
  const { data, error, isLoading, mutate } = useSWR("trash", trashApi.list);
  const items = data?.items ?? [];
  const hasItems = items.length > 0;

  const restore = async (id: string, name: string) => {
    try {
      await trashApi.restore(id);
      mutate();
      toast.success(`Restored "${name}"`);
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to restore item");
    }
  };

  const deletePermanent = async (id: string, name: string) => {
    try {
      await trashApi.delete(id);
      mutate();
      toast.success(`"${name}" permanently deleted`);
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to delete item");
    }
  };

  const emptyAll = async () => {
    try {
      await trashApi.empty();
      mutate();
      toast.success("Trash emptied");
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to empty trash");
    }
  };

  return (
    <div className="mx-auto w-full max-w-5xl p-4 sm:p-6 space-y-5">
      {/* Header */}
      <header className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex min-w-0 items-center gap-2.5">
          <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-muted ring-1 ring-border/60">
            <Trash2 className="h-4.5 w-4.5 text-muted-foreground" aria-hidden="true" />
          </div>
          <div className="flex min-w-0 items-center gap-2">
            <h1 className="truncate text-xl font-semibold leading-none tracking-tight">Trash</h1>
            {hasItems && (
              <Badge variant="secondary" aria-label={`${items.length} item${items.length === 1 ? "" : "s"} in trash`}>
                {items.length}
              </Badge>
            )}
          </div>
        </div>
        {hasItems && (
          <ConfirmDialog
            trigger={
              <Button
                variant="destructive"
                size="sm"
                className="self-start transition-colors sm:self-auto"
                aria-label={`Empty trash (${items.length} item${items.length === 1 ? "" : "s"})`}
              >
                <Trash2 className="h-4 w-4" aria-hidden="true" />
                Empty trash
              </Button>
            }
            title="Empty trash?"
            description={
              <>
                This will permanently delete all <strong>{items.length}</strong>{" "}
                item{items.length === 1 ? "" : "s"}. This action cannot be undone.
              </>
            }
            confirmLabel="Delete all"
            onConfirm={emptyAll}
          />
        )}
      </header>

      {/* Retention banner */}
      <div
        role="note"
        className="flex items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/5 p-3 text-sm"
      >
        <AlertTriangle
          className="mt-0.5 h-4 w-4 shrink-0 text-amber-600 dark:text-amber-400"
          aria-hidden="true"
        />
        <p className="text-foreground/80">
          Files in trash will be deleted permanently after{" "}
          <strong className="text-foreground">{RETENTION_DAYS} days</strong>.
        </p>
      </div>

      {/* Live region — announces empty-trash result to assistive tech */}
      <div className="sr-only" role="status" aria-live="polite" aria-atomic="true">
        {isLoading
          ? "Loading trash"
          : error
            ? "Could not load trash"
            : hasItems
              ? `${items.length} item${items.length === 1 ? "" : "s"} in trash`
              : "Trash is empty"}
      </div>

      {/* Loading state */}
      {isLoading && (
        <div className="divide-y overflow-hidden rounded-lg border" aria-hidden="true">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="flex items-center justify-between gap-3 px-3 py-3 sm:px-4">
              <div className="min-w-0 flex-1 space-y-2">
                <Skeleton className="h-4 w-1/2 max-w-[260px]" />
                <Skeleton className="h-3 w-1/3 max-w-[180px]" />
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <Skeleton className="h-8 w-8 rounded-md" />
                <Skeleton className="h-8 w-8 rounded-md" />
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Error state */}
      {!isLoading && error && (
        <div
          role="alert"
          className="flex flex-col items-center justify-center gap-3 rounded-lg border border-destructive/30 bg-destructive/5 px-6 py-12 text-center"
        >
          <div className="rounded-full bg-destructive/10 p-3 ring-1 ring-destructive/20">
            <AlertCircle className="h-6 w-6 text-destructive" aria-hidden="true" />
          </div>
          <div className="space-y-1">
            <p className="text-sm font-medium text-foreground">Couldn&apos;t load trash</p>
            <p className="max-w-sm text-sm text-muted-foreground">
              {error instanceof Error ? error.message : "Something went wrong while loading items."}
            </p>
          </div>
          <Button variant="outline" size="sm" onClick={() => mutate()} className="mt-1">
            Try again
          </Button>
        </div>
      )}

      {/* Empty state */}
      {!isLoading && !error && !hasItems && (
        <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed px-6 py-16 text-center">
          <div className="rounded-full bg-muted p-3 ring-1 ring-border/60">
            <Trash2 className="h-6 w-6 text-muted-foreground" aria-hidden="true" />
          </div>
          <div className="space-y-1">
            <p className="text-sm font-medium text-foreground">Trash is empty</p>
            <p className="max-w-sm text-sm text-muted-foreground">
              Items you delete will appear here. You can restore them or remove them permanently.
            </p>
          </div>
        </div>
      )}

      {/* List */}
      {!isLoading && !error && hasItems && (
        <ul
          role="list"
          aria-label="Items in trash"
          className="divide-y overflow-hidden rounded-lg border bg-card"
        >
          {items.map((item) => (
            <li
              key={item.id}
              className={cn(
                "group/row flex items-center justify-between gap-3 px-3 py-3 sm:px-4",
                "opacity-75 transition-colors transition-opacity duration-150",
                "hover:bg-muted/50 hover:opacity-100",
                "focus-within:bg-muted/50 focus-within:opacity-100"
              )}
            >
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <p
                    className="truncate text-sm font-medium text-foreground/80 transition-colors group-hover/row:text-foreground"
                    title={item.name}
                  >
                    {item.name}
                  </p>
                  <Badge variant="secondary" className="text-xs capitalize">
                    {item.type}
                  </Badge>
                </div>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  {item.size_bytes ? `${formatBytes(item.size_bytes)} · ` : ""}
                  Deleted {formatRelative(item.deleted_at)}
                </p>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8 transition-colors hover:bg-muted hover:text-foreground"
                  title="Restore"
                  aria-label={`Restore ${item.name}`}
                  onClick={() => restore(item.id, item.name)}
                >
                  <ArchiveRestore className="h-4 w-4" aria-hidden="true" />
                </Button>
                <ConfirmDialog
                  trigger={
                    <Button
                      variant="ghost"
                      size="icon"
                      className={cn(
                        "h-8 w-8 text-destructive transition-colors",
                        "hover:bg-destructive/10 hover:text-destructive",
                        "focus-visible:ring-destructive/30"
                      )}
                      title="Delete permanently"
                      aria-label={`Delete ${item.name} permanently`}
                    >
                      <X className="h-4 w-4" aria-hidden="true" />
                    </Button>
                  }
                  title="Delete permanently?"
                  description={
                    <>
                      <strong>&ldquo;{item.name}&rdquo;</strong> will be deleted forever. This cannot be undone.
                    </>
                  }
                  confirmLabel="Delete forever"
                  onConfirm={() => deletePermanent(item.id, item.name)}
                />
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
