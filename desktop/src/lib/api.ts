import fs from "node:fs";
import path from "node:path";
import { app } from "electron";
import * as tus from "tus-js-client";
import { Agent } from "undici";
import type { StateStore } from "./store";
import type { LoginPayload, RemoteEntity, StorageStats, TokenSignInPayload, User } from "./types";

/** Identifies desktop traffic in the backend access log + a token's last-used view. */
function desktopUserAgent(): string {
  try {
    return `MyCloud-Desktop/${app.getVersion()}`;
  } catch {
    return "MyCloud-Desktop";
  }
}

/** Shape of GET /api/v2/auth/whoami (identity + the token's scopes). */
interface WhoamiResponse {
  id: string;
  username: string;
  email: string;
  role: string;
  auth: string;
  scopes?: string[];
}

/* ------------------------------------------------------------------ */
/*  Undici agent for self-signed TLS certs                            */
/* ------------------------------------------------------------------ */

const insecureDispatcher = new Agent({
  connect: { rejectUnauthorized: false },
});

// tus-js-client uses Node's https.request directly (not undici/fetch), so the
// insecureDispatcher above doesn't apply to uploads. We build a parallel pair
// of httpStacks for tus and choose between them per upload based on the user's
// "Allow self-signed TLS" preference. The stack's constructor options are
// merged straight into https.request, which accepts rejectUnauthorized as a
// standard TLS option.
const defaultTusHttpStack = new tus.DefaultHttpStack({});
const insecureTusHttpStack = new tus.DefaultHttpStack({ rejectUnauthorized: false });

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

/** Max retries for transient network / server errors (5xx). */
const MAX_RETRIES = 3;
const RETRY_BASE_MS = 500;

interface RequestOptions extends Omit<RequestInit, "headers"> {
  noAuth?: boolean;
  retry?: boolean;
  headers?: Record<string, string>;
  /** Internal: current retry attempt (do not set manually). */
  _attempt?: number;
}

interface ApiEnvelope<T> {
  data?: T;
  meta?: Record<string, unknown>;
  error?: {
    code?: string;
    message?: string;
  };
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
    const { noAuth, retry = true, headers = {}, _attempt = 0, ...rest } = options;
    const mergedHeaders: Record<string, string> = { ...headers };

    if (!mergedHeaders["User-Agent"]) {
      mergedHeaders["User-Agent"] = desktopUserAgent();
    }
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
      // Retry on transient network errors (ECONNRESET, ECONNREFUSED, etc.)
      if (_attempt < MAX_RETRIES) {
        const delay = RETRY_BASE_MS * Math.pow(2, _attempt);
        await new Promise((resolve) => setTimeout(resolve, delay));
        return this.request<T>(urlPath, { ...options, _attempt: _attempt + 1 });
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

    // In token (PAT) mode there is no refresh token — a 401 means the token was
    // revoked or expired. Drop the credential so the renderer returns to the
    // sign-in screen (it shows the login view whenever auth.user is null).
    if (
      response.status === 401 &&
      !noAuth &&
      this.store.getState().auth.mode === "token"
    ) {
      this.store.update((state) => {
        state.auth.accessToken = "";
        state.auth.user = null;
        state.auth.scopes = [];
      });
      this.store.pushEvent({
        level: "error",
        message: "Access token is invalid or was revoked. Please reconnect.",
      });
    }

    // Retry on 5xx server errors
    if (response.status >= 500 && _attempt < MAX_RETRIES) {
      const delay = RETRY_BASE_MS * Math.pow(2, _attempt);
      await new Promise((resolve) => setTimeout(resolve, delay));
      return this.request<T>(urlPath, { ...options, _attempt: _attempt + 1 });
    }

    if (!response.ok) {
      let message = response.statusText;
      try {
        const json = (await response.json()) as ApiEnvelope<unknown>;
        message = json.error?.message || message;
      } catch {
        /* no json body */
      }
      throw new ApiError(message || "Request failed", response.status);
    }

    if (response.status === 204) return null;

    const contentType = response.headers.get("content-type") ?? "";
    if (!contentType.includes("application/json")) return null;

    const body = (await response.json()) as ApiEnvelope<T>;
    return (body.data ?? null) as T | null;
  }

