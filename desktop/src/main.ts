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

/* ------------------------------------------------------------------ */
/*  Tray icon (inline SVG → native image)                             */
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
/*  Window                                                             */
/* ------------------------------------------------------------------ */

function createWindow(): void {
  mainWindow = new BrowserWindow({
    width: 430,
    height: 720,
    minWidth: 430,
    minHeight: 720,
    maxWidth: 430,
    maxHeight: 720,
    show: false,
    resizable: false,
    autoHideMenuBar: true,
    backgroundColor: "#171717",
    title: "MyCloud Sync",
    webPreferences: {
      preload: path.join(__dirname, "preload.js"),
      contextIsolation: true,
      nodeIntegration: false,
    },
  });

  mainWindow.loadFile(path.join(__dirname, "..", "renderer", "index.html"));
  mainWindow.setMenuBarVisibility(false);

  mainWindow.once("ready-to-show", () => {
    mainWindow!.show();
  });

  mainWindow.on("close", (event) => {
    if (!(app as unknown as { isQuiting: boolean }).isQuiting) {
      event.preventDefault();
      mainWindow!.hide();
    }
  });
}

/* ------------------------------------------------------------------ */
/*  System tray                                                        */
/* ------------------------------------------------------------------ */

function createTray(): void {
  tray = new Tray(createTrayIcon());
  tray.setToolTip("MyCloud Sync");

  refreshTrayMenu = () => {
    const state = store.getState();
    const menu = Menu.buildFromTemplate([
      { label: "Open MyCloud Sync", click: () => mainWindow?.show() },
      {
        label:
          state.syncStatus.state === "syncing" ? "Syncing..." : "Run Sync Now",
        enabled: state.syncStatus.state !== "syncing",
        click: () => syncEngine.scheduleFullSync(),
      },
      { type: "separator" },
      {
        label: state.auth.user
          ? `Signed in as ${state.auth.user.username}`
          : "Not signed in",
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
    ]);
    tray!.setContextMenu(menu);
  };

  tray.on("click", () => {
    if (mainWindow?.isVisible()) {
      mainWindow.hide();
    } else {
      mainWindow?.show();
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
      const message =
        error instanceof Error ? error.message : String(error);
      console.error(`[IPC] ${channel} failed:`, error);
      store.pushEvent({ level: "error", message });
      emitState();
      throw error;
    }
  });
}

/* ------------------------------------------------------------------ */
/*  Bootstrap                                                          */
/* ------------------------------------------------------------------ */

async function bootstrap(): Promise<void> {
  Menu.setApplicationMenu(null);

  store = new StateStore(app.getPath("userData"));
  api = new ApiClient(store);
  syncEngine = new SyncEngine({ store, api, emitState });

  createWindow();
  createTray();
  syncEngine.start();
  emitState();

  /* ---- simple handlers ------------------------------------------ */

  ipcMain.handle("app:get-state", () => store.getState());
  ipcMain.handle("window:show", () => {
    mainWindow?.show();
    return true;
  });

  /* ---- auth ----------------------------------------------------- */

  ipcHandle("auth:login", async (_event, ...args) => {
    const payload = args[0] as LoginPayload;
    const user = await api.login(payload);
    store.pushEvent({
      level: "success",
      message: `Signed in as ${user.username}`,
    });
    emitState();
    await syncEngine.scheduleFullSync();
    return store.getState();
  });

  ipcHandle("auth:logout", async () => {
    syncEngine.stopAllWatchers();
    api.logout();
    store.pushEvent({ level: "info", message: "Signed out" });
    emitState();
    return store.getState();
  });

  /* ---- folders -------------------------------------------------- */

  ipcMain.handle("folders:pick", async () => {
    const result = await dialog.showOpenDialog(mainWindow!, {
      properties: ["openDirectory"],
    });
    if (result.canceled || result.filePaths.length === 0) return null;
    return result.filePaths[0];
  });

  ipcHandle("folders:add", async (_event, ...args) => {
    const folderPath = args[0] as string;
    const root = await syncEngine.addRoot(folderPath);
    emitState();
    return root;
  });

  ipcHandle("folders:remove", async (_event, ...args) => {
    const rootId = args[0] as string;
    await syncEngine.removeRoot(rootId);
    emitState();
    return store.getState();
  });

  ipcHandle("folders:open", async (_event, ...args) => {
    const rootId = args[0] as string;
    const root = store.getState().roots.find((r) => r.id === rootId);
    if (!root) return false;
    await shell.openPath(root.localPath);
    return true;
  });

  /* ---- misc ----------------------------------------------------- */

  ipcHandle("app:open-web", async () => {
    const apiBaseUrl = (
      store.getState().apiBaseUrl || "http://localhost:8080"
    ).replace(/\/+$/, "");
    await shell.openExternal(apiBaseUrl);
    return true;
  });

  ipcHandle("sync:run-now", async () => {
    await syncEngine.scheduleFullSync();
    emitState();
    return store.getState();
  });
}

/* ------------------------------------------------------------------ */
/*  App lifecycle                                                      */
/* ------------------------------------------------------------------ */

app.whenReady().then(bootstrap);

app.on("activate", () => {
  if (BrowserWindow.getAllWindows().length === 0) {
    createWindow();
  } else {
    mainWindow?.show();
  }
});

app.on("window-all-closed", () => {
  /* keep running in tray */
});

app.on("before-quit", () => {
  store?.destroy();
});
