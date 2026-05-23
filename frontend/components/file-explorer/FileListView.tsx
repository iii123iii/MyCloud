"use client";

import { useMemo, useRef, useState, type KeyboardEvent } from "react";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { FileIcon } from "./FileIcon";
import { FileContextMenu } from "./FileContextMenu";
import { formatBytes, formatRelative } from "@/lib/format";
import { ArrowDown, ArrowUp, ArrowUpDown, Eye, FolderOpen, MoreHorizontal, Star } from "lucide-react";
import { parseServerDate } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { FileItem, FolderItem } from "@/lib/types";

interface Props {
  files: FileItem[];
  folders: FolderItem[];
  onOpenFolder: (folder: FolderItem) => void;
  onPreview: (file: FileItem) => void;
  onMutate: () => void;
  // False when browsing a shared folder read-only (viewer grant): the row
  // context menus hide their write actions. Default true (own drive).
  canWrite?: boolean;
}

type SortKey = "name" | "size" | "modified";
type SortDir = "asc" | "desc";

/**
 * Render a click-target header label with a sort-direction indicator.
 *
 * The indicator is a stable up/down arrow when this column is active, and a
 * neutral two-way arrow otherwise. Keeping the icon size identical across
 * states avoids width-jumping when the user toggles sort.
 */
function SortHeader({
  label,
  active,
  dir,
  onClick,
  align = "left",
}: {
  label: string;
  active: boolean;
  dir: SortDir;
  onClick: () => void;
  align?: "left" | "right";
}) {
  const Icon = active ? (dir === "asc" ? ArrowUp : ArrowDown) : ArrowUpDown;
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        // Tight inline-flex keeps the chevron snug to the label. The rounded
        // hit-target + focus ring give keyboard users a clear affordance.
        "inline-flex items-center gap-1.5 select-none text-foreground/90",
        "rounded-sm -mx-1 px-1 py-0.5",
        "transition-colors duration-150",
        "hover:text-foreground",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background",
        align === "right" && "w-full justify-end",
      )}
    >
      <span>{label}</span>
      <Icon
        className={cn(
          "h-3.5 w-3.5 shrink-0 transition-all duration-200 ease-out",
          active ? "opacity-100 scale-100" : "opacity-40 scale-90",
        )}
        aria-hidden="true"
      />
    </button>
  );
}

