import * as tus from "tus-js-client";
import type {
  AuthTokens, User, FileItem, FolderItem, PaginatedFiles,
  Share, ShareGrant, GrantPermission,
  TrashItem, SearchResult, AdminStats, ActivityLog, UpdateInfo,
} from "./types";

const BASE = process.env.NEXT_PUBLIC_API_URL ?? "";

type ApiEnvelope<T> = {
  data?: T;
  meta?: Record<string, unknown>;
  error?: { code: string; message: string; details?: unknown };
};

const ACCESS_KEY = "mc_access";
const REFRESH_KEY = "mc_refresh";

export const tokenStore = {
  getAccess: (): string | null => (typeof window !== "undefined" ? localStorage.getItem(ACCESS_KEY) : null),
  getRefresh: (): string | null => (typeof window !== "undefined" ? localStorage.getItem(REFRESH_KEY) : null),
  set(tokens: Pick<AuthTokens, "access_token" | "refresh_token">) {
    localStorage.setItem(ACCESS_KEY, tokens.access_token);
    localStorage.setItem(REFRESH_KEY, tokens.refresh_token);
  },
  clear() {
    localStorage.removeItem(ACCESS_KEY);
    localStorage.removeItem(REFRESH_KEY);
  },
};

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public code?: string,
  ) {
    super(message);
  }
}

async function parseEnvelope<T>(res: Response): Promise<ApiEnvelope<T> | null> {
  if (res.status === 204 || res.headers.get("Content-Length") === "0") return null;
  return res.json().catch(() => null);
}

async function requestEnvelope<T>(
  path: string,
  options: RequestInit & { noAuth?: boolean; sharePassword?: string } = {}
): Promise<ApiEnvelope<T> | null> {
  const { noAuth, sharePassword, ...fetchOpts } = options;
  const headers = new Headers(fetchOpts.headers);

  if (!noAuth) {
    const token = tokenStore.getAccess();
    if (token) headers.set("Authorization", `Bearer ${token}`);
  }
  if (sharePassword) headers.set("X-Share-Password", sharePassword);
  if (!headers.has("Content-Type") && !(fetchOpts.body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
  }

  const doFetch = () => fetch(`${BASE}${path}`, { ...fetchOpts, headers });
  let res = await doFetch();

  if (res.status === 401 && !noAuth) {
    const refreshToken = tokenStore.getRefresh();
    if (refreshToken) {
      const refreshRes = await fetch(`${BASE}/api/v2/auth/refresh`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ refresh_token: refreshToken }),
      });
      const refreshBody = await parseEnvelope<AuthTokens>(refreshRes);
      if (refreshRes.ok && refreshBody?.data) {
        tokenStore.set(refreshBody.data);
        headers.set("Authorization", `Bearer ${refreshBody.data.access_token}`);
        res = await doFetch();
      } else {
        tokenStore.clear();
        if (typeof window !== "undefined") window.location.href = "/login";
      }
    }
  }

  if (!res.ok) {
    if (res.status === 429) {
      const retry = res.headers.get("Retry-After");
      const body = await parseEnvelope<never>(res);
      const base = body?.error?.message ?? "Too many requests";
      const suffix = retry ? ` (retry in ${retry}s)` : "";
      throw new ApiError(429, base + suffix, body?.error?.code ?? "rate_limited");
    }
    const body = await parseEnvelope<never>(res);
    throw new ApiError(res.status, body?.error?.message ?? res.statusText, body?.error?.code);
  }

  return parseEnvelope<T>(res);
}

async function request<T>(
  path: string,
  options: RequestInit & { noAuth?: boolean; sharePassword?: string; raw?: boolean } = {}
): Promise<T> {
  const body = await requestEnvelope<T>(path, options);
  return (body?.data ?? undefined) as T;
}

