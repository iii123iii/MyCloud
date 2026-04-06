import fs from "node:fs";
import path from "node:path";
import type { AppState, AppEvent, StateUpdater } from "./types";

const MAX_EVENTS = 40;
const SAVE_DEBOUNCE_MS = 500;

export const DEFAULT_STATE: AppState = {
  apiBaseUrl: "http://localhost:8080",
  allowSelfSignedTls: true,
  auth: {
    accessToken: "",
    refreshToken: "",
    user: null,
  },
  roots: [],
  mappings: {},
  syncStatus: {
    state: "idle",
    message: "Idle",
    activeRootId: null,
    lastSyncAt: null,
    progress: null,
  },
  events: [],
  autoSyncMinutes: 5,
  storageStats: null,
};

export class StateStore {
  private filePath: string;
  private state: AppState;
  private saveTimer: ReturnType<typeof setTimeout> | null = null;
  private dirty = false;

  constructor(baseDir: string) {
    this.filePath = path.join(baseDir, "state.json");
    fs.mkdirSync(baseDir, { recursive: true });
    this.state = this.load();
  }

  /* ---- persistence ------------------------------------------------ */

  private load(): AppState {
    try {
      if (fs.existsSync(this.filePath)) {
        const raw = fs.readFileSync(this.filePath, "utf8");
        return this.normalize(JSON.parse(raw));
      }
    } catch (error) {
      console.error("Failed to load desktop state:", error);
    }
    return structuredClone(DEFAULT_STATE);
  }

  private normalize(input: Partial<AppState>): AppState {
    return {
      apiBaseUrl: input.apiBaseUrl ?? DEFAULT_STATE.apiBaseUrl,
      allowSelfSignedTls: input.allowSelfSignedTls ?? DEFAULT_STATE.allowSelfSignedTls,
      auth: {
        accessToken: input.auth?.accessToken ?? "",
        refreshToken: input.auth?.refreshToken ?? "",
        user: input.auth?.user ?? null,
      },
      syncStatus: {
        state: input.syncStatus?.state ?? "idle",
        message: input.syncStatus?.message ?? "Idle",
        activeRootId: input.syncStatus?.activeRootId ?? null,
        lastSyncAt: input.syncStatus?.lastSyncAt ?? null,
        progress: input.syncStatus?.progress ?? null,
      },
      roots: Array.isArray(input.roots)
        ? input.roots.map((r) => ({
            ...r,
            fileCount: r.fileCount ?? 0,
            dirCount:  r.dirCount ?? 0,
          }))
        : [],
      mappings:
        input.mappings && typeof input.mappings === "object"
          ? input.mappings
          : {},
      events: Array.isArray(input.events)
        ? input.events.slice(0, MAX_EVENTS)
        : [],
      autoSyncMinutes: input.autoSyncMinutes ?? DEFAULT_STATE.autoSyncMinutes,
      storageStats: input.storageStats ?? null,
    };
  }

  /** Debounced write — flushes at most once per SAVE_DEBOUNCE_MS. */
  private scheduleSave(): void {
    this.dirty = true;
    if (this.saveTimer) return;
    this.saveTimer = setTimeout(() => {
      this.saveTimer = null;
      if (this.dirty) {
        this.flushSync();
      }
    }, SAVE_DEBOUNCE_MS);
  }

  /** Immediately write to disk (call on quit / critical events). */
  flushSync(): void {
    this.dirty = false;
    try {
      fs.writeFileSync(
        this.filePath,
        JSON.stringify(this.state, null, 2),
        "utf8",
      );
    } catch (error) {
      console.error("Failed to write state to disk:", error);
    }
  }

  /* ---- public API ------------------------------------------------- */

  getState(): AppState {
    return this.state;
  }

  /**
   * Mutate state in-place. The updater receives the LIVE state object
   * (not a clone) and should mutate it directly. This avoids the O(n)
   * structuredClone overhead that was catastrophic with large mapping
   * tables (tens of thousands of entries).
   *
   * For safety, normalize is only applied on load/save boundaries,
   * not on every mutation.
   */
  update(updater: StateUpdater): AppState {
    updater(this.state);
    this.scheduleSave();
    return this.state;
  }

  pushEvent(
    event: Omit<AppEvent, "id" | "timestamp">,
  ): AppEvent {
    const item: AppEvent = {
      id: `${Date.now()}-${Math.random().toString(16).slice(2, 8)}`,
      timestamp: new Date().toISOString(),
      ...event,
    };
    this.state.events.unshift(item);
    if (this.state.events.length > MAX_EVENTS) {
      this.state.events.length = MAX_EVENTS;
    }
    this.scheduleSave();
    return item;
  }

  /** Call before quit to guarantee no data loss. */
  destroy(): void {
    if (this.saveTimer) {
      clearTimeout(this.saveTimer);
      this.saveTimer = null;
    }
    if (this.dirty) {
      this.flushSync();
    }
  }
}
