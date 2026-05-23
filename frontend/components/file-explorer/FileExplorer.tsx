"use client";

import { useState, useCallback, useDeferredValue, useEffect, useMemo, useRef } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import useSWR, { useSWRConfig } from "swr";
import useSWRInfinite from "swr/infinite";
import { useDropzone } from "react-dropzone";
import { LayoutGrid, List, Upload, FolderPlus, Search, Loader2, AlertCircle, RefreshCw } from "lucide-react";

import { auth as authApi, files as filesApi, folders as foldersApi } from "@/lib/api";
import { useTopic } from "@/components/ws-provider";
import { collectDroppedFiles, planDroppedTree, type DroppedFile } from "@/lib/folder-drop";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { Toggle } from "@/components/ui/toggle";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { FileGrid } from "./FileGrid";
import { FileListView } from "./FileListView";
import { BreadcrumbNav } from "./BreadcrumbNav";
import { SelectionToolbar, useSelection } from "./SelectionToolbar";
import { useUploadQueue } from "@/components/upload/UploadQueueProvider";
import { NewFolderDialog } from "@/components/modals/NewFolderDialog";
import { PreviewModal } from "@/components/modals/PreviewModal";
import { EditFileDialog } from "@/components/modals/EditFileDialog";
import { getEditMode } from "@/lib/file-kind";
import type { FileItem, FolderItem } from "@/lib/types";

const PAGE_SIZE = 60;

interface FileExplorerProps {
  // Base route the breadcrumb/URL sync writes to. Defaults to the owner drive;
  // the shared browser passes "/shared/browse" so navigation stays in-context.
  basePath?: string;
  // When set, the breadcrumb home crumb renders this label and links here
  // instead of the owner root (e.g. "Shared with me" → /shared).
  rootCrumb?: { label: string; href: string };
  // True when this explorer is rooted at a folder shared with the caller.
  // Write affordances are then gated on the folder's effective grant level.
  sharedRoot?: boolean;
}