export function FileListView({ files, folders, onOpenFolder, onPreview, onMutate, canWrite = true }: Props) {
  const [sortKey, setSortKey] = useState<SortKey>("name");
  const [sortDir, setSortDir] = useState<SortDir>("asc");
  // Tracks the currently focused row index for roving-tabindex keyboard nav.
  // -1 means "no row focused yet"; arrow keys land focus on row 0 first.
  const [activeIdx, setActiveIdx] = useState<number>(-1);
  const tbodyRef = useRef<HTMLTableSectionElement | null>(null);

  const toggleSort = (k: SortKey) => {
    if (sortKey === k) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortKey(k);
      setSortDir("asc");
    }
  };

  const sortedFolders = useMemo(() => {
    const arr = [...folders];
    const mul = sortDir === "asc" ? 1 : -1;
    arr.sort((a, b) => {
      switch (sortKey) {
        case "modified": {
          const da = a.updated_at ? parseServerDate(a.updated_at).getTime() : 0;
          const db = b.updated_at ? parseServerDate(b.updated_at).getTime() : 0;
          return (da - db) * mul;
        }
        case "size":
          // Folders have no size — keep their existing order so they stay
          // grouped at the top regardless of file size direction.
          return 0;
        case "name":
        default:
          return a.name.localeCompare(b.name) * mul;
      }
    });
    return arr;
  }, [folders, sortKey, sortDir]);

  const sortedFiles = useMemo(() => {
    const arr = [...files];
    const mul = sortDir === "asc" ? 1 : -1;
    arr.sort((a, b) => {
      switch (sortKey) {
        case "size":
          return (a.size_bytes - b.size_bytes) * mul;
        case "modified": {
          const da = parseServerDate(a.updated_at).getTime();
          const db = parseServerDate(b.updated_at).getTime();
          return (da - db) * mul;
        }
        case "name":
        default:
          return a.name.localeCompare(b.name) * mul;
      }
    });
    return arr;
  }, [files, sortKey, sortDir]);

  const totalRows = sortedFolders.length + sortedFiles.length;

  /**
   * Move focus to the row at `idx` within the tbody. Uses a data-attribute
   * lookup so we don't have to maintain a ref array — the table is small,
   * and querySelector by index is plenty fast at this scale.
   */
  const focusRowAt = (idx: number) => {
    const clamped = Math.max(0, Math.min(idx, totalRows - 1));
    setActiveIdx(clamped);
    const tbody = tbodyRef.current;
    if (!tbody) return;
    const row = tbody.querySelector<HTMLTableRowElement>(`tr[data-row-index="${clamped}"]`);
    row?.focus();
  };

  /**
   * Roving-tabindex keyboard handler. Arrow Up/Down moves focus between rows,
   * Home/End jump to the ends, and Enter/Space activates the row's primary
   * action (open folder or preview file).
   */
  const onRowKeyDown = (
    e: KeyboardEvent<HTMLTableRowElement>,
    idx: number,
    activate: () => void,
  ) => {
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        focusRowAt(idx + 1);
        break;
      case "ArrowUp":
        e.preventDefault();
        focusRowAt(idx - 1);
        break;
      case "Home":
        e.preventDefault();
        focusRowAt(0);
        break;
      case "End":
        e.preventDefault();
        focusRowAt(totalRows - 1);
        break;
      case "Enter":
      case " ":
        e.preventDefault();
        activate();
        break;
    }
  };

  if (files.length === 0 && folders.length === 0) {
    return (
      <div
        role="status"
        aria-live="polite"
        className={cn(
          "flex flex-col items-center justify-center gap-3 py-20",
          "text-muted-foreground",
          "border border-dashed rounded-lg",
        )}
      >
        <div className="flex h-12 w-12 items-center justify-center rounded-full bg-muted/60">
          <FolderOpen className="h-6 w-6" aria-hidden="true" />
        </div>
        <p className="text-sm font-medium text-foreground/80">This folder is empty</p>
        <p className="text-xs">Drag files here or use the upload button to get started.</p>
      </div>
    );
  }

  // aria-sort value for the active column header. Inactive headers get "none".
  const ariaSortFor = (k: SortKey): "ascending" | "descending" | "none" =>
    sortKey === k ? (sortDir === "asc" ? "ascending" : "descending") : "none";

  return (
    <>
      {/* ── Desktop / tablet: table view ───────────────────────────────── */}
      {/* table-fixed pins column widths to the <TableHead> declarations.
          Without it, `max-w-0` on the Name cell (the Tailwind hack for
          truncate inside an auto-layout cell) was collapsing the Name
          column to 0px — the filename rendered but the cell had no width
          to show it in, so the column appeared empty. */}
      <div className="hidden md:block border rounded-lg overflow-hidden bg-background">
        <Table className="table-fixed">
          <TableHeader>
            <TableRow className="bg-muted/40 hover:bg-muted/40 border-b">
              <TableHead className="w-10 pl-3" aria-label="File type" />
              <TableHead className="px-3" aria-sort={ariaSortFor("name")}>
                <SortHeader
                  label="Name"
                  active={sortKey === "name"}
                  dir={sortDir}
                  onClick={() => toggleSort("name")}
                />
              </TableHead>
              <TableHead className="w-28 px-2 text-right" aria-sort={ariaSortFor("size")}>
                <SortHeader
                  label="Size"
                  active={sortKey === "size"}
                  dir={sortDir}
                  onClick={() => toggleSort("size")}
                  align="right"
                />
              </TableHead>
              <TableHead className="w-44 px-2 text-right" aria-sort={ariaSortFor("modified")}>
                <SortHeader
                  label="Modified"
                  active={sortKey === "modified"}
                  dir={sortDir}
                  onClick={() => toggleSort("modified")}
                  align="right"
                />
              </TableHead>
              <TableHead className="w-24 pr-3 text-right">
                <span className="sr-only">Actions</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody ref={tbodyRef}>
            {sortedFolders.map((folder, i) => {
              const rowIdx = i;
              const activate = () => onOpenFolder(folder);
              return (
                <FileContextMenu
                  key={folder.id}
                  folder={folder}
                  onOpenFolder={() => onOpenFolder(folder)}
                  onMutate={onMutate}
                  readOnly={!canWrite}
                  isOwner={!folder.shared}
                >
                  <TableRow
                    data-row-index={rowIdx}
                    tabIndex={activeIdx === rowIdx || (activeIdx === -1 && rowIdx === 0) ? 0 : -1}
                    aria-label={`Folder: ${folder.name}`}
                    className={cn(
                      "group cursor-pointer transition-colors duration-150",
                      "hover:bg-muted/60",
                      "focus:outline-none focus-visible:bg-muted/60",
                      "focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring",
                      rowIdx % 2 === 1 && "bg-muted/20",
                    )}
                    onDoubleClick={activate}
                    onFocus={() => setActiveIdx(rowIdx)}
                    onKeyDown={(e) => onRowKeyDown(e, rowIdx, activate)}
                  >
                    <TableCell className="py-2.5 pl-3 pr-1">
                      <FileIcon isFolder className="h-4 w-4" />
                    </TableCell>
                    <TableCell className="py-2.5 px-3 font-medium">
                      <span className="block truncate" title={folder.name}>
                        {folder.name}
                      </span>
                    </TableCell>
                    <TableCell className="py-2.5 px-2 text-right text-muted-foreground text-xs tabular-nums">
                      —
                    </TableCell>
                    <TableCell className="py-2.5 px-2 text-right text-muted-foreground text-xs tabular-nums">
                      {folder.updated_at ? formatRelative(folder.updated_at) : "—"}
                    </TableCell>
                    <TableCell className="py-2.5 pr-3 pl-1 text-right">
                      <div className="inline-flex items-center justify-end opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity duration-150">
                        <button
                          type="button"
                          className={cn(
                            "h-7 w-7 inline-flex items-center justify-center rounded-md",
                            "text-muted-foreground hover:text-foreground hover:bg-muted",
                            "transition-colors duration-150",
                            "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background",
                          )}
                          onClick={(e) => {
                            // Stop the row's dblclick/select gestures from firing
                            // when the menu trigger is clicked.
                            e.stopPropagation();
                          }}
                          aria-label="More actions"
                          // Programmatically dispatch a right-click on the row so
                          // the wrapping <FileContextMenu> opens at the row — gives
                          // a unified menu without us re-listing every action here.
                          onPointerUp={(e) => {
                            const row = (e.currentTarget as HTMLElement).closest("tr");
                            if (!row) return;
                            const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
                            row.dispatchEvent(
                              new MouseEvent("contextmenu", {
                                bubbles: true,
                                cancelable: true,
                                clientX: rect.left,
                                clientY: rect.bottom,
                              }),
                            );
                          }}
                        >
                          <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
                        </button>
                      </div>
                    </TableCell>
                  </TableRow>
                </FileContextMenu>
              );
            })}

            {sortedFiles.map((file, i) => {
              const rowIdx = sortedFolders.length + i;
              const activate = () => onPreview(file);
              return (
                <FileContextMenu
                  key={file.id}
                  file={file}
                  onPreview={() => onPreview(file)}
                  onMutate={onMutate}
                  readOnly={!canWrite}
                  isOwner={!file.shared}
                >
                  <TableRow
                    data-row-index={rowIdx}
                    tabIndex={activeIdx === rowIdx || (activeIdx === -1 && rowIdx === 0) ? 0 : -1}
                    aria-label={`File: ${file.name}`}
                    className={cn(
                      "group cursor-pointer transition-colors duration-150",
                      "hover:bg-muted/60",
                      "focus:outline-none focus-visible:bg-muted/60",
                      "focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring",
                      rowIdx % 2 === 1 && "bg-muted/20",
                    )}
                    onDoubleClick={activate}
                    onFocus={() => setActiveIdx(rowIdx)}
                    onKeyDown={(e) => onRowKeyDown(e, rowIdx, activate)}
                  >
                    <TableCell className="py-2.5 pl-3 pr-1">
                      <FileIcon mime={file.mime_type} className="h-4 w-4" />
                    </TableCell>
                    <TableCell className="py-2.5 px-3">
                      <div className="flex items-center gap-1.5 min-w-0">
                        <span className="font-medium truncate" title={file.name}>
                          {file.name}
                        </span>
                        {file.is_starred && (
                          <Star
                            className="h-3 w-3 shrink-0 fill-yellow-400 text-yellow-400"
                            aria-label="Starred"
                          />
                        )}
                      </div>
                    </TableCell>
                    <TableCell className="py-2.5 px-2 text-right text-muted-foreground text-xs tabular-nums">
                      {formatBytes(file.size_bytes)}
                    </TableCell>
                    <TableCell className="py-2.5 px-2 text-right text-muted-foreground text-xs tabular-nums">
                      {formatRelative(file.updated_at)}
                    </TableCell>
                    <TableCell className="py-2.5 pr-3 pl-1 text-right">
                      <div className="inline-flex items-center justify-end gap-0.5 opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity duration-150">
                        <button
                          type="button"
                          className={cn(
                            "h-7 w-7 inline-flex items-center justify-center rounded-md",
                            "text-muted-foreground hover:text-foreground hover:bg-muted",
                            "transition-colors duration-150",
                            "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background",
                          )}
                          aria-label="Preview"
                          onClick={(e) => {
                            e.stopPropagation();
                            onPreview(file);
                          }}
                        >
                          <Eye className="h-4 w-4" aria-hidden="true" />
                        </button>
                        <button
                          type="button"
                          className={cn(
                            "h-7 w-7 inline-flex items-center justify-center rounded-md",
                            "text-muted-foreground hover:text-foreground hover:bg-muted",
                            "transition-colors duration-150",
                            "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background",
                          )}
                          aria-label="More actions"
                          onClick={(e) => e.stopPropagation()}
                          onPointerUp={(e) => {
                            const row = (e.currentTarget as HTMLElement).closest("tr");
                            if (!row) return;
                            const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
                            row.dispatchEvent(
                              new MouseEvent("contextmenu", {
                                bubbles: true,
                                cancelable: true,
                                clientX: rect.left,
                                clientY: rect.bottom,
                              }),
                            );
                          }}
                        >
                          <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
                        </button>
                      </div>
                    </TableCell>
                  </TableRow>
                </FileContextMenu>
              );
            })}
          </TableBody>
        </Table>
      </div>

      {/* ── Mobile: card list view ─────────────────────────────────────── */}
      <div className="md:hidden flex flex-col gap-2">
        {sortedFolders.map((folder) => (
          <FileContextMenu
            key={folder.id}
            folder={folder}
            onOpenFolder={() => onOpenFolder(folder)}
            onMutate={onMutate}
            readOnly={!canWrite}
            isOwner={!folder.shared}
          >
            <button
              type="button"
              className={cn(
                "w-full text-left border rounded-lg p-3 flex items-center gap-3",
                "transition-colors duration-150",
                "hover:bg-muted/40 active:bg-muted/60",
                "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background",
              )}
              onClick={() => onOpenFolder(folder)}
              aria-label={`Open folder ${folder.name}`}
            >
              <FileIcon isFolder className="h-5 w-5 shrink-0" />
              <div className="min-w-0 flex-1">
                <div className="font-medium truncate" title={folder.name}>
                  {folder.name}
                </div>
                <div className="text-xs text-muted-foreground truncate tabular-nums">
                  Folder{folder.updated_at ? ` · ${formatRelative(folder.updated_at)}` : ""}
                </div>
              </div>
            </button>
          </FileContextMenu>
        ))}
        {sortedFiles.map((file) => (
          <FileContextMenu
            key={file.id}
            file={file}
            onPreview={() => onPreview(file)}
            onMutate={onMutate}
            readOnly={!canWrite}
            isOwner={!file.shared}
          >
            <button
              type="button"
              className={cn(
                "w-full text-left border rounded-lg p-3 flex items-center gap-3",
                "transition-colors duration-150",
                "hover:bg-muted/40 active:bg-muted/60",
                "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background",
              )}
              onClick={() => onPreview(file)}
              aria-label={`Preview ${file.name}`}
            >
              <FileIcon mime={file.mime_type} className="h-5 w-5 shrink-0" />
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-1.5">
                  <span className="font-medium truncate" title={file.name}>
                    {file.name}
                  </span>
                  {file.is_starred && (
                    <Star
                      className="h-3 w-3 shrink-0 fill-yellow-400 text-yellow-400"
                      aria-label="Starred"
                    />
                  )}
                </div>
                <div className="text-xs text-muted-foreground truncate tabular-nums">
                  {formatBytes(file.size_bytes)} · {formatRelative(file.updated_at)}
                </div>
              </div>
            </button>
          </FileContextMenu>
        ))}
      </div>
    </>
  );
}

