"use client";

import { useState, useEffect } from "react";
import useSWR from "swr";
import { toast } from "sonner";
import { Folder, ChevronRight, Home } from "lucide-react";

import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { files as filesApi, folders as foldersApi } from "@/lib/api";
import type { FileItem, FolderItem } from "@/lib/types";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The file to move (mutually exclusive with folder) */
  file?: FileItem;
  /** The folder to move (mutually exclusive with file) */
  folder?: FolderItem;
  onMoved: () => void;
}

export function MoveDialog({ open, onOpenChange, file, folder, onMoved }: Props) {
  const [pickerPath, setPickerPath]       = useState<FolderItem[]>([]);
  const [pickerFolderId, setPickerFolderId] = useState<string | undefined>(undefined);
  const [moving, setMoving]               = useState(false);

  // Reset picker when dialog opens
  useEffect(() => {
    if (open) {
      setPickerPath([]);
      setPickerFolderId(undefined);
    }
  }, [open]);

  const { data, isLoading } = useSWR(
    open ? `move-picker-${pickerFolderId ?? "root"}` : null,
    () => foldersApi.list(pickerFolderId),
  );

  // Filter out the folder being moved (can't move into itself)
  const available = (data?.folders ?? []).filter((f) => f.id !== folder?.id);

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
      if (file) {
        await filesApi.update(file.id, { folder_id: targetId });
        toast.success(`"${file.name}" moved`);
      } else if (folder) {
        await foldersApi.update(folder.id, { parent_id: targetId });
        toast.success(`"${folder.name}" moved`);
      }
      onMoved();
      onOpenChange(false);
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Move failed");
    } finally {
      setMoving(false);
    }
  };

  const itemName  = file?.name ?? folder?.name ?? "";
  const currentParentId = file?.folder_id ?? folder?.parent_id;
  const isSameLocation  =
    (pickerFolderId ?? undefined) === (currentParentId ?? undefined);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="truncate">Move &ldquo;{itemName}&rdquo;</DialogTitle>
        </DialogHeader>

        {/* Breadcrumb inside picker */}
        <div className="flex items-center gap-1 flex-wrap text-sm min-h-[1.5rem]">
          <button
            className="text-muted-foreground hover:text-foreground flex items-center gap-1"
            onClick={() => navigateTo(-1)}
          >
            <Home className="h-3.5 w-3.5" />
            My Files
          </button>
          {pickerPath.map((f, i) => (
            <span key={f.id} className="flex items-center gap-1">
              <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
              <button
                className="hover:text-foreground max-w-[140px] truncate"
                title={f.name}
                onClick={() => navigateTo(i)}
              >
                {f.name}
              </button>
            </span>
          ))}
        </div>

        {/* Folder list */}
        <div className="border rounded-lg overflow-hidden max-h-60 overflow-y-auto">
          {isLoading ? (
            <p className="p-4 text-sm text-center text-muted-foreground">Loading…</p>
          ) : available.length === 0 ? (
            <p className="p-4 text-sm text-center text-muted-foreground">No subfolders here</p>
          ) : (
            <div className="divide-y">
              {available.map((f) => (
                <button
                  key={f.id}
                  className="w-full flex items-center gap-2 px-3 py-2.5 hover:bg-accent text-left"
                  onClick={() => navigateInto(f)}
                >
                  <Folder className="h-4 w-4 shrink-0 text-muted-foreground" />
                  <span className="text-sm flex-1 truncate" title={f.name}>{f.name}</span>
                  <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
                </button>
              ))}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={handleMove} disabled={moving || isSameLocation}>
            {moving
              ? "Moving…"
              : isSameLocation
              ? "Already here"
              : "Move here"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
