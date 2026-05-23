"use client";

import { useMemo, useState } from "react";
import useSWR from "swr";
import { useRouter } from "next/navigation";
import { shares as sharesApi, grants as grantsApi, requests as requestsApi, files as filesApi, folders as foldersApi, fetchAuthed, type UploadRequest } from "@/lib/api";
import { Share2, Copy, Trash2, Link, Users, X, Clock, Inbox, FolderIcon, Lock, AlertTriangle, Download, File, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { PreviewModal } from "@/components/modals/PreviewModal";
import { formatRelative, parseServerDate } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { Share, ShareGrant, FileItem, GrantPermission } from "@/lib/types";

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
  const router = useRouter();
  const { data: linksData, isLoading: linksLoading, error: linksError, mutate: mutateLinks } =
    useSWR("shares", sharesApi.list);
  const { data: incoming, isLoading: incLoading, error: incError, mutate: mutateIncoming } =
    useSWR("grants-in", () => grantsApi.list("incoming"));
  const { data: outgoing, isLoading: outLoading, error: outError, mutate: mutateOutgoing } =
    useSWR("grants-out", () => grantsApi.list("outgoing"));
  // Surface guest-upload links here so users have a place to see
  // every guest link they've ever created. Creation still happens per
  // folder (right-click → Request files); this view lists + revokes.
  const { data: uploadReqs, isLoading: uploadReqsLoading, error: uploadReqsError, mutate: mutateUploadReqs } =
    useSWR("upload-requests", () =>
      requestsApi.list().then((r) => r.requests ?? []),
    );

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

  // Upload-request links live at /u/<token> — distinct namespace from
  // shares so the public landing knows which handler to dispatch.
  const copyUploadLink = (token: string) => {
    navigator.clipboard.writeText(`${window.location.origin}/u/${token}`);
    toast.success("Upload link copied");
  };
  const removeUploadRequest = async (req: UploadRequest) => {
    try {
      await requestsApi.delete(req.id);
      mutateUploadReqs();
      toast.success("Upload link deleted");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Delete failed");
    }
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

  // Change a recipient's permission (viewer/editor/owner) after sharing —
  // PATCH /shares/grants/{id}, granter-only on the backend. Used by the
  // "Shared by me" tab so the owner can promote/demote without re-sharing.
  const changeGrant = async (grant: ShareGrant, permission: GrantPermission, refresh: () => void) => {
    if (permission === grant.permission) return;
    try {
      await grantsApi.update(grant.id, permission);
      refresh();
      toast.success("Permission updated");
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to update permission");
    }
  };

  // Items shared *with* you are real, accessible files/folders — the backend
  // authorizes grantees on the normal download/preview/archive endpoints
  // (canAccessFile / canAccessFolder honour share_grants). This tab previously
  // only listed names; these handlers expose the actions that already work.
  const [previewFile, setPreviewFile] = useState<FileItem | null>(null);
  const [loadingPreviewId, setLoadingPreviewId] = useState<string | null>(null);

  const openPreview = async (grant: ShareGrant) => {
    if (!grant.file_id) return;
    setLoadingPreviewId(grant.id);
    try {
      // GET /files/{id} runs the same canAccessFile check the preview endpoint
      // does, so it resolves for grantees and yields the mime_type + size the
      // PreviewModal needs to render (or to offer a download fallback).
      const info = await filesApi.getInfo(grant.file_id);
      setPreviewFile(info);
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Couldn't open file");
    } finally {
      setLoadingPreviewId(null);
    }
  };

  const downloadFile = async (grant: ShareGrant) => {
    if (!grant.file_id) return;
    try {
      const res = await fetchAuthed(`/api/v2/files/${grant.file_id}:download`);
      if (!res.ok) throw new Error("Download failed");
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = grant.file_name ?? "download";
      a.click();
      URL.revokeObjectURL(url);
    } catch {
      toast.error("Download failed");
    }
  };

  // Browsing into a shared folder isn't supported server-side (listings filter
  // by owner), but the archive endpoint authorizes grantees per item, so a
  // grantee can still pull the whole folder down as a zip.
  const downloadFolderZip = async (grant: ShareGrant) => {
    if (!grant.folder_id) return;
    try {
      await filesApi.downloadArchive([], [grant.folder_id]);
    } catch {
      toast.error("Download failed");
    }
  };

  // Copy a shared item into the caller's own drive (root). Read access is
  // enough, so this works for viewers too.
  const copyToMyFiles = async (grant: ShareGrant) => {
    try {
      if (grant.file_id) await filesApi.copy(grant.file_id);
      else if (grant.folder_id) await foldersApi.copy(grant.folder_id);
      toast.success("Copied to your files");
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Copy failed");
    }
  };

  return (
    <div className="p-4 sm:p-6 space-y-6 max-w-5xl mx-auto">
      {/* Header */}
      <header className="flex items-center gap-3">
        <div
          className="flex h-10 w-10 sm:h-11 sm:w-11 items-center justify-center rounded-xl bg-primary/10 ring-1 ring-primary/20"
          aria-hidden
        >
          <Share2 className="h-5 w-5 text-primary" />
        </div>
        <div className="min-w-0">
          <h1 className="text-xl sm:text-2xl font-semibold leading-tight tracking-tight">Shared</h1>
          <p className="text-xs sm:text-sm text-muted-foreground">
            Items shared with you and links you&rsquo;ve created.
          </p>
        </div>
      </header>

      <Tabs defaultValue="incoming" className="space-y-4">
        <TabsList
          aria-label="Shared content categories"
          className="flex flex-wrap h-auto w-full justify-start gap-1 sm:w-fit"
        >
          <TabsTrigger value="incoming">
            Shared with me
            {incoming?.grants.length ? (
              <Badge variant="secondary" className="ml-1.5 h-4 px-1.5 text-[10px] font-medium">
                {incoming.grants.length}
              </Badge>
            ) : null}
          </TabsTrigger>
          <TabsTrigger value="outgoing">
            Shared by me
            {outgoing?.grants.length ? (
              <Badge variant="secondary" className="ml-1.5 h-4 px-1.5 text-[10px] font-medium">
                {outgoing.grants.length}
              </Badge>
            ) : null}
          </TabsTrigger>
          <TabsTrigger value="links">
            Public links
            {active.length > 0 ? (
              <Badge variant="secondary" className="ml-1.5 h-4 px-1.5 text-[10px] font-medium">
                {active.length}
              </Badge>
            ) : null}
          </TabsTrigger>
          <TabsTrigger value="uploads">
            Upload links
            {uploadReqs && uploadReqs.length > 0 ? (
              <Badge variant="secondary" className="ml-1.5 h-4 px-1.5 text-[10px] font-medium">
                {uploadReqs.length}
              </Badge>
            ) : null}
          </TabsTrigger>
          <TabsTrigger value="expired">
            Expired
            {expired.length > 0 ? (
              <Badge variant="secondary" className="ml-1.5 h-4 px-1.5 text-[10px] font-medium">
                {expired.length}
              </Badge>
            ) : null}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="incoming" className="mt-0 focus-visible:outline-none">
          {incLoading && <SkeletonList />}
          {!incLoading && incError && (
            <ErrorState message="Couldn't load items shared with you." onRetry={() => mutateIncoming()} />
          )}
          {!incLoading && !incError && !incoming?.grants.length && (
            <EmptyState
              icon={<Users className="h-6 w-6" aria-hidden />}
              title="Nothing shared with you yet"
              description="When someone shares a file or folder with you, it'll appear here."
            />
          )}
          {!!incoming?.grants.length && (
            <ul
              role="list"
              aria-label="Items shared with you"
              className="border rounded-lg divide-y bg-card overflow-hidden"
            >
              {incoming.grants.map((g) => {
                const itemName = g.file_name ?? g.folder_name ?? "Unknown";
                const sharer = g.granter_name ?? g.granted_by;
                const when = formatRelative(g.created_at);
                const isFile = !!g.file_id;
                const busy = loadingPreviewId === g.id;
                const meta = (
                  <div className="min-w-0">
                    <p className="text-sm font-medium truncate" title={itemName}>
                      {itemName}
                    </p>
                    <p className="text-xs text-muted-foreground truncate">
                      <span className="sr-only">Shared by </span>
                      from {sharer}
                      <span aria-hidden> &middot; </span>
                      <span className="sr-only">received </span>
                      {when}
                    </p>
                  </div>
                );
                return (
                  <li
                    key={g.id}
                    className="group flex items-center justify-between gap-3 px-4 py-3 transition-colors hover:bg-accent/50 focus-within:bg-accent/40"
                  >
                    {/* Files open a preview on click; folders open the shared
                        browser, which reuses the drive's FileExplorer rooted at
                        this folder. */}
                    {isFile ? (
                      <button
                        type="button"
                        onClick={() => openPreview(g)}
                        disabled={busy}
                        className="flex items-center gap-3 min-w-0 flex-1 text-left rounded-md -mx-1 px-1 py-0.5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-70"
                        aria-label={`Open ${itemName}`}
                      >
                        {busy ? (
                          <Loader2 className="h-4 w-4 text-muted-foreground shrink-0 animate-spin" aria-hidden />
                        ) : (
                          <File className="h-4 w-4 text-muted-foreground shrink-0" aria-hidden />
                        )}
                        {meta}
                      </button>
                    ) : (
                      <button
                        type="button"
                        onClick={() => router.push(`/shared/browse?folder=${g.folder_id}`)}
                        className="flex items-center gap-3 min-w-0 flex-1 text-left rounded-md -mx-1 px-1 py-0.5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                        aria-label={`Open folder ${itemName}`}
                      >
                        <FolderIcon className="h-4 w-4 text-muted-foreground shrink-0" aria-hidden />
                        {meta}
                      </button>
                    )}
                    <div className="flex items-center gap-1.5 shrink-0">
                      <Badge variant="outline" className="text-xs capitalize">
                        {g.permission}
                      </Badge>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8"
                        onClick={() => (isFile ? downloadFile(g) : downloadFolderZip(g))}
                        aria-label={isFile ? `Download ${itemName}` : `Download ${itemName} as zip`}
                        title={isFile ? "Download" : "Download as zip"}
                      >
                        <Download className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8"
                        onClick={() => copyToMyFiles(g)}
                        aria-label={`Copy ${itemName} to my files`}
                        title="Copy to my files"
                      >
                        <Copy className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 transition-opacity sm:opacity-0 sm:group-hover:opacity-100 sm:group-focus-within:opacity-100"
                        onClick={() => revokeGrant(g, mutateIncoming)}
                        aria-label={`Leave ${itemName} shared by ${sharer}`}
                        title="Leave"
                      >
                        <X className="h-4 w-4" />
                      </Button>
                    </div>
                  </li>
                );
              })}
            </ul>
          )}
        </TabsContent>

        <TabsContent value="outgoing" className="mt-0 focus-visible:outline-none">
          {outLoading && <SkeletonList />}
          {!outLoading && outError && (
            <ErrorState message="Couldn't load items you've shared." onRetry={() => mutateOutgoing()} />
          )}
          {!outLoading && !outError && !outgoing?.grants.length && (
            <EmptyState
              icon={<Users className="h-6 w-6" aria-hidden />}
              title="You haven't shared anything yet"
              description="Share a file or folder with someone and it'll show up here."
            />
          )}
          {!!outgoing?.grants.length && (
            <ul
              role="list"
              aria-label="Items you've shared"
              className="border rounded-lg divide-y bg-card overflow-hidden"
            >
              {outgoing.grants.map((g) => {
                const itemName = g.file_name ?? g.folder_name ?? "Unknown";
                const recipient = g.grantee_name ?? g.grantee_id;
                const when = formatRelative(g.created_at);
                return (
                  <li
                    key={g.id}
                    className="group flex items-center justify-between gap-3 px-4 py-3 transition-colors hover:bg-accent/50 focus-within:bg-accent/40"
                  >
                    <div className="flex items-center gap-3 min-w-0">
                      <Users className="h-4 w-4 text-muted-foreground shrink-0" aria-hidden />
                      <div className="min-w-0">
                        <p className="text-sm font-medium truncate" title={itemName}>
                          {itemName}
                        </p>
                        <p className="text-xs text-muted-foreground truncate">
                          <span className="sr-only">Shared </span>
                          to {recipient}
                          <span aria-hidden> &middot; </span>
                          <span className="sr-only">created </span>
                          {when}
                        </p>
                      </div>
                    </div>
                    <div className="flex items-center gap-1.5 shrink-0">
                      <Select
                        value={g.permission}
                        onValueChange={(v) => changeGrant(g, v as GrantPermission, mutateOutgoing)}
                      >
                        <SelectTrigger
                          className="h-8 w-[116px] text-xs capitalize"
                          aria-label={`Change ${recipient}'s permission on ${itemName}`}
                        >
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="viewer">Viewer</SelectItem>
                          <SelectItem value="editor">Editor</SelectItem>
                          <SelectItem value="owner">Owner</SelectItem>
                        </SelectContent>
                      </Select>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 text-destructive hover:text-destructive transition-opacity sm:opacity-0 sm:group-hover:opacity-100 sm:group-focus-within:opacity-100"
                        onClick={() => revokeGrant(g, mutateOutgoing)}
                        aria-label={`Revoke ${recipient}'s access to ${itemName}`}
                        title="Revoke"
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </li>
                );
              })}
            </ul>
          )}
        </TabsContent>

        <TabsContent value="links" className="mt-0 focus-visible:outline-none">
          {linksLoading && <SkeletonList />}
          {!linksLoading && linksError && (
            <ErrorState message="Couldn't load your share links." onRetry={() => mutateLinks()} />
          )}
          {!linksLoading && !linksError && !active.length && (
            <EmptyState
              icon={<Link className="h-6 w-6" aria-hidden />}
              title="No active links"
              description={"Right-click a file or folder and choose “Share” to create a public link."}
            />
          )}
          {!!active.length && (
            <ShareLinkList shares={active} onCopy={copyLink} onRemove={removeShare} />
          )}
        </TabsContent>

        <TabsContent value="uploads" className="mt-0 focus-visible:outline-none">
          {uploadReqsLoading && <SkeletonList />}
          {!uploadReqsLoading && uploadReqsError && (
            <ErrorState message="Couldn't load upload links." onRetry={() => mutateUploadReqs()} />
          )}
          {!uploadReqsLoading && !uploadReqsError && (!uploadReqs || uploadReqs.length === 0) && (
            <EmptyState
              icon={<Inbox className="h-6 w-6" aria-hidden />}
              title="No guest upload links yet"
              description={
                "Right-click a folder and choose “Request files (guest upload)” to create one — guests can drop files in without an account."
              }
            />
          )}
          {!!uploadReqs && uploadReqs.length > 0 && (
            <ul
              role="list"
              aria-label="Guest upload links"
              className="border rounded-lg divide-y bg-card overflow-hidden"
            >
              {uploadReqs.map((req) => {
                const folder = req.folder_name ?? "(Root folder)";
                const uploads = `${req.used_files} upload${req.used_files === 1 ? "" : "s"} received`;
                const cap = req.max_files != null ? ` · cap ${req.max_files}` : "";
                const expires = req.expires_at ? ` · expires ${formatRelative(req.expires_at)}` : "";
                const created = ` · created ${formatRelative(req.created_at)}`;
                const a11ySummary = [
                  `Upload link for ${folder}`,
                  req.has_password ? "password-protected" : null,
                  uploads,
                  req.max_files != null ? `cap ${req.max_files}` : null,
                  req.expires_at ? `expires ${formatRelative(req.expires_at)}` : null,
                  `created ${formatRelative(req.created_at)}`,
                ]
                  .filter(Boolean)
                  .join(", ");
                return (
                  <li
                    key={req.id}
                    className="group flex items-center justify-between gap-3 px-4 py-3 transition-colors hover:bg-accent/50 focus-within:bg-accent/40"
                  >
                    <div className="flex items-center gap-3 min-w-0">
                      <Inbox className="h-4 w-4 text-muted-foreground shrink-0" aria-hidden />
                      <div className="min-w-0">
                        <p className="text-sm font-medium flex items-center gap-1.5 min-w-0">
                          <FolderIcon className="h-3.5 w-3.5 text-muted-foreground shrink-0" aria-hidden />
                          <span className="truncate" title={folder}>{folder}</span>
                          {req.has_password && (
                            <Lock
                              className="h-3 w-3 text-muted-foreground shrink-0"
                              aria-label="Password-protected"
                            />
                          )}
                        </p>
                        <p className="text-xs text-muted-foreground truncate" aria-label={a11ySummary}>
                          <span aria-hidden>
                            {uploads}
                            {cap}
                            {expires}
                            {created}
                          </span>
                        </p>
                      </div>
                    </div>
                    <div className="flex items-center gap-1.5 shrink-0">
                      <Badge variant="outline" className="text-xs">guest upload</Badge>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8"
                        onClick={() => copyUploadLink(req.token)}
                        aria-label={`Copy upload link for ${folder}`}
                        title="Copy /u/… link"
                      >
                        <Copy className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 text-destructive hover:text-destructive transition-opacity sm:opacity-0 sm:group-hover:opacity-100 sm:group-focus-within:opacity-100"
                        onClick={() => removeUploadRequest(req)}
                        aria-label={`Delete upload link for ${folder}`}
                        title="Delete"
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </li>
                );
              })}
            </ul>
          )}
        </TabsContent>

        <TabsContent value="expired" className="mt-0 focus-visible:outline-none">
          {linksLoading && <SkeletonList />}
          {!linksLoading && linksError && (
            <ErrorState message="Couldn't load expired links." onRetry={() => mutateLinks()} />
          )}
          {!linksLoading && !linksError && !expired.length && (
            <EmptyState
              icon={<Clock className="h-6 w-6" aria-hidden />}
              title="No expired links"
              description="Links that have hit their expiry date or download cap will appear here."
            />
          )}
          {!!expired.length && (
            <ShareLinkList shares={expired} onCopy={copyLink} onRemove={removeShare} expired />
          )}
        </TabsContent>
      </Tabs>

      {previewFile && (
        <PreviewModal
          file={previewFile}
          open={!!previewFile}
          onOpenChange={(open) => {
            if (!open) setPreviewFile(null);
          }}
        />
      )}
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
    <ul
      role="list"
      aria-label={expired ? "Expired share links" : "Active share links"}
      className="border rounded-lg divide-y bg-card overflow-hidden"
    >
      {shares.map((share) => {
        const name = share.file_name ?? "Folder share";
        const created = formatRelative(share.created_at);
        const expiresPart = share.expires_at ? ` · expires ${formatRelative(share.expires_at)}` : "";
        const downloadPart =
          share.download_limit !== undefined
            ? ` · ${share.download_count}/${share.download_limit} downloads`
            : "";
        const a11yMeta = [
          expired ? "expired link" : `${share.permission} access link`,
          `created ${created}`,
          share.expires_at ? `expires ${formatRelative(share.expires_at)}` : null,
          share.download_limit !== undefined
            ? `${share.download_count} of ${share.download_limit} downloads used`
            : null,
        ]
          .filter(Boolean)
          .join(", ");
        return (
          <li
            key={share.id}
            className="group flex items-center justify-between gap-3 px-4 py-3 transition-colors hover:bg-accent/50 focus-within:bg-accent/40"
          >
            <div className="flex items-center gap-3 min-w-0">
              {expired ? (
                <Clock className="h-4 w-4 text-muted-foreground shrink-0" aria-hidden />
              ) : (
                <Link className="h-4 w-4 text-muted-foreground shrink-0" aria-hidden />
              )}
              <div className="min-w-0">
                <p
                  className={cn(
                    "text-sm font-medium truncate",
                    expired && "text-muted-foreground",
                  )}
                  title={name}
                >
                  {name}
                </p>
                <p className="text-xs text-muted-foreground truncate" aria-label={`${name}, ${a11yMeta}`}>
                  <span aria-hidden>
                    {created}
                    {expiresPart}
                    {downloadPart}
                  </span>
                </p>
              </div>
            </div>
            <div className="flex items-center gap-1.5 shrink-0">
              <Badge
                variant={expired ? "secondary" : "outline"}
                className={cn("text-xs", !expired && "capitalize")}
              >
                {expired ? "expired" : share.permission}
              </Badge>
              {!expired && (
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8"
                  onClick={() => onCopy(share.token)}
                  aria-label={`Copy share link for ${name}`}
                  title="Copy link"
                >
                  <Copy className="h-4 w-4" />
                </Button>
              )}
              <Button
                variant="ghost"
                size="icon"
                className="h-8 w-8 text-destructive hover:text-destructive transition-opacity sm:opacity-0 sm:group-hover:opacity-100 sm:group-focus-within:opacity-100"
                onClick={() => onRemove(share)}
                aria-label={`Delete share link for ${name}`}
                title="Delete"
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          </li>
        );
      })}
    </ul>
  );
}

function SkeletonList() {
  return (
    <div
      className="border rounded-lg divide-y bg-card overflow-hidden"
      role="status"
      aria-live="polite"
      aria-busy="true"
      aria-label="Loading"
    >
      {Array.from({ length: 4 }).map((_, i) => (
        <div key={i} className="flex items-center gap-3 px-4 py-3">
          <Skeleton className="h-4 w-4 rounded shrink-0" />
          <div className="flex-1 min-w-0 space-y-1.5">
            <Skeleton className="h-3.5 w-1/2 max-w-[260px]" />
            <Skeleton className="h-2.5 w-32" />
          </div>
          <Skeleton className="h-5 w-14 rounded-full shrink-0" />
          <Skeleton className="h-8 w-8 rounded-md shrink-0" />
        </div>
      ))}
      <span className="sr-only">Loading shared items</span>
    </div>
  );
}

function EmptyState({
  icon,
  title,
  description,
}: {
  icon: React.ReactNode;
  title: string;
  description: string;
}) {
  return (
    <div
      className="flex flex-col items-center justify-center text-center py-12 px-6 rounded-lg border border-dashed bg-card/40"
      role="status"
    >
      <div className="flex h-12 w-12 items-center justify-center rounded-full bg-muted text-muted-foreground mb-3">
        {icon}
      </div>
      <h2 className="text-sm font-semibold">{title}</h2>
      <p className="mt-1 text-xs sm:text-sm text-muted-foreground max-w-sm leading-relaxed">
        {description}
      </p>
    </div>
  );
}

function ErrorState({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div
      className="flex flex-col items-center justify-center text-center py-10 px-6 rounded-lg border border-dashed border-destructive/40 bg-destructive/5"
      role="alert"
    >
      <div className="flex h-12 w-12 items-center justify-center rounded-full bg-destructive/10 text-destructive mb-3">
        <AlertTriangle className="h-6 w-6" aria-hidden />
      </div>
      <h2 className="text-sm font-semibold">Something went wrong</h2>
      <p className="mt-1 text-xs sm:text-sm text-muted-foreground max-w-sm">{message}</p>
      <Button
        variant="outline"
        size="sm"
        className="mt-4"
        onClick={onRetry}
        aria-label="Retry loading"
      >
        Try again
      </Button>
    </div>
  );
}
