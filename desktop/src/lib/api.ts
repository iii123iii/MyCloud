import fs from "node:fs";
import path from "node:path";
import { Agent } from "undici";
import type { StateStore } from "./store";
import type { LoginPayload, RemoteEntity, User } from "./types";

/* ------------------------------------------------------------------ */
/*  Undici agent for self-signed TLS certs                            */
/* ------------------------------------------------------------------ */

const insecureDispatcher = new Agent({
  connect: { rejectUnauthorized: false },
});

/* ------------------------------------------------------------------ */
/*  ApiError — carries HTTP status so callers can inspect it          */
/* ------------------------------------------------------------------ */

export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

/* ------------------------------------------------------------------ */
/*  Request options                                                    */
/* ------------------------------------------------------------------ */

interface RequestOptions extends Omit<RequestInit, "headers"> {
  noAuth?: boolean;
  retry?: boolean;
  headers?: Record<string, string>;
}

/* ------------------------------------------------------------------ */
/*  API Client                                                         */
/* ------------------------------------------------------------------ */

export class ApiClient {
  constructor(private readonly store: StateStore) {}

  /* ---- convenience accessors ------------------------------------- */

  private get baseUrl(): string {
    return (this.store.getState().apiBaseUrl || "").replace(/\/+$/, "");
  }

  private get accessToken(): string {
    return this.store.getState().auth.accessToken;
  }

  private get refreshToken(): string {
    return this.store.getState().auth.refreshToken;
  }

  private get allowSelfSignedTls(): boolean {
    return Boolean(this.store.getState().allowSelfSignedTls);
  }

  /* ---- core request ---------------------------------------------- */

  async request<T = unknown>(
    urlPath: string,
    options: RequestOptions = {},
  ): Promise<T | null> {
    const { noAuth, retry = true, headers = {}, ...rest } = options;
    const mergedHeaders: Record<string, string> = { ...headers };

    if (!noAuth && this.accessToken) {
      mergedHeaders.Authorization = `Bearer ${this.accessToken}`;
    }

    if (
      rest.body &&
      !(rest.body instanceof FormData) &&
      !mergedHeaders["Content-Type"]
    ) {
      mergedHeaders["Content-Type"] = "application/json";
    }

    let response: Response;
    try {
      response = await fetch(`${this.baseUrl}${urlPath}`, {
        ...rest,
        headers: mergedHeaders,
        ...(this.allowSelfSignedTls
          ? { dispatcher: insecureDispatcher as never }
          : {}),
      });
    } catch (error: unknown) {
      const cause = (error as { cause?: { code?: string } })?.cause;
      if (cause?.code === "DEPTH_ZERO_SELF_SIGNED_CERT") {
        throw new ApiError(
          "TLS certificate rejected. Enable self-signed TLS in the desktop app settings.",
          0,
        );
      }
      throw error;
    }

    // Auto-refresh on 401
    if (response.status === 401 && !noAuth && retry && this.refreshToken) {
      const refreshed = await this.tryRefresh();
      if (refreshed) {
        return this.request<T>(urlPath, { ...options, retry: false });
      }
    }

    if (!response.ok) {
      let message = response.statusText;
      try {
        const json = (await response.json()) as { error?: string };
        message = json.error || message;
      } catch {
        /* no json body */
      }
      throw new ApiError(message || "Request failed", response.status);
    }

    if (response.status === 204) return null;

    const contentType = response.headers.get("content-type") ?? "";
    if (!contentType.includes("application/json")) return null;

    return response.json() as Promise<T>;
  }

  /* ---- auth ------------------------------------------------------ */

  private async tryRefresh(): Promise<boolean> {
    try {
      const response = await this.request<{
        access_token: string;
        refresh_token: string;
      }>("/api/auth/refresh", {
        method: "POST",
        noAuth: true,
        body: JSON.stringify({ refresh_token: this.refreshToken }),
      });
      if (!response?.access_token) return false;
      this.store.update((state) => {
        state.auth.accessToken = response.access_token;
        state.auth.refreshToken = response.refresh_token;
        return state;
      });
      return true;
    } catch {
      return false;
    }
  }

  async login(payload: LoginPayload): Promise<User> {
    this.store.update((state) => {
      state.apiBaseUrl = payload.apiBaseUrl.replace(/\/+$/, "");
      state.allowSelfSignedTls = Boolean(payload.allowSelfSignedTls);
      return state;
    });

    const tokens = await this.request<{
      access_token: string;
      refresh_token: string;
    }>("/api/auth/login", {
      method: "POST",
      noAuth: true,
      body: JSON.stringify({
        email: payload.email,
        password: payload.password,
      }),
    });

    const me = await this.request<User>("/api/auth/me", {
      headers: { Authorization: `Bearer ${tokens!.access_token}` },
      noAuth: true,
    });

    this.store.update((state) => {
      state.auth.accessToken = tokens!.access_token;
      state.auth.refreshToken = tokens!.refresh_token;
      state.auth.user = me!;
      return state;
    });

    return me!;
  }

  logout(): void {
    this.store.update((state) => {
      state.auth.accessToken = "";
      state.auth.refreshToken = "";
      state.auth.user = null;
      state.syncStatus.state = "idle";
      state.syncStatus.message = "Signed out";
      state.syncStatus.progress = null;
      return state;
    });
  }

  /* ---- folders --------------------------------------------------- */

  async createFolder(
    name: string,
    parentId: number | null = null,
  ): Promise<RemoteEntity> {
    return (await this.request<RemoteEntity>("/api/folders", {
      method: "POST",
      body: JSON.stringify({
        name,
        ...(parentId ? { parent_id: parentId } : {}),
      }),
    }))!;
  }

  /* ---- files ----------------------------------------------------- */

  async uploadFile(
    filePath: string,
    folderId: number | null,
  ): Promise<RemoteEntity> {
    const fileName = path.basename(filePath);
    const buffer = await fs.promises.readFile(filePath);
    const blob = new Blob([buffer]);

    const form = new FormData();
    form.append("file", blob, fileName);
    if (folderId) {
      form.append("folder_id", String(folderId));
    }

    return (await this.request<RemoteEntity>("/api/files/upload", {
      method: "POST",
      body: form,
    }))!;
  }

  /**
   * Delete a remote file. Treats 404 as success — resource is already gone,
   * which is the desired end state.
   */
  async deleteFile(id: number): Promise<void> {
    try {
      await this.request(`/api/files/${id}`, { method: "DELETE" });
    } catch (error: unknown) {
      if (error instanceof ApiError && error.status === 404) return;
      throw error;
    }
  }

  /**
   * Delete a remote folder. Treats 404 as success — resource is already gone,
   * which is the desired end state.
   */
  async deleteFolder(id: number): Promise<void> {
    try {
      await this.request(`/api/folders/${id}`, { method: "DELETE" });
    } catch (error: unknown) {
      if (error instanceof ApiError && error.status === 404) return;
      throw error;
    }
  }
}
