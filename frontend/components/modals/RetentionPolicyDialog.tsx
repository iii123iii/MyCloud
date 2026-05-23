"use client";

import { useEffect, useState } from "react";
import useSWR from "swr";
import { toast } from "sonner";
import { Clock, Loader2, ShieldAlert, Trash2 } from "lucide-react";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { ApiError } from "@/lib/api";
import { retention, type RetentionAction, type RetentionPolicy } from "@/lib/retention";

interface Props {
  folderId: string;
  folderName?: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

// Same bounds the backend enforces (handlers_retention.go: 1..3650). We
// duplicate them client-side so the field can be marked invalid before
// the round-trip — the server is still the source of truth.
const MIN_DAYS = 1;
const MAX_DAYS = 3650;
const DEFAULT_DAYS = 30;
const DEFAULT_ACTION: RetentionAction = "soft_delete";

const ACTION_LABELS: Record<RetentionAction, string> = {
  soft_delete: "Move to trash",
  hard_delete: "Delete permanently",
  notify: "Notify only (no deletion)",
};

const ACTION_BLURBS: Record<RetentionAction, string> = {
  soft_delete:
    "Older files are moved to Trash on the next sweep. You can restore them from there.",
  hard_delete:
    "Older files are queued for permanent deletion. They go through trash first but the purger will remove them on its next run.",
  notify:
    "Nothing is deleted. An activity log entry records how many files would have matched.",
};

export function RetentionPolicyDialog({
  folderId,
  folderName,
  open,
  onOpenChange,
}: Props) {
  // SWR key is null while the dialog is closed so we don't hit the API
  // for every folder the user merely right-clicks past.
  const swrKey = open && folderId ? ["retention", folderId] : null;
  const {
    data: policy,
    error,
    isLoading,
    mutate,
  } = useSWR<RetentionPolicy | null>(swrKey, () => retention.get(folderId), {
    revalidateOnFocus: false,
  });

  const [days, setDays] = useState<string>(String(DEFAULT_DAYS));
  const [action, setAction] = useState<RetentionAction>(DEFAULT_ACTION);
  const [saving, setSaving] = useState(false);
  const [removing, setRemoving] = useState(false);

  // Sync form state from the loaded policy. Re-runs when the dialog
  // re-opens for a different folder.
  useEffect(() => {
    if (!open) return;
    if (policy) {
      setDays(String(policy.retention_days));
      setAction(policy.action);
    } else {
      setDays(String(DEFAULT_DAYS));
      setAction(DEFAULT_ACTION);
    }
  }, [open, policy]);

  const daysNum = parseInt(days, 10);
  const daysInvalid =
    days.trim() === "" ||
    Number.isNaN(daysNum) ||
    daysNum < MIN_DAYS ||
    daysNum > MAX_DAYS;

  const hasPolicy = Boolean(policy);

  const save = async () => {
    if (daysInvalid) return;
    setSaving(true);
    try {
      await retention.put(folderId, { retention_days: daysNum, action });
      toast.success(hasPolicy ? "Retention policy updated" : "Retention policy set");
      await mutate();
      onOpenChange(false);
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to save policy");
    } finally {
      setSaving(false);
    }
  };

  const remove = async () => {
    setRemoving(true);
    try {
      await retention.remove(folderId);
      toast.success("Retention policy removed");
      await mutate(null, { revalidate: false });
      onOpenChange(false);
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to remove policy");
    } finally {
      setRemoving(false);
    }
  };

  // 403 (folder access) and other non-404 fetch errors surface here. A
  // 404 is mapped to `policy === null` in retention.get and is not an
  // error from this dialog's point of view.
  const fetchError =
    error instanceof ApiError
      ? error.message
      : error instanceof Error
        ? error.message
        : null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[calc(100vw-2rem)] max-w-md">
        <DialogHeader className="space-y-2">
          <DialogTitle className="flex items-center gap-2">
            <Clock className="h-4 w-4 shrink-0" />
            <span>Retention policy</span>
          </DialogTitle>
          {folderName && (
            <DialogDescription className="truncate font-mono text-xs" title={folderName}>
              {folderName}
            </DialogDescription>
          )}
          <DialogDescription className="text-xs">
            Files older than the configured age are processed once an hour by
            the maintenance sweep.
          </DialogDescription>
        </DialogHeader>

        {isLoading ? (
          <div className="space-y-4" aria-busy="true" aria-label="Loading policy">
            <div className="space-y-2">
              <Skeleton className="h-4 w-24" />
              <Skeleton className="h-9 w-full" />
            </div>
            <div className="space-y-2">
              <Skeleton className="h-4 w-16" />
              <Skeleton className="h-9 w-full" />
            </div>
            <Skeleton className="h-12 w-full" />
          </div>
        ) : fetchError ? (
          <div className="flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm">
            <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
            <span className="text-destructive">{fetchError}</span>
          </div>
        ) : (
          <div className="space-y-4">
            {!hasPolicy && (
              <div className="rounded-md border border-dashed bg-muted/30 p-3 text-xs text-muted-foreground">
                No policy is set for this folder yet. Pick an age and an
                action below, then save.
              </div>
            )}

            {hasPolicy && policy && (
              <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                <Badge variant="outline" className="text-xs">Active</Badge>
                <span>
                  Last updated{" "}
                  <span className="text-foreground">
                    {new Date(policy.updated_at).toLocaleString()}
                  </span>
                </span>
              </div>
            )}

            <div className="space-y-2">
              <Label htmlFor="retention-days">Retention age (days)</Label>
              <Input
                id="retention-days"
                type="number"
                inputMode="numeric"
                min={MIN_DAYS}
                max={MAX_DAYS}
                value={days}
                onChange={(e) => setDays(e.target.value)}
                aria-invalid={daysInvalid || undefined}
                aria-describedby={daysInvalid ? "retention-days-error" : "retention-days-help"}
                className={cn(
                  daysInvalid && "border-destructive focus-visible:ring-destructive/40",
                )}
              />
              {daysInvalid ? (
                <p id="retention-days-error" role="alert" className="text-xs text-destructive">
                  Must be a whole number between {MIN_DAYS} and {MAX_DAYS}.
                </p>
              ) : (
                <p id="retention-days-help" className="text-xs text-muted-foreground">
                  Files created more than {daysNum} day{daysNum === 1 ? "" : "s"}{" "}
                  ago will be affected.
                </p>
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="retention-action">Action</Label>
              <Select
                value={action}
                onValueChange={(v) => setAction(v as RetentionAction)}
              >
                <SelectTrigger id="retention-action" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="soft_delete">{ACTION_LABELS.soft_delete}</SelectItem>
                  <SelectItem value="hard_delete">{ACTION_LABELS.hard_delete}</SelectItem>
                  <SelectItem value="notify">{ACTION_LABELS.notify}</SelectItem>
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">{ACTION_BLURBS[action]}</p>
            </div>

            {action === "hard_delete" && (
              <div className="flex items-start gap-2 rounded-md border border-amber-500/40 bg-amber-500/5 p-3 text-xs">
                <ShieldAlert className="mt-0.5 h-3.5 w-3.5 shrink-0 text-amber-600 dark:text-amber-400" />
                <span className="text-amber-700 dark:text-amber-300">
                  Permanent deletion can&apos;t be undone once the purger
                  runs. Consider &ldquo;Move to trash&rdquo; if you&apos;re
                  unsure.
                </span>
              </div>
            )}
          </div>
        )}

        <Separator />

        <DialogFooter className="flex-col-reverse gap-2 sm:flex-row sm:justify-between">
          <div>
            {hasPolicy && !isLoading && !fetchError && (
              <Button
                type="button"
                variant="outline"
                onClick={remove}
                disabled={removing || saving}
                className="text-destructive hover:bg-destructive/10 hover:text-destructive"
              >
                {removing ? (
                  <>
                    <Loader2 className="animate-spin" aria-hidden="true" />
                    <span>Removing…</span>
                  </>
                ) : (
                  <>
                    <Trash2 aria-hidden="true" />
                    <span>Remove policy</span>
                  </>
                )}
              </Button>
            )}
          </div>
          <div className="flex flex-col-reverse gap-2 sm:flex-row">
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={saving || removing}
            >
              Cancel
            </Button>
            <Button
              type="button"
              onClick={save}
              disabled={saving || removing || daysInvalid || isLoading || Boolean(fetchError)}
            >
              {saving ? (
                <>
                  <Loader2 className="animate-spin" aria-hidden="true" />
                  <span>Saving…</span>
                </>
              ) : hasPolicy ? (
                "Save changes"
              ) : (
                "Save policy"
              )}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