export const auth = {
  setupStatus: () => request<{ setup_complete: boolean }>("/api/v2/setup/status", { method: "POST", noAuth: true }),
  setupComplete: (data: { username: string; email: string; password: string }) =>
    request<{ message: string }>("/api/v2/setup/complete", {
      method: "POST", noAuth: true, body: JSON.stringify(data),
    }),

  login: async (data: { email: string; password: string }): Promise<AuthTokens> => {
    const tokens = await request<AuthTokens>("/api/v2/auth/login", {
      method: "POST", noAuth: true, body: JSON.stringify(data),
    });
    tokenStore.set(tokens);
    return tokens;
  },

  register: async (data: { username: string; email: string; password: string }): Promise<AuthTokens> => {
    const tokens = await request<AuthTokens>("/api/v2/auth/register", {
      method: "POST", noAuth: true, body: JSON.stringify(data),
    });
    tokenStore.set(tokens);
    return tokens;
  },

  logout: async () => {
    const refresh_token = tokenStore.getRefresh();
    await request("/api/v2/auth/logout", {
      method: "POST",
      body: JSON.stringify({ refresh_token }),
    }).catch(() => {});
    tokenStore.clear();
  },

  me: () => request<User>("/api/v2/auth/me"),
  myActivity: (limit = 100, before?: string) =>
    request<{ logs: ActivityLog[] }>(
      `/api/v2/auth/me/activity?limit=${limit}${before ? `&before=${encodeURIComponent(before)}` : ""}`,
    ),

  changePassword: (data: { old_password: string; new_password: string }) =>
    request<{ message: string }>("/api/v2/auth/change-password", {
      method: "POST", body: JSON.stringify(data),
    }),

  deleteAccount: (data: { password: string }) =>
    request<void>("/api/v2/auth/account", {
      method: "DELETE", body: JSON.stringify(data),
    }),
};

export const files = {
  list: async (params?: {
    folder_id?: string; sort?: string; order?: string;
    all?: boolean; starred_only?: boolean; page?: number; page_size?: number;
  }): Promise<PaginatedFiles> => {
    const q = new URLSearchParams();
    if (params?.folder_id) q.set("folder_id", params.folder_id);
    if (params?.sort) q.set("sort", params.sort);
    if (params?.order) q.set("order", params.order);
    if (params?.all) q.set("all", "1");
    if (params?.starred_only) q.set("starred_only", "1");
    if (params?.page) q.set("page", String(params.page));
    if (params?.page_size) q.set("page_size", String(params.page_size));
    const body = await requestEnvelope<{ files: FileItem[] }>(`/api/v2/files?${q}`);
    return {
      files: body?.data?.files ?? [],
      total: Number(body?.meta?.total ?? 0),
      page: Number(body?.meta?.page ?? 1),
      page_size: Number(body?.meta?.page_size ?? 50),
      has_more: Boolean(body?.meta?.has_more),
    };
  },

  /**
   * Resumable upload via tus 1.0. Replaces the legacy single-shot POST.
   *
   * Backwards-compatible: callers pass a FormData object so existing code
   * doesn't break. We extract the file + folder_id from it and start a tus
   * upload at /api/v2/files/tus/.
   */
  upload: (formData: FormData, onProgress?: (pct: number) => void) => {
    const file = formData.get("file");
    if (!(file instanceof File)) {
      return Promise.reject(new ApiError(0, "No file in form data"));
    }
    const folderId = formData.get("folder_id");
    const folderStr = typeof folderId === "string" ? folderId : "";

    return new Promise<void>((resolve, reject) => {
      const token = tokenStore.getAccess();
      const endpoint = `${BASE}/api/v2/files/tus/`;
      // Same-origin prefix used to filter resumable entries left over from
      // earlier sessions. If the cached upload URL doesn't share scheme +
      // host with the current page, the browser would refuse to PATCH it
      // (cross-origin preflight + redirect), so drop it.
      const origin = typeof window !== "undefined" ? window.location.origin : "";
      const upload = new tus.Upload(file, {
        endpoint,
        retryDelays: [0, 1000, 3000, 5000, 10000],
        chunkSize: 8 * 1024 * 1024,
        metadata: {
          filename: file.name,
          filetype: file.type || "application/octet-stream",
          folder_id: folderStr,
        },
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        onError: (err) => reject(new ApiError(0, err.message || "Upload failed")),
        onProgress: (sent, total) => {
          if (onProgress && total > 0) onProgress(Math.round((sent / total) * 100));
        },
        onSuccess: () => resolve(),
      });
      upload.findPreviousUploads().then((previous) => {
        const usable = previous.filter((p) => p.uploadUrl && p.uploadUrl.startsWith(origin));
        if (usable.length > 0) {
          upload.resumeFromPreviousUpload(usable[0]);
        }
        upload.start();
      });
    });
  },

  downloadUrl: (id: string) => `${BASE}/api/v2/files/${id}:download`,
  previewUrl: (id: string) => `${BASE}/api/v2/files/${id}:preview`,
  getInfo: (id: string) => request<FileItem>(`/api/v2/files/${id}`),
  update: (id: string, data: Partial<{ name: string; folder_id: string | null; is_starred: boolean }>) =>
    request<{ message: string }>(`/api/v2/files/${id}`, { method: "PATCH", body: JSON.stringify(data) }),
  delete: (id: string) => request<void>(`/api/v2/files/${id}`, { method: "DELETE" }),

  /** Streams a zip of the selected files + folder trees. Triggers browser download. */
  downloadArchive: async (file_ids: string[], folder_ids: string[]) => {
    const token = tokenStore.getAccess();
    const res = await fetch(`${BASE}/api/v2/files:download-archive`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify({ file_ids, folder_ids }),
    });
    if (!res.ok) {
      const body = await parseEnvelope<never>(res);
      throw new ApiError(res.status, body?.error?.message ?? res.statusText, body?.error?.code);
    }
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "mycloud.zip";
    a.click();
    URL.revokeObjectURL(url);
  },
};

