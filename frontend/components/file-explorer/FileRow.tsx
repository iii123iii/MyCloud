"use client";

import { FileIcon } from "./FileIcon";
import { FileContextMenu } from "./FileContextMenu";
import { formatBytes, formatRelative } from "@/lib/format";
import { Star } from "lucide-react";
import { useState } from "react";
import { PreviewModal } from "@/components/modals/PreviewModal";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { FileItem } from "@/lib/types";

interface Props {
  file: FileItem;
  onMutate: () => void;
}

export function FileRow({ file, onMutate }: Props) {
  const [previewOpen, setPreviewOpen] = useState(false);

  // Graceful fallbacks for partial / in-flight metadata. The backend can hand
  // us rows whose name is empty (rare) or whose size/timestamp haven't been
  // finalised yet (e.g. an upload still being post-processed). We avoid
  // rendering an awkward "0 B" or empty date by substituting an em-dash and
  // surfacing a Processing badge for the latter.
  const displayName = file.name?.trim() || "Untitled";
  const hasName = Boolean(file.name?.trim());
  const hasSize = typeof file.size_bytes === "number" && file.size_bytes >= 0;
  const hasDate = Boolean(file.updated_at);
  // Empty / opaque mime is a strong hint that the upload pipeline (extract,
  // thumbnail, transcode) hasn't caught up yet — surface that to the user.
  const isProcessing =
    !file.mime_type || file.mime_type === "application/octet-stream";

  const open = () => setPreviewOpen(true);

  return (
    <>
      <FileContextMenu file={file} onPreview={open} onMutate={onMutate}>
        <div
          role="button"
          tabIndex={0}
          aria-label={hasName ? displayName : "Untitled file"}
          aria-selected={previewOpen || undefined}
          className={cn(
            "group/row relative flex items-center gap-3 px-4 py-2.5",
            "cursor-pointer select-none outline-none",
            "transition-colors duration-150",
            "hover:bg-accent active:bg-accent/80",
            "focus-visible:bg-accent focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset",
            "aria-selected:bg-accent/60",
          )}
          onDoubleClick={open}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === " ") {
              e.preventDefault();
              open();
            }
          }}
        >
          {/* FileIcon already applies aria-hidden in its bare-icon path —
              passing it here would not type-check against its Props. */}
          <FileIcon mime={file.mime_type} className="h-4 w-4 shrink-0" />

          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-1.5 min-w-0">
              <span
                className={cn(
                  "text-sm font-medium truncate",
                  !hasName && "italic text-muted-foreground",
                )}
                title={displayName}
              >
                {displayName}
              </span>
              {file.is_starred && (
                <Star
                  className="h-3 w-3 shrink-0 fill-yellow-400 text-yellow-400"
                  aria-label="Starred"
                  role="img"
                />
              )}
              {isProcessing && (
                <Badge
                  variant="secondary"
                  className="ml-1 h-4 px-1.5 text-[10px] font-medium uppercase tracking-wide"
                  aria-label="File is processing"
                >
                  Processing
                </Badge>
              )}
            </div>
          </div>

          <span
            className={cn(
              "text-xs text-muted-foreground shrink-0 tabular-nums w-20 text-right",
              !hasSize && "italic",
            )}
            aria-label={hasSize ? `Size ${formatBytes(file.size_bytes)}` : "Size unknown"}
          >
            {hasSize ? formatBytes(file.size_bytes) : "—"}
          </span>

          <span
            className={cn(
              "text-xs text-muted-foreground shrink-0 tabular-nums w-24 text-right",
              !hasDate && "italic",
            )}
            aria-label={hasDate ? `Modified ${formatRelative(file.updated_at)}` : "Modified date unknown"}
          >
            {hasDate ? formatRelative(file.updated_at) : "—"}
          </span>
        </div>
      </FileContextMenu>

      {previewOpen && (
        <PreviewModal file={file} open={previewOpen} onOpenChange={setPreviewOpen} />
      )}
    </>
  );
}
