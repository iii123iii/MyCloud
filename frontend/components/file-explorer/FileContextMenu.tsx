"use client";

import { useState } from "react";
import { toast } from "sonner";
import {
  ContextMenu, ContextMenuContent, ContextMenuItem,
  ContextMenuSeparator, ContextMenuTrigger,
} from "@/components/ui/context-menu";
import {
  Download, Eye, Star, StarOff, Share2, Pencil, Trash2, FolderOpen, FolderInput,
} from "lucide-react";
import { files as filesApi, folders as foldersApi, tokenStore } from "@/lib/api";
import { ShareDialog } from "@/components/modals/ShareDialog";
import { RenameDialog } from "@/components/modals/RenameDialog";
import { MoveDialog } from "@/components/modals/MoveDialog";
import type { FileItem, FolderItem } from "@/lib/types";

interface Props {
  children: React.ReactNode;
  file?: FileItem;
  folder?: FolderItem;
  onPreview?: () => void;
  onOpenFolder?: () => void;
  onMutate: () => void;
}

export function FileContextMenu({ children, file, folder, onPreview, onOpenFolder, onMutate }: Props) {
  const [shareOpen, setShareOpen]   = useState(false);
  const [renameOpen, setRenameOpen] = useState(false);
  const [moveOpen, setMoveOpen]     = useState(false);

  const handleDownload = async () => {
    if (!file) return;
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
      toast.error("Download failed");
    }
  };

  const handleStar = async () => {
    if (!file) return;
    try {
      await filesApi.update(file.id, { is_starred: !file.is_starred });
      onMutate();
      toast.success(file.is_starred ? "Removed from starred" : "Added to starred");
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Could not update star");
    }
  };

  const handleDelete = async () => {
    try {
      if (file) {
        await filesApi.delete(file.id);
        toast.success(`${file.name} moved to trash`);
      } else if (folder) {
        await foldersApi.delete(folder.id);
        toast.success(`${folder.name} moved to trash`);
      }
      onMutate();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Delete failed");
    }
  };

  return (
    <>
      <ContextMenu>
        <ContextMenuTrigger>{children}</ContextMenuTrigger>
        <ContextMenuContent className="w-48">
          {folder && onOpenFolder && (
            <ContextMenuItem onClick={onOpenFolder}>
              <FolderOpen className="h-4 w-4 mr-2" />
              Open
            </ContextMenuItem>
          )}
          {file && onPreview && (
            <ContextMenuItem onClick={onPreview}>
              <Eye className="h-4 w-4 mr-2" />
              Preview
            </ContextMenuItem>
          )}
          {file && (
            <ContextMenuItem onClick={handleDownload}>
              <Download className="h-4 w-4 mr-2" />
              Download
            </ContextMenuItem>
          )}
          <ContextMenuSeparator />
          <ContextMenuItem onClick={() => setRenameOpen(true)}>
            <Pencil className="h-4 w-4 mr-2" />
            Rename
          </ContextMenuItem>
          <ContextMenuItem onClick={() => setMoveOpen(true)}>
            <FolderInput className="h-4 w-4 mr-2" />
            Move to
          </ContextMenuItem>
          {file && (
            <ContextMenuItem onClick={handleStar}>
              {file.is_starred
                ? <><StarOff className="h-4 w-4 mr-2" />Remove star</>
                : <><Star    className="h-4 w-4 mr-2" />Star</>
              }
            </ContextMenuItem>
          )}
          {file && (
            <ContextMenuItem onClick={() => setShareOpen(true)}>
              <Share2 className="h-4 w-4 mr-2" />
              Share
            </ContextMenuItem>
          )}
          <ContextMenuSeparator />
          <ContextMenuItem
            onClick={handleDelete}
            className="text-destructive focus:text-destructive"
          >
            <Trash2 className="h-4 w-4 mr-2" />
            Delete
          </ContextMenuItem>
        </ContextMenuContent>
      </ContextMenu>

      {file && (
        <ShareDialog
          open={shareOpen}
          onOpenChange={setShareOpen}
          fileId={file.id}
          fileName={file.name}
        />
      )}

      <RenameDialog
        open={renameOpen}
        onOpenChange={setRenameOpen}
        fileId={file?.id}
        folderId={folder?.id}
        currentName={file?.name ?? folder?.name ?? ""}
        onRenamed={onMutate}
      />

      <MoveDialog
        open={moveOpen}
        onOpenChange={setMoveOpen}
        file={file}
        folder={folder}
        onMoved={onMutate}
      />
    </>
  );
}
