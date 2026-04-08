import path from "node:path";
import {
  app,
  BrowserWindow,
  Tray,
  Menu,
  ipcMain,
  dialog,
  nativeImage,
  shell,
  screen,
  Notification,
  type IpcMainInvokeEvent,
} from "electron";
import { StateStore } from "./lib/store";
import { ApiClient } from "./lib/api";
import { SyncEngine } from "./lib/sync-engine";
import type { AppState, LoginPayload } from "./lib/types";

/* ------------------------------------------------------------------ */
/*  Module-level state                                                 */
/* ------------------------------------------------------------------ */

let mainWindow: BrowserWindow | null = null;
let tray: Tray | null = null;
let store: StateStore;
let api: ApiClient;
let syncEngine: SyncEngine;
let refreshTrayMenu: () => void = () => {};

// Prevents blur→hide from firing while a system dialog is open
let suppressBlur = false;

/* ------------------------------------------------------------------ */
/*  Popup dimensions                                                   */
/* ------------------------------------------------------------------ */

const WIN_WIDTH  = 360;
const WIN_HEIGHT = 560;

/* ------------------------------------------------------------------ */
/*  Tray icon                                                          */
/* ------------------------------------------------------------------ */

function createTrayIcon(): Electron.NativeImage {
  const svg = `
    <svg xmlns="http://www.w3.org/2000/svg" width="64" height="64">
      <rect width="64" height="64" rx="16" fill="#10191c"/>
      <path d="M14 44 25 20h6l11 24h-6.5l-2.2-5.4H22.5L20.3 44H14Zm10.8-10.4h6.2l-3.1-7.8-3.1 7.8Z" fill="#f6efe3"/>
      <circle cx="47" cy="18" r="7" fill="#5ac994"/>
    </svg>`;
  return nativeImage.createFromDataURL(
    `data:image/svg+xml;base64,${Buffer.from(svg).toString("base64")}`,
  );
}

/* ------------------------------------------------------------------ */
/*  State emission                                                     */
/* ------------------------------------------------------------------ */

function emitState(state: AppState = store.getState()): void {
  if (mainWindow && !mainWindow.isDestroyed()) {
    mainWindow.webContents.send("state:updated", state);
  }
  refreshTrayMenu();
}

/* ------------------------------------------------------------------ */
/*  Tray-anchored popup window                                         */
/* ------------------------------------------------------------------ */

function createWindow(): void {
  mainWindow = new BrowserWindow({
    width: WIN_WIDTH,
    height: WIN_HEIGHT,
    // ── Key: no frame, no taskbar entry, stays on top like OneDrive ──
    frame: false,
    skipTaskbar: true,
    alwaysOnTop: true,
    resizable: false,
    movable: false,           // prevent dragging — it's a popup, not a window
    minimizable: false,
    maximizable: false,
    fullscreenable: false,
    show: false,
    transparent: false,
    backgroundColor: "#161616",
    // Windows 11 rounded-corner popup style
    roundedCorners: true,
    hasShadow: true,
    webPreferences: {
      preload: path.join(__dirname, "preload.js"),
      contextIsolation: true,
      nodeIntegration: false,
    },
  });

  mainWindow.loadFile(path.join(__dirname, "..", "renderer", "index.html"));

  // Hide when user clicks outside the popup (blur = clicked elsewhere)
  // suppressBlur is set true while a system dialog (e.g. folder picker) is open
  mainWindow.on("blur", () => {
    if (suppressBlur) return;
    if (mainWindow && !mainWindow.isDestroyed() && mainWindow.isVisible()) {
      mainWindow.hide();
    }
  });

  // Intercept close — always hide, never destroy (keeps tray running)
  mainWindow.on("close", (event) => {
    if (!(app as unknown as { isQuiting: boolean }).isQuiting) {
      event.preventDefault();
      mainWindow!.hide();
    }
  });
}

/* ------------------------------------------------------------------ */
/*  Position the popup above the tray icon (like OneDrive)            */
/* ------------------------------------------------------------------ */

