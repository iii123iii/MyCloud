"use client";

import { useState } from "react";
import { toast } from "sonner";
import {
  ContextMenu, ContextMenuContent, ContextMenuItem,
  ContextMenuSeparator, ContextMenuTrigger,
} from "@/components/ui/context-menu";
import {
  Download, Edit3, Eye, Star, StarOff, Share2, Pencil, MessageSquare, Trash2,
  FolderOpen, FolderInput, History, Archive,
} from "lucide-react";
import { files as filesApi, folders as foldersApi, tokenStore, type FileVersion } from "@/lib/api";
import { ShareDialog } from "@/components/modals/ShareDialog";
import { RenameDialog } from "@/components/modals/RenameDialog";
import { MoveDialog } from "@/components/modals/MoveDialog";
import { VersionHistoryDialog } from "@/components/modals/VersionHistoryDialog";
import { CommentsDialog } from "@/components/modals/CommentsDialog";
import { EditFileDialog } from "@/components/modals/EditFileDialog";
import { PreviewModal } from "@/components/modals/PreviewModal";
import { getEditMode } from "@/lib/file-kind";
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
  const [shareOpen, setShareOpen]         = useState(false);
  const [renameOpen, setRenameOpen]       = useState(false);
  const [moveOpen, setMoveOpen]           = useState(false);
  const [versionsOpen, setVersionsOpen]   = useState(false);
  const [commentsOpen, setCommentsOpen]   = useState(false);
  const [editOpen, setEditOpen]           = useState(false);
  // Holds the FileVersion currently being previewed via the Eye button in
  // VersionHistoryDialog. Separate from the regular PreviewModal (which lives
  // at the FileExplorer level) so version previews don't have to thread
  // through every component.
  const [previewVersion, setPreviewVersion] = useState<FileVersion | null>(null);

  const editable = !!file && getEditMode(file) !== null;

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
          {file && editable && (
            <ContextMenuItem onClick={() => setEditOpen(true)}>
              <Edit3 className="h-4 w-4 mr-2" />
              Edit
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
          {file && (
            <ContextMenuItem onClick={() => setCommentsOpen(true)}>
              <MessageSquare className="h-4 w-4 mr-2" />
              Comments
            </ContextMenuItem>
          )}
          {file && (
            <ContextMenuItem onClick={() => setVersionsOpen(true)}>
              <History className="h-4 w-4 mr-2" />
              Version history
            </ContextMenuItem>
          )}
          {folder && (
            <ContextMenuItem
              onClick={async () => {
                try {
                  await filesApi.downloadArchive([], [folder.id]);
                } catch {
                  toast.error("Download failed");
                }
              }}
            >
              <Archive className="h-4 w-4 mr-2" />
              Download as zip
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

      {file && (
        <VersionHistoryDialog
          open={versionsOpen}
          onOpenChange={setVersionsOpen}
          fileId={file.id}
          fileName={file.name}
          fileMime={file.mime_type}
          onRestored={onMutate}
          onPreviewVersion={(v) => {
            setVersionsOpen(false);
            setPreviewVersion(v);
          }}
        />
      )}

      {file && (
        <CommentsDialog
          open={commentsOpen}
          onOpenChange={setCommentsOpen}
          fileId={file.id}
          fileName={file.name}
        />
      )}

      {file && editable && (
        <EditFileDialog
          open={editOpen}
          onOpenChange={setEditOpen}
          file={file}
          onSaved={onMutate}
        />
      )}

      {/* Dedicated PreviewModal for the version-preview flow. Kept separate
          from the FileExplorer-managed one so we don't have to plumb the
          version state through every component. "Back to current" closes this
          and asks the parent to open the regular preview. */}
      {file && previewVersion && (
        <PreviewModal
          file={file}
          open
          onOpenChange={(o) => !o && setPreviewVersion(null)}
          version={{
            no: previewVersion.version_no,
            createdAt: previewVersion.created_at,
            username: previewVersion.username,
          }}
          onClearVersion={() => {
            setPreviewVersion(null);
            onPreview?.();
          }}
          onRestored={() => {
            onMutate();
            setPreviewVersion(null);
          }}
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
