import fs from "node:fs";
import path from "node:path";
import type { AppState, AppEvent, StateUpdater } from "./types";

const MAX_EVENTS = 40;
const SAVE_DEBOUNCE_MS = 300;

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
    const base = structuredClone(DEFAULT_STATE);
    return {
      ...base,
      ...input,
      auth: { ...base.auth, ...(input.auth ?? {}) },
      syncStatus: { ...base.syncStatus, ...(input.syncStatus ?? {}) },
      roots: Array.isArray(input.roots) ? input.roots : [],
      mappings:
        input.mappings && typeof input.mappings === "object"
          ? input.mappings
          : {},
      events: Array.isArray(input.events)
        ? input.events.slice(0, MAX_EVENTS)
        : [],
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
    fs.writeFileSync(
      this.filePath,
      JSON.stringify(this.state, null, 2),
      "utf8",
    );
  }

  /* ---- public API ------------------------------------------------- */

  getState(): AppState {
    return this.state;
  }

  update(updater: StateUpdater): AppState {
    const draft = structuredClone(this.state);
    const result = updater(draft) ?? draft;
    this.state = this.normalize(result);
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
    this.update((state) => {
      state.events.unshift(item);
      state.events = state.events.slice(0, MAX_EVENTS);
      return state;
    });
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
