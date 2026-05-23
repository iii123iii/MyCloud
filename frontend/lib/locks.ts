// Cooperative file locks — frontend API wrapper.
//
// Mirrors the patterns in lib/api.ts (Bearer auth via tokenStore, envelope
// parsing, ApiError on non-2xx) but lives in its own module so the locking
// feature stays self-contained. We re-implement a minimal request helper
// here rather than exporting one from api.ts because the latter would
// widen its public surface area.

import { tokenStore, ApiError } from "@/lib/api";

const BASE = process.env.NEXT_PUBLIC_API_URL ?? "";

/** A single active (non-expired) lock as returned by the backend. */
export interface FileLock {
  id: string;
  file_id: string;
  user_id: string;
  /** Lock category: "edit" (web editors), "sync" (desktop client), "office" (OnlyOffice). */
  kind: string;
  /** RFC-3339 UTC timestamp at which the lock auto-expires. */
  expires_at: string;
  /** RFC-3339 UTC timestamp of acquisition. */
  created_at: string;
}

/** Backend envelope, kept private — callers consume the unwrapped data. */
type ApiEnvelope<T> = {
  data?: T;
  meta?: Record<string, unknown>;
  error?: { code: string; message: string; details?: unknown };
};

async function parseEnvelope<T>(res: Response): Promise<ApiEnvelope<T> | null> {
  if (res.status === 204 || res.headers.get("Content-Length") === "0") return null;
  return res.json().catch(() => null);
}

async function request<T>(
  path: string,
  options: RequestInit = {},
): Promise<T | undefined> {
  const headers = new Headers(options.headers);
  const token = tokenStore.getAccess();
  if (token) headers.set("Authorization", `Bearer ${token}`);
  if (!headers.has("Content-Type") && !(options.body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
  }

  const res = await fetch(`${BASE}${path}`, { ...options, headers });
  if (!res.ok) {
    const body = await parseEnvelope<never>(res);
    throw new ApiError(
      res.status,
      body?.error?.message ?? res.statusText,
      body?.error?.code,
    );
  }
  const body = await parseEnvelope<T>(res);
  return body?.data;
}

/**
 * Lock-kind argument for acquire/release. Defaults to "edit" (the kind the
 * web UI uses). "sync" is reserved for the desktop client and "office" is
 * managed by the OnlyOffice integration, but they're surfaced here so the
 * UI can display them.
 */
export type LockKind = "edit" | "sync" | "office";

export interface AcquireOptions {
  /** Defaults to "edit". */
  kind?: LockKind;
  /** Seconds. Server caps at 600 (10 min); 0/unset → 60. */
  ttl_seconds?: number;
}

export const locks = {
  /** List all active (non-expired) locks for a file. */
  list: async (fileId: string): Promise<{ locks: FileLock[] }> => {
    const data = await request<{ locks: FileLock[] }>(
      `/api/v2/files/${encodeURIComponent(fileId)}/lock`,
    );
    return data ?? { locks: [] };
  },

  /**
   * Acquire (or refresh, if already held by the caller) a lock.
   * Throws an ApiError with status 409 if another user holds it.
   */
  acquire: async (fileId: string, opts: AcquireOptions = {}): Promise<FileLock> => {
    const body: Record<string, unknown> = {};
    if (opts.kind) body.kind = opts.kind;
    if (typeof opts.ttl_seconds === "number") body.ttl_seconds = opts.ttl_seconds;
    const data = await request<FileLock>(
      `/api/v2/files/${encodeURIComponent(fileId)}/lock`,
      { method: "POST", body: JSON.stringify(body) },
    );
    if (!data) throw new ApiError(0, "empty response from acquire-lock");
    return data;
  },

  /**
   * Release a lock owned by the caller. Idempotent: releasing a lock you
   * don't own (or one that doesn't exist) is a no-op and resolves cleanly.
   */
  release: async (fileId: string, kind: LockKind = "edit"): Promise<void> => {
    await request<void>(
      `/api/v2/files/${encodeURIComponent(fileId)}/lock?kind=${encodeURIComponent(kind)}`,
      { method: "DELETE" },
    );
  },
};
