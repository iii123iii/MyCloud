"use client";

import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { AlertCircle, Loader2, Save, X } from "lucide-react";

import { Dialog, DialogContent } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { files as filesApi, tokenStore } from "@/lib/api";
import { getEditMode } from "@/lib/file-kind";
import type { FileItem } from "@/lib/types";

import { ImageAnnotator, type ImageAnnotatorRef } from "@/components/editor/ImageAnnotator";
import { TextEditor, type TextEditorRef } from "@/components/editor/TextEditor";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  file: FileItem;
  /** Called after a successful save so the parent can revalidate listings. */
  onSaved?: () => void;
}

/**
 * Fullscreen file editor. Branches on getEditMode(file):
 *  - "image" → ImageAnnotator (tldraw)
 *  - "text"  → TextEditor (CodeMirror)
 *
 * Save flow: serialise the buffer back to a File with the original name and
 * MIME, then call filesApi.upload with the original folder_id. Re-uploading
 * the same name into the same folder triggers the backend's versioning path
 * (commitUploadWithVersioning), so the previous bytes land in file_versions
 * and the file id / comments / shares / tags are preserved.
 */
export function EditFileDialog({ open, onOpenChange, file, onSaved }: Props) {
  const mode = getEditMode(file);

  // Loaded source content. For images we hand the editor a File so tldraw's
  // standard "files" handler does the heavy lifting; for text we just pass the
  // string.
  const [sourceFile, setSourceFile] = useState<File | null>(null);
  const [sourceText, setSourceText] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);

  const imageRef = useRef<ImageAnnotatorRef | null>(null);
  const textRef = useRef<TextEditorRef | null>(null);

  // ── Load file contents when the dialog opens ─────────────────────────────
  useEffect(() => {
    if (!open || mode === null) return;
    setLoading(true);
    setLoadError(null);
    setSourceFile(null);
    setSourceText(null);
    setDirty(false);

    const token = tokenStore.getAccess();
    const controller = new AbortController();

    fetch(filesApi.previewUrl(file.id), {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      signal: controller.signal,
    })
      .then(async (res) => {
        if (!res.ok) throw new Error(`Failed to load (HTTP ${res.status})`);
        if (mode === "image") {
          const blob = await res.blob();
          setSourceFile(new File([blob], file.name, { type: file.mime_type }));
        } else {
          setSourceText(await res.text());
        }
      })
      .catch((err) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        console.error(err);
        setLoadError(err instanceof Error ? err.message : "Failed to load file");
      })
      .finally(() => setLoading(false));

    return () => controller.abort();
  }, [open, mode, file.id, file.name, file.mime_type]);

  // ── Persist edits ────────────────────────────────────────────────────────
  const handleSave = async () => {
    if (saving) return;
    setSaving(true);
    try {
      let blob: Blob | null = null;
      if (mode === "image") {
        // Preserve the original raster format so the file's MIME doesn't
        // silently change. JPEG/WEBP are paint-on-white; PNG keeps alpha.
        const fmt =
          file.mime_type === "image/jpeg" || file.mime_type === "image/jpg"
            ? "jpeg"
            : file.mime_type === "image/webp"
              ? "webp"
              : "png";
        blob = (await imageRef.current?.getOutputBlob(fmt)) ?? null;
      } else if (mode === "text") {
        blob = (await textRef.current?.getOutputBlob(file.mime_type)) ?? null;
      }
      if (!blob) {
        toast.error("Nothing to save");
        return;
      }

      const formData = new FormData();
      const outFile = new File([blob], file.name, { type: file.mime_type });
      formData.append("file", outFile);
      if (file.folder_id) formData.append("folder_id", file.folder_id);

      await filesApi.upload(formData);
      toast.success("Saved — previous version kept in history");
      setDirty(false);
      onSaved?.();
      onOpenChange(false);
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Save failed");
    } finally {
      setSaving(false);
    }
  };

  const handleClose = (next: boolean) => {
    if (!next && dirty) {
      if (!window.confirm("Discard unsaved changes?")) return;
    }
    onOpenChange(next);
  };

  if (mode === null) return null;

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent
        showCloseButton={false}
        className="!fixed !inset-0 !top-0 !left-0 !translate-x-0 !translate-y-0 !w-screen !h-screen !max-w-none !max-h-none !rounded-none flex flex-col p-0 gap-0 overflow-hidden"
      >
        {/* Header strip — same layout as PreviewModal so the editor feels like
            "preview, but writable". Pinned to the top via shrink-0 so very
            large files can't push it off-screen. */}
        <div className="sticky top-0 z-10 flex flex-row items-center justify-between gap-2 px-3 sm:px-4 py-3 border-b shrink-0 bg-background max-w-full overflow-hidden">
          <div className="flex items-center gap-2 sm:gap-3 min-w-0 flex-1">
            <TooltipProvider delay={300}>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <span className="text-sm sm:text-base font-medium truncate min-w-0 max-w-full cursor-default">
                      {file.name}
                    </span>
                  }
                />
                <TooltipContent
                  side="bottom"
                  className="max-w-[min(90vw,32rem)] break-all whitespace-normal"
                >
                  {file.name}
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
            {dirty && (
              <span className="text-xs text-amber-600 dark:text-amber-400 shrink-0 whitespace-nowrap">
                · Unsaved
              </span>
            )}
          </div>
          <div className="flex items-center gap-1 sm:gap-2 shrink-0">
            <Button
              variant="default"
              size="sm"
              onClick={handleSave}
              disabled={saving || loading || !!loadError}
              className="shrink-0"
            >
              {saving ? (
                <Loader2 className="h-4 w-4 sm:mr-1.5 animate-spin" />
              ) : (
                <Save className="h-4 w-4 sm:mr-1.5" />
              )}
              <span className="hidden sm:inline">Save</span>
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8 shrink-0"
              onClick={() => handleClose(false)}
              aria-label="Close"
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
        </div>

        {/* Body — scrolls internally so the header (and any editor footer
            controls) stay pinned. min-h-0 lets the flex child actually shrink
            inside the column; max-h-[80vh] caps how tall the inner editor can
            grow before its own content starts scrolling. */}
        <div
          className={cn(
            "relative flex-1 min-h-0 bg-muted/20",
            "max-h-[80vh] overflow-y-auto overflow-x-auto",
          )}
        >
          {loading && (
            <div className="absolute inset-0 flex items-center justify-center text-muted-foreground gap-2">
              <Loader2 className="h-5 w-5 animate-spin" />
              <span className="text-sm">Loading file…</span>
            </div>
          )}

          {!loading && loadError && (
            <div className="absolute inset-0 flex flex-col items-center justify-center text-center px-6 gap-3">
              <AlertCircle className="h-8 w-8 text-muted-foreground" />
              <p className="text-sm text-muted-foreground max-w-sm break-words">
                {loadError}
              </p>
            </div>
          )}

          {!loading && !loadError && mode === "image" && sourceFile && (
            <ImageAnnotator ref={imageRef} imageFile={sourceFile} />
          )}

          {!loading && !loadError && mode === "text" && sourceText !== null && (
            <TextEditor
              ref={textRef}
              initialText={sourceText}
              mime={file.mime_type}
              onDirty={() => setDirty(true)}
            />
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
