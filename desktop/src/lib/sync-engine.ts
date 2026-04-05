import fs from "node:fs";
import path from "node:path";
import chokidar, { type FSWatcher } from "chokidar";
import type { StateStore } from "./store";
import type { ApiClient } from "./api";
import type {
  AppState,
  EmitStateFn,
  RemoteEntity,
  Root,
  RootMappings,
  SyncProgress,
  WalkResult,
} from "./types";

/* ------------------------------------------------------------------ */
/*  Helpers                                                            */
/* ------------------------------------------------------------------ */

function uid(): string {
  return `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`;
}

function normalizeRelativePath(relativePath: string): string {
  if (!relativePath || relativePath === ".") return "";
  return relativePath.split(path.sep).join("/");
}

function parentRelativePath(relativePath: string): string {
  if (!relativePath) return "";
  const normalized = normalizeRelativePath(relativePath);
  const parent = path.posix.dirname(normalized);
  return parent === "." ? "" : parent;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

async function walkDirectory(
  rootPath: string,
  currentPath: string = rootPath,
  output: WalkResult = { directories: [], files: [] },
): Promise<WalkResult> {
  let entries: fs.Dirent[];
  try {
    entries = await fs.promises.readdir(currentPath, { withFileTypes: true });
  } catch {
    return output;
  }
  for (const entry of entries) {
    // Skip hidden files and system artifacts at scan time
    if (
      entry.name.startsWith(".") ||
      /^(Thumbs\.db|desktop\.ini)$/i.test(entry.name) ||
      /\.(tmp|swp|swx|part)$/i.test(entry.name) ||
      entry.name.startsWith("~")
    ) {
      continue;
    }
    const fullPath = path.join(currentPath, entry.name);
    const relativePath = normalizeRelativePath(
      path.relative(rootPath, fullPath),
    );
    if (entry.isDirectory()) {
      output.directories.push(relativePath);
      await walkDirectory(rootPath, fullPath, output);
    } else if (entry.isFile()) {
      output.files.push(relativePath);
    }
  }
  return output;
}

/* ------------------------------------------------------------------ */
/*  Throttled emit — caps UI updates to once per EMIT_INTERVAL_MS     */
/* ------------------------------------------------------------------ */

const EMIT_INTERVAL_MS = 80;

/* ------------------------------------------------------------------ */
/*  SyncEngine                                                         */
/* ------------------------------------------------------------------ */

interface SyncEngineOptions {
  store: StateStore;
  api: ApiClient;
  emitState: EmitStateFn;
}

type FsEventType =
  | "file-upsert"
  | "file-delete"
  | "dir-upsert"
  | "dir-delete";

export class SyncEngine {
  private readonly store: StateStore;
  private readonly api: ApiClient;
  private readonly emitStateFn: EmitStateFn;
  private readonly watchers = new Map<string, FSWatcher>();
  private queue: Promise<void> = Promise.resolve();

  // Throttle state emissions
  private lastEmitTime = 0;
  private emitTimer: ReturnType<typeof setTimeout> | null = null;

  constructor({ store, api, emitState }: SyncEngineOptions) {
    this.store = store;
    this.api = api;
    this.emitStateFn = emitState;
  }

  /* ---- lifecycle ------------------------------------------------- */

  start(): void {
    for (const root of this.store.getState().roots) {
      this.attachWatcher(root);
    }
    if (this.store.getState().auth.user) {
      this.scheduleFullSync();
    }
  }

  stopAllWatchers(): void {
    for (const rootId of [...this.watchers.keys()]) {
      this.detachWatcher(rootId);
    }
  }

  /* ---- state helpers --------------------------------------------- */

  private snapshot(): AppState {
    return this.store.getState();
  }

  /** Throttled UI emit — avoids flooding the renderer during bulk sync. */
  private emit(): void {
    const now = Date.now();
    const elapsed = now - this.lastEmitTime;

    if (elapsed >= EMIT_INTERVAL_MS) {
      this.lastEmitTime = now;
      if (this.emitTimer) {
        clearTimeout(this.emitTimer);
        this.emitTimer = null;
      }
      this.emitStateFn(this.snapshot());
    } else if (!this.emitTimer) {
      this.emitTimer = setTimeout(() => {
        this.emitTimer = null;
        this.lastEmitTime = Date.now();
        this.emitStateFn(this.snapshot());
      }, EMIT_INTERVAL_MS - elapsed);
    }
  }

  /** Force-flush any pending throttled emit (call at end of sync). */
  private flushEmit(): void {
    if (this.emitTimer) {
      clearTimeout(this.emitTimer);
      this.emitTimer = null;
    }
    this.lastEmitTime = Date.now();
    this.emitStateFn(this.snapshot());
  }

  private pushEvent(
    level: "info" | "success" | "error",
    message: string,
    rootId: string | null = null,
  ): void {
    this.store.pushEvent({ level, message, rootId });
    this.emit();
  }

  private setStatus(
    state: "idle" | "syncing" | "error",
    message: string,
    activeRootId: string | null = null,
    progress: SyncProgress | null = null,
  ): void {
    this.store.update((draft) => {
      draft.syncStatus.state = state;
      draft.syncStatus.message = message;
      draft.syncStatus.activeRootId = activeRootId;
      draft.syncStatus.progress = progress;
      if (state === "idle") {
        draft.syncStatus.lastSyncAt = new Date().toISOString();
        draft.syncStatus.progress = null;
      }
      return draft;
    });
    this.emit();
  }

  /* ---- progress tracking ----------------------------------------- */

  private progressTracker(totalItems: number) {
    let completed = 0;
    return {
      tick: (currentFile: string): SyncProgress => {
        completed++;
        const percent =
          totalItems > 0 ? Math.round((completed / totalItems) * 100) : 0;
        return { totalItems, completedItems: completed, percent, currentFile };
      },
    };
  }

  /* ---- queue ----------------------------------------------------- */

  private enqueue(task: () => Promise<void>): Promise<void> {
    this.queue = this.queue.then(task).catch((error: unknown) => {
      console.error(error);
      const msg = errorMessage(error);
      this.setStatus("error", msg || "Sync failed");
      this.pushEvent("error", msg || "Sync failed");
    });
    return this.queue;
  }

  /* ---- auth guard ------------------------------------------------ */

  private ensureAuth(): void {
    if (!this.store.getState().auth.user) {
      throw new Error("Sign in before starting sync");
    }
  }

  /* ---- mapping accessors ----------------------------------------- */

  private getRoot(rootId: string): Root | undefined {
    return this.store.getState().roots.find((r) => r.id === rootId);
  }

  private getMappings(rootId: string): RootMappings {
    return this.store.getState().mappings[rootId] ?? {};
  }

  private ensureMappings(rootId: string): RootMappings {
    this.store.update((state) => {
      state.mappings[rootId] = state.mappings[rootId] ?? {};
      return state;
    });
    return this.getMappings(rootId);
  }

  /* ---- file watching --------------------------------------------- */

  private attachWatcher(root: Root): void {
    if (this.watchers.has(root.id)) return;

    const watcher = chokidar.watch(root.localPath, {
      ignoreInitial: true,
      ignored:
        /(^|[/\\])(\.|Thumbs\.db$|desktop\.ini$)|[/\\]~[^/\\]*$|\.(tmp|swp|swx|part)$/i,
      awaitWriteFinish: {
        stabilityThreshold: 1200,
        pollInterval: 150,
      },
    });

    watcher.on("add", (fp: string) =>
      this.onFsEvent("file-upsert", root.id, fp),
    );
    watcher.on("change", (fp: string) =>
      this.onFsEvent("file-upsert", root.id, fp),
    );
    watcher.on("unlink", (fp: string) =>
      this.onFsEvent("file-delete", root.id, fp),
    );
    watcher.on("addDir", (fp: string) =>
      this.onFsEvent("dir-upsert", root.id, fp),
    );
    watcher.on("unlinkDir", (fp: string) =>
      this.onFsEvent("dir-delete", root.id, fp),
    );
    watcher.on("error", (error: unknown) => {
      const msg = errorMessage(error);
      this.setStatus("error", `Watcher error: ${msg}`, root.id);
      this.pushEvent(
        "error",
        `Watcher error for ${root.label}: ${msg}`,
        root.id,
      );
    });

    this.watchers.set(root.id, watcher);
  }

  private detachWatcher(rootId: string): void {
    const watcher = this.watchers.get(rootId);
    if (watcher) {
      watcher.close();
      this.watchers.delete(rootId);
    }
  }

  /* ---- root management ------------------------------------------- */

  async addRoot(localPath: string): Promise<Root> {
    this.ensureAuth();
    const existing = this.store
      .getState()
      .roots.find((r) => r.localPath === localPath);
    if (existing) return existing;

    const root: Root = {
      id: uid(),
      label: path.basename(localPath),
      localPath,
      remoteRootId: null,
      lastScanAt: null,
    };

    this.store.update((state) => {
      state.roots.push(root);
      state.mappings[root.id] = {};
      return state;
    });
    this.attachWatcher(root);
    this.emit();

    await this.enqueue(() => this.syncRoot(root.id, true));
    return this.getRoot(root.id)!;
  }

  async removeRoot(rootId: string): Promise<void> {
    const root = this.getRoot(rootId);
    if (!root) return;
    const remoteRootId = root.remoteRootId;
    const mappedItems = Object.entries(this.getMappings(rootId)).map(
      ([itemPath, item]) => ({
        itemPath,
        item: { ...item },
      }),
    );

    this.detachWatcher(rootId);
    this.store.update((state) => {
      state.roots = state.roots.filter((r) => r.id !== rootId);
      delete state.mappings[rootId];
      if (state.syncStatus.activeRootId === rootId) {
        state.syncStatus.activeRootId = null;
        state.syncStatus.progress = null;
        if (state.syncStatus.state !== "error") {
          state.syncStatus.state = "idle";
          state.syncStatus.message = "Idle";
        }
      }
      return state;
    });
    this.pushEvent("info", `Stopped syncing ${root.label}`);
    this.emit();

    void this.enqueue(async () => {
      const isAuthed = Boolean(this.store.getState().auth.user);
      if (!isAuthed) return;

      try {
        await this.deleteMappedChildrenSnapshot(rootId, mappedItems);
        if (remoteRootId) {
          await this.api.deleteFolder(remoteRootId);
        }
      } catch (error: unknown) {
        this.pushEvent(
          "error",
          `Could not fully remove remote files for "${root.label}": ${errorMessage(error)}`,
          rootId,
        );
      }
    });
  }

  /* ---- live FS events -------------------------------------------- */

  private async onFsEvent(
    type: FsEventType,
    rootId: string,
    fullPath: string,
  ): Promise<void> {
    await this.enqueue(async () => {
      if (!this.store.getState().auth.user) return;
      const root = this.getRoot(rootId);
      if (!root) return;
      const relativePath = normalizeRelativePath(
        path.relative(root.localPath, fullPath),
      );
      if (!relativePath) return;

      if (type === "dir-upsert") {
        this.setStatus("syncing", `Creating folder ${relativePath}`, rootId);
        await this.ensureRemoteFolder(root, relativePath);
        this.pushEvent("info", `Created folder ${relativePath}`, rootId);
      }

      if (type === "file-upsert") {
        this.setStatus("syncing", `Uploading ${relativePath}`, rootId);
        await this.syncFile(root, relativePath);
        this.pushEvent("success", `Uploaded ${relativePath}`, rootId);
      }

      if (type === "file-delete") {
        this.setStatus("syncing", `Deleting ${relativePath}`, rootId);
        await this.removeFileMapping(rootId, relativePath);
        this.pushEvent("info", `Deleted ${relativePath}`, rootId);
      }

      if (type === "dir-delete") {
        this.setStatus("syncing", `Deleting folder ${relativePath}`, rootId);
        await this.removeDirectoryMapping(rootId, relativePath);
        this.pushEvent("info", `Deleted folder ${relativePath}`, rootId);
      }

      this.setStatus("idle", "Idle", rootId);
    });
  }

  /* ---- full sync ------------------------------------------------- */

  async scheduleFullSync(): Promise<void> {
    if (!this.store.getState().auth.user) {
      this.setStatus("idle", "Sign in to sync");
      return;
    }
    await this.enqueue(async () => {
      for (const root of this.store.getState().roots) {
        await this.syncRoot(root.id, !root.remoteRootId);
      }
      this.setStatus("idle", "Idle");
    });
  }

  private async syncRoot(
    rootId: string,
    createRemoteRoot = false,
  ): Promise<void> {
    this.ensureAuth();
    const root = this.getRoot(rootId);
    if (!root) return;

    this.setStatus("syncing", `Scanning ${root.label}`, rootId);

    // Gracefully handle the local folder no longer existing.
    let dirStat: fs.Stats;
    try {
      dirStat = await fs.promises.stat(root.localPath);
    } catch (error: unknown) {
      if ((error as NodeJS.ErrnoException)?.code === "ENOENT") {
        this.pushEvent(
          "error",
          `Folder not found locally, skipping: ${root.localPath}`,
          rootId,
        );
        this.setStatus("idle", "Idle");
        return;
      }
      throw error;
    }

    if (!dirStat.isDirectory()) {
      this.pushEvent(
        "error",
        `Path is not a folder, skipping: ${root.localPath}`,
        rootId,
      );
      this.setStatus("idle", "Idle");
      return;
    }

    // Create remote root folder if needed
    if (createRemoteRoot || !root.remoteRootId) {
      const created = await this.api.createFolder(root.label);
      this.store.update((state) => {
        const match = state.roots.find((r) => r.id === rootId);
        if (match) match.remoteRootId = created.id;
        return state;
      });
      this.pushEvent("success", `Created remote root ${root.label}`, rootId);
    }

    // Scan local tree
    const scan = await walkDirectory(root.localPath);
    const mappings = this.getMappings(rootId);
    const currentDirectories = new Set(scan.directories);
    const currentFiles = new Set(scan.files);

    // Count total work items for progress tracking
    const staleCount = Object.keys(mappings).filter(
      (p) =>
        (mappings[p].type === "file" && !currentFiles.has(p)) ||
        (mappings[p].type === "folder" && !currentDirectories.has(p)),
    ).length;
    const totalItems =
      scan.directories.length + scan.files.length + staleCount;
    const tracker = this.progressTracker(totalItems);

    // Sync directories (sorted to ensure parents before children)
    for (const dir of [...scan.directories].sort()) {
      const progress = tracker.tick(dir);
      this.setStatus("syncing", `Creating folder ${dir}`, rootId, progress);
      await this.ensureRemoteFolder(this.getRoot(rootId)!, dir);
    }

    // Sync files
    for (const file of [...scan.files].sort()) {
      const progress = tracker.tick(file);
      this.setStatus("syncing", `Uploading ${file}`, rootId, progress);
      await this.syncFile(this.getRoot(rootId)!, file);
    }

    // Clean up stale mappings (remote items that no longer exist locally)
    const mappedPaths = Object.keys(mappings).sort((a, b) =>
      b.localeCompare(a),
    );
    for (const mappedPath of mappedPaths) {
      const item = mappings[mappedPath];
      if (item.type === "file" && !currentFiles.has(mappedPath)) {
        const progress = tracker.tick(mappedPath);
        this.setStatus("syncing", `Removing ${mappedPath}`, rootId, progress);
        await this.removeFileMapping(rootId, mappedPath);
      }
      if (item.type === "folder" && !currentDirectories.has(mappedPath)) {
        const progress = tracker.tick(mappedPath);
        this.setStatus(
          "syncing",
          `Removing folder ${mappedPath}`,
          rootId,
          progress,
        );
        await this.removeDirectoryMapping(rootId, mappedPath);
      }
    }

    // Mark scan complete
    this.store.update((state) => {
      const match = state.roots.find((r) => r.id === rootId);
      if (match) match.lastScanAt = new Date().toISOString();
      return state;
    });
    this.pushEvent("success", `Synced ${root.label}`, rootId);
    this.flushEmit();
  }

  /* ---- remote folder management ---------------------------------- */

  private async ensureRemoteFolder(
    root: Root,
    relativePath: string,
  ): Promise<number | null> {
    if (!relativePath) return root.remoteRootId;
    const mappings = this.ensureMappings(root.id);
    const normalized = normalizeRelativePath(relativePath);
    const existing = mappings[normalized];
    if (existing?.type === "folder") return existing.remoteId;

    const parentRel = parentRelativePath(normalized);
    const parentId = parentRel
      ? await this.ensureRemoteFolder(root, parentRel)
      : root.remoteRootId;

    const created = await this.api.createFolder(
      path.posix.basename(normalized),
      parentId,
    );

    this.store.update((state) => {
      state.mappings[root.id] = state.mappings[root.id] ?? {};
      state.mappings[root.id][normalized] = {
        type: "folder",
        remoteId: created.id,
      };
      return state;
    });
    return created.id;
  }

  /* ---- single file sync ------------------------------------------ */

  private async syncFile(
    root: Root,
    relativePath: string,
  ): Promise<number | null> {
    const normalized = normalizeRelativePath(relativePath);
    const fullPath = path.join(root.localPath, relativePath);

    let fileStat: fs.Stats;
    try {
      fileStat = await fs.promises.stat(fullPath);
    } catch (error: unknown) {
      if ((error as NodeJS.ErrnoException)?.code === "ENOENT") {
        await this.removeFileMapping(root.id, normalized);
        return null;
      }
      throw error;
    }

    const signature = `${fileStat.size}:${Math.floor(fileStat.mtimeMs)}`;
    const mappings = this.ensureMappings(root.id);
    const existing = mappings[normalized];

    // Skip unchanged files
    if (existing?.type === "file" && existing.signature === signature) {
      return existing.remoteId;
    }

    const parentRel = parentRelativePath(normalized);
    const folderId = parentRel
      ? await this.ensureRemoteFolder(root, parentRel)
      : root.remoteRootId;

    // Upload first, handle race condition if file is deleted mid-upload
    let uploaded: RemoteEntity;
    try {
      uploaded = await this.api.uploadFile(fullPath, folderId);
    } catch (error: unknown) {
      if ((error as NodeJS.ErrnoException)?.code === "ENOENT") {
        await this.removeFileMapping(root.id, normalized);
        return null;
      }
      throw error;
    }

    // Delete old remote file only after new upload succeeds
    if (existing?.type === "file") {
      await this.api.deleteFile(existing.remoteId);
    }

    this.store.update((state) => {
      state.mappings[root.id] = state.mappings[root.id] ?? {};
      state.mappings[root.id][normalized] = {
        type: "file",
        remoteId: uploaded.id,
        signature,
      };
      return state;
    });
    return uploaded.id;
  }

  /* ---- mapping cleanup ------------------------------------------- */

  private async removeFileMapping(
    rootId: string,
    relativePath: string,
  ): Promise<void> {
    const normalized = normalizeRelativePath(relativePath);
    const existing = this.getMappings(rootId)[normalized];
    if (existing?.type !== "file") return;

    // Best-effort remote delete — 404 is already handled in api.deleteFile.
    // Any other error is logged but must not prevent the local mapping removal.
    try {
      await this.api.deleteFile(existing.remoteId);
    } catch (error: unknown) {
      this.pushEvent(
        "error",
        `Could not delete remote file ${normalized}: ${errorMessage(error)}`,
        rootId,
      );
    }

    // Always remove the local mapping.
    this.store.update((state) => {
      if (state.mappings[rootId]) {
        delete state.mappings[rootId][normalized];
      }
      return state;
    });
  }

  private async removeDirectoryMapping(
    rootId: string,
    relativePath: string,
  ): Promise<void> {
    const normalized = normalizeRelativePath(relativePath);
    const mappings = this.getMappings(rootId);
    const descendants = Object.keys(mappings)
      .filter((k) => k === normalized || k.startsWith(`${normalized}/`))
      .sort((a, b) => b.localeCompare(a));

    for (const itemPath of descendants) {
      const item = mappings[itemPath];
      if (!item) continue;

      try {
        if (item.type === "file") {
          await this.api.deleteFile(item.remoteId);
        } else if (item.type === "folder") {
          await this.api.deleteFolder(item.remoteId);
        }
      } catch (error: unknown) {
        this.pushEvent(
          "error",
          `Could not delete remote item ${itemPath}: ${errorMessage(error)}`,
          rootId,
        );
      }

      // Always remove the local mapping even if remote delete failed.
      this.store.update((state) => {
        if (state.mappings[rootId]) {
          delete state.mappings[rootId][itemPath];
        }
        return state;
      });
    }
  }

  private async deleteMappedChildren(rootId: string): Promise<void> {
    const mappings = this.getMappings(rootId);
    const items = Object.keys(mappings).sort((a, b) => b.localeCompare(a));
    for (const itemPath of items) {
      const item = mappings[itemPath];

      try {
        if (item.type === "file") {
          await this.api.deleteFile(item.remoteId);
        } else if (item.type === "folder") {
          await this.api.deleteFolder(item.remoteId);
        }
      } catch (error: unknown) {
        this.pushEvent(
          "error",
          `Could not delete remote item ${itemPath}: ${errorMessage(error)}`,
          rootId,
        );
      }

      // Always clear the local mapping entry.
      this.store.update((state) => {
        if (state.mappings[rootId]) {
          delete state.mappings[rootId][itemPath];
        }
        return state;
      });
    }
  }

  private async deleteMappedChildrenSnapshot(
    rootId: string,
    items: Array<{
      itemPath: string;
      item: RootMappings[string];
    }>,
  ): Promise<void> {
    for (const { itemPath, item } of items.sort((a, b) =>
      b.itemPath.localeCompare(a.itemPath),
    )) {
      try {
        if (item.type === "file") {
          await this.api.deleteFile(item.remoteId);
        } else if (item.type === "folder") {
          await this.api.deleteFolder(item.remoteId);
        }
      } catch (error: unknown) {
        this.pushEvent(
          "error",
          `Could not delete remote item ${itemPath}: ${errorMessage(error)}`,
          rootId,
        );
      }
    }
  }
}
