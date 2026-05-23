"use client";

import { useState, useEffect } from "react";
import useSWR from "swr";
import { toast } from "sonner";
import { Folder, ChevronRight, Home } from "lucide-react";

import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter,
} from "@/components/ui/dialog";
import {
  Tooltip, TooltipContent, TooltipProvider, TooltipTrigger,
} from "@/components/ui/tooltip";
import { Button } from "@/components/ui/button";
import { files as filesApi, folders as foldersApi } from "@/lib/api";
import type { FileItem, FolderItem } from "@/lib/types";
import { cn } from "@/lib/utils";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The file to move (mutually exclusive with folder) */
  file?: FileItem;
  /** The folder to move (mutually exclusive with file) */
  folder?: FolderItem;
  // Batch mode: the multi-select toolbar uses this dialog as a
  // pure folder picker. In batch mode we skip the internal single-item
  // PATCH and hand the picked folder id back via onMoved; the caller
  // dispatches a single /files:batch move instead.
  batchMode?: boolean;
  fileId?: string;
  onMoved: (folderId?: string | null) => void | Promise<void>;
}

export function MoveDialog({ open, onOpenChange, file, folder, batchMode, fileId, onMoved }: Props) {
  const [pickerPath, setPickerPath]       = useState<FolderItem[]>([]);
  const [pickerFolderId, setPickerFolderId] = useState<string | undefined>(undefined);
  const [moving, setMoving]               = useState(false);
  // Which tree the picker browses: the caller's own drive, or folders shared
  // with them. The latter lets an editor relocate items into a shared folder
  // (the backend reassigns ownership on a cross-owner move).
  const [rootMode, setRootMode]           = useState<"own" | "shared">("own");

  // Reset picker when dialog opens
  useEffect(() => {
    if (open) {
      setPickerPath([]);
      setPickerFolderId(undefined);
      setRootMode("own");
    }
  }, [open]);

  const { data, isLoading } = useSWR(
    open ? `move-picker-${rootMode}-${pickerFolderId ?? "root"}` : null,
    () =>
      rootMode === "shared" && !pickerFolderId
        ? foldersApi.sharedRoots()
        : foldersApi.list(pickerFolderId),
  );

  const switchRoot = (mode: "own" | "shared") => {
    setRootMode(mode);
    setPickerPath([]);
    setPickerFolderId(undefined);
  };

  // IDs the picker must never show or navigate into. Covers:
  //   - single-folder move from the context menu (folder.id),
  //   - batch move where the toolbar's anchor id may itself be a folder
  //     in the selection (fileId).
  // We can't cheaply compute the full descendant set client-side; the
  // backend rejects moves into descendants with 400 "validation_error",
  // so anything below the picker's reach falls through to that guard.
  const forbiddenIds = new Set<string>();
  if (folder?.id) forbiddenIds.add(folder.id);
  if (batchMode && fileId) forbiddenIds.add(fileId);

  const available = (data?.folders ?? []).filter((f) => !forbiddenIds.has(f.id));

  const navigateInto = (f: FolderItem) => {
    setPickerPath((p) => [...p, f]);
    setPickerFolderId(f.id);
  };

  const navigateTo = (index: number) => {
    if (index < 0) {
      setPickerPath([]);
      setPickerFolderId(undefined);
    } else {
      const newPath = pickerPath.slice(0, index + 1);
      setPickerPath(newPath);
      setPickerFolderId(newPath[newPath.length - 1].id);
    }
  };

  const handleMove = async () => {
    setMoving(true);
    try {
      const targetId = pickerFolderId ?? null;
      if (batchMode) {
        // Caller (selection toolbar) owns the batch dispatch.
        await onMoved(targetId);
      } else if (file) {
        await filesApi.update(file.id, { folder_id: targetId });
        toast.success(`"${file.name}" moved`);
        await onMoved(targetId);
      } else if (folder) {
        await foldersApi.update(folder.id, { parent_id: targetId });
        toast.success(`"${folder.name}" moved`);
        await onMoved(targetId);
      }
      onOpenChange(false);
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Move failed");
    } finally {
      setMoving(false);
    }
  };

  const itemName  = file?.name ?? folder?.name ?? "";
  const currentParentId = file?.folder_id ?? folder?.parent_id;
  // In batch mode the dialog is just a folder picker — we don't know each
  // selected item's parent, so we can't tell whether the picked folder is a
  // no-op. Allow the move; the backend will skip anything already there.
  const isSameLocation = batchMode
    ? false
    : (pickerFolderId ?? undefined) === (currentParentId ?? undefined);
  // Block "Move here" when the picker is currently *inside* the folder
  // being moved (or one of its ancestors), which would create a cycle.
  // The available-list filter already prevents navigating into `forbiddenIds`
  // from the picker, but defending against externally-driven state (e.g.
  // a future deep-link into the picker) is cheap and the click target is
  // the most user-visible affordance.
  const movingIntoSelf =
    !!pickerFolderId && forbiddenIds.has(pickerFolderId);
  const movingIntoDescendant = pickerPath.some((p) => forbiddenIds.has(p.id));
  // "Shared with me" at the top level lists shared roots, not a concrete
  // destination — require navigating into one before allowing the move.
  const atSharedRoot = rootMode === "shared" && !pickerFolderId;
  const disableMove = moving || isSameLocation || movingIntoSelf || movingIntoDescendant || atSharedRoot;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {/* Grid layout with min-h-0 row allows the tree (middle row) to scroll
          while header + footer stay pinned; max-h caps overall dialog height
          on small screens. */}
      <DialogContent
        className={cn(
          "max-w-md grid-rows-[auto_auto_minmax(0,1fr)_auto]",
          "max-h-[calc(100dvh-2rem)] sm:max-h-[85vh]",
        )}
      >
        <TooltipProvider delay={300}>
          <DialogHeader className="min-w-0">
            <DialogTitle className="truncate">
              Move {file ? "file" : "folder"}
            </DialogTitle>
            <Tooltip>
              <TooltipTrigger
                render={
                  <DialogDescription
                    className="truncate font-mono text-xs min-w-0"
                    title={itemName}
                  >
                    {itemName}
                  </DialogDescription>
                }
              />
              <TooltipContent
                side="bottom"
                align="start"
                sideOffset={6}
                className="max-w-[min(90vw,32rem)] break-all font-mono"
              >
                {itemName}
              </TooltipContent>
            </Tooltip>
          </DialogHeader>

          <div className="space-y-2 min-w-0">
          {/* Source-tree switcher: own drive vs folders shared with you. */}
          <div className="flex items-center gap-1">
            <Button
              type="button"
              variant={rootMode === "own" ? "secondary" : "ghost"}
              size="sm"
              className="h-7 px-2.5 text-xs"
              onClick={() => switchRoot("own")}
            >
              My Files
            </Button>
            <Button
              type="button"
              variant={rootMode === "shared" ? "secondary" : "ghost"}
              size="sm"
              className="h-7 px-2.5 text-xs"
              onClick={() => switchRoot("shared")}
            >
              Shared with me
            </Button>
          </div>

          {/* Breadcrumb inside picker — flex-wrap keeps it on-screen on
              narrow viewports; min-w-0 lets children truncate instead of
              forcing horizontal overflow. */}
          <div className="flex items-center gap-1 flex-wrap text-sm min-h-[1.5rem] min-w-0 max-w-full overflow-x-auto">
            <button
              className="text-muted-foreground hover:text-foreground flex items-center gap-1 shrink-0"
              onClick={() => navigateTo(-1)}
            >
              <Home className="h-3.5 w-3.5" />
              {rootMode === "shared" ? "Shared with me" : "My Files"}
            </button>
            {pickerPath.map((f, i) => (
              <span key={f.id} className="flex items-center gap-1 min-w-0 max-w-full">
                <ChevronRight className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <button
                        className="hover:text-foreground max-w-[140px] sm:max-w-[200px] truncate min-w-0"
                        title={f.name}
                        onClick={() => navigateTo(i)}
                      >
                        {f.name}
                      </button>
                    }
                  />
                  <TooltipContent
                    side="bottom"
                    align="start"
                    sideOffset={6}
                    className="max-w-[min(90vw,32rem)] break-all"
                  >
                    {f.name}
                  </TooltipContent>
                </Tooltip>
              </span>
            ))}
          </div>
          </div>

          {/* Folder list — scrollable middle region. max-h-[60vh] caps it on
              tall screens; min-h-0 lets the grid row actually shrink so the
              footer stays pinned + visible. */}
          <div className="border rounded-lg overflow-hidden max-h-[60vh] min-h-0 overflow-y-auto">
            {isLoading ? (
              <p className="p-4 text-sm text-center text-muted-foreground">Loading…</p>
            ) : available.length === 0 ? (
              <p className="p-4 text-sm text-center text-muted-foreground">
                {atSharedRoot ? "Nothing shared with you" : "No subfolders here"}
              </p>
            ) : (
              <div className="divide-y">
                {available.map((f) => (
                  <Tooltip key={f.id}>
                    <TooltipTrigger
                      render={
                        <button
                          className="w-full flex items-center gap-2 px-3 py-2.5 hover:bg-accent text-left min-w-0"
                          onClick={() => navigateInto(f)}
                        >
                          <Folder className="h-4 w-4 shrink-0 text-muted-foreground" />
                          <span className="text-sm flex-1 truncate min-w-0" title={f.name}>{f.name}</span>
                          <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
                        </button>
                      }
                    />
                    <TooltipContent
                      side="right"
                      align="center"
                      sideOffset={6}
                      className="max-w-[min(90vw,32rem)] break-all"
                    >
                      {f.name}
                    </TooltipContent>
                  </Tooltip>
                ))}
              </div>
            )}
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button onClick={handleMove} disabled={disableMove}>
              {moving
                ? "Moving…"
                : movingIntoSelf
                ? "Can't move into itself"
                : movingIntoDescendant
                ? "Can't move into a subfolder"
                : atSharedRoot
                ? "Pick a folder"
                : isSameLocation
                ? "Already here"
                : "Move here"}
            </Button>
          </DialogFooter>
        </TooltipProvider>
      </DialogContent>
    </Dialog>
  );
}
