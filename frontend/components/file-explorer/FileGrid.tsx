"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Check, Star, MousePointerClick, AlertCircle } from "lucide-react";
import { FileIcon } from "./FileIcon";
import { FileContextMenu } from "./FileContextMenu";
import { InlineRename } from "./InlineRename";
import { EmptyState } from "./EmptyState";
import { MarqueeOverlay } from "./MarqueeOverlay";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { formatBytes, formatRelative } from "@/lib/format";
import { files as filesApi, folders as foldersApi, request } from "@/lib/api";
import { toast } from "sonner";
import type { FileItem, FolderItem } from "@/lib/types";
import { cn } from "@/lib/utils";

interface Props {
  files: FileItem[];
  folders: FolderItem[];
  onOpenFolder: (folder: FolderItem) => void;
  onPreview: (file: FileItem) => void;
  onMutate: () => void;
  // Multi-select wiring — optional so existing callers (e.g. shared/
  // trash pages) keep working without selection state.
  selectedIds?: Set<string>;
  onItemClick?: (id: string, e: React.MouseEvent) => void;
  // Marquee selection — called with the ids hit by the drag rectangle
  // (additive=true when modifier held, false when starting fresh).
  onMarquee?: (ids: string[], additive: boolean) => void;
  onClearSelection?: () => void;
  // Empty state CTAs.
  onUploadClick?: () => void;
  onNewFolderClick?: () => void;
  // False when the current folder is shared read-only (viewer grant): hides
  // per-item write actions and the empty-state upload/new-folder CTAs.
  canWrite?: boolean;
  // Optional UX states — purely additive so existing callers keep working.
  // `loading` swaps the grid for a skeleton matrix; `error` shows a graceful
  // inline error card with an optional retry handler.
  loading?: boolean;
  error?: string | null;
  onRetry?: () => void;
}

// Shared layout primitives used by both folder and file cards. Pulled out so
// the selected/hover treatments stay consistent across both code paths.
const CARD_BASE =
  "group/card relative flex flex-col gap-2 p-3 rounded-xl border cursor-pointer select-none min-w-0 overflow-hidden " +
  "transition-[background-color,border-color,box-shadow,transform] duration-200 ease-out " +
  // Subtle elevation on hover — no transform so layout doesn't jitter while
  // the marquee is being dragged across cards.
  "hover:shadow-md hover:shadow-foreground/5 " +
  // Keyboard focus ring — uses focus-visible so mouse clicks don't paint a
  // ring, but tab/arrow navigation gets a clear indicator.
  "outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background " +
  // Subtle press feedback for the active state.
  "active:scale-[0.99]";

const CARD_UNSELECTED =
  "bg-card border-border/60 " +
  // Gradient kicks in on hover for a touch of "premium" without being loud.
  "hover:bg-gradient-to-b hover:from-accent/40 hover:to-card hover:border-border";

const CARD_SELECTED =
  "bg-primary/[0.08] border-primary/70 ring-2 ring-primary/30 shadow-sm";

// Returns a short extension label (uppercased, max 4 chars) for the badge
// shown on non-image cards. Falls back to a generic "FILE" when we can't
// derive anything useful from the name.
function extLabel(name: string): string {
  const dot = name.lastIndexOf(".");
  if (dot <= 0 || dot === name.length - 1) return "FILE";
  const ext = name.slice(dot + 1).toUpperCase();
  return ext.length > 4 ? ext.slice(0, 4) : ext;
}

// Tiny check pill drawn in the top-left of selected cards. Sits above the
// thumbnail so it reads even when an image preview is loaded.
function SelectionCheck() {
  return (
    <span
      aria-hidden
      className="absolute top-2 left-2 z-10 inline-flex h-5 w-5 items-center justify-center rounded-full bg-primary text-primary-foreground shadow-sm ring-2 ring-background transition-transform duration-200 ease-out scale-100"
    >
      <Check className="h-3 w-3" strokeWidth={3} />
    </span>
  );
}