export const folders = {
  list: (parent_id?: string) => request<{ folders: FolderItem[] }>(`/api/v2/folders${parent_id ? `?parent_id=${parent_id}` : ""}`),
  path: (id: string) => request<{ folders: FolderItem[] }>(`/api/v2/folders/${id}/path`),
  get: (id: string) => request<FolderItem>(`/api/v2/folders/${id}`),
  create: (data: { name: string; parent_id?: string | null }) =>
    request<FolderItem>("/api/v2/folders", { method: "POST", body: JSON.stringify(data) }),
  update: (id: string, data: { name?: string; parent_id?: string | null }) =>
    request<{ message: string }>(`/api/v2/folders/${id}`, { method: "PATCH", body: JSON.stringify(data) }),
  delete: (id: string) => request<void>(`/api/v2/folders/${id}`, { method: "DELETE" }),
};

export const shares = {
  list: () => request<{ shares: Share[] }>("/api/v2/shares"),
  create: (data: { file_id?: string; folder_id?: string; permission?: string; password?: string; expires_at?: string; download_limit?: number }) =>
    request<{ token: string; url: string }>("/api/v2/shares", { method: "POST", body: JSON.stringify(data) }),
  delete: (id: string) => request<void>(`/api/v2/shares/${id}`, { method: "DELETE" }),
  resolve: (token: string, sharePassword?: string) =>
    request<{ permission: string; file_id?: string; file_name?: string; file_size?: number; mime_type?: string; folder_id?: string }>(
      `/api/v2/public/shares/${token}`,
      { noAuth: true, sharePassword }
    ),
  downloadUrl: (token: string) => `${BASE}/api/v2/public/shares/${token}:download`,
};

export interface FileVersion {
  version_no: number;
  size_bytes: number;
  created_at: string;
  created_by: string;
  username?: string;
}

export const versions = {
  list: (fileId: string) =>
    request<{ versions: FileVersion[] }>(`/api/v2/files/${fileId}/versions`),
  restore: (fileId: string, versionNo: number) =>
    request<{ message: string; now_current: number }>(
      `/api/v2/files/${fileId}/versions/${versionNo}:restore`,
      { method: "POST" },
    ),
  downloadUrl: (fileId: string, versionNo: number) =>
    `${BASE}/api/v2/files/${fileId}/versions/${versionNo}:download`,
  previewUrl: (fileId: string, versionNo: number) =>
    `${BASE}/api/v2/files/${fileId}/versions/${versionNo}:preview`,
};

