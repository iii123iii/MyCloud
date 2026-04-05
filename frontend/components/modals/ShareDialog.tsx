"use client";

import { useState } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import { Copy, Link } from "lucide-react";

import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { shares as sharesApi } from "@/lib/api";

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
  const [creating, setCreating] = useState(false);

  const create = async () => {
    setCreating(true);
    try {
      const res = await sharesApi.create({
        file_id:   fileId,
        folder_id: folderId,
        permission,
        password:   password || undefined,
        expires_at: expiresAt || undefined,
      });
      const url = `${window.location.origin}/s/${res.token}`;
      setShareUrl(url);
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to create share");
    } finally {
      setCreating(false);
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
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Link className="h-4 w-4" />
            Share {fileName ? `"${fileName}"` : "item"}
          </DialogTitle>
          <DialogDescription>
            Create a public link anyone can use to access this {fileId ? "file" : "folder"}.
          </DialogDescription>
        </DialogHeader>

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
            <Button className="w-full" onClick={create} disabled={creating}>
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
      </DialogContent>
    </Dialog>
  );
}