// Folder-tile star — wired to the user_favorites pin list (folders have no
// is_starred column; favourites is the equivalent surface). Shows always
// when the folder is pinned, on hover otherwise. Click is stopPropagation'd
// so it doesn't fall through to the tile's open/select handler.
function FolderFavoriteStar({
  folder,
  onMutate,
}: {
  folder: FolderItem;
  onMutate: () => void;
}) {
  const favorited = !!folder.is_favorited;
  const [busy, setBusy] = useState(false);
  return (
    <button
      type="button"
      aria-label={favorited ? "Remove from favourites" : "Add to favourites"}
      aria-pressed={favorited}
      disabled={busy}
      // The favourites endpoints (POST/DELETE /me/favorites) route through
      // the central request() so a stale token gets refreshed mid-call.
      onClick={async (e) => {
        e.stopPropagation();
        if (busy) return;
        setBusy(true);
        try {
          if (favorited) {
            await request<void>(
              `/api/v2/me/favorites/${folder.id}`,
              { method: "DELETE" },
            );
          } else {
            await request<void>(`/api/v2/me/favorites`, {
              method: "POST",
              body: JSON.stringify({ folder_id: folder.id }),
            });
          }
          onMutate();
        } catch (err) {
          toast.error(err instanceof Error ? err.message : "Could not update favourites");
        } finally {
          setBusy(false);
        }
      }}
      className={cn(
        "absolute top-2 right-2 z-10 inline-flex h-7 w-7 items-center justify-center rounded-full",
        "transition-all duration-150 ease-out",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        favorited
          ? "opacity-100 text-yellow-500 hover:bg-yellow-500/10"
          : "opacity-0 group-hover/card:opacity-100 group-focus-visible/card:opacity-100 text-muted-foreground hover:bg-muted hover:text-foreground",
      )}
    >
      <Star
        className={cn(
          "h-3.5 w-3.5 drop-shadow-sm transition-transform duration-150",
          favorited && "fill-yellow-400 text-yellow-400",
        )}
        aria-hidden="true"
      />
    </button>
  );
}

// Hover hint badge — only visible on hover (and only when the card isn't
// selected, to avoid double-stacking with the check pill).
function OpenHint() {
  return (
    <span className="pointer-events-none absolute bottom-2 right-2 z-10 inline-flex items-center gap-1 rounded-md bg-popover/90 px-1.5 py-0.5 text-[10px] font-medium text-popover-foreground opacity-0 shadow-sm ring-1 ring-border backdrop-blur-sm transition-opacity duration-150 group-hover/card:opacity-100 group-focus-visible/card:opacity-100">
      <MousePointerClick className="h-3 w-3" />
      Open
    </span>
  );
}

interface FileThumbProps {
  file: FileItem;
}

// Thumbnail area — renders an image preview for image MIMEs and falls back
// to the type-coloured FileIcon plus an extension badge for everything
// else. The container is square so cards line up cleanly across a row.
function FileThumb({ file }: FileThumbProps) {
  const isImage = file.mime_type.startsWith("image/");
  const [imgFailed, setImgFailed] = useState(false);
  const [imgLoaded, setImgLoaded] = useState(false);

  return (
    <div className="relative w-full aspect-[5/4] rounded-lg bg-muted/40 border border-border/40 overflow-hidden flex items-center justify-center">
      {isImage && !imgFailed ? (
        <>
          {/* Subtle placeholder shimmer while the thumbnail decodes — keeps
              the tile from popping when the bytes finally land. */}
          {!imgLoaded && (
            <div
              aria-hidden
              className="absolute inset-0 animate-pulse bg-muted/60"
            />
          )}
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src={`/api/v2/files/${file.id}/thumb`}
            alt=""
            loading="lazy"
            decoding="async"
            draggable={false}
            onError={() => setImgFailed(true)}
            onLoad={() => setImgLoaded(true)}
            className={cn(
              "h-full w-full object-cover transition-[transform,opacity] duration-300 ease-out group-hover/card:scale-[1.02]",
              imgLoaded ? "opacity-100" : "opacity-0",
            )}
          />
        </>
      ) : (
        <>
          <FileIcon
            mime={file.mime_type}
            className="h-12 w-12 shrink-0 transition-transform duration-200 ease-out group-hover/card:scale-[1.04]"
          />
          {/* Extension chip — sits over the icon, small and unobtrusive */}
          <span className="absolute bottom-1.5 left-1.5 inline-flex items-center rounded-md bg-background/85 px-1.5 py-0.5 text-[9px] font-semibold tracking-wide text-foreground/80 ring-1 ring-border backdrop-blur-sm">
            {extLabel(file.name)}
          </span>
        </>
      )}
    </div>
  );
}

