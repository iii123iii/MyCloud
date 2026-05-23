"use client";

// "Request files" dialog for a specific folder.
//
// Right-click a folder → Request files → this dialog opens, lets the user
// set expiry / password / max-files, then POSTs to /api/v2/upload-requests
// and shows the public URL the user can share with anyone. The token's
// uploads land in the specified folder, owned by the calling user, counted
// against their quota — anonymous uploaders never see anyone else's files.

import { useState } from "react";
import { toast } from "sonner";
import {
  CalendarClock,
  Hash,
  Lock,
  Copy,
  Check,
  Loader2,
  Link as LinkIcon,
  Sparkles,
  PartyPopper,
} from "lucide-react";

import { requests as requestsApi } from "@/lib/api";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter,
  DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import {
  Tooltip, TooltipContent, TooltipProvider, TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  folderId: string;
  folderName: string;
}

export function RequestFilesDialog({ open, onOpenChange, folderId, folderName }: Props) {
  const [maxFiles, setMaxFiles] = useState<string>("");
  const [expiresInDays, setExpiresInDays] = useState<string>("7");
  const [password, setPassword] = useState<string>("");
  const [busy, setBusy] = useState(false);
  // After creation we hold the URL so the user can copy it before closing.
  const [createdUrl, setCreatedUrl] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const reset = () => {
    setMaxFiles("");
    setExpiresInDays("7");
    setPassword("");
    setCreatedUrl(null);
    setCopied(false);
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    try {
      const payload: {
        folder_id?: string;
        expires_at?: string;
        max_files?: number;
        password?: string;
      } = { folder_id: folderId };
      if (maxFiles.trim() !== "") {
        const n = parseInt(maxFiles, 10);
        if (Number.isFinite(n) && n > 0) payload.max_files = n;
      }
      if (expiresInDays.trim() !== "") {
        const d = parseInt(expiresInDays, 10);
        if (Number.isFinite(d) && d > 0) {
          const t = new Date();
          t.setDate(t.getDate() + d);
          payload.expires_at = t.toISOString();
        }
      }
      if (password.trim() !== "") payload.password = password.trim();
      const res = await requestsApi.create(payload);
      // Backend returns `url` relative — build the absolute version so the
      // user can paste it anywhere and it actually resolves.
      const absolute = typeof window !== "undefined"
        ? `${window.location.origin}${res.url}`
        : res.url;
      setCreatedUrl(absolute);
      toast.success("Upload link ready");
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to create";
      toast.error(msg);
    } finally {
      setBusy(false);
    }
  };

  const copy = async () => {
    if (!createdUrl) return;
    try {
      await navigator.clipboard.writeText(createdUrl);
      setCopied(true);
      toast.success("Link copied");
      setTimeout(() => setCopied(false), 1500);
    } catch {
      toast.error("Could not copy");
    }
  };

  // Small summary of the constraints chosen on the configure step, so the
  // ready screen reminds the user what they just made without scrolling back.
  const summaryBits: string[] = [];
  if (expiresInDays.trim() !== "") {
    const d = parseInt(expiresInDays, 10);
    if (Number.isFinite(d) && d > 0) summaryBits.push(`expires in ${d} day${d === 1 ? "" : "s"}`);
  } else {
    summaryBits.push("never expires");
  }
  if (maxFiles.trim() !== "") {
    const n = parseInt(maxFiles, 10);
    if (Number.isFinite(n) && n > 0) summaryBits.push(`up to ${n} file${n === 1 ? "" : "s"}`);
  }
  if (password.trim() !== "") summaryBits.push("password protected");

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) reset();
        onOpenChange(o);
      }}
    >
      <DialogContent className="max-w-md max-h-[90vh] grid-rows-[auto_1fr_auto] overflow-hidden">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 min-w-0">
            {createdUrl ? (
              <>
                <PartyPopper className="size-5 text-primary shrink-0" />
                <span className="truncate">Your link is ready</span>
              </>
            ) : (
              <>
                <LinkIcon className="size-5 shrink-0" />
                <span className="truncate">Request files</span>
              </>
            )}
          </DialogTitle>
          <DialogDescription className="min-w-0">
            {createdUrl ? (
              <>
                Share this link with anyone you want to receive files from — uploads will land in{" "}
                <TooltipProvider>
                  <Tooltip>
                    <TooltipTrigger
                      render={
                        <strong className="inline-block max-w-[14ch] sm:max-w-[20ch] truncate align-bottom cursor-default">
                          {folderName}
                        </strong>
                      }
                    />
                    <TooltipContent className="max-w-xs break-all">{folderName}</TooltipContent>
                  </Tooltip>
                </TooltipProvider>
                .
              </>
            ) : (
              <>
                Generate a public link. Anyone with the URL can upload — files land in{" "}
                <TooltipProvider>
                  <Tooltip>
                    <TooltipTrigger
                      render={
                        <strong className="inline-block max-w-[14ch] sm:max-w-[20ch] truncate align-bottom cursor-default">
                          {folderName}
                        </strong>
                      }
                    />
                    <TooltipContent className="max-w-xs break-all">{folderName}</TooltipContent>
                  </Tooltip>
                </TooltipProvider>{" "}
                under your account.
              </>
            )}
          </DialogDescription>
        </DialogHeader>

        {createdUrl ? (
          // ---------- Ready screen ----------
          <div
            key="ready"
            className="space-y-4 animate-in fade-in-0 slide-in-from-bottom-1 duration-200 min-h-0 overflow-y-auto"
          >
            <div className="rounded-lg border bg-muted/40 p-3 space-y-2 min-w-0">
              <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <Sparkles className="size-3.5 shrink-0" />
                <span className="truncate">Public upload link</span>
              </div>
              <div className="flex flex-col gap-2 sm:flex-row min-w-0">
                <Input
                  value={createdUrl}
                  readOnly
                  onFocus={(e) => e.currentTarget.select()}
                  className={cn(
                    "text-xs font-mono bg-background min-w-0 flex-1",
                  )}
                />
                <Button
                  variant={copied ? "default" : "outline"}
                  size="icon"
                  onClick={copy}
                  aria-label={copied ? "Copied" : "Copy link"}
                  className="transition-colors shrink-0 self-end sm:self-auto"
                >
                  {copied
                    ? <Check className="size-4 animate-in zoom-in-50 duration-150" />
                    : <Copy className="size-4" />}
                </Button>
              </div>
              {summaryBits.length > 0 && (
                <p className="text-[11px] text-muted-foreground pt-0.5 break-words">
                  {summaryBits.join(" · ")}
                </p>
              )}
            </div>

            <Separator />
          </div>
        ) : (
          // ---------- Configure screen ----------
          <form
            key="configure"
            onSubmit={submit}
            id="request-files-form"
            className="space-y-4 animate-in fade-in-0 slide-in-from-bottom-1 duration-200 min-h-0 overflow-y-auto"
          >
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div className="space-y-1.5 min-w-0">
                <Label htmlFor="expires" className="gap-1.5">
                  <CalendarClock className="size-3.5 shrink-0" />
                  Expiry
                </Label>
                <Input
                  id="expires"
                  type="number"
                  min="1"
                  placeholder="Never"
                  value={expiresInDays}
                  onChange={(e) => setExpiresInDays(e.target.value)}
                  className="w-full min-w-0"
                />
                <p className="text-[11px] text-muted-foreground leading-tight break-words">
                  Days until the link stops accepting uploads.
                </p>
              </div>
              <div className="space-y-1.5 min-w-0">
                <Label htmlFor="max" className="gap-1.5">
                  <Hash className="size-3.5 shrink-0" />
                  Max files
                </Label>
                <Input
                  id="max"
                  type="number"
                  min="1"
                  placeholder="Unlimited"
                  value={maxFiles}
                  onChange={(e) => setMaxFiles(e.target.value)}
                  className="w-full min-w-0"
                />
                <p className="text-[11px] text-muted-foreground leading-tight break-words">
                  Total uploads allowed across all visitors.
                </p>
              </div>
            </div>

            <div className="space-y-1.5 min-w-0">
              <Label htmlFor="pwd" className="gap-1.5 flex-wrap">
                <Lock className="size-3.5 shrink-0" />
                Password
                <span className="font-normal text-muted-foreground">(optional)</span>
              </Label>
              <Input
                id="pwd"
                type="password"
                placeholder="Leave blank for none"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="new-password"
                className="w-full min-w-0"
              />
              <p className="text-[11px] text-muted-foreground leading-tight break-words">
                Visitors must enter this before they can upload.
              </p>
            </div>
          </form>
        )}

        {createdUrl ? (
          <DialogFooter className="gap-2">
            <Button variant="ghost" onClick={reset}>
              Create another
            </Button>
            <Button onClick={() => onOpenChange(false)}>
              Done
            </Button>
          </DialogFooter>
        ) : (
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={busy}
            >
              Cancel
            </Button>
            <Button type="submit" form="request-files-form" disabled={busy}>
              {busy
                ? <Loader2 className="size-4 animate-spin" />
                : <LinkIcon className="size-4" />}
              {busy ? "Generating…" : "Generate link"}
            </Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}
