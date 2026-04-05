import { contextBridge, ipcRenderer, type IpcRendererEvent } from "electron";

export interface DesktopApi {
  getState: () => Promise<unknown>;
  login: (payload: unknown) => Promise<unknown>;
  logout: () => Promise<unknown>;
  pickFolder: () => Promise<string | null>;
  addFolder: (folderPath: string) => Promise<unknown>;
  removeFolder: (rootId: string) => Promise<unknown>;
  syncNow: () => Promise<unknown>;
  openFolder: (rootId: string) => Promise<boolean>;
  openWeb: () => Promise<boolean>;
  showWindow: () => Promise<boolean>;
  onStateChanged: (handler: (value: unknown) => void) => () => void;
}

const desktopApi: DesktopApi = {
  getState: () => ipcRenderer.invoke("app:get-state"),
  login: (payload) => ipcRenderer.invoke("auth:login", payload),
  logout: () => ipcRenderer.invoke("auth:logout"),
  pickFolder: () => ipcRenderer.invoke("folders:pick"),
  addFolder: (folderPath) => ipcRenderer.invoke("folders:add", folderPath),
  removeFolder: (rootId) => ipcRenderer.invoke("folders:remove", rootId),
  syncNow: () => ipcRenderer.invoke("sync:run-now"),
  openFolder: (rootId) => ipcRenderer.invoke("folders:open", rootId),
  openWeb: () => ipcRenderer.invoke("app:open-web"),
  showWindow: () => ipcRenderer.invoke("window:show"),
  onStateChanged: (handler) => {
    const listener = (_event: IpcRendererEvent, value: unknown) =>
      handler(value);
    ipcRenderer.on("state:updated", listener);
    return () => ipcRenderer.removeListener("state:updated", listener);
  },
};

contextBridge.exposeInMainWorld("desktopApi", desktopApi);
