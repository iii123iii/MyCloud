import type {
  AuthTokens, User, FileItem, FolderItem, PaginatedFiles,
  Share, TrashItem, SearchResult, AdminStats, ActivityLog, UpdateInfo,
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

  upload: (formData: FormData, onProgress?: (pct: number) => void) => {
    const attempt = (accessToken: string | null): Promise<void> =>
      new Promise<void>((resolve, reject) => {
        const xhr = new XMLHttpRequest();
        xhr.open("POST", `${BASE}/api/v2/files:upload`);
        if (accessToken) xhr.setRequestHeader("Authorization", `Bearer ${accessToken}`);
        xhr.upload.onprogress = (e) => {
          if (e.lengthComputable && onProgress) onProgress(Math.round((e.loaded / e.total) * 100));
        };
        xhr.onload = async () => {
          if (xhr.status < 300) return resolve();
          try {
            const parsed = JSON.parse(xhr.responseText) as ApiEnvelope<never>;
            reject(new ApiError(xhr.status, parsed.error?.message ?? xhr.statusText, parsed.error?.code));
          } catch {
            reject(new ApiError(xhr.status, xhr.statusText));
          }
        };
        xhr.onerror = () => reject(new ApiError(0, "Network error"));
        xhr.send(formData);
      });

    return attempt(tokenStore.getAccess()).catch(async (err) => {
      if (err instanceof ApiError && err.status === 401) {
        const refreshToken = tokenStore.getRefresh();
        if (refreshToken) {
          const res = await fetch(`${BASE}/api/v2/auth/refresh`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ refresh_token: refreshToken }),
          });
          const body = await parseEnvelope<AuthTokens>(res);
          if (res.ok && body?.data) {
            tokenStore.set(body.data);
            return attempt(body.data.access_token);
          }
        }
        tokenStore.clear();
        if (typeof window !== "undefined") window.location.href = "/login";
      }
      throw err;
    });
  },

  downloadUrl: (id: string) => `${BASE}/api/v2/files/${id}:download`,
  previewUrl: (id: string) => `${BASE}/api/v2/files/${id}:preview`,
  getInfo: (id: string) => request<FileItem>(`/api/v2/files/${id}`),
  update: (id: string, data: Partial<{ name: string; folder_id: string | null; is_starred: boolean }>) =>
    request<{ message: string }>(`/api/v2/files/${id}`, { method: "PATCH", body: JSON.stringify(data) }),
  delete: (id: string) => request<void>(`/api/v2/files/${id}`, { method: "DELETE" }),
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
  create: (data: { file_id?: string; folder_id?: string; permission?: string; password?: string; expires_at?: string }) =>
    request<{ token: string; url: string }>("/api/v2/shares", { method: "POST", body: JSON.stringify(data) }),
  delete: (id: string) => request<void>(`/api/v2/shares/${id}`, { method: "DELETE" }),
  resolve: (token: string, sharePassword?: string) =>
    request<{ permission: string; file_id?: string; file_name?: string; file_size?: number; mime_type?: string; folder_id?: string }>(
      `/api/v2/public/shares/${token}`,
      { noAuth: true, sharePassword }
    ),
  downloadUrl: (token: string) => `${BASE}/api/v2/public/shares/${token}:download`,
};

export const trash = {
  list: () => request<{ items: TrashItem[] }>("/api/v2/trash"),
  restore: (id: string) => request<{ message: string }>(`/api/v2/trash/${id}:restore`, { method: "POST" }),
  delete: (id: string) => request<void>(`/api/v2/trash/${id}`, { method: "DELETE" }),
  empty: () => request<void>("/api/v2/trash", { method: "DELETE" }),
};

export const search = {
  query: (q: string) => request<{ results: SearchResult[] }>(`/api/v2/search?q=${encodeURIComponent(q)}`),
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