export interface Comment {
  id: string;
  user_id: string;
  username: string;
  body: string;
  created_at: string;
  updated_at: string;
  editable: boolean;
}

export const comments = {
  list: (fileId: string) =>
    request<{ comments: Comment[] }>(`/api/v2/files/${fileId}/comments`),
  create: (fileId: string, body: string) =>
    request<{ id: string }>(`/api/v2/files/${fileId}/comments`, {
      method: "POST",
      body: JSON.stringify({ body }),
    }),
  update: (id: string, body: string) =>
    request<{ message: string }>(`/api/v2/comments/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ body }),
    }),
  delete: (id: string) =>
    request<void>(`/api/v2/comments/${id}`, { method: "DELETE" }),
};

export interface SmartFolderQuery {
  q?: string;
  mime_prefix?: string;
  tags?: string[];
  modified_after?: string;
  modified_before?: string;
  starred?: boolean;
  in_folder?: string;
}

export interface SmartFolder {
  id: string;
  name: string;
  query: SmartFolderQuery;
  color?: string;
  created_at: string;
}

export const smartFolders = {
  list: () => request<{ smart_folders: SmartFolder[] }>("/api/v2/smart-folders"),
  create: (data: { name: string; query: SmartFolderQuery; color?: string }) =>
    request<{ id: string }>("/api/v2/smart-folders", { method: "POST", body: JSON.stringify(data) }),
  update: (id: string, data: Partial<{ name: string; query: SmartFolderQuery; color: string }>) =>
    request<{ message: string }>(`/api/v2/smart-folders/${id}`, { method: "PATCH", body: JSON.stringify(data) }),
  delete: (id: string) => request<void>(`/api/v2/smart-folders/${id}`, { method: "DELETE" }),
  results: (id: string) => request<{ files: FileItem[]; name: string }>(`/api/v2/smart-folders/${id}/results`),
};

export interface UploadRequest {
  id: string;
  token: string;
  url: string;
  folder_id?: string;
  folder_name?: string;
  expires_at?: string;
  max_files?: number;
  used_files: number;
  has_password: boolean;
  created_at: string;
}

export const requests = {
  list: () => request<{ requests: UploadRequest[] }>("/api/v2/upload-requests"),
  create: (data: { folder_id?: string; expires_at?: string; max_files?: number; password?: string }) =>
    request<{ id: string; token: string; url: string }>("/api/v2/upload-requests", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  delete: (id: string) => request<void>(`/api/v2/upload-requests/${id}`, { method: "DELETE" }),
  resolve: (token: string, password?: string) =>
    request<{
      token: string;
      folder_name?: string;
      expires_at?: string;
      max_files?: number;
      uploads_remaining?: number;
      used_files: number;
    }>(`/api/v2/public/uploads/${token}`, { noAuth: true, sharePassword: password }),
  uploadUrl: (token: string) => `${BASE}/api/v2/public/uploads/${token}`,
};

export interface Tag {
  id: string;
  name: string;
  color: string;
  created_at: string;
}

export const tags = {
  list: () => request<{ tags: Tag[] }>("/api/v2/tags"),
  create: (data: { name: string; color?: string }) =>
    request<{ id: string; name: string; color: string }>("/api/v2/tags", {
      method: "POST", body: JSON.stringify(data),
    }),
  update: (id: string, data: { name?: string; color?: string }) =>
    request<{ message: string }>(`/api/v2/tags/${id}`, {
      method: "PATCH", body: JSON.stringify(data),
    }),
  delete: (id: string) => request<void>(`/api/v2/tags/${id}`, { method: "DELETE" }),
  attachFile: (fileId: string, tagId: string) =>
    request<void>(`/api/v2/files/${fileId}/tags/${tagId}`, { method: "POST" }),
  detachFile: (fileId: string, tagId: string) =>
    request<void>(`/api/v2/files/${fileId}/tags/${tagId}`, { method: "DELETE" }),
  attachFolder: (folderId: string, tagId: string) =>
    request<void>(`/api/v2/folders/${folderId}/tags/${tagId}`, { method: "POST" }),
  detachFolder: (folderId: string, tagId: string) =>
    request<void>(`/api/v2/folders/${folderId}/tags/${tagId}`, { method: "DELETE" }),
};

export const grants = {
  list: (direction: "incoming" | "outgoing") =>
    request<{ grants: ShareGrant[] }>(`/api/v2/shares/grants?direction=${direction}`),
  create: (data: { file_id?: string; folder_id?: string; grantee: string; permission?: GrantPermission }) =>
    request<{ id: string; permission: GrantPermission; grantee_id: string }>("/api/v2/shares/grants", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  update: (id: string, permission: GrantPermission) =>
    request<{ message: string }>(`/api/v2/shares/grants/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ permission }),
    }),
  delete: (id: string) => request<void>(`/api/v2/shares/grants/${id}`, { method: "DELETE" }),
};

