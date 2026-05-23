"use client";

// Floating multi-select toolbar.
//
// Renders when selectedIds is non-empty; sits at the bottom of the viewport
// like a sticky bar. Each action dispatches to the backend's batch API,
// so 50 files delete in one round-trip instead of 50.
//
// Parents are responsible for tracking the selection state via
// useSelection() below — the hook handles shift-click range and
// ctrl/cmd-click toggle semantics so callers just hand it an array of
// "currently visible" ids and a click event.

import { useCallback, useState } from "react";
import { Trash2, Move, Share2, X, Star, StarOff, Download, Loader2 } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { files as filesApi, request } from "@/lib/api";
import { MoveDialog } from "@/components/modals/MoveDialog";
import { ShareDialog } from "@/components/modals/ShareDialog";
import type { FileItem, FolderItem } from "@/lib/types";

// Route through the central API client so 401 → refresh → retry,
// rate-limit (429) handling, and uniform error parsing apply here too.
async function postBatch<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, { method: "POST", body: JSON.stringify(body) });
}

type BatchAction = "delete" | "trash" | "restore" | "star" | "unstar" | "move";

type Props = {
  selectedIds: string[];
  // The currently-visible file/folder lists so Share can find a single
  // item's metadata, Download can iterate names for the zip filename, and
  // we can partition selectedIds into file_ids vs folder_ids before posting.
  files?: FileItem[];
  folders?: FolderItem[];
  onClear: () => void;
  // onSuccess is called after a successful batch so parents can mutate
  // their SWR caches. For move batches we also pass the destination
  // folder id so the caller can invalidate the *destination* listing,
  // not just the source folder the user was viewing.
  onSuccess: (action: BatchAction, targetFolderId?: string | null) => void;
  // False when the selection lives in a shared folder the user can only view:
  // Move/Trash hide. Star/Share additionally hide when any selected item is
  // shared (owner-only actions). Default true (own drive).
  canWrite?: boolean;
};