export function FileExplorer({ basePath = "/dashboard", rootCrumb, sharedRoot = false }: FileExplorerProps = {}) {
  const router       = useRouter();
  const searchParams = useSearchParams();

  const [folderId, setFolderId]       = useState<string | undefined>(undefined);
  const [folderPath, setFolderPath]   = useState<FolderItem[]>([]);
  const [viewMode, setViewMode]       = useState<"grid" | "list">("grid");
  const [searchQ, setSearchQ]         = useState("");
  const [previewFile, setPreviewFile] = useState<FileItem | null>(null);
  const [editFile, setEditFile]       = useState<FileItem | null>(null);
  const [newFolderOpen, setNewFolderOpen] = useState(false);
  const [isLoadMoreVisible, setIsLoadMoreVisible] = useState(false);
  // UI polish: sticky toolbar gets a drop-shadow once the page has scrolled,
  // search input ref so "/" and Ctrl/Cmd+F focus it, and a derived selection
  // count read off the floating SelectionToolbar (rendered by
  // ContentWithSelection — left untouched per scope).
  const [scrolled, setScrolled] = useState(false);
  const [selectionCount, setSelectionCount] = useState(0);
  const searchInputRef = useRef<HTMLInputElement>(null);
  // Tracks the folderId we last reconciled with the URL. Local navigation
  // handlers stamp this BEFORE calling router.replace so that, when the URL
  // change re-fires the reconcile effect, the effect sees state already
  // agrees and short-circuits instead of tearing the view down and rebuilding.
  const syncedFolderIdRef = useRef<string | undefined>(undefined);

  // Sticky-toolbar shadow on scroll.
  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 4);
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  // Global "/" and Ctrl/Cmd+F focus the search input. Skips when typing
  // elsewhere so we don't hijack other inputs.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement | null;
      const typing =
        t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.isContentEditable);
      if (e.key === "/" && !typing && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault();
        searchInputRef.current?.focus();
        searchInputRef.current?.select();
      } else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "f") {
        // Only override if we're not already typing in another input.
        if (typing && t !== searchInputRef.current) return;
        e.preventDefault();
        searchInputRef.current?.focus();
        searchInputRef.current?.select();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // Derive selection count by watching the floating SelectionToolbar — the
  // selection state itself lives inside <ContentWithSelection/> and we keep
  // that wrapper untouched on purpose. The toolbar renders
  // "{N} selected" in a fixed bottom bar; we observe it instead of
  // re-plumbing the hook.
  useEffect(() => {
    const parse = () => {
      const el = document.querySelector<HTMLElement>(
        "[data-no-hotkeys='true']",
      );
      if (!el) {
        setSelectionCount(0);
        return;
      }
      const m = el.textContent?.match(/(\d+)\s+selected/);
      setSelectionCount(m ? parseInt(m[1], 10) : 0);
    };
    parse();
    const observer = new MutationObserver(parse);
    observer.observe(document.body, {
      childList: true,
      subtree: true,
      characterData: true,
    });
    return () => observer.disconnect();
  }, []);

  // ── Reconcile state with URL ─────────────────────────────────────────
  // Reacts to URL changes only (not folderId), and uses syncedFolderIdRef
  // to distinguish "URL caught up to a local navigation we already made"
  // from "external URL change we need to honour". Without the ref, between
  // a local setFolderId() and router.replace() landing, this effect would
  // briefly see urlFolderId !== folderId and could reset state — visible
  // to users as a flicker back to root on every folder click.
  useEffect(() => {
    const urlFolderId = searchParams.get("folder") || undefined;
    if (urlFolderId === syncedFolderIdRef.current) return;
    if (!urlFolderId) {
      syncedFolderIdRef.current = undefined;
      setFolderId(undefined);
      setFolderPath([]);
      return;
    }
    syncedFolderIdRef.current = urlFolderId;
    (async () => {
      try {
        const data = await foldersApi.path(urlFolderId);
        const path = data.folders ?? [];
        setFolderPath(path);
        setFolderId(urlFolderId);
      } catch {
        // Path fetch failed. In a shared browse this usually means the grant
        // was revoked or the folder removed — bounce back to the shared list
        // rather than silently showing the wrong tree. On the owner drive we
        // just stay put (transient error, SWR will surface the broader issue).
        if (sharedRoot && rootCrumb) router.replace(rootCrumb.href);
      }
    })();
  }, [searchParams]);

  // ── ?open=<fileId>: auto-open preview after navigating from a search hit ──
  // The command palette routes file-result clicks here. We wait until the
  // listed pages include the file (the URL `folder` param feeds the loader
  // above), then open PreviewModal and strip `open` so refresh doesn't loop.
  const autoOpenedRef = useRef<string | null>(null);

  const updateUrl = (newFolderId: string | undefined) => {
    const params = new URLSearchParams(searchParams.toString());
    if (newFolderId) params.set("folder", newFolderId);
    else             params.delete("folder");
    router.replace(`${basePath}?${params.toString()}`);
  };

  // ── Infinite scroll: paginated file loading ───────────────────────────
  const {
    data: filePages,
    setSize: setFilePageCount,
    mutate: mutateFiles,
    isValidating: filesLoading,
    error: filesError,
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

  // Effective permission for the folder currently in view (the deepest crumb
  // carries it for shared paths). Outside a shared context writes are always
  // allowed; inside one, only editor/owner grants may mutate. Until the path
  // resolves we default to read-only so write affordances don't flash for a
  // viewer.
  const currentFolder = folderPath[folderPath.length - 1];
  const folderPerm = currentFolder?.permission;
  const canWrite = !sharedRoot || folderPerm === "editor" || folderPerm === "owner";

  // Global SWR mutate — used for invalidating keys owned by other components
  // (favourites in the sidebar, the *destination* folder's listing during
  // a move). The local mutateFiles/mutateFolders only cover the folder we
  // happen to be viewing.
  const { mutate: globalMutate } = useSWRConfig();

  // refreshAfterMutation: after a successful create / move / rename / delete,
  // invalidate every cache that could now be stale. `targetFolderId` is
  // passed by MoveDialog (via FileContextMenu.onMutate and
  // SelectionToolbar.onSuccess) so we can refresh the *destination* folder
  // — without this, moved items appear stuck in their old folder until the
  // 30-second SWR poll catches up.
  const refreshAfterMutation = useCallback(
    async (targetFolderId?: string | null) => {
      // Current folder's files + folder list.
      mutateFiles();
      mutateFolders();
      // Re-fetch the breadcrumb path. If the user moved an ancestor of the
      // current folder, the crumbs are now wrong and need refreshing.
      if (folderId) {
        try {
          const data = await foldersApi.path(folderId);
          setFolderPath(data.folders ?? []);
        } catch {
          // Path fetch failed (folder may have been deleted by the same op);
          // SWR errors on the file listing will surface the broader problem.
        }
      }
      // Destination folder's listings — only set for move ops. `null`
      // signals "moved to root", which becomes the "root" cache key.
      if (targetFolderId !== undefined) {
        const destKey = targetFolderId ?? "root";
        globalMutate(`folders-${destKey}`);
        globalMutate(
          (key) =>
            Array.isArray(key) && key[0] === "files" && key[1] === destKey,
          undefined,
          { revalidate: true },
        );
      }
      // Favourites caches folder names; always invalidate so a rename /
      // move / delete of a favourited folder shows up immediately in the
      // sidebar instead of waiting for the next refocus.
      globalMutate("favorites");
    },
    [folderId, mutateFiles, mutateFolders, globalMutate],
  );

  // ── Live invalidation via WebSocket folder events ─────────────────────
  // Backend publishActivity broadcasts on `user:<id>` and `folder:<id>`
  // whenever a folder is created / updated / moved / deleted (or a file
  // inside one is touched). We use those events to keep SWR caches fresh
  // for changes that happened in another tab or from another device.
  const { data: me } = useSWR("me", authApi.me, { shouldRetryOnError: false });
  const userId = me?.id;

  const handleUserEvent = useCallback(
    (env: { type?: string; data?: unknown }) => {
      if (env.type !== "event") return;
      const data = env.data as
        | {
            action?: string;
            resource_type?: string;
            resource_id?: string;
            details_raw?: string;
          }
        | undefined;
      if (!data || data.resource_type !== "folder") return;
      // details_raw is a JSON-encoded string published by the backend
      // (see app.go publishActivity). Parse defensively — older rows or
      // rename payloads may not carry parent ids.
      let parsed: Record<string, unknown> = {};
      if (typeof data.details_raw === "string" && data.details_raw) {
        try {
          parsed = (JSON.parse(data.details_raw) as Record<string, unknown>) ?? {};
        } catch {
          /* malformed — fall through with no parent hints */
        }
      }
      const oldParent =
        typeof parsed["old_parent_id"] === "string"
          ? (parsed["old_parent_id"] as string)
          : undefined;
      const newParent =
        typeof parsed["new_parent_id"] === "string"
          ? (parsed["new_parent_id"] as string)
          : undefined;
      // Folder lifecycle events — invalidate both source and destination
      // folder listings + the sidebar favourites cache.
      globalMutate(`folders-${oldParent ?? "root"}`);
      globalMutate(`folders-${newParent ?? "root"}`);
      globalMutate("favorites");
      // If the moved/renamed folder is one we have in the breadcrumb path,
      // refresh the path so the crumb label updates.
      if (
        folderId &&
        (data.resource_id === folderId ||
          folderPath.some((f) => f.id === data.resource_id))
      ) {
        foldersApi
          .path(folderId)
          .then((d) => setFolderPath(d.folders ?? []))
          .catch(() => {
            /* ignore */
          });
      }
    },
    [globalMutate, folderId, folderPath],
  );
  useTopic(userId ? `user:${userId}` : null, handleUserEvent);

  // Per-folder topic — refreshes the current folder's files + subfolder
  // listing whenever anything inside it changes (new file uploaded by
  // another device, comment added, etc.).
  const handleFolderEvent = useCallback(
    (env: { type?: string }) => {
      if (env.type !== "event") return;
      mutateFiles();
      mutateFolders();
    },
    [mutateFiles, mutateFolders],
  );
  useTopic(folderId ? `folder:${folderId}` : null, handleFolderEvent);

  // Auto-open the preview for a file pointed at by ?open=<id> once it lands
  // in the listing. Strips the param afterwards.
  useEffect(() => {
    const wanted = searchParams.get("open");
    if (!wanted) return;
    if (autoOpenedRef.current === wanted) return;
    const hit = allFiles.find((f) => f.id === wanted);
    if (!hit) return;
    autoOpenedRef.current = wanted;
    setPreviewFile(hit);
    const params = new URLSearchParams(searchParams.toString());
    params.delete("open");
    const qs = params.toString();
    router.replace(qs ? `${basePath}?${qs}` : basePath);
  }, [allFiles, searchParams, router, basePath]);

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

  // Upload queue: every upload mirrors into the persistent panel so
  // users see all in-flight + recent uploads in one place. Toasts still
  // fire for immediate feedback; the panel survives page navigation via
  // sessionStorage.
  const queue = useUploadQueue();

  // ── Drag-and-drop upload (files + folders) ────────────────────────────
  // All progress / success / failure feedback lives in the floating upload
  // queue panel (components/upload/UploadQueueProvider.tsx). We don't fire
  // toasts here because the panel already shows per-file status, the
  // aggregate progress bar, and an explicit error row with a retry button —
  // duplicating that as toasts was noisy.
  const uploadOne = useCallback(async (file: File, targetFolder: string | undefined, label?: string) => {
    const fd = new FormData();
    fd.append("file", file);
    if (targetFolder) fd.append("folder_id", targetFolder);
    const display = label ?? file.name;
    const qid = `upload-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    queue.enqueue({ id: qid, name: display, size: file.size });
    queue.update(qid, { status: "uploading" });
    try {
      await filesApi.upload(fd, (pct) => {
        queue.update(qid, { progress: pct / 100 });
      });
      queue.update(qid, { status: "done", progress: 1 });
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Upload failed";
      queue.update(qid, { status: "error", error: msg });
    }
  }, [queue]);

  // Flat drop (no folders) — used when dropzone's default getFilesFromEvent runs.
  const onDrop = useCallback(async (accepted: File[]) => {
    for (const file of accepted) {
      await uploadOne(file, folderId);
    }
    mutateFiles();
  }, [folderId, mutateFiles, uploadOne]);

  // Folder-aware drop — walks the FS entry tree, creates the directory
  // hierarchy server-side, then uploads each file in parallel batches.
  // Files route through uploadOne so the queue panel shows per-file
  // progress; no toasts are fired here (the panel covers all feedback).
  const onFolderAwareDrop = useCallback(async (items: DroppedFile[]) => {
    const { directories, filesByDir } = planDroppedTree(items);
    try {
      // Create folder hierarchy, breadth-first.
      const dirIdMap = new Map<string, string>();
      for (const dir of directories) {
        const segs = dir.split("/");
        const parentDir = segs.slice(0, -1).join("/");
        const parentId = parentDir ? dirIdMap.get(parentDir) : folderId;
        try {
          const created = await foldersApi.create({ name: segs[segs.length - 1], parent_id: parentId ?? null });
          dirIdMap.set(dir, created.id);
        } catch {
          // If duplicate, look up via list — best effort. For now, skip and let
          // backend reject the upload if folder really doesn't exist.
        }
      }
      // Upload files in chunks of 4. uploadOne handles enqueue/progress/error
      // reporting via the upload queue panel.
      const CONCURRENCY = 4;
      const entries = Array.from(filesByDir.entries());
      const jobs: Array<() => Promise<void>> = [];
      for (const [dir, files] of entries) {
        const targetFolderId = dir ? dirIdMap.get(dir) : folderId;
        for (const item of files) {
          jobs.push(() => uploadOne(item.file, targetFolderId, item.relativePath));
        }
      }
      const workers = Array.from({ length: CONCURRENCY }, async () => {
        while (jobs.length) {
          const job = jobs.shift();
          if (job) await job();
        }
      });
      await Promise.all(workers);
    } finally {
      mutateFiles();
      mutateFolders();
    }
  }, [folderId, mutateFiles, mutateFolders, uploadOne]);

  const { getRootProps, getInputProps, isDragActive, open: openFileDialog } = useDropzone({
    onDrop,
    noClick: true,
    noKeyboard: true,
    disabled: !!previewFile || !canWrite,
    // Custom extractor: prefer folder-aware walker when DataTransferItem is available.
    getFilesFromEvent: async (evt): Promise<File[]> => {
      const dt = (evt as DragEvent).dataTransfer;
      if (!dt) {
        const fe = evt as unknown as { target: HTMLInputElement };
        return Array.from(fe.target?.files ?? []);
      }
      const items = await collectDroppedFiles(dt);
      // If anything has a slash in its relativePath, we have folders.
      const hasFolders = items.some((it) => it.relativePath.includes("/"));
      if (hasFolders) {
        // Sidestep dropzone's onDrop — handle the whole tree ourselves and
        // return [] so it doesn't try to upload them as flat files.
        void onFolderAwareDrop(items);
        return [];
      }
      return items.map((it) => it.file);
    },
  });

  // ── Folder navigation ─────────────────────────────────────────────────
  // Each handler stamps syncedFolderIdRef BEFORE updateUrl so the reconcile
  // effect, when it next runs from the URL change, sees a matching value
  // and skips the reset/refetch path.
  const openFolder = (folder: FolderItem) => {
    syncedFolderIdRef.current = folder.id;
    setFolderPath((p) => [...p, folder]);
    setFolderId(folder.id);
    updateUrl(folder.id);
    setSearchQ("");
  };

  const navigateTo = (index: number) => {
    if (index < 0) {
      // In a shared context the "home" crumb returns to /shared (the share
      // root) rather than the owner's drive root, which the grantee can't see.
      if (sharedRoot && rootCrumb) {
        router.push(rootCrumb.href);
        return;
      }
      syncedFolderIdRef.current = undefined;
      setFolderPath([]);
      setFolderId(undefined);
      updateUrl(undefined);
    } else {
      const newPath = folderPath.slice(0, index + 1);
      const newId = newPath[newPath.length - 1].id;
      syncedFolderIdRef.current = newId;
      setFolderPath(newPath);
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

  // Surface the dragging state as a body attribute so the global CSS in
  // globals.css can paint a full-window drop affordance — covers the rare
  // case where the dropzone's local overlay is scrolled out of view.
  useEffect(() => {
    if (typeof document === "undefined") return;
    if (isDragActive) {
      document.body.setAttribute("data-dragging", "true");
    } else {
      document.body.removeAttribute("data-dragging");
    }
    return () => {
      if (typeof document !== "undefined") document.body.removeAttribute("data-dragging");
    };
  }, [isDragActive]);

  return (
    <div {...getRootProps()} className="relative min-h-[400px]">
      <input {...getInputProps()} />

      {isDragActive && (
        <div className="absolute inset-0 z-50 flex items-center justify-center rounded-xl border-2 border-dashed border-primary bg-primary/5 backdrop-blur-sm">
          <div className="text-center animate-in zoom-in-95 fade-in duration-200">
            <Upload className="h-12 w-12 mx-auto mb-3 text-primary animate-bounce" />
            <p className="text-base font-semibold text-primary">Drop files to upload</p>
            <p className="text-xs text-primary/70 mt-1">Folders are supported too</p>
          </div>
        </div>
      )}

      {/* Toolbar — sticky so the controls stay reachable when scrolled; a
          subtle shadow appears once the page leaves the top to anchor it. */}
      <TooltipProvider delay={300}>
        <div
          role="toolbar"
          aria-label="File explorer toolbar"
          className={cn(
            "sticky top-0 z-20 -mx-2 px-2 py-2.5 mb-5",
            "bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/70",
            "transition-shadow duration-200 ease-out",
            scrolled && "shadow-sm",
          )}
        >
          <div className="flex flex-col sm:flex-row sm:flex-wrap sm:items-center gap-2.5 sm:gap-3">
            {/* Left cluster: breadcrumb + counts */}
            <div className="flex items-center gap-2 min-w-0 flex-1 sm:flex-initial">
              <BreadcrumbNav path={folderPath} onNavigate={navigateTo} rootCrumb={rootCrumb} />
              {totalFiles > 0 && (
                <span className="text-xs text-muted-foreground tabular-nums shrink-0">
                  {totalFiles.toLocaleString()} file{totalFiles !== 1 ? "s" : ""}
                </span>
              )}
              <span
                role="status"
                aria-live="polite"
                aria-atomic="true"
                className={cn(
                  "text-xs font-medium text-primary tabular-nums shrink-0",
                  "px-1.5 py-0.5 rounded-md bg-primary/10 border border-primary/20",
                  "transition-opacity duration-150",
                  selectionCount > 0 ? "opacity-100" : "sr-only",
                )}
              >
                {selectionCount > 0 ? `${selectionCount} selected` : ""}
              </span>
            </div>

            {/* Right cluster: search + view toggle + actions */}
            <div className="flex items-center gap-2 sm:ml-auto flex-wrap">
              <div className="relative flex-1 sm:flex-initial">
                <Search
                  className="absolute left-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground pointer-events-none"
                  aria-hidden="true"
                />
                <Input
                  ref={searchInputRef}
                  className="pl-8 pr-12 w-full sm:w-64 transition-shadow"
                  // Hint: surface DSL syntax so users discover field operators.
                  placeholder="Search (name:*.pdf, tag:work)"
                  value={searchQ}
                  onChange={(e) => setSearchQ(e.target.value)}
                  title="Filter the current folder. Tip: prefix with name:, mime:, tag:, size:>1mb, after:YYYY-MM-DD, owner:me, starred:true. (Press / to focus)"
                  aria-label="Search files"
                  aria-keyshortcuts="/ Control+F Meta+F"
                  type="search"
                />
                <kbd
                  className="hidden sm:inline-flex pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 h-5 select-none items-center gap-1 rounded border bg-muted px-1.5 font-mono text-[10px] font-medium text-muted-foreground"
                  aria-hidden="true"
                >
                  /
                </kbd>
              </div>

              <Separator orientation="vertical" className="h-6 hidden sm:block" />

              {/* Segmented view-mode control. Joined toggles with shared
                  border so they read as a single unit, not two siblings. */}
              <div
                className="inline-flex items-center rounded-lg border bg-background p-0.5 shrink-0 transition-colors"
                role="group"
                aria-label="View mode"
              >
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <Toggle
                        pressed={viewMode === "grid"}
                        onPressedChange={(p) => setViewMode(p ? "grid" : "list")}
                        aria-label="Grid view"
                        aria-pressed={viewMode === "grid"}
                        aria-keyshortcuts="g"
                        title="Grid view"
                        size="sm"
                        className={cn(
                          "rounded-md transition-all duration-150 ease-out",
                          "hover:bg-muted/70",
                          "focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:ring-offset-0",
                          "data-[state=on]:bg-muted data-[state=on]:shadow-sm data-[state=on]:text-foreground",
                        )}
                      >
                        <LayoutGrid className="h-4 w-4" aria-hidden="true" />
                      </Toggle>
                    }
                  />
                  <TooltipContent>Grid view</TooltipContent>
                </Tooltip>
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <Toggle
                        pressed={viewMode === "list"}
                        onPressedChange={(p) => setViewMode(p ? "list" : "grid")}
                        aria-label="List view"
                        aria-pressed={viewMode === "list"}
                        aria-keyshortcuts="l"
                        title="List view"
                        size="sm"
                        className={cn(
                          "rounded-md transition-all duration-150 ease-out",
                          "hover:bg-muted/70",
                          "focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:ring-offset-0",
                          "data-[state=on]:bg-muted data-[state=on]:shadow-sm data-[state=on]:text-foreground",
                        )}
                      >
                        <List className="h-4 w-4" aria-hidden="true" />
                      </Toggle>
                    }
                  />
                  <TooltipContent>List view</TooltipContent>
                </Tooltip>
              </div>

              {/* Write actions are hidden when browsing a shared folder
                  without editor rights (read-only viewers). */}
              {canWrite && (
                <>
                  <Separator orientation="vertical" className="h-6 hidden sm:block" />

                  {/* Action hierarchy: secondary "New folder" (outline) and
                      primary "Upload" (default). Labels collapse to icon-only
                      on the smallest screens via Tooltip. */}
                  <Tooltip>
                    <TooltipTrigger
                      render={
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => setNewFolderOpen(true)}
                          aria-label="New folder"
                          title="Create a new folder in the current directory"
                          className="transition-colors"
                        >
                          <FolderPlus className="h-4 w-4 sm:mr-1.5" aria-hidden="true" />
                          <span className="hidden sm:inline">New folder</span>
                        </Button>
                      }
                    />
                    <TooltipContent>New folder</TooltipContent>
                  </Tooltip>
                  <Tooltip>
                    <TooltipTrigger
                      render={
                        <Button
                          size="sm"
                          onClick={openFileDialog}
                          aria-label="Upload files"
                          title="Upload files (or drop them anywhere)"
                          className="transition-colors"
                        >
                          <Upload className="h-4 w-4 sm:mr-1.5" aria-hidden="true" />
                          <span className="hidden sm:inline">Upload</span>
                        </Button>
                      }
                    />
                    <TooltipContent>Upload files</TooltipContent>
                  </Tooltip>
                </>
              )}
            </div>
          </div>
        </div>
      </TooltipProvider>

      {/* Error state — surfaces SWR failure with a retry affordance. */}
      {filesError && allFiles.length === 0 && !filesLoading && (
        <Alert variant="destructive" className="mb-4">
          <AlertCircle aria-hidden="true" />
          <AlertTitle>Couldn&apos;t load files</AlertTitle>
          <AlertDescription>
            <p>
              {filesError instanceof Error
                ? filesError.message
                : "Something went wrong while loading this folder."}
            </p>
            <Button
              variant="outline"
              size="sm"
              className="mt-2 w-fit transition-colors"
              onClick={() => mutateFiles()}
              aria-label="Retry loading files"
            >
              <RefreshCw className="h-3.5 w-3.5 mr-1.5" aria-hidden="true" />
              Try again
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {/* Initial loading skeleton — shape mirrors the active view mode so
          there's minimal layout shift when real data lands. */}
      {filesLoading && !filesError && allFiles.length === 0 ? (
        <div
          role="status"
          aria-busy="true"
          aria-label="Loading files"
          className="animate-in fade-in duration-200"
        >
          <span className="sr-only">Loading files…</span>
          {viewMode === "grid" ? (
            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-3">
              {Array.from({ length: 12 }).map((_, i) => (
                <div key={i} className="space-y-2">
                  <Skeleton className="aspect-square w-full rounded-lg" />
                  <Skeleton className="h-3.5 w-4/5" />
                  <Skeleton className="h-3 w-2/5" />
                </div>
              ))}
            </div>
          ) : (
            <div className="space-y-1.5">
              {Array.from({ length: 10 }).map((_, i) => (
                <div
                  key={i}
                  className="flex items-center gap-3 px-2 py-2.5 rounded-md"
                >
                  <Skeleton className="h-8 w-8 rounded-md shrink-0" />
                  <Skeleton className="h-3.5 flex-1 max-w-[40%]" />
                  <Skeleton className="h-3 w-16 hidden sm:block" />
                  <Skeleton className="h-3 w-20 hidden md:block" />
                </div>
              ))}
            </div>
          )}
        </div>
      ) : !filesError ? (
        <div
          // Smooth crossfade when toggling between grid and list views.
          key={viewMode}
          className="animate-in fade-in duration-200"
        >
          {/* Multi-select state is derived from the currently visible
              file ids (folders are excluded from batch ops since the backend
              doesn't accept folder ids in the file_ids array). */}
          <ContentWithSelection
            viewMode={viewMode}
            files={filtered}
            folders={filteredFolders}
            onOpenFolder={openFolder}
            onPreview={setPreviewFile}
            onMutate={refreshAfterMutation}
            onUploadClick={openFileDialog}
            onNewFolderClick={() => setNewFolderOpen(true)}
            canWrite={canWrite}
          />
        </div>
      ) : null}

      {/* Infinite scroll trigger */}
      {hasMore && (
        <div ref={loadMoreRef} className="flex justify-center py-6">
          {filesLoading ? (
            <Loader2
              className="h-5 w-5 animate-spin text-muted-foreground"
              aria-label="Loading more files"
              role="status"
            />
          ) : (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setFilePageCount((s) => s + 1)}
              className="transition-colors"
            >
              Load more
            </Button>
          )}
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
          onEdit={
            canWrite && getEditMode(previewFile) !== null
              ? () => {
                  // Hand off from preview → editor without losing the file.
                  // Gated by canWrite so viewers of a shared file get a
                  // read-only preview (no Edit button).
                  setEditFile(previewFile);
                  setPreviewFile(null);
                }
              : undefined
          }
        />
      )}

      {editFile && (
        <EditFileDialog
          open={!!editFile}
          onOpenChange={(o) => !o && setEditFile(null)}
          file={editFile}
          onSaved={() => refreshAfterMutation()}
        />
      )}
    </div>
  );
}

// Small wrapper that hosts the selection state next to the grid/list
// renderer. Lives here so the parent FileExplorer keeps its lean state
// surface while the file-grid views get selection wiring for free.
function ContentWithSelection({
  viewMode, files, folders, onOpenFolder, onPreview, onMutate,
  onUploadClick, onNewFolderClick, canWrite,
}: {
  viewMode: "grid" | "list";
  files: FileItem[];
  folders: FolderItem[];
  onOpenFolder: (f: FolderItem) => void;
  onPreview: (f: FileItem) => void;
  // Move ops forward their destination folder id so the parent can also
  // invalidate the destination's SWR cache, not just the source.
  onMutate: (targetFolderId?: string | null) => void;
  onUploadClick: () => void;
  onNewFolderClick: () => void;
  // False when the current folder is shared read-only — hides empty-state
  // upload/new-folder actions. Per-item write gating is derived from each
  // item's own shared/permission fields in the grid + toolbar.
  canWrite: boolean;
}) {
  // Selection now spans files AND folders. /files:batch accepts both
  // file_ids and folder_ids; the SelectionToolbar partitions the visible
  // selection by item id back into the two arrays before posting. The
  // ordered union (folders first, then files) matches the grid's render
  // order so Shift+click range selection feels natural across the boundary.
  const itemIds = useMemo(
    () => [...folders.map((f) => f.id), ...files.map((f) => f.id)],
    [folders, files],
  );
  const { selectedIds, selectedSet, onClick, clear, setSelected } = useSelection(itemIds);

  // Marquee handler — replace selection on a fresh drag, merge when the
  // user held Ctrl/Cmd/Shift to add to an existing selection.
  const onMarquee = useCallback(
    (ids: string[], additive: boolean) => {
      setSelected((prev) => {
        if (!additive) return new Set(ids);
        const next = new Set(prev);
        for (const id of ids) next.add(id);
        return next;
      });
    },
    [setSelected],
  );

  // Keyboard shortcuts inside the file area:
  //   Esc       → clear selection
  //   Ctrl/⌘-A  → select all visible files
  //   Delete    → trash the selection (delegates to the toolbar's trash btn)
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement | null;
      if (t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.isContentEditable)) {
        return;
      }
      if (e.key === "Escape" && selectedIds.length > 0) {
        clear();
        e.preventDefault();
      } else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "a") {
        setSelected(new Set(itemIds));
        e.preventDefault();
      } else if (e.key === "Delete" && selectedIds.length > 0) {
        const btn = document.querySelector<HTMLButtonElement>(
          "[data-selection-toolbar='trash']",
        );
        btn?.click();
        e.preventDefault();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [selectedIds, clear, setSelected, itemIds]);

  return (
    <>
      {viewMode === "grid" ? (
        <FileGrid
          files={files}
          folders={folders}
          onOpenFolder={onOpenFolder}
          onPreview={onPreview}
          onMutate={onMutate}
          selectedIds={selectedSet}
          onItemClick={onClick}
          onMarquee={onMarquee}
          onClearSelection={clear}
          onUploadClick={onUploadClick}
          onNewFolderClick={onNewFolderClick}
          canWrite={canWrite}
        />
      ) : (
        <FileListView
          files={files}
          folders={folders}
          onOpenFolder={onOpenFolder}
          onPreview={onPreview}
          onMutate={onMutate}
          canWrite={canWrite}
        />
      )}
      <SelectionToolbar
        selectedIds={selectedIds}
        files={files}
        folders={folders}
        canWrite={canWrite}
        onClear={clear}
        onSuccess={(_action, targetFolderId) => {
          // targetFolderId is set only for batch moves; pass it through so
          // refreshAfterMutation can invalidate the destination listing.
          onMutate(targetFolderId);
          clear();
        }}
      />
    </>
  );
}