/**
 * Loading-state skeleton rows matching the table layout above. Parent file
 * explorers can render this while data is fetching to avoid a layout shift
 * when rows pop in. Kept here (and exported alongside the main view) so the
 * skeleton stays in sync with the column widths.
 */
export function FileListViewSkeleton({ rows = 8 }: { rows?: number }) {
  // Pre-compute slightly varied widths so the skeleton doesn't look like a
  // grid of identical bars. The pattern is deterministic so re-renders are
  // stable and React can keep the same `key` per row without flicker.
  const nameWidths = ["w-1/2", "w-3/5", "w-2/3", "w-3/4", "w-2/5", "w-1/2", "w-3/5", "w-2/3"];
  return (
    <div role="status" aria-busy="true" aria-label="Loading files">
      <span className="sr-only">Loading files…</span>
      <div className="hidden md:block border rounded-lg overflow-hidden bg-background">
        <Table className="table-fixed">
          <TableHeader>
            <TableRow className="bg-muted/40 hover:bg-muted/40 border-b">
              <TableHead className="w-10 pl-3" />
              <TableHead className="px-3">Name</TableHead>
              <TableHead className="w-28 px-2 text-right">Size</TableHead>
              <TableHead className="w-44 px-2 text-right">Modified</TableHead>
              <TableHead className="w-24 pr-3" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {Array.from({ length: rows }).map((_, i) => (
              <TableRow
                key={i}
                className={cn("border-b", i % 2 === 1 && "bg-muted/20")}
                aria-hidden="true"
              >
                <TableCell className="py-2 pl-3 pr-1">
                  <div className="h-4 w-4 rounded bg-muted animate-pulse" />
                </TableCell>
                <TableCell className="py-2 px-2">
                  <div className={cn("h-4 rounded bg-muted animate-pulse", nameWidths[i % nameWidths.length])} />
                </TableCell>
                <TableCell className="py-2 px-2 text-right">
                  <div className="h-3.5 w-12 rounded bg-muted animate-pulse ml-auto" />
                </TableCell>
                <TableCell className="py-2 px-2 text-right">
                  <div className="h-3.5 w-20 rounded bg-muted animate-pulse ml-auto" />
                </TableCell>
                <TableCell className="py-2 pr-3 pl-1" />
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      <div className="md:hidden flex flex-col gap-2">
        {Array.from({ length: rows }).map((_, i) => (
          <div
            key={i}
            className="border rounded-lg p-3 flex items-center gap-3"
            aria-hidden="true"
          >
            <div className="h-5 w-5 rounded bg-muted animate-pulse shrink-0" />
            <div className="flex-1 space-y-2">
              <div className={cn("h-3.5 rounded bg-muted animate-pulse", nameWidths[i % nameWidths.length])} />
              <div className="h-3 w-1/3 rounded bg-muted animate-pulse" />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