export const trash = {
  list: () => request<{ items: TrashItem[] }>("/api/v2/trash"),
  restore: (id: string) => request<{ message: string }>(`/api/v2/trash/${id}:restore`, { method: "POST" }),
  delete: (id: string) => request<void>(`/api/v2/trash/${id}`, { method: "DELETE" }),
  empty: () => request<void>("/api/v2/trash", { method: "DELETE" }),
};

export const search = {
  query: (q: string, scope: "name" | "content" | "both" = "both") =>
    request<{ results: SearchResult[] }>(`/api/v2/search?q=${encodeURIComponent(q)}&scope=${scope}`),
};

export interface Photo {
  id: string;
  name: string;
  size_bytes: number;
  mime_type: string;
  width?: number;
  height?: number;
  shot_at: string;
  created_at: string;
}

export const photos = {
  list: (from?: string, to?: string) => {
    const params = new URLSearchParams();
    if (from) params.set("from", from);
    if (to) params.set("to", to);
    return request<{ photos: Photo[] }>(`/api/v2/photos?${params}`);
  },
  thumbUrl: (id: string) => `${BASE}/api/v2/files/${id}/thumb`,
};

export const admin = {
  users: () => request<{ users: User[] }>("/api/v2/admin/users"),
  createUser: (data: Partial<User> & { password: string }) =>
    request<{ id: string }>("/api/v2/admin/users", { method: "POST", body: JSON.stringify(data) }),
  updateUser: (id: string, data: Partial<User & { password: string }>) =>
    request<{ message: string }>(`/api/v2/admin/users/${id}`, { method: "PATCH", body: JSON.stringify(data) }),
  deleteUser: (id: string) => request<void>(`/api/v2/admin/users/${id}`, { method: "DELETE" }),
  stats: () => request<AdminStats>("/api/v2/admin/stats"),
  logs: (limit = 100) => request<{ logs: ActivityLog[] }>(`/api/v2/admin/logs?limit=${limit}`),
  getSettings: () => request<Record<string, string>>("/api/v2/admin/settings"),
  putSettings: (data: Record<string, string>) =>
    request<{ message: string }>("/api/v2/admin/settings", { method: "PUT", body: JSON.stringify(data) }),
  checkUpdate: () => request<UpdateInfo>("/api/v2/admin/updates/check"),
  fetchUpdateStatus: () => request<Pick<UpdateInfo, "update_in_progress" | "update_status" | "update_status_message">>("/api/v2/admin/updates/status"),
  applyUpdate: (target?: string) => request<{ message: string }>(`/api/v2/admin/updates/apply${target ? `?target=${encodeURIComponent(target)}` : ""}`, { method: "POST" }),
  fetchUpdateLog: () => request<{ lines: string[] }>("/api/v2/admin/updates/log"),
};
