"use client";

import { useMemo } from "react";
import useSWR from "swr";
import { shares as sharesApi, grants as grantsApi } from "@/lib/api";
import { Share2, Copy, Trash2, Link, Users, X, Clock } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { formatRelative, parseServerDate } from "@/lib/format";
import type { Share, ShareGrant } from "@/lib/types";

function isShareExpired(share: Share): boolean {
  if (share.expires_at) {
    const exp = parseServerDate(share.expires_at);
    if (!isNaN(exp.getTime()) && exp.getTime() <= Date.now()) return true;
  }
  if (share.download_limit !== undefined && share.download_count >= share.download_limit) {
    return true;
  }
  return false;
}

export default function SharedPage() {
  const { data: linksData, isLoading: linksLoading, mutate: mutateLinks } =
    useSWR("shares", sharesApi.list);
  const { data: incoming, isLoading: incLoading, mutate: mutateIncoming } =
    useSWR("grants-in", () => grantsApi.list("incoming"));
  const { data: outgoing, isLoading: outLoading, mutate: mutateOutgoing } =
    useSWR("grants-out", () => grantsApi.list("outgoing"));

  const { active, expired } = useMemo(() => {
    const all = linksData?.shares ?? [];
    return {
      active: all.filter((s) => !isShareExpired(s)),
      expired: all.filter(isShareExpired),
    };
  }, [linksData]);

  const copyLink = (token: string) => {
    navigator.clipboard.writeText(`${window.location.origin}/s/${token}`);
    toast.success("Link copied to clipboard");
  };

  const removeShare = async (share: Share) => {
    try {
      await sharesApi.delete(share.id);
      mutateLinks();
      toast.success("Share removed");
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to remove share");
    }
  };

  const revokeGrant = async (grant: ShareGrant, refresh: () => void) => {
    try {
      await grantsApi.delete(grant.id);
      refresh();
      toast.success("Access revoked");
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to revoke");
    }
  };

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center gap-2">
        <Share2 className="h-5 w-5" />
        <h1 className="text-xl font-semibold">Shared</h1>
      </div>

      <Tabs defaultValue="incoming">
        <TabsList>
          <TabsTrigger value="incoming">Shared with me</TabsTrigger>
          <TabsTrigger value="outgoing">Shared by me</TabsTrigger>
          <TabsTrigger value="links">Public links{active.length > 0 ? ` (${active.length})` : ""}</TabsTrigger>
          <TabsTrigger value="expired">Expired{expired.length > 0 ? ` (${expired.length})` : ""}</TabsTrigger>
        </TabsList>

        <TabsContent value="incoming" className="mt-4">
          {incLoading && <SkeletonList />}
          {!incLoading && !incoming?.grants.length && (
            <p className="text-muted-foreground text-sm">Nothing has been shared with you yet.</p>
          )}
          {!!incoming?.grants.length && (
            <div className="border rounded-lg divide-y">
              {incoming.grants.map((g) => (
                <div key={g.id} className="flex items-center justify-between px-4 py-3 gap-3">
                  <div className="flex items-center gap-3 min-w-0">
                    <Users className="h-4 w-4 text-muted-foreground shrink-0" />
                    <div className="min-w-0">
                      <p className="text-sm font-medium truncate">
                        {g.file_name ?? g.folder_name ?? "Unknown"}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        from {g.granter_name ?? g.granted_by} · {formatRelative(g.created_at)}
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-2 shrink-0">
                    <Badge variant="outline" className="text-xs">{g.permission}</Badge>
                    <Button variant="ghost" size="icon" className="h-8 w-8"
                      onClick={() => revokeGrant(g, mutateIncoming)} aria-label="Leave">
                      <X className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </TabsContent>

        <TabsContent value="outgoing" className="mt-4">
          {outLoading && <SkeletonList />}
          {!outLoading && !outgoing?.grants.length && (
            <p className="text-muted-foreground text-sm">You haven&rsquo;t shared anything with anyone yet.</p>
          )}
          {!!outgoing?.grants.length && (
            <div className="border rounded-lg divide-y">
              {outgoing.grants.map((g) => (
                <div key={g.id} className="flex items-center justify-between px-4 py-3 gap-3">
                  <div className="flex items-center gap-3 min-w-0">
                    <Users className="h-4 w-4 text-muted-foreground shrink-0" />
                    <div className="min-w-0">
                      <p className="text-sm font-medium truncate">
                        {g.file_name ?? g.folder_name ?? "Unknown"}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        to {g.grantee_name ?? g.grantee_id} · {formatRelative(g.created_at)}
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-2 shrink-0">
                    <Badge variant="outline" className="text-xs">{g.permission}</Badge>
                    <Button variant="ghost" size="icon" className="h-8 w-8 text-destructive hover:text-destructive"
                      onClick={() => revokeGrant(g, mutateOutgoing)} aria-label="Revoke">
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </TabsContent>

        <TabsContent value="links" className="mt-4">
          {linksLoading && <SkeletonList />}
          {!linksLoading && !active.length && (
            <p className="text-muted-foreground text-sm">
              No active links. Right-click a file and choose &ldquo;Share&rdquo; to create one.
            </p>
          )}
          {!!active.length && (
            <ShareLinkList shares={active} onCopy={copyLink} onRemove={removeShare} />
          )}
        </TabsContent>

        <TabsContent value="expired" className="mt-4">
          {linksLoading && <SkeletonList />}
          {!linksLoading && !expired.length && (
            <p className="text-muted-foreground text-sm">No expired links.</p>
          )}
          {!!expired.length && (
            <ShareLinkList shares={expired} onCopy={copyLink} onRemove={removeShare} expired />
          )}
        </TabsContent>
      </Tabs>
    </div>
  );
}

interface ShareLinkListProps {
  shares: Share[];
  onCopy: (token: string) => void;
  onRemove: (share: Share) => void;
  expired?: boolean;
}

function ShareLinkList({ shares, onCopy, onRemove, expired }: ShareLinkListProps) {
  return (
    <div className="border rounded-lg divide-y">
      {shares.map((share) => (
        <div key={share.id} className="flex items-center justify-between px-4 py-3 gap-3">
          <div className="flex items-center gap-3 min-w-0">
            {expired ? (
              <Clock className="h-4 w-4 text-muted-foreground shrink-0" />
            ) : (
              <Link className="h-4 w-4 text-muted-foreground shrink-0" />
            )}
            <div className="min-w-0">
              <p className={`text-sm font-medium truncate ${expired ? "text-muted-foreground" : ""}`}>
                {share.file_name ?? "Folder share"}
              </p>
              <p className="text-xs text-muted-foreground">
                {formatRelative(share.created_at)}
                {share.expires_at && ` · expires ${formatRelative(share.expires_at)}`}
                {share.download_limit !== undefined && ` · ${share.download_count}/${share.download_limit} downloads`}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <Badge variant={expired ? "secondary" : "outline"} className="text-xs">
              {expired ? "expired" : share.permission}
            </Badge>
            {!expired && (
              <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => onCopy(share.token)}>
                <Copy className="h-4 w-4" />
              </Button>
            )}
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8 text-destructive hover:text-destructive"
              onClick={() => onRemove(share)}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        </div>
      ))}
    </div>
  );
}

function SkeletonList() {
  return (
    <div className="space-y-2">
      {Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-14 rounded" />)}
    </div>
  );
}
