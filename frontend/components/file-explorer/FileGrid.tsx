"use client";

import { Star, FolderOpen, Upload } from "lucide-react";
import { FileIcon } from "./FileIcon";
import { FileContextMenu } from "./FileContextMenu";
import { formatBytes } from "@/lib/format";
import type { FileItem, FolderItem } from "@/lib/types";

interface Props {
  files: FileItem[];
  folders: FolderItem[];
  onOpenFolder: (folder: FolderItem) => void;
  onPreview: (file: FileItem) => void;
  onMutate: () => void;
}

export function FileGrid({ files, folders, onOpenFolder, onPreview, onMutate }: Props) {
  if (files.length === 0 && folders.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-24 text-muted-foreground gap-3">
        <div className="rounded-full bg-muted p-4">
          <Upload className="h-8 w-8 opacity-40" />
        </div>
        <div className="text-center space-y-1">
          <p className="text-sm font-medium">This folder is empty</p>
          <p className="text-xs">Drop files here or click Upload to get started.</p>
        </div>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-3">
      {folders.map((folder) => (
        <FileContextMenu
          key={folder.id}
          folder={folder}
          onOpenFolder={() => onOpenFolder(folder)}
          onMutate={onMutate}
        >
          <div
            className="group relative flex flex-col items-center gap-2 p-3 rounded-lg border bg-card hover:bg-accent/60 hover:border-accent-foreground/10 cursor-pointer select-none transition-colors"
            onDoubleClick={() => onOpenFolder(folder)}
            title={folder.name}
          >
            <FileIcon isFolder className="h-10 w-10 shrink-0" />
            <p className="text-xs font-medium text-center w-full overflow-hidden text-ellipsis whitespace-nowrap leading-tight">
              {folder.name}
            </p>
          </div>
        </FileContextMenu>
      ))}

      {files.map((file) => (
        <FileContextMenu
          key={file.id}
          file={file}
          onPreview={() => onPreview(file)}
          onMutate={onMutate}
        >
          <div
            className="group relative flex flex-col items-center gap-2 p-3 rounded-lg border bg-card hover:bg-accent/60 hover:border-accent-foreground/10 cursor-pointer select-none transition-colors"
            onDoubleClick={() => onPreview(file)}
            title={file.name}
          >
            {file.is_starred && (
              <Star className="absolute top-2 right-2 h-3 w-3 fill-yellow-400 text-yellow-400" />
            )}
            <FileIcon mime={file.mime_type} className="h-10 w-10 shrink-0" />
            <div className="w-full text-center min-w-0">
              <p className="text-xs font-medium overflow-hidden text-ellipsis whitespace-nowrap leading-tight">
                {file.name}
              </p>
              <p className="text-[10px] text-muted-foreground mt-0.5">
                {formatBytes(file.size_bytes)}
              </p>
            </div>
          </div>
        </FileContextMenu>
      ))}
    </div>
  );
}
