import {
  FileText, FileImage, FileVideo, FileAudio, FileArchive,
  FileSpreadsheet, FileCode, Folder, File,
} from "lucide-react";
import { cn } from "@/lib/utils";

interface Props {
  mime?: string;
  isFolder?: boolean;
  className?: string;
}

export function FileIcon({ mime = "", isFolder, className }: Props) {
  if (isFolder) return <Folder className={cn("text-yellow-500", className)} />;
  if (mime.startsWith("image/"))        return <FileImage  className={cn("text-emerald-500", className)} />;
  if (mime.startsWith("video/"))        return <FileVideo  className={cn("text-violet-500",  className)} />;
  if (mime.startsWith("audio/"))        return <FileAudio  className={cn("text-pink-500",    className)} />;
  if (mime === "application/pdf")       return <FileText   className={cn("text-red-500",     className)} />;
  if (mime.startsWith("text/"))         return <FileCode   className={cn("text-sky-500",     className)} />;
  if (mime.includes("zip") || mime.includes("compressed") || mime.includes("tar"))
    return <FileArchive className={cn("text-amber-500", className)} />;
  if (mime.includes("excel") || mime.includes("spreadsheet"))
    return <FileSpreadsheet className={cn("text-green-600", className)} />;
  return <File className={cn("text-muted-foreground", className)} />;
}
