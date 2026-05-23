// ─── Auth ─────────────────────────────────────────────────────────────────────
export interface User {
  id: string;
  username: string;
  email: string;
  role: "admin" | "user";
  quota_bytes: number;
  used_bytes: number;
  is_active: boolean;
  must_change_password: boolean;
  created_at: string;
}

export interface AuthTokens {
  access_token: string;
  refresh_token: string;
  // Server-provided JTI for the access token. The WebSocket provider matches
  // session_revoked events against this to kick this device out immediately.
  // Previously decoded from the JWT payload on the client; now sent
  // explicitly so we don't depend on JWT shape.
  access_jti?: string;
  refresh_jti?: string;
  // Session family ID — links access+refresh pairs across refresh rotations.
  family?: string;
  token_type: string;
  user_id: string;
  username: string;
  email?: string;
  role: "admin" | "user";
  must_change_password?: boolean;
}

// ─── Files & Folders ─────────────────────────────────────────────────────────
export interface FileItem {
  id: string;
  name: string;
  size_bytes: number;
  mime_type: string;
  folder_id?: string;
  is_starred: boolean;
  is_deleted?: boolean;
  created_at: string;
  updated_at: string;
  // Present when the item is not owned by the caller (shared via a grant).
  // owner_id is the resource owner; permission is the caller's effective grant
  // level. Used to gate write affordances when browsing a shared folder.
  owner_id?: string;
  shared?: boolean;
  permission?: GrantPermission;
}

export interface FolderItem {
  id: string;
  name: string;
  parent_id?: string;
  created_at: string;
  updated_at?: string;
  // Mirrors files.is_starred for UX parity. Backed by the user_favorites
  // table (sidebar pin list) — there is no folder.is_starred column. List
  // endpoints LEFT JOIN to populate it; toggling routes through the
  // favourites endpoints (or /files:batch with folder_ids for batch ops).
  is_favorited?: boolean;
  // Set when browsing a folder shared with the caller — see FileItem above.
  owner_id?: string;
  shared?: boolean;
  permission?: GrantPermission;
}

export type DriveItem =
  | ({ itemType: "file" } & FileItem)
  | ({ itemType: "folder" } & FolderItem);

export interface TrashItem {
  type: "file" | "folder";
  id: string;
  name: string;
  size_bytes?: number;
  mime_type?: string;
  deleted_at: string;
}

// ─── Shares ──────────────────────────────────────────────────────────────────
export interface Share {
  id: string;
  token: string;
  permission: "read" | "write";
  expires_at?: string;
  download_limit?: number;
  download_count: number;
  created_at: string;
  file_id?: string;
  file_name?: string;
  folder_id?: string;
}

// ─── Grants (per-user sharing) ───────────────────────────────────────────────
export type GrantPermission = "viewer" | "editor" | "owner";

export interface ShareGrant {
  id: string;
  permission: GrantPermission;
  created_at: string;
  grantee_id: string;
  granted_by: string;
  grantee_name?: string;
  granter_name?: string;
  file_id?: string;
  file_name?: string;
  folder_id?: string;
  folder_name?: string;
}

// ─── Search ──────────────────────────────────────────────────────────────────
export interface SearchResult {
  type: "file" | "folder";
  id: string;
  name: string;
  size_bytes?: number;
  mime_type?: string;
  is_starred?: boolean;
  updated_at: string;
}

// ─── Pagination ─────────────────────────────────────────────────────────────
export interface PaginatedFiles {
  files: FileItem[];
  total: number;
  page: number;
  page_size: number;
  has_more: boolean;
}

// ─── Updates ─────────────────────────────────────────────────────────────────
export interface UpdateInfo {
  current: string;
  latest: string;
  update_available: boolean;
  release_url: string;
  release_name: string;
  published_at: string;
  release_notes: string;
  apply_supported: boolean;
  apply_message: string;
  update_in_progress: boolean;
  update_status: "idle" | "running" | "failed" | "succeeded";
  update_status_message: string;
  update_log_path: string;
  last_started_target: string;
}

// ─── Admin ───────────────────────────────────────────────────────────────────
export interface AdminStats {
  total_users: number;
  total_files: number;
  total_storage_used: number;
  total_quota: number;
}

export interface ActivityLog {
  id: number;
  action: string;
  username?: string;
  resource_type?: string;
  ip_address?: string;
  created_at: string;
}
