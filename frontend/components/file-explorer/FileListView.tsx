"use client";

import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { FileIcon } from "./FileIcon";
import { FileContextMenu } from "./FileContextMenu";
import { formatBytes, formatRelative } from "@/lib/format";
import { Star } from "lucide-react";
import type { FileItem, FolderItem } from "@/lib/types";

interface Props {
  files: FileItem[];
  folders: FolderItem[];
  onOpenFolder: (folder: FolderItem) => void;
  onPreview: (file: FileItem) => void;
  onMutate: () => void;
}

export function FileListView({ files, folders, onOpenFolder, onPreview, onMutate }: Props) {
  if (files.length === 0 && folders.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-20 text-muted-foreground gap-2">
        <p className="text-sm">This folder is empty.</p>
      </div>
    );
  }

  return (
    <div className="border rounded-lg overflow-hidden">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-8" />
            <TableHead>Name</TableHead>
            <TableHead className="w-28 text-right">Size</TableHead>
            <TableHead className="w-36 text-right">Modified</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {folders.map((folder) => (
            <FileContextMenu
              key={folder.id}
              folder={folder}
              onOpenFolder={() => onOpenFolder(folder)}
              onMutate={onMutate}
            >
              <TableRow
                className="cursor-pointer"
                onDoubleClick={() => onOpenFolder(folder)}
              >
                <TableCell className="py-2">
                  <FileIcon isFolder className="h-4 w-4" />
                </TableCell>
                <TableCell className="py-2 font-medium max-w-0">
                  <span
                    className="block truncate"
                    title={folder.name}
                  >
                    {folder.name}
                  </span>
                </TableCell>
                <TableCell className="py-2 text-right text-muted-foreground text-sm shrink-0">—</TableCell>
                <TableCell className="py-2 text-right text-muted-foreground text-sm shrink-0">
                  {folder.updated_at ? formatRelative(folder.updated_at) : "—"}
                </TableCell>
              </TableRow>
            </FileContextMenu>
          ))}

          {files.map((file) => (
            <FileContextMenu
              key={file.id}
              file={file}
              onPreview={() => onPreview(file)}
              onMutate={onMutate}
            >
              <TableRow
                className="cursor-pointer"
                onDoubleClick={() => onPreview(file)}
              >
                <TableCell className="py-2">
                  <FileIcon mime={file.mime_type} className="h-4 w-4" />
                </TableCell>
                <TableCell className="py-2 max-w-0">
                  <div className="flex items-center gap-1.5 min-w-0">
                    <span
                      className="font-medium truncate"
                      title={file.name}
                    >
                      {file.name}
                    </span>
                    {file.is_starred && (
                      <Star className="h-3 w-3 shrink-0 fill-yellow-400 text-yellow-400" />
                    )}
                  </div>
                </TableCell>
                <TableCell className="py-2 text-right text-muted-foreground text-sm shrink-0">
                  {formatBytes(file.size_bytes)}
                </TableCell>
                <TableCell className="py-2 text-right text-muted-foreground text-sm shrink-0">
                  {formatRelative(file.updated_at)}
                </TableCell>
              </TableRow>
            </FileContextMenu>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
