"use client";

import { useState } from "react";
import { toast } from "sonner";
import {
  ContextMenu, ContextMenuContent, ContextMenuItem,
  ContextMenuSeparator, ContextMenuTrigger,
} from "@/components/ui/context-menu";
import {
  Download, Edit3, Eye, Star, StarOff, Share2, Pencil, MessageSquare, Trash2,
  FolderOpen, FolderInput, History, Archive, Inbox, Lock, ShieldCheck, Copy,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { files as filesApi, folders as foldersApi, fetchAuthed, request, type FileVersion } from "@/lib/api";
import { ShareDialog } from "@/components/modals/ShareDialog";
import { RenameDialog } from "@/components/modals/RenameDialog";
import { MoveDialog } from "@/components/modals/MoveDialog";
import { VersionHistoryDialog } from "@/components/modals/VersionHistoryDialog";
import { CommentsDialog } from "@/components/modals/CommentsDialog";
import { EditFileDialog } from "@/components/modals/EditFileDialog";
import { PreviewModal } from "@/components/modals/PreviewModal";
import { RequestFilesDialog } from "@/components/modals/RequestFilesDialog";
import { LockManagerDialog } from "@/components/modals/LockManagerDialog";
import { RetentionPolicyDialog } from "@/components/modals/RetentionPolicyDialog";
import { getEditMode } from "@/lib/file-kind";
import type { FileItem, FolderItem } from "@/lib/types";

interface Props {
  children: React.ReactNode;
  file?: FileItem;
  folder?: FolderItem;
  onPreview?: () => void;
  onOpenFolder?: () => void;
  // After a successful mutation. Move passes the destination folder id
  // (forwarded by MoveDialog.onMoved) so the parent can invalidate the
  // destination listing too — not just the source folder we were viewing.
  onMutate: (targetFolderId?: string | null) => void;
  // Read-only context (a viewer grant on a shared item): hides the write
  // actions (edit/rename/move/delete/locks). Default false — own drive.
  readOnly?: boolean;
  // Whether the caller owns the item. Owner-only actions (share, request
  // files, retention, file star) hide when false. Default true.
  isOwner?: boolean;
}

/**
 * Shared className for every item in this menu. Centralised so spacing,
 * alignment, typography, and the icon→text gap stay consistent — and so
 * focus/hover transitions (driven by the shadcn primitive) read uniformly.
 */
const itemClass =
  "gap-2.5 px-2 py-1.5 text-sm transition-colors duration-150 [&_svg]:size-4 [&_svg]:shrink-0";

export function FileContextMenu({ children, file, folder, onPreview, onOpenFolder, onMutate, readOnly = false, isOwner = true }: Props) {
  const [shareOpen, setShareOpen]         = useState(false);
  const [renameOpen, setRenameOpen]       = useState(false);
  const [moveOpen, setMoveOpen]           = useState(false);
  const [versionsOpen, setVersionsOpen]   = useState(false);
  const [commentsOpen, setCommentsOpen]   = useState(false);
  const [editOpen, setEditOpen]           = useState(false);
  // Per-folder "Request files" dialog. Generates a guest upload link
  // pre-bound to this folder so anonymous uploaders drop straight in.
  const [requestOpen, setRequestOpen]     = useState(false);
  // File-level cooperative locks and per-folder retention policies.
  const [locksOpen, setLocksOpen]         = useState(false);
  const [retentionOpen, setRetentionOpen] = useState(false);
  // Holds the FileVersion currently being previewed via the Eye button in
  // VersionHistoryDialog. Separate from the regular PreviewModal (which lives
  // at the FileExplorer level) so version previews don't have to thread
  // through every component.
  const [previewVersion, setPreviewVersion] = useState<FileVersion | null>(null);

  const editable = !!file && getEditMode(file) !== null;
  // Rename / Move / Delete need *some* target; otherwise show them disabled
  // (rather than hiding) so the menu's vertical rhythm stays predictable.
  const hasTarget = !!file || !!folder;
  const targetLabel = folder ? "folder" : "file";
  // Shared (not owned by the caller). Drives the "Copy to my files" action,
  // which is available to anyone who can read the item — including viewers.
  const isShared = !!(file?.shared || folder?.shared);

  const handleDownload = async () => {
    if (!file) return;
    try {
      // fetchAuthed transparently refreshes a stale access token mid-flight
      // (was previously a bare fetch that 401'd on expiry).
      const res = await fetchAuthed(`/api/v2/files/${file.id}:download`);
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

  const handleCopyToMyFiles = async () => {
    try {
      if (file) await filesApi.copy(file.id);
      else if (folder) await foldersApi.copy(folder.id);
      toast.success("Copied to your files");
      onMutate();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Copy failed");
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

  // Folder star = sidebar favourites pin toggle. Distinct from file star
  // (which writes files.is_starred) — folders have no equivalent column.
  const handleFolderFavorite = async () => {
    if (!folder) return;
    try {
      if (folder.is_favorited) {
        await request<void>(`/api/v2/me/favorites/${folder.id}`, {
          method: "DELETE",
        });
        toast.success("Removed from favourites");
      } else {
        await request<void>("/api/v2/me/favorites", {
          method: "POST",
          body: JSON.stringify({ folder_id: folder.id }),
        });
        toast.success("Added to favourites");
      }
      onMutate();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Could not update favourites");
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
        <ContextMenuContent
          aria-label={folder ? "Folder actions" : file ? "File actions" : "Item actions"}
          className="w-56"
        >
          {folder && onOpenFolder && (
            <ContextMenuItem onClick={onOpenFolder} className={itemClass}>
              <FolderOpen aria-hidden="true" />
              <span>Open</span>
            </ContextMenuItem>
          )}
          {file && onPreview && (
            <ContextMenuItem onClick={onPreview} className={itemClass}>
              <Eye aria-hidden="true" />
              <span>Preview</span>
            </ContextMenuItem>
          )}
          {file && !readOnly && (
            <ContextMenuItem
              onClick={() => setEditOpen(true)}
              disabled={!editable}
              title={editable ? undefined : "This file type can't be edited in-app"}
              className={itemClass}
            >
              <Edit3 aria-hidden="true" />
              <span>Edit</span>
            </ContextMenuItem>
          )}
          {file && (
            <ContextMenuItem onClick={handleDownload} className={itemClass}>
              <Download aria-hidden="true" />
              <span>Download</span>
            </ContextMenuItem>
          )}
          {/* Copy a shared item into the caller's own drive. Read-only access
              is enough, so this shows for viewers too. */}
          {isShared && hasTarget && (
            <ContextMenuItem onClick={handleCopyToMyFiles} className={itemClass}>
              <Copy aria-hidden="true" />
              <span>Copy to my files</span>
            </ContextMenuItem>
          )}
          {!readOnly && <ContextMenuSeparator />}
          {!readOnly && (
            <>
              <ContextMenuItem
                onClick={() => setRenameOpen(true)}
                disabled={!hasTarget}
                title={hasTarget ? undefined : "Nothing selected to rename"}
                className={itemClass}
              >
                <Pencil aria-hidden="true" />
                <span>Rename</span>
              </ContextMenuItem>
              <ContextMenuItem
                onClick={() => setMoveOpen(true)}
                disabled={!hasTarget}
                title={hasTarget ? undefined : "Nothing selected to move"}
                className={itemClass}
              >
                <FolderInput aria-hidden="true" />
                <span>Move to</span>
              </ContextMenuItem>
            </>
          )}
          {file && isOwner && (
            <ContextMenuItem onClick={handleStar} className={itemClass}>
              {file.is_starred ? (
                <>
                  <StarOff aria-hidden="true" />
                  <span>Remove star</span>
                </>
              ) : (
                <>
                  <Star aria-hidden="true" />
                  <span>Star</span>
                </>
              )}
            </ContextMenuItem>
          )}
          {folder && (
            <ContextMenuItem onClick={handleFolderFavorite} className={itemClass}>
              {folder.is_favorited ? (
                <>
                  <StarOff aria-hidden="true" />
                  <span>Remove from favourites</span>
                </>
              ) : (
                <>
                  <Star aria-hidden="true" />
                  <span>Add to favourites</span>
                </>
              )}
            </ContextMenuItem>
          )}
          {/* Share — owner only. The backend's POST /api/v2/shares accepts
              file_id OR folder_id (mutually exclusive); ShareDialog routes the
              right one. */}
          {isOwner && (
            <ContextMenuItem
              onClick={() => setShareOpen(true)}
              disabled={!hasTarget}
              title={hasTarget ? undefined : "Nothing selected to share"}
              className={itemClass}
            >
              <Share2 aria-hidden="true" />
              <span>Share {targetLabel}</span>
            </ContextMenuItem>
          )}
          {/* Folder-only — set up a public link that lets guests
              upload INTO this folder without an account. */}
          {folder && isOwner && (
            <ContextMenuItem onClick={() => setRequestOpen(true)} className={itemClass}>
              <Inbox aria-hidden="true" />
              <span>Request files (guest upload)</span>
            </ContextMenuItem>
          )}
          {file && (
            <ContextMenuItem onClick={() => setCommentsOpen(true)} className={itemClass}>
              <MessageSquare aria-hidden="true" />
              <span>Comments</span>
            </ContextMenuItem>
          )}
          {file && (
            <ContextMenuItem onClick={() => setVersionsOpen(true)} className={itemClass}>
              <History aria-hidden="true" />
              <span>Version history</span>
            </ContextMenuItem>
          )}
          {file && !readOnly && (
            <ContextMenuItem onClick={() => setLocksOpen(true)} className={itemClass}>
              <Lock aria-hidden="true" />
              <span>Manage locks</span>
            </ContextMenuItem>
          )}
          {folder && isOwner && (
            <ContextMenuItem onClick={() => setRetentionOpen(true)} className={itemClass}>
              <ShieldCheck aria-hidden="true" />
              <span>Retention policy</span>
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
              className={itemClass}
            >
              <Archive aria-hidden="true" />
              <span>Download as zip</span>
            </ContextMenuItem>
          )}
          {!readOnly && <ContextMenuSeparator />}
          {!readOnly && (
            <ContextMenuItem
              variant="destructive"
              onClick={handleDelete}
              disabled={!hasTarget}
              title={hasTarget ? `Move ${targetLabel} to trash` : "Nothing selected to delete"}
              className={cn(itemClass)}
            >
              <Trash2 aria-hidden="true" />
              <span>Delete</span>
            </ContextMenuItem>
          )}
        </ContextMenuContent>
      </ContextMenu>

      {/* ShareDialog handles file OR folder — pass whichever applies. */}
      {(file || folder) && (
        <ShareDialog
          open={shareOpen}
          onOpenChange={setShareOpen}
          fileId={file?.id}
          folderId={folder?.id}
          fileName={file?.name ?? folder?.name ?? ""}
        />
      )}

      {/* Folder-only — opens RequestFilesDialog seeded with this folder. */}
      {folder && (
        <RequestFilesDialog
          open={requestOpen}
          onOpenChange={setRequestOpen}
          folderId={folder.id}
          folderName={folder.name}
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

      {file && (
        <LockManagerDialog
          open={locksOpen}
          onOpenChange={setLocksOpen}
          fileId={file.id}
          fileName={file.name}
        />
      )}

      {folder && (
        <RetentionPolicyDialog
          open={retentionOpen}
          onOpenChange={setRetentionOpen}
          folderId={folder.id}
          folderName={folder.name}
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
