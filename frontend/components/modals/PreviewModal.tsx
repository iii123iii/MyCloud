"use client";

import { useState, useEffect, useRef } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Download, X } from "lucide-react";
import { files as filesApi, tokenStore } from "@/lib/api";
import { isPreviewable, formatBytes } from "@/lib/format";
import type { FileItem } from "@/lib/types";

interface Props {
  file: FileItem;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function PreviewModal({ file, open, onOpenChange }: Props) {
  const [blobUrl, setBlobUrl] = useState<string | null>(null);
  const [textContent, setTextContent] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const blobUrlRef = useRef<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setLoading(true);
    setBlobUrl(null);
    setTextContent(null);

    const token = tokenStore.getAccess();
    fetch(filesApi.previewUrl(file.id), {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    })
      .then(async (res) => {
        if (file.mime_type.startsWith("text/")) {
          const text = await res.text();
          setTextContent(text);
        } else {
          const blob = await res.blob();
          const url = URL.createObjectURL(blob);
          blobUrlRef.current = url;
          setBlobUrl(url);
        }
      })
      .catch(console.error)
      .finally(() => setLoading(false));

    return () => {
      if (blobUrlRef.current) {
        URL.revokeObjectURL(blobUrlRef.current);
        blobUrlRef.current = null;
      }
    };
  }, [open, file.id, file.mime_type]);

  const handleDownload = async () => {
    try {
      const token = tokenStore.getAccess();
      const res = await fetch(filesApi.downloadUrl(file.id), {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });
      if (!res.ok) throw new Error("Download failed");
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = file.name;
      a.click();
      URL.revokeObjectURL(url);
    } catch {
      // Silently fail — user sees error in network tab
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false} className="max-w-5xl w-full max-h-[90vh] flex flex-col p-0 gap-0">
        <DialogHeader className="flex flex-row items-center justify-between px-4 py-3 border-b shrink-0">
          <DialogTitle className="text-base font-medium truncate pr-4">{file.name}</DialogTitle>
          <div className="flex items-center gap-2 shrink-0">
            <span className="text-xs text-muted-foreground">{formatBytes(file.size_bytes)}</span>
            <Button variant="outline" size="sm" onClick={handleDownload}>
              <Download className="h-4 w-4 mr-1.5" />
              Download
            </Button>
            <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => onOpenChange(false)}>
              <X className="h-4 w-4" />
            </Button>
          </div>
        </DialogHeader>

        <div className="flex-1 overflow-auto min-h-0 p-4 flex items-center justify-center bg-muted/20">
          {loading && (
            <p className="text-muted-foreground text-sm">Loading preview…</p>
          )}

          {!loading && !isPreviewable(file.mime_type) && (
            <div className="text-center space-y-2">
              <p className="text-muted-foreground text-sm">Preview not available for this file type.</p>
              <Button variant="outline" onClick={handleDownload}>
                <Download className="h-4 w-4 mr-1.5" /> Download to view
              </Button>
            </div>
          )}

          {!loading && blobUrl && file.mime_type.startsWith("image/") && (
            // eslint-disable-next-line @next/next/no-img-element
            <img src={blobUrl} alt={file.name} className="max-w-full max-h-full object-contain rounded" />
          )}

          {!loading && blobUrl && file.mime_type.startsWith("video/") && (
            <video src={blobUrl} controls className="max-w-full max-h-full rounded" />
          )}

          {!loading && blobUrl && file.mime_type.startsWith("audio/") && (
            <div className="w-full max-w-lg space-y-3">
              <p className="text-sm font-medium text-center">{file.name}</p>
              <audio src={blobUrl} controls className="w-full" />
            </div>
          )}

          {!loading && blobUrl && file.mime_type === "application/pdf" && (
            <iframe src={blobUrl} className="w-full h-full min-h-[60vh] rounded border" title={file.name} />
          )}

          {!loading && textContent !== null && (
            <pre className="w-full h-full overflow-auto text-xs font-mono bg-muted rounded p-4 whitespace-pre-wrap">
              {textContent}
            </pre>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