  /* ---- auth ------------------------------------------------------ */

  private async tryRefresh(): Promise<boolean> {
    try {
      const response = await this.request<{
        access_token: string;
        refresh_token: string;
      }>("/api/v2/auth/refresh", {
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
    }>("/api/v2/auth/login", {
      method: "POST",
      noAuth: true,
      body: JSON.stringify({
        email: payload.email,
        password: payload.password,
      }),
    });

    const me = await this.request<User>("/api/v2/auth/me", {
      headers: { Authorization: `Bearer ${tokens!.access_token}` },
      noAuth: true,
    });

    this.store.update((state) => {
      state.auth.mode = "password";
      state.auth.accessToken = tokens!.access_token;
      state.auth.refreshToken = tokens!.refresh_token;
      state.auth.user = me!;
      state.auth.scopes = [];
      return state;
    });

    return me!;
  }

  /**
   * Sign in with a Personal Access Token (mc_pat_…). Validates the token and
   * fetches the identity via the scope-exempt /auth/whoami endpoint (so the
   * token needs only files:read+files:write, not account:read), then stores it
   * as the bearer with no refresh token. Warns when the token can't write.
   */
  async loginWithToken(payload: TokenSignInPayload): Promise<User> {
    this.store.update((state) => {
      state.apiBaseUrl = payload.apiBaseUrl.replace(/\/+$/, "");
      state.allowSelfSignedTls = Boolean(payload.allowSelfSignedTls);
      return state;
    });

    const token = payload.token.trim();
    let who: WhoamiResponse | null;
    try {
      who = await this.request<WhoamiResponse>("/api/v2/auth/whoami", {
        headers: { Authorization: `Bearer ${token}` },
        noAuth: true,
      });
    } catch (error: unknown) {
      if (error instanceof ApiError && error.status === 401) {
        throw new ApiError("Invalid or revoked token", 401);
      }
      throw error;
    }
    if (!who) throw new ApiError("Token validation failed", 0);

    const scopes = who.scopes ?? [];
    this.store.update((state) => {
      state.auth.mode = "token";
      state.auth.accessToken = token;
      state.auth.refreshToken = "";
      state.auth.user = { id: who!.id, username: who!.username, email: who!.email, role: who!.role };
      state.auth.scopes = scopes;
      return state;
    });

    if (!scopes.includes("*") && !scopes.includes("files:write")) {
      this.store.pushEvent({
        level: "info",
        message: "This token is read-only (no files:write) — uploads and deletes will fail.",
      });
    }
    return this.store.getState().auth.user!;
  }

  logout(): void {
    this.store.update((state) => {
      state.auth.mode = "password";
      state.auth.accessToken = "";
      state.auth.refreshToken = "";
      state.auth.user = null;
      state.auth.scopes = [];
      state.syncStatus.state = "idle";
      state.syncStatus.message = "Signed out";
      state.syncStatus.progress = null;
      return state;
    });
  }

  /* ---- folders --------------------------------------------------- */

  async createFolder(
    name: string,
    parentId: string | null = null,
  ): Promise<RemoteEntity> {
    return (await this.request<RemoteEntity>("/api/v2/folders", {
      method: "POST",
      body: JSON.stringify({
        name,
        ...(parentId ? { parent_id: parentId } : {}),
      }),
    }))!;
  }

  /* ---- files ----------------------------------------------------- */

  /**
   * Resumable upload via the tus protocol. The returned id comes from the
   * X-File-ID response header set by the backend's PreFinishResponseCallback.
   *
   * resumeUrl, if provided, allows resuming an interrupted upload from a prior
   * sync cycle. The caller passes back whatever was persisted in the local
   * mapping. The new upload URL (if a fresh upload) is delivered via the
   * onResumeUrl callback so callers can persist it before the bytes start.
   */
  async uploadFile(
    filePath: string,
    folderId: string | null,
    options?: {
      resumeUrl?: string;
      onResumeUrl?: (url: string) => void;
      onProgress?: (sent: number, total: number) => void;
    },
  ): Promise<RemoteEntity> {
    const fileName = path.basename(filePath);
    const stats = await fs.promises.stat(filePath);
    const stream = fs.createReadStream(filePath);
    const endpoint = `${this.baseUrl}/api/v2/files/tus/`;
    const accessToken = this.accessToken;

    return new Promise<RemoteEntity>((resolve, reject) => {
      const upload = new tus.Upload(stream as unknown as Blob, {
        endpoint,
        uploadUrl: options?.resumeUrl,
        retryDelays: [0, 1000, 3000, 5000, 10000],
        chunkSize: 8 * 1024 * 1024,
        uploadSize: stats.size,
        // Self-signed TLS only matters here — tus doesn't go through undici.
        httpStack: this.allowSelfSignedTls ? insecureTusHttpStack : defaultTusHttpStack,
        metadata: {
          filename: fileName,
          filetype: "application/octet-stream",
          folder_id: folderId ?? "",
        },
        headers: {
          "User-Agent": desktopUserAgent(),
          ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
        },
        onAfterResponse: async (req, res) => {
          const url = (upload as unknown as { url?: string }).url;
          if (url && req.getMethod() === "POST" && options?.onResumeUrl) {
            options.onResumeUrl(url);
          }
          // Tus places the file ID in X-File-ID on the final 201.
          const fileID = res.getHeader("X-File-ID");
          if (fileID) {
            (upload as unknown as { _fileID?: string })._fileID = fileID;
          }
        },
        onError: (err) => reject(new ApiError(err.message || "Upload failed", 0)),
        onProgress: (sent, total) => options?.onProgress?.(sent, total),
        onSuccess: () => {
          const fileID = (upload as unknown as { _fileID?: string })._fileID ?? "";
          resolve({ id: fileID } as RemoteEntity);
        },
      });
      upload.start();
    });
  }

  /**
   * Delete a remote file. Treats 404 as success — resource is already gone,
   * which is the desired end state.
   */
  async deleteFile(id: string): Promise<void> {
    try {
      await this.request(`/api/v2/files/${id}`, { method: "DELETE" });
    } catch (error: unknown) {
      if (error instanceof ApiError && error.status === 404) return;
      throw error;
    }
  }

  /**
   * Delete a remote folder. Treats 404 as success — resource is already gone,
   * which is the desired end state.
   */
  async deleteFolder(id: string): Promise<void> {
    try {
      await this.request(`/api/v2/folders/${id}`, { method: "DELETE" });
    } catch (error: unknown) {
      if (error instanceof ApiError && error.status === 404) return;
      throw error;
    }
  }

  /**
   * Rename and/or move a remote folder. Used by the sync engine when it
   * detects a local folder rename (matching child fingerprint across an
   * unlinkDir/addDir pair) — preserves the remote folder ID and every
   * descendant's IDs / comments / grants / versions, in contrast to the
   * delete-and-recreate path. `parentId` undefined ⇒ keep current parent.
   */
  async renameFolder(
    id: string,
    name: string,
    parentId?: string | null,
  ): Promise<void> {
    const body: Record<string, unknown> = { name };
    if (parentId !== undefined) body.parent_id = parentId;
    await this.request(`/api/v2/folders/${id}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    });
  }

  /* ---- storage stats -------------------------------------------- */

  /** Fetch used_bytes / quota_bytes / file_count / folder_count. */
  async getStorageStats(): Promise<StorageStats | null> {
    try {
      return await this.request<StorageStats>("/api/v2/storage/stats");
    } catch {
      return null;
    }
  }
}
