"use client";

import { useState, useEffect, useRef } from "react";
import { toast } from "sonner";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { AlertCircle, ArrowLeft, ChevronLeft, ChevronRight, Download, Loader2, MessageSquare, Pencil, RotateCcw, X } from "lucide-react";
import { CommentsPanel } from "./CommentsPanel";
import { files as filesApi, versions as versionsApi, tokenStore } from "@/lib/api";
import {
  isPreviewable,
  formatBytes,
  formatRelative,
  PREVIEWABLE_SPREADSHEET_TYPES,
  PREVIEWABLE_WORD_TYPES,
} from "@/lib/format";
import { MIME_TO_LANG, isTextBased, getEditMode } from "@/lib/file-kind";
import type { FileItem } from "@/lib/types";

export interface PreviewVersion {
  no: number;
  createdAt: string;
  username?: string;
}

interface Props {
  file: FileItem;
  /** Full list of files in the current view — used for prev/next navigation. */
  files?: FileItem[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Called when the user navigates to a different file. */
  onNavigate?: (file: FileItem) => void;
  /**
   * When set, preview the historical bytes for this version of the file
   * instead of the current contents. Hides comments + edit; shows a Restore
   * button instead. Prev/next navigation is also suppressed.
   */
  version?: PreviewVersion;
  /** Clear the version overlay → back to the current contents. */
  onClearVersion?: () => void;
  /** Called after a successful version restore — caller should refresh state. */
  onRestored?: () => void;
  /**
   * Open the file in the editor. When set + the file is editable, an Edit
   * button appears in the header alongside Download.
   */
  onEdit?: () => void;
}

// (MIME_TO_LANG + isTextBased are imported from @/lib/file-kind so the editor
//  components can reuse the same classification.)

// ─── CSV helpers ────────────────────────────────────────────────────────────

const MAX_TABLE_ROWS = 1000;
const MAX_TEXT_PREVIEW_BYTES = 2 * 1024 * 1024;
const MAX_DOCUMENT_PREVIEW_BYTES = 10 * 1024 * 1024;
const MAX_BINARY_PREVIEW_BYTES = 25 * 1024 * 1024;

function getPreviewSizeLimitMessage(file: FileItem): string | null {
  const { mime_type: mime, size_bytes: size, name } = file;

  if (!isPreviewable(mime)) {
    return "Preview not available for this file type.";
  }
  if (isTextBased(mime, name) && size > MAX_TEXT_PREVIEW_BYTES) {
    return `This text file is too large to preview in the browser (${formatBytes(size)}). Download it instead.`;
  }
  if ((PREVIEWABLE_SPREADSHEET_TYPES.has(mime) || PREVIEWABLE_WORD_TYPES.has(mime)) && size > MAX_DOCUMENT_PREVIEW_BYTES) {
    return `This document is too large to render in the browser (${formatBytes(size)}). Download it instead.`;
  }
  if (
    (mime.startsWith("image/") || mime.startsWith("video/") || mime.startsWith("audio/") || mime === "application/pdf") &&
    size > MAX_BINARY_PREVIEW_BYTES
  ) {
    return `This file is too large for inline preview (${formatBytes(size)}). Download it instead.`;
  }
  return null;
}

/** Parse CSV text into a 2D array, handling double-quoted fields. */
function parseCsv(text: string): string[][] {
  const rows: string[][] = [];
  for (const line of text.trim().split("\n")) {
    const cells: string[] = [];
    let cur = "";
    let inQ = false;
    for (let i = 0; i < line.length; i++) {
      const ch = line[i];
      if (ch === '"') {
        if (inQ && line[i + 1] === '"') { cur += '"'; i++; }
        else { inQ = !inQ; }
      } else if (ch === "," && !inQ) {
        cells.push(cur); cur = "";
      } else {
        cur += ch;
      }
    }
    cells.push(cur);
    rows.push(cells);
  }
  return rows;
}

// ─── Spreadsheet table sub-component ────────────────────────────────────────

function SheetTable({
  sheetData,
  truncated,
}: {
  sheetData: string[][];
  truncated: boolean;
}) {
  return (
    <div className="flex-1 overflow-auto bg-background">
      <table className="text-xs border-collapse min-w-full">
        <thead>
          <tr>
            {sheetData[0]?.map((cell, i) => (
              <th
                key={i}
                className="border-b border-r border-border px-3 py-2 bg-muted text-left font-semibold sticky top-0 whitespace-nowrap"
              >
                {cell}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {sheetData.slice(1).map((row, i) => (
            <tr key={i} className={i % 2 === 0 ? "bg-background" : "bg-muted/30"}>
              {row.map((cell, j) => (
                <td key={j} className="border-b border-r border-border px-3 py-1.5 whitespace-nowrap">
                  {cell}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
      {truncated && (
        <p className="text-xs text-muted-foreground text-center py-2 border-t border-border">
          Showing first {MAX_TABLE_ROWS} rows. Download the file to see all data.
        </p>
      )}
    </div>
  );
}

// ─── Spreadsheet viewer (tabs + table) ──────────────────────────────────────

function SpreadsheetViewer({ data }: { data: Record<string, { rows: string[][]; truncated: boolean }> }) {
  const sheetNames = Object.keys(data);
  const [active, setActive] = useState(sheetNames[0] ?? "");

  return (
    <div className="w-full h-full flex flex-col rounded border border-border overflow-hidden">
      {sheetNames.length > 1 && (
        <div className="flex gap-0.5 px-2 pt-1.5 bg-muted/60 border-b border-border shrink-0 overflow-x-auto">
          {sheetNames.map((name) => (
            <button
              key={name}
              onClick={() => setActive(name)}
              className={`px-3 py-1 text-xs rounded-t-sm border border-b-0 border-border whitespace-nowrap transition-colors ${
                active === name
                  ? "bg-background font-medium"
                  : "bg-muted/40 text-muted-foreground hover:bg-muted"
              }`}
            >
              {name}
            </button>
          ))}
        </div>
      )}
      {data[active] && (
        <SheetTable sheetData={data[active].rows} truncated={data[active].truncated} />
      )}
    </div>
  );
}

// ─── Main modal ─────────────────────────────────────────────────────────────

export function PreviewModal({
  file, files = [], open, onOpenChange, onNavigate,
  version, onClearVersion, onRestored, onEdit,
}: Props) {
  const [blobUrl, setBlobUrl]                     = useState<string | null>(null);
  const [textContent, setTextContent]             = useState<string | null>(null);
  const [highlightedHtml, setHighlightedHtml]     = useState<string | null>(null);
  const [csvData, setCsvData]                     = useState<{ rows: string[][]; truncated: boolean } | null>(null);
  const [spreadsheetData, setSpreadsheetData]     = useState<Record<string, { rows: string[][]; truncated: boolean }> | null>(null);
  const [docHtml, setDocHtml]                     = useState<string | null>(null);
  const [docError, setDocError]                   = useState<string | null>(null);
  const [fetchError, setFetchError]               = useState<string | null>(null);
  const [previewNotice, setPreviewNotice]         = useState<string | null>(null);
  const [loading, setLoading]                     = useState(true);
  const [showComments, setShowComments]           = useState(false);
  const blobUrlRef = useRef<string | null>(null);

  const isVersionView = !!version;

  // ── Compute navigation neighbours (only among previewable files) ─────────
  // Disabled while viewing a historical version — the version belongs to one
  // specific file and stepping to a neighbour would conflate timelines.
  const previewableFiles = isVersionView ? [] : files.filter((f) => isPreviewable(f.mime_type));
  const currentIndex     = previewableFiles.findIndex((f) => f.id === file.id);
  const prevFile         = currentIndex > 0 ? previewableFiles[currentIndex - 1] : null;
  const nextFile         = currentIndex < previewableFiles.length - 1 ? previewableFiles[currentIndex + 1] : null;
  const hasNav           = previewableFiles.length > 1;

  // ── Keyboard navigation ──────────────────────────────────────────────────
  useEffect(() => {
    if (!open || !onNavigate) return;
    const handleKey = (e: KeyboardEvent) => {
      // Don't hijack arrow keys while the user is typing in an input / textarea.
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
      if (e.key === "ArrowLeft"  && prevFile) onNavigate(prevFile);
      if (e.key === "ArrowRight" && nextFile) onNavigate(nextFile);
    };
    window.addEventListener("keydown", handleKey);
    return () => window.removeEventListener("keydown", handleKey);
  }, [open, prevFile, nextFile, onNavigate]);

  // ── Fetch preview content ─────────────────────────────────────────────────
  useEffect(() => {
    if (!open) return;
    setLoading(true);
    setBlobUrl(null);
    setTextContent(null);
    setHighlightedHtml(null);
    setCsvData(null);
    setSpreadsheetData(null);
    setDocHtml(null);
    setDocError(null);
    setFetchError(null);
    setPreviewNotice(null);

    const mime = file.mime_type;
    const previewNotice = getPreviewSizeLimitMessage(file);

    if (previewNotice) {
      setPreviewNotice(previewNotice);
      setLoading(false);
      return;
    }

    const token = tokenStore.getAccess();
    const controller = new AbortController();

    const fetchUrl = version
      ? versionsApi.previewUrl(file.id, version.no)
      : filesApi.previewUrl(file.id);

    fetch(fetchUrl, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      signal: controller.signal,
    })
      .then(async (res) => {
        if (!res.ok) {
          throw new Error(`Preview request failed with status ${res.status}`);
        }
        // ── Spreadsheets ──────────────────────────────────────────────────
        if (PREVIEWABLE_SPREADSHEET_TYPES.has(mime)) {
          const buffer = await res.arrayBuffer();
          const XLSX = await import("xlsx");
          const wb = XLSX.read(buffer, { type: "array" });
          const parsed: Record<string, { rows: string[][]; truncated: boolean }> = {};
          for (const name of wb.SheetNames) {
            const raw = XLSX.utils.sheet_to_json<string[]>(wb.Sheets[name], {
              header: 1,
              defval: "",
            });
            const truncated = raw.length > MAX_TABLE_ROWS + 1;
            parsed[name] = {
              rows: (truncated ? raw.slice(0, MAX_TABLE_ROWS + 1) : raw) as string[][],
              truncated,
            };
          }
          setSpreadsheetData(parsed);
          return;
        }

        // ── Word documents ─────────────────────────────────────────────────
        if (PREVIEWABLE_WORD_TYPES.has(mime)) {
          const buffer = await res.arrayBuffer();
          try {
            const mammoth = await import("mammoth");
            const result = await mammoth.convertToHtml({ arrayBuffer: buffer });
            setDocHtml(result.value);
          } catch {
            setDocError(
              mime === "application/msword"
                ? "Legacy .doc format cannot be previewed in the browser. Download the file and open it in Word or Google Docs."
                : "Failed to render this document. Try downloading it instead."
            );
          }
          return;
        }

        // ── Plain text / code ──────────────────────────────────────────────
        if (isTextBased(mime, file.name)) {
          const text = await res.text();

          if (mime === "text/csv") {
            const all = parseCsv(text);
            const truncated = all.length > MAX_TABLE_ROWS + 1;
            setCsvData({ rows: truncated ? all.slice(0, MAX_TABLE_ROWS + 1) : all, truncated });
            return;
          }

          const lang = MIME_TO_LANG[mime];
          if (lang) {
            try {
              const { codeToHtml } = await import("shiki");
              const html = await codeToHtml(text, { lang, theme: "github-light" });
              setHighlightedHtml(html);
            } catch {
              setTextContent(text);
            }
          } else {
            setTextContent(text);
          }
          return;
        }

        // ── Binary (image / video / audio / PDF) ──────────────────────────
        const blob = await res.blob();
        const url = URL.createObjectURL(blob);
        blobUrlRef.current = url;
        setBlobUrl(url);
      })
      .catch((err) => {
        if (err instanceof DOMException && err.name === "AbortError") {
          return;
        }
        console.error(err);
        setFetchError("Failed to load preview. The file may be unavailable or too large to display.");
      })
      .finally(() => setLoading(false));

    return () => {
      controller.abort();
      if (blobUrlRef.current) {
        URL.revokeObjectURL(blobUrlRef.current);
        blobUrlRef.current = null;
      }
    };
    // Refetch is driven by id/mime/version-no only — name/size changes don't
    // require reloading the blob, and we don't want to refetch on every
    // re-render that creates a new `file` object identity.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, file.id, file.mime_type, file.name, version?.no]);

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
      setFetchError("Download failed.");
      // Silently fail — user sees error in network tab
    }
  };

  const mime = file.mime_type;
  // Word docs get a full-page Google-Docs-style layout; everything else keeps the centred card layout.
  const isDocPreview = PREVIEWABLE_WORD_TYPES.has(mime);
  const editable = !isVersionView && !!onEdit && getEditMode(file) !== null;

  const handleRestoreVersion = async () => {
    if (!version) return;
    try {
      await versionsApi.restore(file.id, version.no);
      toast.success(`Restored version ${version.no}`);
      onRestored?.();
      onClearVersion?.();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Restore failed");
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        className="!fixed !inset-0 !top-0 !left-0 !translate-x-0 !translate-y-0 !w-screen !h-screen !max-w-none !max-h-none !rounded-none flex flex-col p-0 gap-0"
      >
        {/* ── Header ── */}
        <DialogHeader className="flex flex-row items-center justify-between px-4 py-3 border-b shrink-0 bg-background">
          <div className="flex items-center gap-3 min-w-0">
            <DialogTitle className="text-base font-medium truncate">{file.name}</DialogTitle>
            {hasNav && (
              <span className="text-xs text-muted-foreground tabular-nums shrink-0">
                {currentIndex + 1} / {previewableFiles.length}
              </span>
            )}
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <span className="text-xs text-muted-foreground">{formatBytes(file.size_bytes)}</span>
            {!isVersionView && (
              <Button
                variant={showComments ? "default" : "ghost"}
                size="icon"
                className="h-8 w-8"
                onClick={() => setShowComments((v) => !v)}
                aria-label="Toggle comments"
              >
                <MessageSquare className="h-4 w-4" />
              </Button>
            )}
            {editable && (
              <Button variant="ghost" size="icon" className="h-8 w-8" onClick={onEdit} aria-label="Edit file">
                <Pencil className="h-4 w-4" />
              </Button>
            )}
            {isVersionView && (
              <Button variant="default" size="sm" onClick={handleRestoreVersion}>
                <RotateCcw className="h-4 w-4 mr-1.5" />
                Restore v{version.no}
              </Button>
            )}
            <Button variant="outline" size="sm" onClick={handleDownload}>
              <Download className="h-4 w-4 mr-1.5" />
              Download
            </Button>
            <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => onOpenChange(false)}>
              <X className="h-4 w-4" />
            </Button>
          </div>
        </DialogHeader>

        {isVersionView && (
          <div className="flex items-center justify-between gap-3 px-4 py-2 border-b bg-amber-50 dark:bg-amber-950/30 text-amber-900 dark:text-amber-200 shrink-0">
            <div className="text-xs min-w-0 truncate">
              Viewing <span className="font-mono">v{version.no}</span>
              {" "}from {formatRelative(version.createdAt)}
              {version.username ? <> · by {version.username}</> : null}
            </div>
            {onClearVersion && (
              <Button
                variant="ghost"
                size="sm"
                className="h-7 px-2 text-amber-900 dark:text-amber-200 hover:bg-amber-100 dark:hover:bg-amber-900/40"
                onClick={onClearVersion}
              >
                <ArrowLeft className="h-3.5 w-3.5 mr-1" />
                Back to current
              </Button>
            )}
          </div>
        )}

        {/* ── Content area (relative so nav arrows can be absolutely positioned) ── */}
        <div className="relative flex-1 min-h-0 flex">

          {/* Prev arrow */}
          {hasNav && prevFile && onNavigate && (
            <button
              onClick={() => onNavigate(prevFile)}
              className="absolute left-3 top-1/2 -translate-y-1/2 z-10 flex items-center justify-center w-10 h-10 rounded-full bg-black/50 hover:bg-black/70 text-white transition-colors shadow-lg"
              aria-label="Previous file"
            >
              <ChevronLeft className="h-6 w-6" />
            </button>
          )}

          {/* Next arrow */}
          {hasNav && nextFile && onNavigate && (
            <button
              onClick={() => onNavigate(nextFile)}
              className="absolute right-3 top-1/2 -translate-y-1/2 z-10 flex items-center justify-center w-10 h-10 rounded-full bg-black/50 hover:bg-black/70 text-white transition-colors shadow-lg"
              aria-label="Next file"
            >
              <ChevronRight className="h-6 w-6" />
            </button>
          )}

          {/* ── Doc preview: Google-Docs-style scrollable paper ── */}
          <div className={`flex-1 min-h-0 overflow-auto ${
            isDocPreview
              ? "bg-neutral-200 dark:bg-neutral-800 flex justify-center py-10 px-6"
              : "p-4 flex items-center justify-center bg-muted/20"
          }`}>

            {/* Loading spinner */}
            {loading && (
              <div className="flex flex-col items-center gap-3 text-muted-foreground">
                <Loader2 className="h-8 w-8 animate-spin" />
                <p className="text-sm">Loading preview…</p>
              </div>
            )}

            {/* Fetch error */}
            {!loading && fetchError && (
              <div className="flex flex-col items-center gap-3 text-center max-w-sm">
                <AlertCircle className="h-8 w-8 text-muted-foreground" />
                <p className="text-sm text-muted-foreground">{fetchError}</p>
                <Button variant="outline" size="sm" onClick={handleDownload}>
                  <Download className="h-4 w-4 mr-1.5" />
                  Download instead
                </Button>
              </div>
            )}

            {!loading && !fetchError && previewNotice && (
              <div className="flex flex-col items-center gap-2 text-center max-w-sm">
                <p className="text-muted-foreground text-sm">{previewNotice}</p>
                <Button variant="outline" size="sm" onClick={handleDownload}>
                  <Download className="h-4 w-4 mr-1.5" />
                  Download
                </Button>
              </div>
            )}

            {!loading && !fetchError && !previewNotice && !isPreviewable(mime) && (
              <div className="flex flex-col items-center gap-2 text-center">
                <p className="text-muted-foreground text-sm">Preview not available for this file type.</p>
                <Button variant="outline" size="sm" onClick={handleDownload}>
                  <Download className="h-4 w-4 mr-1.5" />
                  Download
                </Button>
              </div>
            )}

            {/* Images — draggable=false prevents the native image drag from triggering the upload dropzone */}
            {!loading && blobUrl && mime.startsWith("image/") && (
              <div
                className="w-full h-full flex items-center justify-center rounded"
                style={{
                  backgroundImage:
                    "linear-gradient(45deg,#d1d5db 25%,transparent 25%)," +
                    "linear-gradient(-45deg,#d1d5db 25%,transparent 25%)," +
                    "linear-gradient(45deg,transparent 75%,#d1d5db 75%)," +
                    "linear-gradient(-45deg,transparent 75%,#d1d5db 75%)",
                  backgroundSize: "16px 16px",
                  backgroundPosition: "0 0, 0 8px, 8px -8px, -8px 0px",
                  backgroundColor: "#f9fafb",
                }}
              >
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img
                  src={blobUrl}
                  alt={file.name}
                  draggable={false}
                  className="max-w-full max-h-full object-contain drop-shadow-sm select-none"
                />
              </div>
            )}

            {/* Videos */}
            {!loading && blobUrl && mime.startsWith("video/") && (
              <video src={blobUrl} controls className="max-w-full max-h-full rounded" />
            )}

            {/* Audio */}
            {!loading && blobUrl && mime.startsWith("audio/") && (
              <div className="w-full max-w-lg space-y-3">
                <p className="text-sm font-medium text-center">{file.name}</p>
                <audio src={blobUrl} controls className="w-full" />
              </div>
            )}

            {/* PDF */}
            {!loading && blobUrl && mime === "application/pdf" && (
              <iframe src={blobUrl} className="w-full h-full min-h-[60vh] rounded border" title={file.name} />
            )}

            {/* CSV → table */}
            {!loading && csvData && (
              <div className="w-full h-full flex flex-col rounded border border-border overflow-hidden">
                <SheetTable sheetData={csvData.rows} truncated={csvData.truncated} />
              </div>
            )}

            {/* Spreadsheet (xlsx / xls / ods) → tabbed table */}
            {!loading && spreadsheetData && (
              <SpreadsheetViewer data={spreadsheetData} />
            )}

            {/* Word document → Google-Docs-style white paper */}
            {!loading && docHtml !== null && (
              <div
                className="
                  w-full max-w-[860px] self-start
                  bg-white dark:bg-neutral-900
                  shadow-xl rounded-sm
                  px-16 py-14 text-[15px] leading-relaxed text-gray-900 dark:text-gray-100
                  [&_h1]:text-3xl  [&_h1]:font-bold    [&_h1]:mt-8  [&_h1]:mb-4
                  [&_h2]:text-2xl  [&_h2]:font-bold    [&_h2]:mt-7  [&_h2]:mb-3
                  [&_h3]:text-xl   [&_h3]:font-semibold [&_h3]:mt-6  [&_h3]:mb-2
                  [&_h4]:text-lg   [&_h4]:font-semibold [&_h4]:mt-4  [&_h4]:mb-1
                  [&_p]:mb-4 [&_p]:leading-[1.75]
                  [&_ul]:list-disc   [&_ul]:ml-7 [&_ul]:mb-4
                  [&_ol]:list-decimal [&_ol]:ml-7 [&_ol]:mb-4
                  [&_li]:mb-1.5
                  [&_strong]:font-semibold
                  [&_em]:italic
                  [&_u]:underline
                  [&_a]:text-blue-600 [&_a]:underline [&_a]:underline-offset-2
                  [&_blockquote]:border-l-4 [&_blockquote]:border-gray-300 [&_blockquote]:pl-5 [&_blockquote]:italic [&_blockquote]:text-gray-500 [&_blockquote]:my-4
                  [&_hr]:my-6 [&_hr]:border-gray-200
                  [&_pre]:bg-gray-50 [&_pre]:rounded [&_pre]:p-4 [&_pre]:text-sm [&_pre]:font-mono [&_pre]:overflow-x-auto [&_pre]:mb-4
                  [&_code]:bg-gray-100 [&_code]:rounded [&_code]:px-1 [&_code]:py-0.5 [&_code]:text-sm [&_code]:font-mono
                  [&_img]:max-w-full [&_img]:rounded
                  [&_table]:border-collapse [&_table]:w-full [&_table]:mb-6 [&_table]:text-sm
                  [&_td]:border [&_td]:border-gray-200 [&_td]:px-4 [&_td]:py-2
                  [&_th]:border [&_th]:border-gray-200 [&_th]:px-4 [&_th]:py-2 [&_th]:font-semibold [&_th]:bg-gray-50 [&_th]:text-left
                "
                // mammoth generates sanitized HTML from trusted OOXML content
                dangerouslySetInnerHTML={{ __html: docHtml }}
              />
            )}

            {/* Word document error / unsupported format */}
            {!loading && docError && (
              <div className="text-center max-w-sm space-y-2">
                <p className="text-sm text-muted-foreground">{docError}</p>
                <Button variant="outline" size="sm" onClick={handleDownload}>
                  <Download className="h-4 w-4 mr-1.5" />
                  Download
                </Button>
              </div>
            )}

            {/* Syntax-highlighted code / JSON / XML / etc. */}
            {!loading && highlightedHtml && (
              <div
                className="w-full h-full overflow-auto rounded text-xs [&>pre]:p-4 [&>pre]:rounded [&>pre]:min-h-full [&>pre]:overflow-visible"
                // shiki generates safe, sanitized HTML — no user content is rendered as markup
                dangerouslySetInnerHTML={{ __html: highlightedHtml }}
              />
            )}

            {/* Plain text fallback */}
            {!loading && textContent !== null && (
              <pre className="w-full h-full overflow-auto text-xs font-mono bg-muted rounded p-4 whitespace-pre-wrap">
                {textContent}
              </pre>
            )}
          </div>

          {showComments && !isVersionView && <CommentsPanel fileId={file.id} />}
        </div>
      </DialogContent>
    </Dialog>
  );
}
