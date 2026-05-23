"use client";

import useSWR from "swr";
import { toast } from "sonner";
import { History, Download, Eye, RotateCcw, MoreHorizontal } from "lucide-react";

import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import { versions as versionsApi, type FileVersion, tokenStore } from "@/lib/api";
import { formatBytes, formatRelative, isPreviewable } from "@/lib/format";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  fileId: string;
  fileName?: string;
  /**
   * Mime type of the current file. When set, used to grey out the Preview
   * button for versions whose underlying file isn't previewable in the browser.
   */
  fileMime?: string;
  onRestored?: () => void;
  /**
   * Open this historical version in PreviewModal. The parent is responsible
   * for closing this dialog and rendering the preview.
   */
  onPreviewVersion?: (version: FileVersion) => void;
}

export function VersionHistoryDialog({
  open, onOpenChange, fileId, fileName, fileMime, onRestored, onPreviewVersion,
}: Props) {
  const swrKey = open ? ["versions", fileId] : null;
  const { data, isLoading, mutate } = useSWR(swrKey, () => versionsApi.list(fileId));

  const restore = async (versionNo: number) => {
    try {
      await versionsApi.restore(fileId, versionNo);
      mutate();
      onRestored?.();
      toast.success(`Restored version ${versionNo}`);
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to restore");
    }
  };

  const download = async (versionNo: number) => {
    const url = versionsApi.downloadUrl(fileId, versionNo);
    const token = tokenStore.getAccess();
    try {
      const res = await fetch(url, token ? { headers: { Authorization: `Bearer ${token}` } } : undefined);
      if (!res.ok) throw new Error("Download failed");
      const blob = await res.blob();
      const objURL = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = objURL;
      a.download = `${fileName ?? "file"} (v${versionNo})`;
      a.click();
      URL.revokeObjectURL(objURL);
    } catch {
      toast.error("Download failed");
    }
  };

  // Shared helper — whether previewing this version's mime type is supported.
  const previewDisabled = !!fileMime && !isPreviewable(fileMime);
  const previewTitle = previewDisabled
    ? "No preview for this file type"
    : "Preview this version";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg flex flex-col gap-4 max-h-[calc(100dvh-2rem)] overflow-hidden">
        <DialogHeader className="shrink-0">
          <DialogTitle className="flex items-center gap-2 min-w-0">
            <History className="h-4 w-4 shrink-0" />
            <span className="truncate">Version history</span>
          </DialogTitle>
          <DialogDescription className="truncate" title={fileName ?? undefined}>
            {fileName ? `Previous versions of "${fileName}"` : "Previous versions"}
          </DialogDescription>
        </DialogHeader>

        {isLoading && (
          <div className="space-y-2">
            {Array.from({ length: 3 }).map((_, i) => <Skeleton key={i} className="h-10 rounded" />)}
          </div>
        )}

        {!isLoading && !data?.versions.length && (
          <p className="text-sm text-muted-foreground py-4">No history yet. Re-upload a file with the same name in the same folder to create a version.</p>
        )}

        {!isLoading && !!data?.versions.length && (
          <TooltipProvider delay={300}>
            <div className="-mx-4 px-4 max-h-[60vh] overflow-y-auto overflow-x-hidden">
              {/* ── Desktop / tablet (sm+): table layout ── */}
              <Table className="hidden sm:table">
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-[12%]">Version</TableHead>
                    <TableHead className="w-[18%]">Size</TableHead>
                    <TableHead className="w-[24%]">When</TableHead>
                    <TableHead className="w-[22%]">By</TableHead>
                    <TableHead className="w-[24%]"></TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {data.versions.map((v) => {
                    const userLabel = v.username ?? "—";
                    return (
                      <TableRow key={v.version_no}>
                        <TableCell className="font-mono text-xs whitespace-nowrap">v{v.version_no}</TableCell>
                        <TableCell className="text-xs whitespace-nowrap tabular-nums">{formatBytes(v.size_bytes)}</TableCell>
                        <TableCell className="text-xs text-muted-foreground whitespace-nowrap tabular-nums">{formatRelative(v.created_at)}</TableCell>
                        <TableCell className="text-xs text-muted-foreground max-w-0">
                          <Tooltip>
                            <TooltipTrigger
                              render={
                                <span className="block truncate cursor-default">{userLabel}</span>
                              }
                            />
                            <TooltipContent>{userLabel}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-1">
                            {onPreviewVersion && (
                              <Button
                                size="icon"
                                variant="ghost"
                                className="h-7 w-7 shrink-0"
                                onClick={() => onPreviewVersion(v)}
                                disabled={previewDisabled}
                                aria-label="Preview"
                                title={previewTitle}
                              >
                                <Eye className="h-3.5 w-3.5" />
                              </Button>
                            )}
                            <Button size="icon" variant="ghost" className="h-7 w-7 shrink-0"
                              onClick={() => download(v.version_no)} aria-label="Download">
                              <Download className="h-3.5 w-3.5" />
                            </Button>
                            <Button size="icon" variant="ghost" className="h-7 w-7 shrink-0"
                              onClick={() => restore(v.version_no)} aria-label="Restore">
                              <RotateCcw className="h-3.5 w-3.5" />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>

              {/* ── Mobile (<sm): stacked rows + ⋮ overflow menu ── */}
              <ul className="sm:hidden divide-y">
                {data.versions.map((v) => {
                  const userLabel = v.username ?? "—";
                  return (
                    <li key={v.version_no} className="flex items-start gap-2 py-2.5">
                      <div className="flex-1 min-w-0 flex flex-col gap-0.5">
                        <div className="flex items-center gap-2 min-w-0">
                          <span className="font-mono text-xs shrink-0">v{v.version_no}</span>
                          <span className="text-xs whitespace-nowrap tabular-nums text-muted-foreground shrink-0">
                            {formatBytes(v.size_bytes)}
                          </span>
                          <span className="text-xs text-muted-foreground whitespace-nowrap tabular-nums truncate">
                            · {formatRelative(v.created_at)}
                          </span>
                        </div>
                        <Tooltip>
                          <TooltipTrigger
                            render={
                              <span className="text-xs text-muted-foreground truncate cursor-default block">
                                by {userLabel}
                              </span>
                            }
                          />
                          <TooltipContent>{userLabel}</TooltipContent>
                        </Tooltip>
                      </div>

                      <DropdownMenu>
                        <DropdownMenuTrigger
                          render={
                            <Button
                              size="icon"
                              variant="ghost"
                              className={cn("h-8 w-8 shrink-0")}
                              aria-label="Version actions"
                            >
                              <MoreHorizontal className="h-4 w-4" />
                            </Button>
                          }
                        />
                        <DropdownMenuContent
                          align="end"
                          side="bottom"
                          sideOffset={4}
                          className="min-w-40"
                        >
                          {onPreviewVersion && (
                            <DropdownMenuItem
                              onClick={() => onPreviewVersion(v)}
                              disabled={previewDisabled}
                            >
                              <Eye className="h-4 w-4" />
                              <span>Preview</span>
                            </DropdownMenuItem>
                          )}
                          <DropdownMenuItem onClick={() => download(v.version_no)}>
                            <Download className="h-4 w-4" />
                            <span>Download</span>
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => restore(v.version_no)}>
                            <RotateCcw className="h-4 w-4" />
                            <span>Restore</span>
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </li>
                  );
                })}
              </ul>
            </div>
          </TooltipProvider>
        )}

        <DialogFooter showCloseButton className="shrink-0" />
      </DialogContent>
    </Dialog>
  );
}
