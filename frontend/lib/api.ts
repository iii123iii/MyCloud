import type {
  AuthTokens, User, FileItem, FolderItem, PaginatedFiles,
  Share, TrashItem, SearchResult, AdminStats, ActivityLog,
} from "./types";

const BASE = process.env.NEXT_PUBLIC_API_URL ?? "";

// ─── Token storage (client-side only) ────────────────────────────────────────
const ACCESS_KEY  = "mc_access";
const REFRESH_KEY = "mc_refresh";

export const tokenStore = {
  getAccess:  (): string | null => (typeof window !== "undefined" ? localStorage.getItem(ACCESS_KEY)  : null),
  getRefresh: (): string | null => (typeof window !== "undefined" ? localStorage.getItem(REFRESH_KEY) : null),
  set(tokens: Pick<AuthTokens, "access_token" | "refresh_token">) {
    localStorage.setItem(ACCESS_KEY,  tokens.access_token);
    localStorage.setItem(REFRESH_KEY, tokens.refresh_token);
  },
  clear() {
    localStorage.removeItem(ACCESS_KEY);
    localStorage.removeItem(REFRESH_KEY);
  },
};

// ─── Core fetch wrapper ───────────────────────────────────────────────────────
async function request<T>(
  path: string,
  options: RequestInit & { noAuth?: boolean; sharePassword?: string } = {}
): Promise<T> {
  const { noAuth, sharePassword, ...fetchOpts } = options;
  const headers = new Headers(fetchOpts.headers);

  if (!noAuth) {
    const token = tokenStore.getAccess();
    if (token) headers.set("Authorization", `Bearer ${token}`);
  }
  if (sharePassword) {
    headers.set("X-Share-Password", sharePassword);
  }
  if (!headers.has("Content-Type") && !(fetchOpts.body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
  }

  const res = await fetch(`${BASE}${path}`, { ...fetchOpts, headers });

  // Auto-refresh on 401
  if (res.status === 401 && !noAuth) {
    const refreshToken = tokenStore.getRefresh();
    if (refreshToken) {
      const refreshRes = await fetch(`${BASE}/api/auth/refresh`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ refresh_token: refreshToken }),
      });
      if (refreshRes.ok) {
        const newTokens = await refreshRes.json();
        tokenStore.set(newTokens);
        headers.set("Authorization", `Bearer ${newTokens.access_token}`);
        const retryRes = await fetch(`${BASE}${path}`, { ...fetchOpts, headers });
        if (!retryRes.ok) {
          const err = await retryRes.json().catch(() => ({ error: retryRes.statusText }));
          throw new ApiError(retryRes.status, err.error ?? "Request failed");
        }
        if (retryRes.status === 204) return undefined as T;
        return retryRes.json();
      }
    }
    tokenStore.clear();
    if (typeof window !== "undefined") window.location.href = "/login";
    throw new ApiError(401, "Session expired");
  }

  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new ApiError(res.status, err.error ?? "Request failed");
  }

  if (res.status === 204 || res.headers.get("Content-Length") === "0") return undefined as T;
  return res.json();
}

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

// ─── Auth ─────────────────────────────────────────────────────────────────────
export const auth = {
  setupStatus: () => request<{ setup_complete: boolean }>("/api/setup/status", { noAuth: true }),
  setupComplete: (data: { username: string; email: string; password: string }) =>
    request<{ message: string }>("/api/setup/complete", {
      method: "POST", noAuth: true, body: JSON.stringify(data),
    }),

  login: async (data: { email: string; password: string }): Promise<AuthTokens> => {
    const tokens = await request<AuthTokens>("/api/auth/login", {
      method: "POST", noAuth: true, body: JSON.stringify(data),
    });
    tokenStore.set(tokens);
    return tokens;
  },

  register: async (data: { username: string; email: string; password: string }): Promise<AuthTokens> => {
    const tokens = await request<AuthTokens>("/api/auth/register", {
      method: "POST", noAuth: true, body: JSON.stringify(data),
    });
    tokenStore.set(tokens);
    return tokens;
  },

  logout: async () => {
    const refresh_token = tokenStore.getRefresh();
    await request("/api/auth/logout", {
      method: "POST",
      body: JSON.stringify({ refresh_token }),
    }).catch(() => {});
    tokenStore.clear();
  },

  me: () => request<User>("/api/auth/me"),

  changePassword: (data: { old_password: string; new_password: string }) =>
    request<{ message: string }>("/api/auth/change-password", {
      method: "POST", body: JSON.stringify(data),
    }),
};

