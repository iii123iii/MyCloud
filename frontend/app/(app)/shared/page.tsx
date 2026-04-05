"use client";

import useSWR from "swr";
import { shares as sharesApi } from "@/lib/api";
import { Share2, Copy, Trash2, Link } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { formatRelative } from "@/lib/format";
import type { Share } from "@/lib/types";

export default function SharedPage() {
  const { data, isLoading, mutate } = useSWR("shares", sharesApi.list);

  const copyLink = (token: string) => {
    navigator.clipboard.writeText(`${window.location.origin}/s/${token}`);
    toast.success("Link copied to clipboard");
  };

  const remove = async (share: Share) => {
    try {
      await sharesApi.delete(share.id);
      mutate();
      toast.success("Share removed");
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to remove share");
    }
  };

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center gap-2">
        <Share2 className="h-5 w-5" />
        <h1 className="text-xl font-semibold">Shared by me</h1>
      </div>

      {isLoading && (
        <div className="space-y-2">
          {Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-14 rounded" />)}
        </div>
      )}

      {!isLoading && !data?.shares.length && (
        <p className="text-muted-foreground text-sm">
          No active shares. Right-click a file and choose &ldquo;Share&rdquo; to create a link.
        </p>
      )}

      {data?.shares && data.shares.length > 0 && (
        <div className="border rounded-lg divide-y">
          {data.shares.map((share) => (
            <div key={share.id} className="flex items-center justify-between px-4 py-3 gap-3">
              <div className="flex items-center gap-3 min-w-0">
                <Link className="h-4 w-4 text-muted-foreground shrink-0" />
                <div className="min-w-0">
                  <p className="text-sm font-medium truncate">{share.file_name ?? "Folder share"}</p>
                  <p className="text-xs text-muted-foreground">
                    {formatRelative(share.created_at)}
                    {share.expires_at && ` · expires ${formatRelative(share.expires_at)}`}
                  </p>
                </div>
              </div>
              <div className="flex items-center gap-2 shrink-0">
                <Badge variant="outline" className="text-xs">{share.permission}</Badge>
                <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => copyLink(share.token)}>
                  <Copy className="h-4 w-4" />
                </Button>
                <Button variant="ghost" size="icon" className="h-8 w-8 text-destructive hover:text-destructive"
                  onClick={() => remove(share)}>
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
