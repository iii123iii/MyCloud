"use client";

import { FileIcon } from "./FileIcon";
import { FileContextMenu } from "./FileContextMenu";
import { formatBytes, formatRelative } from "@/lib/format";
import { Star } from "lucide-react";
import { useState } from "react";
import { PreviewModal } from "@/components/modals/PreviewModal";
import type { FileItem } from "@/lib/types";

interface Props {
  file: FileItem;
  onMutate: () => void;
}

export function FileRow({ file, onMutate }: Props) {
  const [previewOpen, setPreviewOpen] = useState(false);

  return (
    <>
      <FileContextMenu file={file} onPreview={() => setPreviewOpen(true)} onMutate={onMutate}>
        <div
          className="flex items-center gap-3 px-4 py-2.5 hover:bg-accent cursor-pointer"
          onDoubleClick={() => setPreviewOpen(true)}
        >
          <FileIcon mime={file.mime_type} className="h-4 w-4 shrink-0" />
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-1.5">
              <span className="text-sm font-medium truncate">{file.name}</span>
              {file.is_starred && <Star className="h-3 w-3 fill-yellow-400 text-yellow-400 shrink-0" />}
            </div>
          </div>
          <span className="text-xs text-muted-foreground shrink-0">{formatBytes(file.size_bytes)}</span>
          <span className="text-xs text-muted-foreground shrink-0 w-24 text-right">
            {formatRelative(file.updated_at)}
          </span>
        </div>
      </FileContextMenu>

      {previewOpen && (
        <PreviewModal file={file} open={previewOpen} onOpenChange={setPreviewOpen} />
      )}
    </>
  );
}
