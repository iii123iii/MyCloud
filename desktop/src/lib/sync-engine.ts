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

/** Skip hidden files, temp files, and system artifacts. */
function shouldSkipEntry(name: string): boolean {
  return (
    name.startsWith(".") ||
    name.startsWith("~") ||
    /^(Thumbs\.db|desktop\.ini)$/i.test(name) ||
    /\.(tmp|swp|swx|part)$/i.test(name)
  );
}

/**
 * Iterative directory walker — safe for arbitrarily deep trees.
 * Uses a stack instead of recursion to avoid stack overflow with
 * deeply nested directories (tens of thousands of files).
 */
async function walkDirectory(rootPath: string): Promise<WalkResult> {
  const output: WalkResult = { directories: [], files: [] };
  const stack: string[] = [rootPath];

  while (stack.length > 0) {
    const currentPath = stack.pop()!;
    let entries: fs.Dirent[];
    try {
      entries = await fs.promises.readdir(currentPath, { withFileTypes: true });
    } catch {
      continue;
    }
    for (const entry of entries) {
      if (shouldSkipEntry(entry.name)) continue;

      const fullPath = path.join(currentPath, entry.name);
      const relativePath = normalizeRelativePath(
        path.relative(rootPath, fullPath),
      );
      if (entry.isDirectory()) {
        output.directories.push(relativePath);
        stack.push(fullPath);
      } else if (entry.isFile()) {
        output.files.push(relativePath);
      }
    }
  }
  return output;
}

/* ------------------------------------------------------------------ */
/*  Concurrency limiter for parallel uploads                          */
/* ------------------------------------------------------------------ */

async function parallelMap<T>(
  items: T[],
  concurrency: number,
  fn: (item: T, index: number) => Promise<void>,
): Promise<void> {
  let idx = 0;
  const total = items.length;

  async function worker(): Promise<void> {
    while (idx < total) {
      const i = idx++;
      await fn(items[i], i);
    }
  }

  const workers: Promise<void>[] = [];
  for (let w = 0; w < Math.min(concurrency, total); w++) {
    workers.push(worker());
  }
  await Promise.all(workers);
}

/* ------------------------------------------------------------------ */
/*  Throttled emit — caps UI updates to once per EMIT_INTERVAL_MS     */
/* ------------------------------------------------------------------ */

const EMIT_INTERVAL_MS = 80;

/** Number of concurrent file uploads during bulk sync. */
const UPLOAD_CONCURRENCY = 6;

/**
 * How long (ms) to batch rapid FS events before processing.
 * Prevents a bulk file paste (e.g. 5 000 files) from creating
 * 5 000 individual queue items — they get coalesced instead.
 */
const FS_EVENT_DEBOUNCE_MS = 500;

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

interface PendingFsEvent {
  type: FsEventType;
  rootId: string;
  fullPath: string;
}

export class SyncEngine {
  private readonly store: StateStore;
  private readonly api: ApiClient;
  private readonly emitStateFn: EmitStateFn;
  private readonly watchers = new Map<string, FSWatcher>();
  private queue: Promise<void> = Promise.resolve();

  // Throttle state emissions
  private lastEmitTime = 0;
  private emitTimer: ReturnType<typeof setTimeout> | null = null;

