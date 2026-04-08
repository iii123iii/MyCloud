"use client";

import { useState, useCallback, useDeferredValue, useEffect, useMemo, useRef } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import useSWR from "swr";
import useSWRInfinite from "swr/infinite";
import { useDropzone } from "react-dropzone";
import { toast } from "sonner";
import { LayoutGrid, List, Upload, FolderPlus, Search, Loader2 } from "lucide-react";

import { files as filesApi, folders as foldersApi } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";
import { Toggle } from "@/components/ui/toggle";
import { FileGrid } from "./FileGrid";
import { FileListView } from "./FileListView";
import { BreadcrumbNav } from "./BreadcrumbNav";
import { NewFolderDialog } from "@/components/modals/NewFolderDialog";
import { PreviewModal } from "@/components/modals/PreviewModal";
import type { FileItem, FolderItem } from "@/lib/types";

const PAGE_SIZE = 60;

export function FileExplorer() {
  const router       = useRouter();
  const searchParams = useSearchParams();

  const [folderId, setFolderId]       = useState<string | undefined>(undefined);
  const [folderPath, setFolderPath]   = useState<FolderItem[]>([]);
  const [viewMode, setViewMode]       = useState<"grid" | "list">("grid");
  const [searchQ, setSearchQ]         = useState("");
  const [previewFile, setPreviewFile] = useState<FileItem | null>(null);
  const [newFolderOpen, setNewFolderOpen] = useState(false);
  const [isLoadMoreVisible, setIsLoadMoreVisible] = useState(false);

  // ── Restore state from URL on first mount ─────────────────────────────
  useEffect(() => {
    const urlFolderId = searchParams.get("folder");
    if (!urlFolderId) return;
    (async () => {
      let path: FolderItem[] = [];
      try {
        const data = await foldersApi.path(urlFolderId);
        path = data.folders ?? [];
      } catch {
        return;
      }
      if (path.length > 0) {
        setFolderPath(path);
        setFolderId(path[path.length - 1].id);
      }
    })();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const updateUrl = (newFolderId: string | undefined) => {
    const params = new URLSearchParams(searchParams.toString());
    if (newFolderId) params.set("folder", newFolderId);
    else             params.delete("folder");
    router.replace(`/dashboard?${params.toString()}`);
  };

  // ── Infinite scroll: paginated file loading ───────────────────────────
  const {
    data: filePages,
    setSize: setFilePageCount,
    mutate: mutateFiles,
    isValidating: filesLoading,
  } = useSWRInfinite(
    (pageIndex, previousPageData) => {
      if (previousPageData && !previousPageData.has_more) return null;
      return ["files", folderId ?? "root", pageIndex + 1, PAGE_SIZE] as const;
    },
    ([, currentFolderId, page]) => {
      return filesApi.list({
        folder_id: currentFolderId === "root" ? undefined : currentFolderId,
        page,
        page_size: PAGE_SIZE,
      });
    },
    {
      revalidateFirstPage: true,
      persistSize: false,
      parallel: false,
    }
  );

  const { data: foldersData, mutate: mutateFolders } = useSWR(
    `folders-${folderId ?? "root"}`,
    () => foldersApi.list(folderId),
    {
      refreshInterval: 30_000,
      refreshWhenHidden: false,
      refreshWhenOffline: false,
      revalidateOnFocus: true,
    }
  );

  const allFiles = filePages
    ? Array.from(
        new Map(filePages.flatMap((p) => p.files).map((file) => [file.id, file])).values(),
      )
    : [];
  const hasMore = filePages
    ? (filePages[filePages.length - 1]?.has_more ?? false) || allFiles.length < (filePages[0]?.total ?? 0)
    : false;
  const totalFiles  = filePages?.[0]?.total ?? 0;
  const folderList  = foldersData?.folders ?? [];

  const mutateAll = () => { mutateFiles(); mutateFolders(); };

  // ── Infinite scroll trigger ───────────────────────────────────────────
  const loadMoreRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!loadMoreRef.current) return;
    const observer = new IntersectionObserver(
      ([entry]) => {
        setIsLoadMoreVisible(entry.isIntersecting);
      },
      { rootMargin: "200px" }
    );
    observer.observe(loadMoreRef.current);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    if (!isLoadMoreVisible || !hasMore || filesLoading) return;
    setFilePageCount((size) => size + 1);
  }, [filesLoading, hasMore, isLoadMoreVisible, setFilePageCount]);

  // Reset pagination when folder changes
  useEffect(() => {
    setIsLoadMoreVisible(false);
    setFilePageCount(1);
  }, [folderId, setFilePageCount]);

  // ── Drag-and-drop upload ──────────────────────────────────────────────
  const onDrop = useCallback(async (accepted: File[]) => {
    for (const file of accepted) {
      const fd = new FormData();
      fd.append("file", file);
      if (folderId) fd.append("folder_id", folderId);
      const tid = toast.loading(`Uploading ${file.name}…`);
      try {
        await filesApi.upload(fd, (pct) =>
          toast.loading(`Uploading ${file.name} — ${pct}%`, { id: tid })
        );
        toast.success(`${file.name} uploaded`, { id: tid });
        mutateFiles();
      } catch (err: unknown) {
        toast.error(err instanceof Error ? err.message : "Upload failed", { id: tid });
      }
    }
  }, [folderId, mutateFiles]);

  const { getRootProps, getInputProps, isDragActive, open: openFileDialog } = useDropzone({
    onDrop, noClick: true, noKeyboard: true,
    // Disable while the preview modal is open so dragging an image inside it
    // doesn't accidentally trigger an upload.
    disabled: !!previewFile,
  });

  // ── Folder navigation ─────────────────────────────────────────────────
  const openFolder = (folder: FolderItem) => {
    setFolderPath((p) => [...p, folder]);
    setFolderId(folder.id);
    updateUrl(folder.id);
    setSearchQ("");
  };

  const navigateTo = (index: number) => {
    if (index < 0) {
      setFolderPath([]);
      setFolderId(undefined);
      updateUrl(undefined);
    } else {
      const newPath = folderPath.slice(0, index + 1);
      setFolderPath(newPath);
      const newId = newPath[newPath.length - 1].id;
      setFolderId(newId);
      updateUrl(newId);
    }
    setSearchQ("");
  };

  // ── Filtered lists ────────────────────────────────────────────────────
  const deferredSearchQ = useDeferredValue(searchQ);
  const lowerQ = deferredSearchQ.trim().toLowerCase();
  const filtered = useMemo(
    () => (lowerQ ? allFiles.filter((f) => f.name.toLowerCase().includes(lowerQ)) : allFiles),
    [allFiles, lowerQ]
  );
  const filteredFolders = useMemo(
    () => (lowerQ ? folderList.filter((f) => f.name.toLowerCase().includes(lowerQ)) : folderList),
    [folderList, lowerQ]
  );

  return (
    <div {...getRootProps()} className="relative min-h-[400px]">
      <input {...getInputProps()} />

      {isDragActive && (
        <div className="absolute inset-0 z-50 flex items-center justify-center rounded-xl border-2 border-dashed border-primary bg-primary/5">
          <div className="text-center">
            <Upload className="h-10 w-10 mx-auto mb-2 text-primary" />
            <p className="text-sm font-medium text-primary">Drop files to upload</p>
          </div>
        </div>
      )}

      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-2 mb-4">
        <BreadcrumbNav path={folderPath} onNavigate={navigateTo} />

        {totalFiles > 0 && (
          <span className="text-xs text-muted-foreground ml-1 tabular-nums">
            {totalFiles} file{totalFiles !== 1 ? "s" : ""}
          </span>
        )}

        <div className="ml-auto flex items-center gap-2">
          <div className="relative">
            <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground pointer-events-none" />
            <Input
              className="pl-8 w-48"
              placeholder="Filter…"
              value={searchQ}
              onChange={(e) => setSearchQ(e.target.value)}
            />
          </div>
          <Separator orientation="vertical" className="h-6" />
          <Toggle
            pressed={viewMode === "grid"}
            onPressedChange={(p) => setViewMode(p ? "grid" : "list")}
            aria-label="Grid view"
            size="sm"
          >
            <LayoutGrid className="h-4 w-4" />
          </Toggle>
          <Toggle
            pressed={viewMode === "list"}
            onPressedChange={(p) => setViewMode(p ? "list" : "grid")}
            aria-label="List view"
            size="sm"
          >
            <List className="h-4 w-4" />
          </Toggle>
          <Separator orientation="vertical" className="h-6" />
          <Button variant="outline" size="sm" onClick={() => setNewFolderOpen(true)}>
            <FolderPlus className="h-4 w-4 mr-1.5" />
            New folder
          </Button>
          <Button size="sm" onClick={openFileDialog}>
            <Upload className="h-4 w-4 mr-1.5" />
            Upload
          </Button>
        </div>
      </div>

      {/* Content */}
      {viewMode === "grid" ? (
        <FileGrid
          files={filtered}
          folders={filteredFolders}
          onOpenFolder={openFolder}
          onPreview={setPreviewFile}
          onMutate={mutateAll}
        />
      ) : (
        <FileListView
          files={filtered}
          folders={filteredFolders}
          onOpenFolder={openFolder}
          onPreview={setPreviewFile}
          onMutate={mutateAll}
        />
      )}

      {/* Infinite scroll trigger */}
      {hasMore && (
        <div ref={loadMoreRef} className="flex justify-center py-6">
          {filesLoading ? (
            <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
          ) : (
            <Button variant="ghost" size="sm" onClick={() => setFilePageCount((s) => s + 1)}>
              Load more
            </Button>
          )}
        </div>
      )}
      {filesLoading && !hasMore && allFiles.length === 0 && (
        <div className="flex justify-center py-12">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      )}

      {/* Dialogs */}
      <NewFolderDialog
        open={newFolderOpen}
        onOpenChange={setNewFolderOpen}
        parentId={folderId}
        onCreated={mutateFolders}
      />

      {previewFile && (
        <PreviewModal
          file={previewFile}
          files={filtered}
          open={!!previewFile}
          onOpenChange={(o) => !o && setPreviewFile(null)}
          onNavigate={setPreviewFile}
        />
      )}
    </div>
  );
}
