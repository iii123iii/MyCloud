"use client";

import { useState } from "react";
import useSWR from "swr";
import { toast } from "sonner";
import { Lock, LockOpen, Loader2, UserCircle2 } from "lucide-react";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Separator } from "@/components/ui/separator";
import { cn } from "@/lib/utils";
import { ApiError } from "@/lib/api";
import { locks as locksApi, type FileLock } from "@/lib/locks";
import { useAuth } from "@/hooks/useAuth";
import { formatRelative } from "@/lib/format";

interface Props {
  fileId: string;
  fileName?: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function LockManagerDialog({ fileId, fileName, open, onOpenChange }: Props) {
  const { user } = useAuth();

  // Only fetch while open so closing the dialog stops re-validation chatter.
  // SWR auto-revalidates on focus, which keeps the holder list fresh while
  // the user has it open (e.g. somebody else just acquired).
  const swrKey = open ? ["locks", fileId] : null;
  const { data, isLoading, mutate } = useSWR(swrKey, () => locksApi.list(fileId));

  const [acquiring, setAcquiring] = useState(false);
  const [releasingId, setReleasingId] = useState<string | null>(null);

  const allLocks = data?.locks ?? [];
  // The web UI only manages "edit" locks; "sync" and "office" are shown for
  // visibility but the user can't release someone else's via this dialog.
  const editLocks = allLocks.filter((l) => l.kind === "edit");
  const ownEdit = user ? editLocks.find((l) => l.user_id === user.id) : undefined;
  const otherEdit = editLocks.find((l) => l.user_id !== user?.id);

  const acquire = async () => {
    setAcquiring(true);
    try {
      await locksApi.acquire(fileId, { kind: "edit" });
      await mutate();
      toast.success("Lock acquired");
    } catch (err: unknown) {
      if (err instanceof ApiError && err.status === 409) {
        toast.error(err.message || "Already locked by another user");
        // Refresh to surface the new holder.
        mutate();
      } else {
        toast.error(err instanceof Error ? err.message : "Failed to acquire lock");
      }
    } finally {
      setAcquiring(false);
    }
  };

  const release = async (lock: FileLock) => {
    setReleasingId(lock.id);
    try {
      // Server uses (file_id, kind, user_id) to identify which row to drop;
      // we only need to pass the kind. Idempotent server-side.
      await locksApi.release(fileId, lock.kind as "edit" | "sync" | "office");
      await mutate();
      toast.success("Lock released");
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to release lock");
    } finally {
      setReleasingId(null);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Lock className="h-4 w-4 shrink-0" />
            <span>File lock</span>
          </DialogTitle>
          {fileName && (
            <DialogDescription className="truncate font-mono text-xs" title={fileName}>
              {fileName}
            </DialogDescription>
          )}
          <DialogDescription className="text-xs">
            Locks coordinate concurrent edits. Acquiring an edit lock tells
            other collaborators you&apos;re working on this file.
          </DialogDescription>
        </DialogHeader>

        {isLoading && (
          <div className="space-y-2">
            <Skeleton className="h-12 rounded" />
            <Skeleton className="h-12 rounded" />
          </div>
        )}

        {!isLoading && (
          <div className="space-y-3">
            <div className="space-y-1.5">
              <p className="text-xs font-medium text-muted-foreground">
                Active locks
              </p>
              {allLocks.length === 0 ? (
                <div className="rounded-md border border-dashed px-3 py-4 text-center text-xs text-muted-foreground">
                  No active locks.
                </div>
              ) : (
                <div className="divide-y rounded-md border">
                  {allLocks.map((l) => {
                    const isMine = user?.id === l.user_id;
                    const isReleasing = releasingId === l.id;
                    return (
                      <div
                        key={l.id}
                        className={cn(
                          "flex items-center justify-between gap-2 px-2.5 py-2 text-sm",
                          isMine && "bg-muted/40",
                        )}
                      >
                        <div className="flex min-w-0 items-center gap-2">
                          <UserCircle2 className="h-4 w-4 shrink-0 text-muted-foreground" />
                          <div className="min-w-0">
                            <div className="flex items-center gap-1.5">
                              <span className="truncate font-medium">
                                {isMine ? "You" : l.user_id}
                              </span>
                              <Badge
                                variant="outline"
                                className="text-[10px] uppercase tracking-wide"
                              >
                                {l.kind}
                              </Badge>
                            </div>
                            <p className="text-[11px] text-muted-foreground">
                              Acquired {formatRelative(l.created_at)} · expires{" "}
                              {formatRelative(l.expires_at)}
                            </p>
                          </div>
                        </div>
                        {isMine && (
                          <Button
                            size="sm"
                            variant="ghost"
                            className="shrink-0"
                            onClick={() => release(l)}
                            disabled={isReleasing}
                            aria-busy={isReleasing}
                          >
                            {isReleasing ? (
                              <>
                                <Loader2 className="size-3.5 animate-spin" />
                                <span>Releasing…</span>
                              </>
                            ) : (
                              <>
                                <LockOpen className="size-3.5" />
                                <span>Release</span>
                              </>
                            )}
                          </Button>
                        )}
                      </div>
                    );
                  })}
                </div>
              )}
            </div>

            <Separator />

            {!ownEdit && (
              <>
                {otherEdit && (
                  <p
                    role="alert"
                    className="rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-900 dark:text-amber-200"
                  >
                    Another user currently holds the edit lock. Acquiring will
                    fail until it expires or they release it.
                  </p>
                )}
                <Button
                  className="w-full"
                  onClick={acquire}
                  disabled={acquiring || !!otherEdit}
                  aria-busy={acquiring}
                >
                  {acquiring ? (
                    <>
                      <Loader2 className="size-3.5 animate-spin" />
                      <span>Acquiring…</span>
                    </>
                  ) : (
                    <>
                      <Lock className="size-3.5" />
                      <span>Acquire edit lock</span>
                    </>
                  )}
                </Button>
              </>
            )}

            {ownEdit && (
              <p className="text-xs text-muted-foreground">
                You hold the edit lock. It auto-expires{" "}
                {formatRelative(ownEdit.expires_at)} unless refreshed.
              </p>
            )}
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
