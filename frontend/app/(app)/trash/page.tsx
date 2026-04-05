"use client";

import useSWR from "swr";
import { trash as trashApi } from "@/lib/api";
import { Trash2, RotateCcw, X, AlertTriangle } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { formatBytes, formatRelative } from "@/lib/format";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger,
} from "@/components/ui/dialog";

export default function TrashPage() {
  const { data, isLoading, mutate } = useSWR("trash", trashApi.list);

  const restore = async (id: string) => {
    try {
      await trashApi.restore(id);
      mutate();
      toast.success("Item restored");
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to restore item");
    }
  };

  const deletePermanent = async (id: string) => {
    try {
      await trashApi.delete(id);
      mutate();
      toast.success("Permanently deleted");
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
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Trash2 className="h-5 w-5" />
          <h1 className="text-xl font-semibold">Trash</h1>
        </div>
        {data?.items && data.items.length > 0 && (
          <Dialog>
            <DialogTrigger render={<Button variant="destructive" size="sm" />}>
              Empty trash
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>Empty trash?</DialogTitle>
                <DialogDescription>
                  This will permanently delete all {data.items.length} item(s). This action cannot be undone.
                </DialogDescription>
              </DialogHeader>
              <DialogFooter>
                <Button variant="destructive" onClick={emptyAll}>Delete all</Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        )}
      </div>

      <div className="flex items-center gap-2 p-3 border rounded-lg bg-muted/40 text-sm text-muted-foreground">
        <AlertTriangle className="h-4 w-4 shrink-0" />
        Items in trash are permanently deleted after 30 days.
      </div>

      {isLoading && (
        <div className="space-y-2">
          {Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} className="h-14 rounded" />)}
        </div>
      )}

      {!isLoading && !data?.items.length && (
        <p className="text-muted-foreground text-sm">Trash is empty.</p>
      )}

      {data?.items && data.items.length > 0 && (
        <div className="border rounded-lg divide-y">
          {data.items.map((item) => (
            <div key={item.id} className="flex items-center justify-between px-4 py-3 gap-3">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <p className="text-sm font-medium truncate">{item.name}</p>
                  <Badge variant="secondary" className="text-xs">{item.type}</Badge>
                </div>
                <p className="text-xs text-muted-foreground">
                  {item.size_bytes ? formatBytes(item.size_bytes) + " · " : ""}
                  Deleted {formatRelative(item.deleted_at)}
                </p>
              </div>
              <div className="flex items-center gap-1 shrink-0">
                <Button variant="ghost" size="icon" className="h-8 w-8" title="Restore"
                  onClick={() => restore(item.id)}>
                  <RotateCcw className="h-4 w-4" />
                </Button>
                <Dialog>
                  <DialogTrigger render={<Button variant="ghost" size="icon" className="h-8 w-8 text-destructive hover:text-destructive" />}>
                    <X className="h-4 w-4" />
                  </DialogTrigger>
                  <DialogContent>
                    <DialogHeader>
                      <DialogTitle>Permanently delete?</DialogTitle>
                      <DialogDescription>
                        &ldquo;{item.name}&rdquo; will be deleted forever.
                      </DialogDescription>
                    </DialogHeader>
                    <DialogFooter>
                      <Button variant="destructive" onClick={() => deletePermanent(item.id)}>Delete forever</Button>
                    </DialogFooter>
                  </DialogContent>
                </Dialog>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