function showAtTray(): void {
  if (!mainWindow || !tray) return;

  const trayBounds = tray.getBounds();
  const display    = screen.getDisplayNearestPoint({ x: trayBounds.x, y: trayBounds.y });
  const workArea   = display.workArea;

  // Center popup horizontally on the tray icon
  let x = Math.round(trayBounds.x + trayBounds.width / 2 - WIN_WIDTH / 2);
  // Default: just above the tray
  let y = Math.round(trayBounds.y - WIN_HEIGHT - 8);

  // If tray is at top of screen, flip to below instead
  if (y < workArea.y) {
    y = trayBounds.y + trayBounds.height + 8;
  }

  // Clamp to work area so popup never goes off-screen
  x = Math.max(workArea.x + 4, Math.min(x, workArea.x + workArea.width - WIN_WIDTH - 4));

  mainWindow.setPosition(x, y, false);
  mainWindow.show();
  mainWindow.focus();
}

/* ------------------------------------------------------------------ */
/*  System tray                                                        */
/* ------------------------------------------------------------------ */

function createTray(): void {
  tray = new Tray(createTrayIcon());
  tray.setToolTip("MyCloud Sync");

  refreshTrayMenu = () => {
    const state = store.getState();
    const syncLabel =
      state.syncStatus.state === "syncing" ? "Syncing…" :
      state.syncStatus.state === "paused"  ? "Paused"  : "Idle";

    tray!.setContextMenu(Menu.buildFromTemplate([
      { label: "Open MyCloud Sync", click: () => showAtTray() },
      { type: "separator" },
      { label: syncLabel, enabled: false },
      {
        label: state.syncStatus.state === "paused" ? "Resume" : "Pause",
        click: () => {
          if (syncEngine.isPaused) syncEngine.resume();
          else syncEngine.pause();
          emitState();
        },
      },
      {
        label: "Sync Now",
        enabled: state.syncStatus.state !== "syncing",
        click: () => { void syncEngine.scheduleFullSync(); },
      },
      { type: "separator" },
      {
        label: state.auth.user ? `${state.auth.user.username}` : "Not signed in",
        enabled: false,
      },
      { type: "separator" },
      {
        label: "Quit",
        click: () => {
          (app as unknown as { isQuiting: boolean }).isQuiting = true;
          app.quit();
        },
      },
    ]));
  };

  // Left-click toggles the popup
  tray.on("click", () => {
    if (mainWindow?.isVisible()) {
      mainWindow.hide();
    } else {
      showAtTray();
    }
  });

  refreshTrayMenu();
}

/* ------------------------------------------------------------------ */
/*  Safe IPC handler wrapper                                           */
/* ------------------------------------------------------------------ */

function ipcHandle(
  channel: string,
  handler: (event: IpcMainInvokeEvent, ...args: unknown[]) => Promise<unknown>,
): void {
  ipcMain.handle(channel, async (event, ...args) => {
    try {
      return await handler(event, ...args);
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : String(error);
      console.error(`[IPC] ${channel} failed:`, error);
      store.pushEvent({ level: "error", message });
      emitState();
      throw error;
    }
  });
}

/* ------------------------------------------------------------------ */
/*  Sync-done notification                                             */
/* ------------------------------------------------------------------ */

let lastSyncState: string | null = null;

function watchSyncNotifications(): void {
  setInterval(() => {
    const state = store.getState().syncStatus.state;
    if (lastSyncState === "syncing" && state === "idle") {
      if (Notification.isSupported()) {
        new Notification({
          title: "MyCloud Sync",
          body: "All folders are up to date.",
          icon: createTrayIcon(),
        }).show();
      }
    }
    if (lastSyncState === "syncing" && state === "error") {
      if (Notification.isSupported()) {
        new Notification({
          title: "MyCloud Sync — Error",
          body: store.getState().syncStatus.message || "Sync failed",
          icon: createTrayIcon(),
        }).show();
      }
    }
    lastSyncState = state;
  }, 2_000);
}

/* ------------------------------------------------------------------ */
/*  Bootstrap                                                          */
/* ------------------------------------------------------------------ */