// Shared grid track classes — keeps the loading skeletons and the real grid
// in lock-step so the layout doesn't shift when data arrives.
const GRID_TRACK_CLASSES =
  "grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-3 sm:gap-4";

// Single placeholder tile, used to fill the grid while data loads. Mirrors
// the real card's dimensions so the layout stays stable on first paint.
function SkeletonTile() {
  return (
    <div
      aria-hidden
      className="flex flex-col gap-2 p-3 rounded-xl border border-border/60 bg-card"
    >
      <Skeleton className="w-full aspect-[5/4] rounded-lg" />
      <div className="w-full min-w-0 space-y-1.5 pt-0.5">
        <Skeleton className="h-3 w-4/5" />
        <Skeleton className="h-2 w-2/5" />
      </div>
    </div>
  );
}

// Inline error panel — graceful, dismissible-feeling card that keeps the
// grid layout intact instead of throwing the user to an empty page.
function GridError({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div
      role="alert"
      className="flex min-h-[40vh] flex-col items-center justify-center px-6 py-16 text-center"
    >
      <div className="mb-4 inline-flex h-14 w-14 items-center justify-center rounded-full bg-destructive/10 text-destructive ring-1 ring-destructive/20">
        <AlertCircle className="h-7 w-7" strokeWidth={1.75} />
      </div>
      <h3 className="mb-1 text-base font-semibold tracking-tight text-foreground">
        Something went wrong
      </h3>
      <p className="max-w-sm text-sm text-muted-foreground">{message}</p>
      {onRetry && (
        <Button
          variant="outline"
          size="sm"
          onClick={onRetry}
          className="mt-5"
        >
          Try again
        </Button>
      )}
    </div>
  );
}

