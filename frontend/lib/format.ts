import { formatDistanceToNow, format } from "date-fns";

export function formatBytes(bytes: number): string {
  if (bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

export function formatDate(dateStr: string): string {
  try {
    return format(new Date(dateStr), "MMM d, yyyy");
  } catch {
    return dateStr;
  }
}

export function formatRelative(dateStr: string): string {
  try {
    return formatDistanceToNow(new Date(dateStr), { addSuffix: true });
  } catch {
    return dateStr;
  }
}

export function mimeToLabel(mime: string): string {
  if (mime.startsWith("image/"))       return "Image";
  if (mime.startsWith("video/"))       return "Video";
  if (mime.startsWith("audio/"))       return "Audio";
  if (mime === "application/pdf")      return "PDF";
  if (mime.startsWith("text/"))        return "Text";
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

export function isPreviewable(mime: string): boolean {
  return (
    mime.startsWith("image/") ||
    mime.startsWith("video/") ||
    mime.startsWith("audio/") ||
    mime === "application/pdf" ||
    mime.startsWith("text/")
  );
}
