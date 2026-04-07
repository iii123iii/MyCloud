"use client";

import { useState, useEffect, useRef } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Download, X } from "lucide-react";
import { codeToHtml } from "shiki";
import { files as filesApi, tokenStore } from "@/lib/api";
import {
  isPreviewable,
  formatBytes,
  PREVIEWABLE_APPLICATION_TYPES,
  PREVIEWABLE_SPREADSHEET_TYPES,
  PREVIEWABLE_WORD_TYPES,
} from "@/lib/format";
import type { FileItem } from "@/lib/types";

interface Props {
  file: FileItem;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

// Maps MIME types to shiki language identifiers
const MIME_TO_LANG: Record<string, string> = {
  "text/javascript":           "javascript",
  "text/typescript":           "typescript",
  "text/html":                 "html",
  "text/css":                  "css",
  "text/xml":                  "xml",
  "text/markdown":             "markdown",
  "text/x-markdown":           "markdown",
  "text/yaml":                 "yaml",
  "text/x-yaml":               "yaml",
  "text/x-python":             "python",
  "text/x-java":               "java",
  "text/x-c":                  "c",
  "text/x-c++src":             "cpp",
  "text/x-csharp":             "csharp",
  "text/x-go":                 "go",
  "text/x-rust":               "rust",
  "text/x-sh":                 "shellscript",
  "text/x-shellscript":        "shellscript",
  "text/x-ruby":               "ruby",
  "text/x-php":                "php",
  "text/x-swift":              "swift",
  "text/x-kotlin":             "kotlin",
  "application/json":          "json",
  "application/ld+json":       "json",
  "application/xml":           "xml",
  "application/javascript":    "javascript",
  "application/x-javascript":  "javascript",
  "application/x-yaml":        "yaml",
  "application/x-sh":          "shellscript",
  "application/x-shellscript": "shellscript",
  "application/x-httpd-php":   "php",
  "application/graphql":       "graphql",
};

/** Returns true for MIME types that should be fetched as plain text */
function isTextBased(mime: string): boolean {
  return mime.startsWith("text/") || PREVIEWABLE_APPLICATION_TYPES.has(mime);
}

// ─── CSV helpers ────────────────────────────────────────────────────────────

const MAX_TABLE_ROWS = 1000;

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

export function PreviewModal({ file, open, onOpenChange }: Props) {
  const [blobUrl, setBlobUrl]                     = useState<string | null>(null);
  const [textContent, setTextContent]             = useState<string | null>(null);
  const [highlightedHtml, setHighlightedHtml]     = useState<string | null>(null);
  const [csvData, setCsvData]                     = useState<{ rows: string[][]; truncated: boolean } | null>(null);
  const [spreadsheetData, setSpreadsheetData]     = useState<Record<string, { rows: string[][]; truncated: boolean }> | null>(null);
  const [docHtml, setDocHtml]                     = useState<string | null>(null);
  const [docError, setDocError]                   = useState<string | null>(null);
  const [loading, setLoading]                     = useState(true);
  const blobUrlRef = useRef<string | null>(null);

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

    const token = tokenStore.getAccess();
    const mime = file.mime_type;

    fetch(filesApi.previewUrl(file.id), {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    })
      .then(async (res) => {
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
        if (isTextBased(mime)) {
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

  const mime = file.mime_type;

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

          {!loading && !isPreviewable(mime) && (
            <div className="text-center">
              <p className="text-muted-foreground text-sm">Preview not available for this file type.</p>
            </div>
          )}

          {/* Images */}
          {!loading && blobUrl && mime.startsWith("image/") && (
            // eslint-disable-next-line @next/next/no-img-element
            <img src={blobUrl} alt={file.name} className="max-w-full max-h-full object-contain rounded" />
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

          {/* Word document → rendered HTML */}
          {!loading && docHtml !== null && (
            <div className="w-full h-full overflow-auto bg-white rounded border border-border p-8 max-w-3xl mx-auto
              [&_h1]:text-2xl [&_h1]:font-bold [&_h1]:mt-6 [&_h1]:mb-3
              [&_h2]:text-xl  [&_h2]:font-bold [&_h2]:mt-5 [&_h2]:mb-2
              [&_h3]:text-lg  [&_h3]:font-semibold [&_h3]:mt-4 [&_h3]:mb-2
              [&_p]:mb-3 [&_p]:leading-relaxed
              [&_ul]:list-disc [&_ul]:ml-6 [&_ul]:mb-3
              [&_ol]:list-decimal [&_ol]:ml-6 [&_ol]:mb-3
              [&_li]:mb-1
              [&_strong]:font-semibold
              [&_em]:italic
              [&_a]:text-blue-600 [&_a]:underline
              [&_blockquote]:border-l-4 [&_blockquote]:border-border [&_blockquote]:pl-4 [&_blockquote]:italic [&_blockquote]:text-muted-foreground [&_blockquote]:my-3
              [&_table]:border-collapse [&_table]:w-full [&_table]:mb-4
              [&_td]:border [&_td]:border-border [&_td]:px-3 [&_td]:py-1.5 [&_td]:text-sm
              [&_th]:border [&_th]:border-border [&_th]:px-3 [&_th]:py-1.5 [&_th]:text-sm [&_th]:font-semibold [&_th]:bg-muted"
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
      </DialogContent>
    </Dialog>
  );
}