async function bootstrap(): Promise<void> {
  // Single-instance guard — second launch just shows the popup
  const gotLock = app.requestSingleInstanceLock();
  if (!gotLock) { app.quit(); return; }
  app.on("second-instance", () => showAtTray());

  Menu.setApplicationMenu(null);

  store       = new StateStore(app.getPath("userData"));
  api         = new ApiClient(store);
  syncEngine  = new SyncEngine({ store, api, emitState });

  createWindow();
  createTray();
  syncEngine.start();
  emitState();
  watchSyncNotifications();

  /* ---- window --------------------------------------------------- */

  ipcMain.handle("app:get-state",  () => store.getState());
  ipcMain.handle("window:show",    () => { showAtTray(); return true; });
  ipcMain.handle("window:hide",    () => { mainWindow?.hide(); return true; });

  /* ---- auth ----------------------------------------------------- */

  ipcHandle("auth:login", async (_event, ...args) => {
    const payload = args[0] as LoginPayload;
    const user = await api.login(payload);
    syncEngine.start();
    store.pushEvent({ level: "success", message: `Signed in as ${user.username}` });
    emitState();
    // Fetch storage stats right after login
    void api.getStorageStats().then((s) => {
      if (s) { store.update((st) => { st.storageStats = s; }); emitState(); }
    });
    return store.getState();
  });

  ipcHandle("auth:logout", async () => {
    syncEngine.stopAllWatchers();
    api.logout();
    store.update((s) => { s.storageStats = null; });
    store.pushEvent({ level: "info", message: "Signed out" });
    emitState();
    return store.getState();
  });

  /* ---- folders -------------------------------------------------- */

  ipcMain.handle("folders:pick", async () => {
    // Suppress blur→hide so the popup doesn't vanish while the folder dialog is open
    suppressBlur = true;
    mainWindow?.setAlwaysOnTop(false);
    const result = await dialog.showOpenDialog({ properties: ["openDirectory"] });
    mainWindow?.setAlwaysOnTop(true);
    suppressBlur = false;
    // Re-focus the popup after dialog closes
    if (mainWindow && !mainWindow.isDestroyed()) mainWindow.focus();
    if (result.canceled || result.filePaths.length === 0) return null;
    return result.filePaths[0];
  });

  ipcHandle("folders:add", async (_event, ...args) => {
    const root = await syncEngine.addRoot(args[0] as string);
    emitState();
    return root;
  });

  ipcHandle("folders:remove", async (_event, ...args) => {
    await syncEngine.removeRoot(args[0] as string);
    emitState();
    return store.getState();
  });

  ipcHandle("folders:open", async (_event, ...args) => {
    const root = store.getState().roots.find((r) => r.id === args[0]);
    if (!root) return false;
    await shell.openPath(root.localPath);
    return true;
  });

  /* ---- sync ----------------------------------------------------- */

  ipcHandle("sync:run-now", async () => {
    await syncEngine.scheduleFullSync();
    emitState();
    return store.getState();
  });

  ipcMain.handle("sync:pause", () => {
    syncEngine.pause();
    emitState();
    return store.getState();
  });

  ipcMain.handle("sync:resume", () => {
    syncEngine.resume();
    emitState();
    return store.getState();
  });

  ipcMain.handle("sync:set-auto-interval", (_event, minutes: number) => {
    syncEngine.setAutoSyncMinutes(minutes);
    emitState();
    return store.getState();
  });

  /* ---- web ------------------------------------------------------ */

  ipcHandle("app:open-web", async () => {
    const base = (store.getState().apiBaseUrl || "http://localhost:8080").replace(/\/+$/, "");
    await shell.openExternal(base);
    return true;
  });

  /* ---- storage stats ------------------------------------------- */

  ipcMain.handle("app:get-storage-stats", async () => {
    if (!store.getState().auth.user) return null;
    const stats = await api.getStorageStats();
    if (stats) { store.update((s) => { s.storageStats = stats; }); emitState(); }
    return stats;
  });

  // Refresh stats every 5 minutes while logged in
  setInterval(async () => {
    if (!store.getState().auth.user) return;
    const stats = await api.getStorageStats();
    if (stats) { store.update((s) => { s.storageStats = stats; }); emitState(); }
  }, 5 * 60_000);

  // Fetch once at startup if already logged in
  if (store.getState().auth.user) {
    void api.getStorageStats().then((s) => {
      if (s) { store.update((st) => { st.storageStats = s; }); emitState(); }
    });
  }
}

/* ------------------------------------------------------------------ */
/*  App lifecycle                                                      */
/* ------------------------------------------------------------------ */

app.whenReady().then(bootstrap);

app.on("window-all-closed", () => {
  /* keep running in tray */
});

app.on("before-quit", () => {
  store?.destroy();
});
