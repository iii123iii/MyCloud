"use client";

import { useState } from "react";
import useSWR from "swr";
import { toast } from "sonner";
import {
  Check,
  Copy,
  Eye,
  Link as LinkIcon,
  Pencil,
  UserCircle2,
  Users,
  X,
} from "lucide-react";

import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { shares as sharesApi, grants as grantsApi } from "@/lib/api";
import { useAuth } from "@/hooks/useAuth";
import type { GrantPermission } from "@/lib/types";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  fileId?: string;
  folderId?: string;
  fileName?: string;
}

export function ShareDialog({ open, onOpenChange, fileId, folderId, fileName }: Props) {
  const { user } = useAuth();
  const [shareUrl, setShareUrl] = useState<string | null>(null);
  const [permission, setPermission] = useState<"read" | "write">("read");
  const [password, setPassword] = useState("");
  const [expiresAt, setExpiresAt] = useState("");
  const [downloadLimit, setDownloadLimit] = useState("");
  // One-time-view shares. Server enforces atomic single-use via
  // viewed_at + single_view in the download path.
  const [singleView, setSingleView] = useState(false);
  const [creating, setCreating] = useState(false);
  const [copied, setCopied] = useState(false);

  // Per-user grant state
  const [grantee, setGrantee] = useState("");
  const [grantPermission, setGrantPermission] = useState<GrantPermission>("viewer");
  const [granting, setGranting] = useState(false);

  // Inline validation: download_limit must be a positive integer when present.
  const limitNum = parseInt(downloadLimit, 10);
  const downloadLimitInvalid =
    downloadLimit.trim() !== "" && (isNaN(limitNum) || limitNum < 1);

  // Only fetch grants when dialog is open
  const grantsKey = open ? ["grants-on", fileId, folderId] : null;
  const { data: grantsData, mutate: refetchGrants } = useSWR(grantsKey, async () => {
    const res = await grantsApi.list("outgoing");
    return {
      grants: res.grants.filter((g) => (fileId ? g.file_id === fileId : g.folder_id === folderId)),
    };
  });

  const createLink = async () => {
    if (downloadLimitInvalid) return;
    setCreating(true);
    try {
      const res = await sharesApi.create({
        file_id:        fileId,
        folder_id:      folderId,
        permission,
        password:       password || undefined,
        expires_at:     expiresAt || undefined,
        download_limit: !isNaN(limitNum) && limitNum > 0 ? limitNum : undefined,
        single_view:    singleView || undefined,
      });
      const url = `${window.location.origin}/s/${res.token}`;
      setShareUrl(url);
      toast.success("Share link created");
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to create share");
    } finally {
      setCreating(false);
    }
  };

  const addGrant = async () => {
    if (!grantee.trim()) return;
    setGranting(true);
    try {
      await grantsApi.create({
        file_id: fileId,
        folder_id: folderId,
        grantee: grantee.trim(),
        permission: grantPermission,
      });
      setGrantee("");
      refetchGrants();
      toast.success(`Shared with ${grantee.trim()}`);
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to share");
    } finally {
      setGranting(false);
    }
  };

  const revokeGrant = async (id: string) => {
    try {
      await grantsApi.delete(id);
      refetchGrants();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to revoke");
    }
  };

  const changeGrantPermission = async (id: string, permission: GrantPermission) => {
    try {
      await grantsApi.update(id, permission);
      refetchGrants();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to update permission");
    }
  };

  const copy = async () => {
    if (!shareUrl) return;
    try {
      await navigator.clipboard.writeText(shareUrl);
    } catch {
      // Clipboard may be unavailable (e.g. insecure context); still flash feedback.
    }
    setCopied(true);
    toast.success("Link copied to clipboard");
    window.setTimeout(() => setCopied(false), 1000);
  };

  const handleClose = () => {
    setShareUrl(null);
    setPassword("");
    setExpiresAt("");
    setDownloadLimit("");
    setGrantee("");
    setCopied(false);
    onOpenChange(false);
  };

  const kind = fileId ? "file" : folderId ? "folder" : "item";

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="flex max-h-[calc(100vh-2rem)] w-[calc(100vw-2rem)] max-w-md flex-col gap-4 overflow-hidden sm:max-w-lg">
        <TooltipProvider delay={300}>
        <DialogHeader className="shrink-0 space-y-2">
          <DialogTitle className="flex min-w-0 items-center gap-2">
            <LinkIcon className="h-4 w-4 shrink-0" />
            <span className="truncate">Share {kind}</span>
          </DialogTitle>
          {fileName && (
            <DialogDescription className="truncate font-mono text-xs" title={fileName}>
              {fileName}
            </DialogDescription>
          )}
          <DialogDescription className="flex min-w-0 items-center gap-1.5 text-xs">
            <UserCircle2 className="h-3.5 w-3.5 shrink-0" />
            <span className="truncate">
              Sharing as{" "}
              <span className="font-medium text-foreground">
                {user?.username ?? "you"}
              </span>
            </span>
          </DialogDescription>
        </DialogHeader>

        <Tabs defaultValue="people" className="flex min-h-0 flex-1 flex-col">
          <TabsList className="w-full shrink-0">
            <TabsTrigger value="people" className="min-w-0 flex-1">
              <Users className="h-4 w-4 shrink-0" />
              <span className="truncate">People</span>
            </TabsTrigger>
            <TabsTrigger value="link" className="min-w-0 flex-1">
              <LinkIcon className="h-4 w-4 shrink-0" />
              <span className="truncate">Public link</span>
            </TabsTrigger>
          </TabsList>

          <TabsContent value="people" className="mt-4 flex min-h-0 flex-1 flex-col gap-4">
            {/* Pinned add-person controls. */}
            <div className="shrink-0 space-y-2">
              <Label htmlFor="share-grantee">Add person</Label>
              <div className="flex min-w-0 flex-col gap-2 sm:flex-row">
                <Input
                  id="share-grantee"
                  placeholder="username or email"
                  value={grantee}
                  onChange={(e) => setGrantee(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && grantee.trim() && !granting) {
                      e.preventDefault();
                      addGrant();
                    }
                  }}
                  className="min-w-0 flex-1"
                />
                <Select value={grantPermission} onValueChange={(v) => setGrantPermission(v as GrantPermission)}>
                  <SelectTrigger className="w-full sm:w-32 sm:shrink-0">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent side="bottom" sideOffset={4} alignItemWithTrigger={false}>
                    <SelectItem value="viewer">Viewer</SelectItem>
                    <SelectItem value="editor">Editor</SelectItem>
                    <SelectItem value="owner">Owner</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <Button className="w-full" onClick={addGrant} disabled={granting || !grantee.trim()}>
                {granting ? "Sharing…" : "Share"}
              </Button>
            </div>

            {grantsData?.grants && grantsData.grants.length > 0 && (
              <div className="flex min-h-0 flex-1 flex-col gap-2">
                <Separator className="shrink-0" />
                <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-hidden">
                  <Label className="shrink-0">Already shared with</Label>
                  <div className="max-h-[60vh] min-h-0 flex-1 divide-y overflow-y-auto overflow-x-hidden rounded border">
                    {grantsData.grants.map((g) => {
                      const label = g.grantee_name ?? g.grantee_id;
                      return (
                        <div key={g.id} className="flex min-w-0 items-center justify-between gap-2 px-2.5 py-1.5 text-sm">
                          <Tooltip>
                            <TooltipTrigger
                              render={
                                <span className="min-w-0 flex-1 truncate cursor-default">
                                  {label}
                                </span>
                              }
                            />
                            <TooltipContent className="max-w-[min(20rem,90vw)] break-all">
                              {label}
                            </TooltipContent>
                          </Tooltip>
                          <div className="flex shrink-0 items-center gap-1">
                            <Select value={g.permission} onValueChange={(v) => changeGrantPermission(g.id, v as GrantPermission)}>
                              <SelectTrigger className="h-7 w-[104px] text-xs capitalize" aria-label={`Change ${label}'s permission`}>
                                <SelectValue />
                              </SelectTrigger>
                              <SelectContent side="bottom" sideOffset={4} alignItemWithTrigger={false}>
                                <SelectItem value="viewer">Viewer</SelectItem>
                                <SelectItem value="editor">Editor</SelectItem>
                                <SelectItem value="owner">Owner</SelectItem>
                              </SelectContent>
                            </Select>
                            <Button variant="ghost" size="icon" className="h-6 w-6"
                              onClick={() => revokeGrant(g.id)} aria-label="Revoke">
                              <X className="h-3.5 w-3.5" />
                            </Button>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                </div>
              </div>
            )}
          </TabsContent>

          <TabsContent value="link" className="mt-4 flex min-h-0 flex-1 flex-col gap-4">
            {!shareUrl ? (
              <>
              <div className="max-h-[60vh] min-h-0 flex-1 space-y-4 overflow-y-auto overflow-x-hidden pr-1">
                <div className="space-y-2">
                  <Label>Permission</Label>
                  {/* Segmented control replaces the dropdown. */}
                  <div
                    role="radiogroup"
                    aria-label="Permission"
                    className="flex w-full flex-wrap rounded-md border bg-muted p-0.5"
                  >
                    <button
                      type="button"
                      role="radio"
                      aria-checked={permission === "read"}
                      onClick={() => setPermission("read")}
                      className={cn(
                        "flex min-w-0 flex-1 basis-0 items-center justify-center gap-1.5 rounded-[5px] px-3 py-1.5 text-sm font-medium transition-colors",
                        permission === "read"
                          ? "bg-background text-foreground shadow-sm"
                          : "text-muted-foreground hover:text-foreground",
                      )}
                    >
                      <Eye className="h-3.5 w-3.5 shrink-0" />
                      <span className="truncate">View only</span>
                    </button>
                    <button
                      type="button"
                      role="radio"
                      aria-checked={permission === "write"}
                      onClick={() => setPermission("write")}
                      className={cn(
                        "flex min-w-0 flex-1 basis-0 items-center justify-center gap-1.5 rounded-[5px] px-3 py-1.5 text-sm font-medium transition-colors",
                        permission === "write"
                          ? "bg-background text-foreground shadow-sm"
                          : "text-muted-foreground hover:text-foreground",
                      )}
                    >
                      <Pencil className="h-3.5 w-3.5 shrink-0" />
                      <span className="truncate">Can edit</span>
                    </button>
                  </div>
                </div>

                <Separator />

                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="min-w-0 space-y-2">
                    <Label htmlFor="share-password">Password</Label>
                    <Input
                      id="share-password"
                      type="password"
                      placeholder="Leave blank for none"
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      className="w-full"
                    />
                  </div>
                  <div className="min-w-0 space-y-2">
                    <Label htmlFor="share-expires" className="truncate">Expiry — leave blank for never</Label>
                    <Input
                      id="share-expires"
                      type="datetime-local"
                      value={expiresAt}
                      onChange={(e) => setExpiresAt(e.target.value)}
                      className="w-full"
                    />
                  </div>
                </div>

                <div className="min-w-0 space-y-2">
                  <Label htmlFor="share-limit">Download limit</Label>
                  <Input
                    id="share-limit"
                    type="number"
                    min={1}
                    inputMode="numeric"
                    placeholder="Unlimited"
                    value={downloadLimit}
                    onChange={(e) => setDownloadLimit(e.target.value)}
                    aria-invalid={downloadLimitInvalid || undefined}
                    aria-describedby={downloadLimitInvalid ? "share-limit-error" : undefined}
                    className={cn(
                      "w-full",
                      downloadLimitInvalid && "border-destructive focus-visible:ring-destructive/40",
                    )}
                  />
                  {downloadLimitInvalid && (
                    <p id="share-limit-error" className="text-xs text-destructive">
                      Must be a positive whole number.
                    </p>
                  )}
                </div>

                <Separator />

                {/* One-time-view checkbox. Sets single_view=true; the
                    server atomically expires the share on first download. */}
                <label className="flex min-w-0 cursor-pointer select-none items-start gap-2.5 rounded-md border p-3 text-sm transition-colors hover:bg-muted/40">
                  <input
                    type="checkbox"
                    className="mt-0.5 h-4 w-4 shrink-0 accent-primary"
                    checked={singleView}
                    onChange={(e) => setSingleView(e.target.checked)}
                  />
                  <span className="min-w-0 space-y-0.5">
                    <span className="block font-medium">One-time view</span>
                    <span className="block text-xs text-muted-foreground">
                      Link self-destructs after the first download. Good for
                      sensitive documents.
                    </span>
                  </span>
                </label>
              </div>

              {/* Pinned primary action footer. */}
              <div className="shrink-0">
                <Button
                  className="w-full"
                  onClick={createLink}
                  disabled={creating || downloadLimitInvalid}
                >
                  {creating ? "Creating…" : "Create share link"}
                </Button>
              </div>
              </>
            ) : (
              <>
              <div className="max-h-[60vh] min-h-0 flex-1 space-y-3 overflow-y-auto overflow-x-hidden pr-1">
                <p className="text-sm text-muted-foreground">Your share link is ready:</p>
                <div className="flex min-w-0 items-center gap-2">
                  <Tooltip>
                    <TooltipTrigger
                      render={
                        <Input
                          value={shareUrl}
                          readOnly
                          className="min-w-0 flex-1 truncate font-mono text-xs"
                          title={shareUrl}
                        />
                      }
                    />
                    <TooltipContent className="max-w-[min(24rem,90vw)] break-all font-mono">
                      {shareUrl}
                    </TooltipContent>
                  </Tooltip>
                  <Button
                    variant="outline"
                    size="icon"
                    onClick={copy}
                    aria-label={copied ? "Copied" : "Copy link"}
                    className="shrink-0"
                  >
                    {copied ? (
                      <Check className="h-4 w-4 text-emerald-600 dark:text-emerald-400" />
                    ) : (
                      <Copy className="h-4 w-4" />
                    )}
                  </Button>
                </div>
              </div>

              {/* Pinned primary action footer. */}
              <div className="shrink-0 space-y-3">
                <Separator />
                <Button variant="ghost" className="w-full" onClick={() => setShareUrl(null)}>
                  Create another link
                </Button>
              </div>
              </>
            )}
          </TabsContent>
        </Tabs>
        </TooltipProvider>
      </DialogContent>
    </Dialog>
  );
}
