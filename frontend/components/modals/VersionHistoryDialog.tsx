"use client";

import useSWR from "swr";
import { toast } from "sonner";
import { History, Download, Eye, RotateCcw } from "lucide-react";

import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";
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

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <History className="h-4 w-4" />
            Version history
          </DialogTitle>
          <DialogDescription>
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
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Version</TableHead>
                <TableHead>Size</TableHead>
                <TableHead>When</TableHead>
                <TableHead>By</TableHead>
                <TableHead></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.versions.map((v) => (
                <TableRow key={v.version_no}>
                  <TableCell className="font-mono text-xs">v{v.version_no}</TableCell>
                  <TableCell className="text-xs">{formatBytes(v.size_bytes)}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">{formatRelative(v.created_at)}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">{v.username ?? "—"}</TableCell>
                  <TableCell className="text-right">
                    <div className="flex justify-end gap-1">
                      {onPreviewVersion && (
                        <Button
                          size="icon"
                          variant="ghost"
                          className="h-7 w-7"
                          onClick={() => onPreviewVersion(v)}
                          disabled={!!fileMime && !isPreviewable(fileMime)}
                          aria-label="Preview"
                          title={
                            fileMime && !isPreviewable(fileMime)
                              ? "No preview for this file type"
                              : "Preview this version"
                          }
                        >
                          <Eye className="h-3.5 w-3.5" />
                        </Button>
                      )}
                      <Button size="icon" variant="ghost" className="h-7 w-7"
                        onClick={() => download(v.version_no)} aria-label="Download">
                        <Download className="h-3.5 w-3.5" />
                      </Button>
                      <Button size="icon" variant="ghost" className="h-7 w-7"
                        onClick={() => restore(v.version_no)} aria-label="Restore">
                        <RotateCcw className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </DialogContent>
    </Dialog>
  );
}
