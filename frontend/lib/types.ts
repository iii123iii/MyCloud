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
}

export interface FolderItem {
  id: string;
  name: string;
  parent_id?: string;
  created_at: string;
  updated_at?: string;
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
  created_at: string;
  file_id?: string;
  file_name?: string;
  folder_id?: string;
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