// ─── Files ────────────────────────────────────────────────────────────────────
export const files = {
  list: (params?: {
    folder_id?: string; sort?: string; order?: string;
    all?: boolean; starred_only?: boolean; page?: number; page_size?: number;
  }) => {
    const q = new URLSearchParams();
    if (params?.folder_id) q.set("folder_id", params.folder_id);
    if (params?.sort)      q.set("sort",      params.sort);
    if (params?.order)     q.set("order",     params.order);
    if (params?.all)       q.set("all",       "1");
    if (params?.starred_only) q.set("starred_only", "1");
    if (params?.page)      q.set("page",      String(params.page));
    if (params?.page_size) q.set("page_size", String(params.page_size));
    return request<PaginatedFiles>(`/api/files?${q}`);
  },

  upload: (formData: FormData, onProgress?: (pct: number) => void) => {
    // Use XMLHttpRequest for progress tracking.
    // Wraps with a single auto-refresh retry on 401.
    const attempt = (accessToken: string | null): Promise<void> =>
      new Promise<void>((resolve, reject) => {
        const xhr = new XMLHttpRequest();
        xhr.open("POST", `${BASE}/api/files/upload`);
        if (accessToken) xhr.setRequestHeader("Authorization", `Bearer ${accessToken}`);
        xhr.upload.onprogress = (e) => {
          if (e.lengthComputable && onProgress) onProgress(Math.round((e.loaded / e.total) * 100));
        };
        xhr.onload = () => {
          if (xhr.status < 300) return resolve();
          reject(new ApiError(xhr.status, xhr.responseText));
        };
        xhr.onerror = () => reject(new ApiError(0, "Network error"));
        xhr.send(formData);
      });

    return attempt(tokenStore.getAccess()).catch(async (err) => {
      // Auto-refresh on 401, then retry once
      if (err instanceof ApiError && err.status === 401) {
        const refreshToken = tokenStore.getRefresh();
        if (refreshToken) {
          const res = await fetch(`${BASE}/api/auth/refresh`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ refresh_token: refreshToken }),
          });
          if (res.ok) {
            const newTokens = await res.json();
            tokenStore.set(newTokens);
            return attempt(newTokens.access_token);
          }
        }
        tokenStore.clear();
        if (typeof window !== "undefined") window.location.href = "/login";
      }
      throw err;
    });
  },

  downloadUrl: (id: string) => `${BASE}/api/files/${id}/download`,
  previewUrl:  (id: string) => `${BASE}/api/files/${id}/preview`,

  getInfo: (id: string) => request<FileItem>(`/api/files/${id}`),

  update: (id: string, data: Partial<{ name: string; folder_id: string | null; is_starred: boolean }>) =>
    request<{ message: string }>(`/api/files/${id}`, { method: "PATCH", body: JSON.stringify(data) }),

  delete: (id: string) => request<void>(`/api/files/${id}`, { method: "DELETE" }),
};

// ─── Folders ─────────────────────────────────────────────────────────────────
export const folders = {
  list: (parent_id?: string) => {
    const q = parent_id ? `?parent_id=${parent_id}` : "";
    return request<{ folders: FolderItem[] }>(`/api/folders${q}`);
  },
  get: (id: string) => request<FolderItem>(`/api/folders/${id}`),
  create: (data: { name: string; parent_id?: string }) =>
    request<FolderItem>("/api/folders", { method: "POST", body: JSON.stringify(data) }),
  update: (id: string, data: { name?: string; parent_id?: string | null }) =>
    request<{ message: string }>(`/api/folders/${id}`, { method: "PATCH", body: JSON.stringify(data) }),
  delete: (id: string) => request<void>(`/api/folders/${id}`, { method: "DELETE" }),
};

// ─── Shares ──────────────────────────────────────────────────────────────────
export const shares = {
  list: () => request<{ shares: Share[] }>("/api/shares"),
  create: (data: { file_id?: string; folder_id?: string; permission?: string; password?: string; expires_at?: string }) =>
    request<{ token: string; url: string }>("/api/shares", { method: "POST", body: JSON.stringify(data) }),
  delete: (id: string) => request<void>(`/api/shares/${id}`, { method: "DELETE" }),
  resolve: (token: string, sharePassword?: string) =>
    request<{ permission: string; file_id?: string; file_name?: string; file_size?: number; mime_type?: string }>(
      `/api/s/${token}`,
      { noAuth: true, sharePassword }
    ),
  downloadUrl: (token: string) => `${BASE}/api/s/${token}/download`,
};

// ─── Trash ───────────────────────────────────────────────────────────────────
export const trash = {
  list:    () => request<{ items: TrashItem[] }>("/api/trash"),
  restore: (id: string) => request<{ message: string }>(`/api/trash/${id}/restore`, { method: "POST" }),
  delete:  (id: string) => request<void>(`/api/trash/${id}`, { method: "DELETE" }),
  empty:   () => request<void>("/api/trash/empty", { method: "DELETE" }),
};

// ─── Search ──────────────────────────────────────────────────────────────────
export const search = {
  query: (q: string) => request<{ results: SearchResult[] }>(`/api/search?q=${encodeURIComponent(q)}`),
};

// ─── Admin ───────────────────────────────────────────────────────────────────
export const admin = {
  users:         () => request<{ users: User[] }>("/api/admin/users"),
  createUser:    (data: Partial<User> & { password: string }) =>
    request<{ id: string }>("/api/admin/users", { method: "POST", body: JSON.stringify(data) }),
  updateUser:    (id: string, data: Partial<User & { password: string }>) =>
    request<{ message: string }>(`/api/admin/users/${id}`, { method: "PATCH", body: JSON.stringify(data) }),
  deleteUser:    (id: string) => request<void>(`/api/admin/users/${id}`, { method: "DELETE" }),
  stats:         () => request<AdminStats>("/api/admin/stats"),
  logs:          (limit = 100) => request<{ logs: ActivityLog[] }>(`/api/admin/logs?limit=${limit}`),
  getSettings:   () => request<Record<string, string>>("/api/admin/settings"),
  putSettings:   (data: Record<string, string>) =>
    request<{ message: string }>("/api/admin/settings", { method: "PUT", body: JSON.stringify(data) }),
};