export function SelectionToolbar({ selectedIds, files, folders, onClear, onSuccess, canWrite = true }: Props) {
  // RULES OF HOOKS: all hooks must be called unconditionally in the same
  // order on every render. We previously had an early `return null` between
  // useState and useCallback which tripped React error #310 the moment a
  // selection appeared / disappeared. Keep every hook above the conditional.
  // `activeAction` doubles as the busy flag (truthy = batch in flight) and
  // tells the render layer which button should show the spinner. Keeping
  // both in a single state avoids out-of-sync flickers.
  const [activeAction, setActiveAction] = useState<BatchAction | "download" | null>(null);
  const busy = activeAction !== null;
  // Dialog visibility — MoveDialog accepts a single fileId today, so for
  // batch moves we open it as a folder picker per-item via the batch API.
  const [movePickerOpen, setMovePickerOpen] = useState(false);
  const [shareOpen, setShareOpen] = useState(false);

  // Partition the selection by type — folders take the folder_ids slot,
  // everything else (files) goes to file_ids. The visible lists drive the
  // partition since selectedIds is just opaque strings.
  const folderIdSet = new Set((folders ?? []).map((f) => f.id));
  const selectedFolderIds = selectedIds.filter((id) => folderIdSet.has(id));
  const selectedFileIds = selectedIds.filter((id) => !folderIdSet.has(id));

  const runBatch = useCallback(
    async (action: BatchAction, params?: Record<string, unknown>) => {
      setActiveAction(action);
      try {
        const res = await postBatch<{ results: Array<{ id: string; ok: boolean; error?: string }> }>(
          "/api/v2/files:batch",
          {
            action,
            file_ids: selectedFileIds,
            folder_ids: selectedFolderIds,
            params,
          },
        );
        const failed = (res.results ?? []).filter((r) => !r.ok);
        if (failed.length === 0) {
          toast.success(`${action} applied to ${selectedIds.length} item(s)`);
        } else {
          toast.warning(`${action}: ${selectedIds.length - failed.length} ok, ${failed.length} failed`);
        }
        // Only "move" carries a destination folder id in params — other
        // actions pass undefined so the caller falls back to refreshing
        // just the current folder.
        const targetFolderId =
          action === "move"
            ? ((params?.folder_id as string | null | undefined) ?? null)
            : undefined;
        onSuccess(action, targetFolderId);
        onClear();
      } catch (err) {
        const msg = err instanceof Error ? err.message : "Batch failed";
        toast.error(msg);
      } finally {
        setActiveAction(null);
      }
    },
    [selectedIds, selectedFileIds, selectedFolderIds, onSuccess, onClear],
  );

  // Download → zip via the existing /files:download-archive endpoint.
  // The endpoint already accepts file_ids and folder_ids; we partition the
  // selection and ship both so a mixed download walks the folder trees
  // server-side.
  const downloadZip = useCallback(async () => {
    setActiveAction("download");
    try {
      await filesApi.downloadArchive(selectedFileIds, selectedFolderIds);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Download failed");
    } finally {
      setActiveAction(null);
    }
  }, [selectedFileIds, selectedFolderIds]);

  // Single-item branch: sharing only makes sense for one item in v1. It can
  // be a file OR a folder — ShareDialog handles both.
  const canShare = selectedIds.length === 1;
  const singleFile = canShare ? files?.find((f) => f.id === selectedIds[0]) : undefined;
  const singleFolder = canShare ? folders?.find((f) => f.id === selectedIds[0]) : undefined;
  const singleShareable = singleFile ?? singleFolder;

  // Star vs Unstar — show exactly one based on the combined selection. For
  // a heterogeneous selection (some files, some folders), we treat the
  // group as "all starred" only when every file is starred AND every
  // folder is favourited. Otherwise the dominant action is Star, which
  // both stars un-starred files and favourites un-favourited folders in
  // one batch round-trip.
  const selectedFiles = files?.filter((f) => selectedFileIds.includes(f.id)) ?? [];
  const selectedFolders =
    folders?.filter((f) => selectedFolderIds.includes(f.id)) ?? [];
  // Any shared (non-owned) item disables owner-only actions (file star, share).
  const hasShared =
    selectedFiles.some((f) => f.shared) || selectedFolders.some((f) => f.shared);
  const filesAllStarred =
    selectedFiles.length === 0 || selectedFiles.every((f) => f.is_starred);
  const foldersAllFavorited =
    selectedFolders.length === 0 ||
    selectedFolders.every((f) => f.is_favorited);
  const allStarred =
    selectedIds.length > 0 && filesAllStarred && foldersAllFavorited;
  const starAction: "star" | "unstar" = allStarred ? "unstar" : "star";

  // Conditional render goes AFTER the hooks above — never between them.
  if (selectedIds.length === 0) return null;

  const count = selectedIds.length;
  // Leading icon shared spacing — uniform optical alignment across labels.
  const iconCls = "size-4 mr-1.5";
  // Spinner overrides the action's normal icon while that action is in flight.
  const renderLeadingIcon = (
    action: BatchAction | "download",
    Icon: React.ComponentType<{ className?: string }>,
  ) =>
    activeAction === action ? (
      <Loader2 className={cn(iconCls, "animate-spin")} aria-hidden="true" />
    ) : (
      <Icon className={iconCls} />
    );

  return (
    <>
      <div
        data-no-hotkeys="true"
        role="toolbar"
        aria-label={`${count} selected`}
        aria-busy={busy}
        aria-orientation="horizontal"
        className={cn(
          "fixed bottom-4 left-1/2 -translate-x-1/2 z-40",
          // Layout: tight, consistent button-group rhythm.
          "flex items-center gap-0.5 px-2 py-1.5 text-sm",
          // Floating chip surface: subtle ring + layered shadow for depth.
          "rounded-xl border bg-background/95 backdrop-blur",
          "shadow-lg ring-1 ring-black/5 dark:ring-white/10",
          // Smooth surface transitions + enter animation when it appears.
          "transition-all duration-200 ease-out",
          "animate-in fade-in-0 slide-in-from-bottom-2 zoom-in-95",
          // Slightly suppress interactivity affordances while batching.
          busy && "cursor-progress",
        )}
      >
        <span
          className="px-2 font-medium text-foreground tabular-nums leading-none whitespace-nowrap"
          aria-live="polite"
        >
          <span className="text-foreground">{count}</span>{" "}
          <span className="text-muted-foreground font-normal">selected</span>
        </span>

        <Separator orientation="vertical" className="mx-1 h-5" />

        {!hasShared && (
          <Button
            variant="ghost"
            size="sm"
            disabled={busy}
            onClick={() => runBatch(starAction)}
            aria-label={`${starAction === "unstar" ? "Unstar" : "Star"} ${count} item${count === 1 ? "" : "s"}`}
            className="transition-colors"
          >
            {starAction === "unstar"
              ? renderLeadingIcon("unstar", StarOff)
              : renderLeadingIcon("star", Star)}
            <span>{starAction === "unstar" ? "Unstar" : "Star"}</span>
          </Button>
        )}
        {canWrite && (
          <Button
            variant="ghost"
            size="sm"
            disabled={busy}
            onClick={() => setMovePickerOpen(true)}
            aria-label={`Move ${count} item${count === 1 ? "" : "s"}`}
            aria-haspopup="dialog"
            className="transition-colors"
          >
            {renderLeadingIcon("move", Move)}
            <span>Move</span>
          </Button>
        )}
        <Button
          variant="ghost"
          size="sm"
          disabled={busy}
          onClick={downloadZip}
          aria-label={`Download ${count} item${count === 1 ? "" : "s"} as zip`}
          className="transition-colors"
        >
          {renderLeadingIcon("download", Download)}
          <span>Download .zip</span>
        </Button>
        {canShare && singleShareable && !hasShared && (
          <Button
            variant="ghost"
            size="sm"
            disabled={busy}
            onClick={() => setShareOpen(true)}
            aria-label={`Share ${singleShareable.name}`}
            aria-haspopup="dialog"
            className="transition-colors"
          >
            <Share2 className={iconCls} />
            <span>Share</span>
          </Button>
        )}

        {canWrite && (
          <>
            <Separator orientation="vertical" className="mx-1 h-5" />

            <Button
              variant="ghost"
              size="sm"
              disabled={busy}
              data-selection-toolbar="trash"
              onClick={() => runBatch("trash")}
              aria-label={`Move ${count} item${count === 1 ? "" : "s"} to trash`}
              className="text-destructive hover:bg-destructive/10 hover:text-destructive focus-visible:ring-destructive/30 transition-colors"
            >
              {renderLeadingIcon("trash", Trash2)}
              <span>Trash</span>
            </Button>
          </>
        )}

        <TooltipProvider delay={300}>
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon-sm"
                  onClick={onClear}
                  aria-label="Clear selection"
                  aria-keyshortcuts="Escape"
                  className="ml-0.5 transition-colors"
                >
                  <X className="size-4" />
                </Button>
              }
            />
            <TooltipContent side="top" sideOffset={6}>
              Clear selection (Esc)
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      </div>

      {/* Batch move: MoveDialog's onMoved callback hands us the picked
          folder id; we dispatch it through the same /files:batch move
          action so the multi-item operation lands in one round-trip with
          a single operation_id for rollback. */}
      {movePickerOpen && (
        <MoveDialog
          open={movePickerOpen}
          onOpenChange={setMovePickerOpen}
          // Use the first selected id as the "anchor" so MoveDialog's
          // current-folder breadcrumb has something to render. The batch
          // call then moves all of them.
          fileId={selectedIds[0]}
          batchMode
          onMoved={async (newFolderId) => {
            await runBatch("move", { folder_id: newFolderId ?? null });
          }}
        />
      )}

      {shareOpen && singleShareable && (
        <ShareDialog
          open={shareOpen}
          onOpenChange={setShareOpen}
          fileId={singleFile?.id}
          folderId={singleFolder?.id}
          fileName={singleShareable.name}
        />
      )}
    </>
  );
}

