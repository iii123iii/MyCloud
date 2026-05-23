import { tokenStore, ApiError } from "@/lib/api";

// Per-folder retention policy API. The backend lives at
// /api/v2/folders/{id}/retention with GET/PUT/DELETE. We keep the wrapper
// small and self-contained because `request<T>` in lib/api.ts is
// module-private — pulling it out into a shared export felt like more
// surface area for a single feature.

const BASE = process.env.NEXT_PUBLIC_API_URL ?? "";

type ApiEnvelope<T> = {
  data?: T;
  meta?: Record<string, unknown>;
  error?: { code: string; message: string; details?: unknown };
};

/**
 * RetentionAction mirrors the three values the Go handler accepts in
 * lower-case form. The handler normalises whatever the client sends but
 * we constrain the type so the dialog can drive a <Select> off it.
 *
 *  - soft_delete: move matching files to trash, restorable from there.
 *  - hard_delete: same trash path, but the maintenance purger will
 *    permanently delete them on its next run.
 *  - notify:      no destructive action; an activity log row is written
 *    each sweep summarising what would have been removed.
 */
export type RetentionAction = "soft_delete" | "hard_delete" | "notify";

/**
 * Shape returned by GET /api/v2/folders/{id}/retention. Fields match the
 * `retentionPolicy` struct in backend-go/internal/app/handlers_retention.go.
 * `updated_at` is an RFC3339 string from Go's `time.Time` JSON encoding.
 */
export interface RetentionPolicy {
  folder_id: string;
  retention_days: number;
  action: RetentionAction;
  created_by: string;
  updated_at: string;
}

/** Body for PUT /api/v2/folders/{id}/retention. */
export interface RetentionPolicyInput {
  retention_days: number;
  action: RetentionAction;
}

async function parseEnvelope<T>(res: Response): Promise<ApiEnvelope<T> | null> {
  if (res.status === 204 || res.headers.get("Content-Length") === "0") return null;
  return res.json().catch(() => null);
}

// Minimal request helper. We deliberately do NOT replicate the refresh-on-401
// dance from lib/api.ts — by the time a user opens this dialog the auth
// flow in api.ts has already kept their access token fresh. If it expires
// mid-request the user will see a single failed call and the next click
// (which goes through api.ts) will refresh.
async function request<T>(
  path: string,
  options: RequestInit = {},
): Promise<T | null> {
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

  const envelope = await parseEnvelope<T>(res);
  return (envelope?.data ?? null) as T | null;
}

export const retention = {
  /**
   * Fetch the policy for a folder. Returns null when none is set
   * (backend responds 404 + not_found). Other errors propagate as
   * ApiError so SWR can surface them.
   */
  get: async (folderId: string): Promise<RetentionPolicy | null> => {
    try {
      return await request<RetentionPolicy>(
        `/api/v2/folders/${encodeURIComponent(folderId)}/retention`,
      );
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) return null;
      throw err;
    }
  },

  /**
   * Upsert a policy. The handler echoes back a partial object on success
   * (no created_by/updated_at), so callers should re-fetch via get() if
   * they need the full record.
   */
  put: (folderId: string, policy: RetentionPolicyInput) =>
    request<{ folder_id: string; retention_days: number; action: RetentionAction }>(
      `/api/v2/folders/${encodeURIComponent(folderId)}/retention`,
      { method: "PUT", body: JSON.stringify(policy) },
    ),

  /** Delete the policy. Backend responds 204 No Content on success. */
  remove: (folderId: string) =>
    request<void>(
      `/api/v2/folders/${encodeURIComponent(folderId)}/retention`,
      { method: "DELETE" },
    ),
};
