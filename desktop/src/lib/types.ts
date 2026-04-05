/* ------------------------------------------------------------------ */
/*  Shared type definitions for the MyCloud desktop app               */
/* ------------------------------------------------------------------ */

export interface User {
  id: number;
  username: string;
  email: string;
  role: string;
}

export interface AuthState {
  accessToken: string;
  refreshToken: string;
  user: User | null;
}

export interface SyncProgress {
  totalItems: number;
  completedItems: number;
  /** 0–100 percentage (integer) */
  percent: number;
  currentFile: string;
}

export interface SyncStatus {
  state: "idle" | "syncing" | "error";
  message: string;
  activeRootId: string | null;
  lastSyncAt: string | null;
  progress: SyncProgress | null;
}

export interface Root {
  id: string;
  label: string;
  localPath: string;
  remoteRootId: number | null;
  lastScanAt: string | null;
}

export interface MappingEntry {
  type: "file" | "folder";
  remoteId: number;
  signature?: string;
}

export type RootMappings = Record<string, MappingEntry>;
export type AllMappings = Record<string, RootMappings>;

export interface AppEvent {
  id: string;
  timestamp: string;
  level: "info" | "success" | "error";
  message: string;
  rootId?: string | null;
}

export interface AppState {
  apiBaseUrl: string;
  allowSelfSignedTls: boolean;
  auth: AuthState;
  roots: Root[];
  mappings: AllMappings;
  syncStatus: SyncStatus;
  events: AppEvent[];
}

export interface LoginPayload {
  apiBaseUrl: string;
  email: string;
  password: string;
  allowSelfSignedTls: boolean;
}

export interface WalkResult {
  directories: string[];
  files: string[];
}

export interface RemoteEntity {
  id: number;
  [key: string]: unknown;
}

export type StateUpdater = (draft: AppState) => AppState | void;
export type EmitStateFn = (state: AppState) => void;