export function FileGrid({
  files, folders, onOpenFolder, onPreview, onMutate,
  selectedIds, onItemClick, onMarquee, onClearSelection,
  onUploadClick, onNewFolderClick, canWrite = true,
  loading, error, onRetry,
}: Props) {
  // The wrapper ref hands the marquee overlay a coordinate space to
  // hit-test against. Both empty and populated states use it so the user
  // can still draw a (no-op) marquee on an empty folder if they want.
  const wrapperRef = useRef<HTMLDivElement | null>(null);
  // Ref to the actual grid track so we can measure column count for arrow
  // navigation. Read from layout (offsetTop) so it stays accurate across
  // responsive breakpoints without us hardcoding the column counts.
  const gridRef = useRef<HTMLDivElement | null>(null);

  // Keyboard arrow navigation between tiles. We compute the column count by
  // walking the rendered children and grouping by offsetTop — works for
  // every breakpoint without duplicating the Tailwind responsive ladder.
  const handleGridKeyDown = useCallback((e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== "ArrowLeft" && e.key !== "ArrowRight" && e.key !== "ArrowUp" && e.key !== "ArrowDown") {
      return;
    }
    const grid = gridRef.current;
    if (!grid) return;
    const tiles = Array.from(
      grid.querySelectorAll<HTMLElement>('[data-grid-tile="true"]'),
    );
    if (tiles.length === 0) return;
    const active = document.activeElement as HTMLElement | null;
    const currentIdx = active ? tiles.indexOf(active) : -1;
    if (currentIdx === -1) {
      // No tile focused yet — focus the first one on any arrow key.
      e.preventDefault();
      tiles[0].focus();
      return;
    }
    // Derive column count from the first row by counting tiles that share
    // the first tile's offsetTop.
    const firstTop = tiles[0].offsetTop;
    let cols = 0;
    for (const t of tiles) {
      if (t.offsetTop === firstTop) cols++;
      else break;
    }
    if (cols < 1) cols = 1;
    let nextIdx = currentIdx;
    if (e.key === "ArrowRight") nextIdx = Math.min(tiles.length - 1, currentIdx + 1);
    else if (e.key === "ArrowLeft") nextIdx = Math.max(0, currentIdx - 1);
    else if (e.key === "ArrowDown") nextIdx = Math.min(tiles.length - 1, currentIdx + cols);
    else if (e.key === "ArrowUp") nextIdx = Math.max(0, currentIdx - cols);
    if (nextIdx !== currentIdx) {
      e.preventDefault();
      tiles[nextIdx].focus();
    }
  }, []);

  // Roving tabindex — exactly one tile is in the tab order at a time so
  // Tab moves into/out of the grid as a single stop, and arrow keys then
  // navigate between tiles.
  const [rovingIndex, setRovingIndex] = useState(0);
  // Reset the roving index whenever the underlying list shrinks below the
  // currently-focused position. Prevents an out-of-bounds tabindex after
  // delete / refresh.
  useEffect(() => {
    const total = folders.length + files.length;
    // Clamp the keyboard-roving focus to a valid index whenever the list
    // shrinks. Derived state, but unavoidable here — the previously focused
    // index would otherwise sit off the end of the grid after a delete.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (total > 0 && rovingIndex >= total) setRovingIndex(0);
  }, [folders.length, files.length, rovingIndex]);

  // Loading state — render the same grid track filled with skeleton tiles
  // so the page doesn't jump when data resolves.
  if (loading) {
    return (
      <div className="relative min-h-[60vh]" aria-busy="true" aria-live="polite">
        <span className="sr-only">Loading files…</span>
        <div className={GRID_TRACK_CLASSES}>
          {Array.from({ length: 12 }).map((_, i) => (
            <SkeletonTile key={i} />
          ))}
        </div>
      </div>
    );
  }

  // Error state — graceful inline card with retry. Sits in place of the
  // grid; the marquee overlay is intentionally suppressed since there's
  // nothing to select.
  if (error) {
    return <GridError message={error} onRetry={onRetry} />;
  }

  if (files.length === 0 && folders.length === 0) {
    // Empty state — replaces the bare "folder empty" with actionable CTAs.
    // In a read-only shared folder the CTAs are suppressed (nothing to do).
    return (
      <EmptyState
        onUpload={canWrite ? onUploadClick : undefined}
        onNewFolder={canWrite ? onNewFolderClick : undefined}
      />
    );
  }

  return (
    // `select-none` kills text selection while marquee-dragging — without
    // it, file/folder names get highlighted as the pointer crosses them.
    // `min-h-[60vh]` ensures there's always enough empty canvas below the
    // grid to start a marquee even in folders with only a handful of
    // items (otherwise the wrapper would shrink-wrap and there'd be
    // nowhere to mousedown).
    <div ref={wrapperRef} className="relative select-none min-h-[60vh]">
      {onMarquee && (
        <MarqueeOverlay
          containerRef={wrapperRef}
          onSelectionChange={onMarquee}
          onClickEmpty={onClearSelection}
        />
      )}
    <div
      ref={gridRef}
      className={GRID_TRACK_CLASSES}
      role="grid"
      aria-label="Files and folders"
      aria-rowcount={folders.length + files.length}
      onKeyDown={handleGridKeyDown}
    >
      {folders.map((folder, idx) => {
        const selected = selectedIds?.has(folder.id) ?? false;
        const tileIdx = idx;
        const updatedLabel = folder.updated_at ? formatRelative(folder.updated_at) : "";
        const ariaLabel = updatedLabel
          ? `Folder: ${folder.name}, updated ${updatedLabel}`
          : `Folder: ${folder.name}`;
        return (
          <FileContextMenu
            key={folder.id}
            folder={folder}
            onOpenFolder={() => onOpenFolder(folder)}
            onMutate={onMutate}
            readOnly={!canWrite}
            isOwner={!folder.shared}
          >
            <div
              data-folder-id={folder.id}
              data-grid-tile="true"
              role="gridcell"
              aria-selected={selected}
              aria-label={ariaLabel}
              tabIndex={tileIdx === rovingIndex ? 0 : -1}
              onFocus={() => setRovingIndex(tileIdx)}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  onOpenFolder(folder);
                }
              }}
              className={cn(CARD_BASE, selected ? CARD_SELECTED : CARD_UNSELECTED)}
              // Modifier-click (Ctrl/Cmd/Shift) participates in multi-select
              // — /files:batch now accepts folder_ids alongside file_ids.
              // A plain click without modifiers still opens the folder so
              // the everyday "click to navigate" interaction is unchanged.
              onClick={(e) => {
                const modifier =
                  e.shiftKey || e.ctrlKey || e.metaKey;
                if (modifier && onItemClick) {
                  onItemClick(folder.id, e);
                  return;
                }
                onOpenFolder(folder);
              }}
              onDoubleClick={() => onOpenFolder(folder)}
              title={folder.name}
            >
              {selected && <SelectionCheck />}
              {!selected && <OpenHint />}

              {/* Favourite-pin star — surfaced on hover, or always when
                  pinned. Toggling routes through user_favorites since
                  folders have no is_starred column. */}
              <FolderFavoriteStar
                folder={folder}
                onMutate={onMutate}
              />

              {/* Folder thumbnail area — same aspect-ratio as file cards
                  so the grid stays visually aligned across mixed rows. */}
              <div className="relative w-full aspect-[5/4] rounded-lg bg-gradient-to-br from-yellow-500/10 via-amber-500/5 to-transparent border border-border/40 overflow-hidden flex items-center justify-center">
                <FileIcon
                  isFolder
                  className="h-14 w-14 shrink-0 drop-shadow-sm transition-transform duration-200 ease-out group-hover/card:scale-[1.04]"
                />
              </div>

              <div className="w-full min-w-0 space-y-0.5">
                {/* Inline rename — F2 / double-click name to edit */}
                {canWrite ? (
                  <InlineRename
                    className="text-xs font-medium leading-snug text-foreground"
                    name={folder.name}
                    onRename={async (newName) => {
                      await foldersApi.update(folder.id, { name: newName });
                      onMutate();
                    }}
                  />
                ) : (
                  <p className="text-xs font-medium leading-snug text-foreground truncate" title={folder.name}>
                    {folder.name}
                  </p>
                )}
                <p className="text-[10px] leading-tight text-muted-foreground truncate">
                  Folder
                  {folder.updated_at ? ` · ${formatRelative(folder.updated_at)}` : ""}
                </p>
              </div>
            </div>
          </FileContextMenu>
        );
      })}

      {files.map((file, idx) => {
        const selected = selectedIds?.has(file.id) ?? false;
        const tileIdx = folders.length + idx;
        const updatedLabel = file.updated_at ? formatRelative(file.updated_at) : "";
        const sizeLabel = formatBytes(file.size_bytes);
        const ariaLabel = [
          `File: ${file.name}`,
          sizeLabel,
          updatedLabel ? `updated ${updatedLabel}` : "",
          file.is_starred ? "starred" : "",
        ]
          .filter(Boolean)
          .join(", ");
        return (
          <FileContextMenu
            key={file.id}
            file={file}
            onPreview={() => onPreview(file)}
            onMutate={onMutate}
            readOnly={!canWrite}
            isOwner={!file.shared}
          >
            <div
              data-file-id={file.id}
              data-grid-tile="true"
              role="gridcell"
              aria-selected={selected}
              aria-label={ariaLabel}
              tabIndex={tileIdx === rovingIndex ? 0 : -1}
              onFocus={() => setRovingIndex(tileIdx)}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  onPreview(file);
                }
              }}
              className={cn(CARD_BASE, selected ? CARD_SELECTED : CARD_UNSELECTED)}
              onClick={(e) => onItemClick?.(file.id, e)}
              onDoubleClick={() => onPreview(file)}
              title={file.name}
            >
              {selected && <SelectionCheck />}
              {!selected && <OpenHint />}

              {/* Star sits in the top-right; mirrored across both the
                  selected and unselected states so its position never
                  jumps between renders. */}
              {file.is_starred && !file.shared && (
                <Star
                  aria-label="Starred"
                  className="absolute top-2 right-2 z-10 h-3.5 w-3.5 fill-yellow-400 text-yellow-400 drop-shadow-sm"
                />
              )}

              <FileThumb file={file} />

              <div className="w-full min-w-0 space-y-0.5">
                {/* Inline rename for files. Note the dialog still
                    works as a backup via FileContextMenu. */}
                {canWrite ? (
                  <InlineRename
                    className="text-xs font-medium leading-snug text-foreground"
                    name={file.name}
                    onRename={async (newName) => {
                      await filesApi.update(file.id, { name: newName });
                      onMutate();
                    }}
                  />
                ) : (
                  <p className="text-xs font-medium leading-snug text-foreground truncate" title={file.name}>
                    {file.name}
                  </p>
                )}
                {/* Subtitle: size + relative date, kept on a single line
                    via truncate so the card never grows vertically on
                    long values. */}
                <p className="text-[10px] leading-tight text-muted-foreground truncate">
                  {sizeLabel}
                  {file.updated_at ? ` · ${formatRelative(file.updated_at)}` : ""}
                </p>
              </div>
            </div>
          </FileContextMenu>
        );
      })}
    </div>
    </div>
  );
}
