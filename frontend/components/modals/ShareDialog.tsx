"use client";

import { useState } from "react";
import useSWR from "swr";
import { toast } from "sonner";
import { Copy, Link, X } from "lucide-react";

import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Badge } from "@/components/ui/badge";
import { shares as sharesApi, grants as grantsApi } from "@/lib/api";
import type { GrantPermission } from "@/lib/types";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  fileId?: string;
  folderId?: string;
  fileName?: string;
}

export function ShareDialog({ open, onOpenChange, fileId, folderId, fileName }: Props) {
  const [shareUrl, setShareUrl] = useState<string | null>(null);
  const [permission, setPermission] = useState<"read" | "write">("read");
  const [password, setPassword] = useState("");
  const [expiresAt, setExpiresAt] = useState("");
  const [downloadLimit, setDownloadLimit] = useState("");
  const [creating, setCreating] = useState(false);

  // Per-user grant state
  const [grantee, setGrantee] = useState("");
  const [grantPermission, setGrantPermission] = useState<GrantPermission>("viewer");
  const [granting, setGranting] = useState(false);

  // Only fetch grants when dialog is open
  const grantsKey = open ? ["grants-on", fileId, folderId] : null;
  const { data: grantsData, mutate: refetchGrants } = useSWR(grantsKey, async () => {
    const res = await grantsApi.list("outgoing");
    return {
      grants: res.grants.filter((g) => (fileId ? g.file_id === fileId : g.folder_id === folderId)),
    };
  });

  const createLink = async () => {
    setCreating(true);
    try {
      const limitNum = parseInt(downloadLimit, 10);
      const res = await sharesApi.create({
        file_id:        fileId,
        folder_id:      folderId,
        permission,
        password:       password || undefined,
        expires_at:     expiresAt || undefined,
        download_limit: !isNaN(limitNum) && limitNum > 0 ? limitNum : undefined,
      });
      const url = `${window.location.origin}/s/${res.token}`;
      setShareUrl(url);
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

  const copy = () => {
    if (!shareUrl) return;
    navigator.clipboard.writeText(shareUrl);
    toast.success("Link copied to clipboard");
  };

  const handleClose = () => {
    setShareUrl(null);
    setPassword("");
    setExpiresAt("");
    setDownloadLimit("");
    setGrantee("");
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Link className="h-4 w-4 shrink-0" />
            Share {fileId ? "file" : folderId ? "folder" : "item"}
          </DialogTitle>
          {fileName && (
            <DialogDescription className="truncate font-mono text-xs" title={fileName}>
              {fileName}
            </DialogDescription>
          )}
          <DialogDescription>
            Share with people or generate a public link.
          </DialogDescription>
        </DialogHeader>

        <Tabs defaultValue="people">
          <TabsList className="w-full">
            <TabsTrigger value="people">People</TabsTrigger>
            <TabsTrigger value="link">Public link</TabsTrigger>
          </TabsList>

          <TabsContent value="people" className="mt-4 space-y-3">
            <div className="space-y-1.5">
              <Label>Add person</Label>
              <div className="flex gap-2">
                <Input
                  placeholder="username or email"
                  value={grantee}
                  onChange={(e) => setGrantee(e.target.value)}
                />
                <Select value={grantPermission} onValueChange={(v) => setGrantPermission(v as GrantPermission)}>
                  <SelectTrigger className="w-28">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
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
              <>
                <Separator />
                <div className="space-y-1.5">
                  <Label>Already shared with</Label>
                  <div className="border rounded divide-y">
                    {grantsData.grants.map((g) => (
                      <div key={g.id} className="flex items-center justify-between px-2 py-1.5 gap-2 text-sm">
                        <span className="truncate">{g.grantee_name ?? g.grantee_id}</span>
                        <div className="flex items-center gap-1 shrink-0">
                          <Badge variant="outline" className="text-xs">{g.permission}</Badge>
                          <Button variant="ghost" size="icon" className="h-6 w-6"
                            onClick={() => revokeGrant(g.id)} aria-label="Revoke">
                            <X className="h-3.5 w-3.5" />
                          </Button>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              </>
            )}
          </TabsContent>

          <TabsContent value="link" className="mt-4">
            {!shareUrl ? (
              <div className="space-y-4">
                <div className="space-y-1.5">
                  <Label>Permission</Label>
                  <Select value={permission} onValueChange={(v) => setPermission(v as "read" | "write")}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="read">View only</SelectItem>
                      <SelectItem value="write">Can edit</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <Label>Password (optional)</Label>
                  <Input
                    type="password"
                    placeholder="Leave blank for no password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                  />
                </div>
                <div className="space-y-1.5">
                  <Label>Expires at (optional)</Label>
                  <Input
                    type="datetime-local"
                    value={expiresAt}
                    onChange={(e) => setExpiresAt(e.target.value)}
                  />
                </div>
                <div className="space-y-1.5">
                  <Label>Download limit (optional)</Label>
                  <Input
                    type="number"
                    min={1}
                    placeholder="Unlimited"
                    value={downloadLimit}
                    onChange={(e) => setDownloadLimit(e.target.value)}
                  />
                </div>
                <Button className="w-full" onClick={createLink} disabled={creating}>
                  {creating ? "Creating…" : "Create share link"}
                </Button>
              </div>
            ) : (
              <div className="space-y-3">
                <p className="text-sm text-muted-foreground">Your share link is ready:</p>
                <div className="flex gap-2">
                  <Input value={shareUrl} readOnly className="text-xs" />
                  <Button variant="outline" size="icon" onClick={copy}>
                    <Copy className="h-4 w-4" />
                  </Button>
                </div>
                <Separator />
                <Button variant="ghost" className="w-full" onClick={() => setShareUrl(null)}>
                  Create another link
                </Button>
              </div>
            )}
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}