// useSelection encapsulates shift-range + ctrl-toggle semantics so
// FileExplorer doesn't grow an extra ~50 lines of click handling.
export function useSelection(allVisibleIds: string[]) {
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [lastClicked, setLastClicked] = useState<string | null>(null);

  const onClick = useCallback(
    (id: string, e: React.MouseEvent) => {
      // The selection set is file-only — /files:batch can't operate on
      // folder ids and returns "no access" if it sees one. Anything not in
      // the visible file list is silently rejected so a stray caller
      // (e.g. a folder tile that mis-wires onClick) can't poison the set.
      if (!allVisibleIds.includes(id)) return;
      if (e.shiftKey && lastClicked) {
        // Range selection between lastClicked and id (inclusive).
        const from = allVisibleIds.indexOf(lastClicked);
        const to = allVisibleIds.indexOf(id);
        if (from >= 0 && to >= 0) {
          const [lo, hi] = from < to ? [from, to] : [to, from];
          setSelected((prev) => {
            const next = new Set(prev);
            for (let i = lo; i <= hi; i++) next.add(allVisibleIds[i]);
            return next;
          });
          return;
        }
      }
      const modKey = e.ctrlKey || e.metaKey;
      if (modKey) {
        setSelected((prev) => {
          const next = new Set(prev);
          if (next.has(id)) next.delete(id);
          else next.add(id);
          return next;
        });
        setLastClicked(id);
        return;
      }
      // Plain click — single-select.
      setSelected(new Set([id]));
      setLastClicked(id);
    },
    [allVisibleIds, lastClicked],
  );

  const clear = useCallback(() => {
    setSelected(new Set());
    setLastClicked(null);
  }, []);

  return {
    selectedIds: Array.from(selected),
    selectedSet: selected,
    onClick,
    clear,
    // setSelected lets external mechanisms (marquee, Ctrl+A) drive the
    // selection state directly. Callers may pass either a Set or an
    // updater fn — same surface as React's setState.
    setSelected,
  };
}