  // Batched FS event processing — groups rapid events before processing
  private pendingFsEvents: PendingFsEvent[] = [];
  private fsEventTimer: ReturnType<typeof setTimeout> | null = null;

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
    if (this.fsEventTimer) {
      clearTimeout(this.fsEventTimer);
      this.fsEventTimer = null;
      this.pendingFsEvents.length = 0;
    }
    for (const rootId of [...this.watchers.keys()]) {
      this.detachWatcher(rootId);
    }
  }

  /* ---- pause / resume -------------------------------------------- */

  get isPaused(): boolean {
    return this.store.getState().syncStatus.state === "paused";
  }

  pause(): void {
    this.store.update((s) => {
      s.syncStatus.state = "paused";
      s.syncStatus.message = "Paused";
      s.syncStatus.progress = null;
    });
    this.emit();
  }

  resume(): void {
    this.store.update((s) => {
      if (s.syncStatus.state === "paused") {
        s.syncStatus.state = "idle";
        s.syncStatus.message = "Idle";
      }
    });
    this.emit();
  }

  /* ---- auto-sync interval ---------------------------------------- */

  setAutoSyncMinutes(minutes: number): void {
    this.store.update((s) => { s.autoSyncMinutes = minutes; });
    this.emit();
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
    // Never overwrite "paused" with "idle" — user explicitly paused
    if (this.isPaused && state === "idle") return;

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

    await this.enqueue(async () => {
      await this.syncRoot(root.id, true);
      const syncStatus = this.store.getState().syncStatus;
      if (syncStatus.state === "syncing" && syncStatus.activeRootId === root.id) {
        this.setStatus("idle", "Idle", root.id);
      }
    });
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

  /**
   * Debounced FS event handler.
   * Rapid events (e.g. pasting 5 000 files) are batched into one queue
   * item, deduped, and then processed with concurrency.
   */
  private onFsEvent(
    type: FsEventType,
    rootId: string,
    fullPath: string,
  ): void {
    this.pendingFsEvents.push({ type, rootId, fullPath });

    if (this.fsEventTimer) return;
    this.fsEventTimer = setTimeout(() => {
      this.fsEventTimer = null;
      const batch = this.pendingFsEvents.splice(0);
      if (batch.length === 0) return;

      void this.enqueue(async () => {
        if (!this.store.getState().auth.user) return;

        // Deduplicate: keep last event per path (e.g. add→change = change)
        const deduped = new Map<string, PendingFsEvent>();
        for (const evt of batch) {
          const root = this.getRoot(evt.rootId);
          if (!root) continue;
          const rel = normalizeRelativePath(
            path.relative(root.localPath, evt.fullPath),
          );
          if (!rel) continue;
          deduped.set(`${evt.rootId}::${rel}`, evt);
        }

        const events = [...deduped.values()];
        if (events.length === 0) return;

        // Separate into categories for ordered processing
        const dirUpserts: PendingFsEvent[] = [];
        const fileUpserts: PendingFsEvent[] = [];
        const fileDeletes: PendingFsEvent[] = [];
        const dirDeletes: PendingFsEvent[] = [];

        for (const evt of events) {
          if (evt.type === "dir-upsert") dirUpserts.push(evt);
          else if (evt.type === "file-upsert") fileUpserts.push(evt);
          else if (evt.type === "file-delete") fileDeletes.push(evt);
          else if (evt.type === "dir-delete") dirDeletes.push(evt);
        }

        const rootId = events[0].rootId;
        this.setStatus("syncing", `Processing ${events.length} changes`, rootId);

        // 1) Create directories first (sorted so parents come before children)
        for (const evt of dirUpserts.sort((a, b) => a.fullPath.localeCompare(b.fullPath))) {
          const root = this.getRoot(evt.rootId);
          if (!root) continue;
          const rel = normalizeRelativePath(path.relative(root.localPath, evt.fullPath));
          await this.ensureRemoteFolder(root, rel);
        }

        // 2) Upload files concurrently
        await parallelMap(fileUpserts, UPLOAD_CONCURRENCY, async (evt) => {
          const root = this.getRoot(evt.rootId);
          if (!root) return;
          const rel = normalizeRelativePath(path.relative(root.localPath, evt.fullPath));
          this.setStatus("syncing", `Uploading ${rel}`, evt.rootId);
          await this.syncFile(root, rel);
        });

        // 3) Delete files
        for (const evt of fileDeletes) {
          const root = this.getRoot(evt.rootId);
          if (!root) continue;
          const rel = normalizeRelativePath(path.relative(root.localPath, evt.fullPath));
          await this.removeFileMapping(evt.rootId, rel);
        }

        // 4) Delete directories (deepest first)
        for (const evt of dirDeletes.sort((a, b) => b.fullPath.localeCompare(a.fullPath))) {
          const root = this.getRoot(evt.rootId);
          if (!root) continue;
          const rel = normalizeRelativePath(path.relative(root.localPath, evt.fullPath));
          await this.removeDirectoryMapping(evt.rootId, rel);
        }

        this.pushEvent(
          "success",
          `Processed ${events.length} file system change${events.length === 1 ? "" : "s"}`,
          rootId,
        );
        this.setStatus("idle", "Idle", rootId);
      });
    }, FS_EVENT_DEBOUNCE_MS);
  }

  /* ---- full sync ------------------------------------------------- */

  async scheduleFullSync(): Promise<void> {
    if (this.isPaused) return;
    if (!this.store.getState().auth.user) {
      this.setStatus("idle", "Sign in to sync");
      return;
    }
    await this.enqueue(async () => {
      if (this.isPaused) return;
      for (const root of this.store.getState().roots) {
        if (this.isPaused) break;
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

    // Scan local tree (iterative — won't overflow on deep trees)
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

    // Sync files — run UPLOAD_CONCURRENCY uploads in parallel
    const sortedFiles = [...scan.files].sort();
    await parallelMap(sortedFiles, UPLOAD_CONCURRENCY, async (file) => {
      const progress = tracker.tick(file);
      this.setStatus("syncing", `Uploading ${file}`, rootId, progress);
      await this.syncFile(this.getRoot(rootId)!, file);
    });

    // Clean up stale mappings (remote items that no longer exist locally)
    const staleMappings = Object.keys(mappings)
      .filter(
        (p) =>
          (mappings[p].type === "file" && !currentFiles.has(p)) ||
          (mappings[p].type === "folder" && !currentDirectories.has(p)),
      )
      .sort((a, b) => b.localeCompare(a));

    for (const mappedPath of staleMappings) {
      const item = mappings[mappedPath];
      const progress = tracker.tick(mappedPath);
      if (item.type === "file") {
        this.setStatus("syncing", `Removing ${mappedPath}`, rootId, progress);
        await this.removeFileMapping(rootId, mappedPath);
      } else if (item.type === "folder") {
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
  ): Promise<string | null> {
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
  ): Promise<string | null> {
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
