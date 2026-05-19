import { formatDistanceToNow, format } from "date-fns";

export function formatBytes(bytes: number): string {
  if (bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

export function formatDate(dateStr: string): string {
  const d = parseServerDate(dateStr);
  if (isNaN(d.getTime())) return dateStr;
  try {
    return format(d, "MMM d, yyyy");
  } catch {
    return dateStr;
  }
}

export function formatRelative(dateStr: string): string {
  const d = parseServerDate(dateStr);
  if (isNaN(d.getTime())) return dateStr;
  try {
    return formatDistanceToNow(d, { addSuffix: true });
  } catch {
    return dateStr;
  }
}

/**
 * Parse a datetime string emitted by the Go backend.
 *
 * The MariaDB driver formats DATETIME columns as "2006-01-02 15:04:05" (UTC,
 * space separator, no zone suffix). new Date() in JavaScript treats that as
 * a local-time string on Safari and as Invalid Date on some engines. Replace
 * the space with "T" and append "Z" to coerce it to a proper ISO-8601 UTC
 * timestamp. Strings that already include "T" or a timezone are passed
 * through unchanged.
 */
export function parseServerDate(dateStr: string): Date {
  if (/[Tt]/.test(dateStr) || /[+-]\d{2}:?\d{2}$|Z$/.test(dateStr)) {
    return new Date(dateStr);
  }
  return new Date(dateStr.replace(" ", "T") + "Z");
}

export function formatServerDateTime(dateStr: string): string {
  const d = parseServerDate(dateStr);
  if (isNaN(d.getTime())) return dateStr;
  return d.toLocaleString();
}

export function mimeToLabel(mime: string): string {
  if (mime.startsWith("image/"))       return "Image";
  if (mime.startsWith("video/"))       return "Video";
  if (mime.startsWith("audio/"))       return "Audio";
  if (mime === "application/pdf")      return "PDF";
  if (mime === "application/json")     return "JSON";
  if (mime === "application/xml")      return "XML";
  if (mime === "application/javascript") return "JavaScript";
  if (mime === "application/x-yaml" || mime === "text/yaml") return "YAML";
  if (mime === "text/csv")             return "CSV";
  if (mime === "text/markdown" || mime === "text/x-markdown") return "Markdown";
  if (mime.startsWith("text/"))        return "Text";
  if (PREVIEWABLE_SPREADSHEET_TYPES.has(mime))  return "Spreadsheet";
  if (PREVIEWABLE_WORD_TYPES.has(mime))          return "Word";
  if (mime.includes("zip") || mime.includes("compressed")) return "Archive";
  if (mime.includes("word"))           return "Word";
  if (mime.includes("excel") || mime.includes("spreadsheet")) return "Spreadsheet";
  if (mime.includes("powerpoint") || mime.includes("presentation")) return "Slides";
  return "File";
}

export function mimeToColor(mime: string): string {
  if (mime.startsWith("image/"))  return "text-emerald-600";
  if (mime.startsWith("video/"))  return "text-violet-600";
  if (mime.startsWith("audio/"))  return "text-pink-600";
  if (mime === "application/pdf") return "text-red-600";
  if (mime.startsWith("text/"))   return "text-sky-600";
  return "text-muted-foreground";
}

/** application/* MIME types that are text-based and can be previewed */
export const PREVIEWABLE_APPLICATION_TYPES = new Set([
  "application/json",
  "application/xml",
  "application/javascript",
  "application/x-javascript",
  "application/x-yaml",
  "application/x-sh",
  "application/x-shellscript",
  "application/x-httpd-php",
  "application/graphql",
  "application/ld+json",
]);

/** Spreadsheet MIME types rendered as interactive tables via SheetJS */
export const PREVIEWABLE_SPREADSHEET_TYPES = new Set([
  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", // .xlsx
  "application/vnd.ms-excel",                                           // .xls
  "application/vnd.oasis.opendocument.spreadsheet",                     // .ods
]);

/** Word-processor MIME types rendered via mammoth */
export const PREVIEWABLE_WORD_TYPES = new Set([
  "application/vnd.openxmlformats-officedocument.wordprocessingml.document", // .docx
  "application/msword",                                                        // .doc (best-effort)
]);

export function isPreviewable(mime: string): boolean {
  return (
    mime.startsWith("image/") ||
    mime.startsWith("video/") ||
    mime.startsWith("audio/") ||
    mime === "application/pdf" ||
    mime.startsWith("text/") ||
    PREVIEWABLE_APPLICATION_TYPES.has(mime) ||
    PREVIEWABLE_SPREADSHEET_TYPES.has(mime) ||
    PREVIEWABLE_WORD_TYPES.has(mime)
  );
}
