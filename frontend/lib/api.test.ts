// Tests for the central API client. Covers:
//   - tokenStore localStorage round-trips
//   - fetch wrapper: Authorization header, JSON content-type, error envelope
//   - 401 → refresh-token → retry pipeline
//   - 401 refresh failure → tokenStore cleared + redirect
//   - 429 surfaces Retry-After in the ApiError message
//   - fetchAuthed transparently runs the refresh dance for raw fetch
//     consumers (downloads, previews).

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  tokenStore,
  request,
  requestEnvelope,
  fetchAuthed,
  ApiError,
} from "./api";

// Test the API client against a mocked global fetch. The client only
// reaches the network via `fetch`, so vi.spyOn(globalThis, "fetch") gives
// us full control of every request the client makes — including the
// internal refresh call.

beforeEach(() => {
  // Reset the spy chain between tests.
  vi.restoreAllMocks();
  tokenStore.clear();
});

afterEach(() => {
  vi.restoreAllMocks();
});

function jsonResponse(body: unknown, init: ResponseInit = {}): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
    ...init,
  });
}

// ─── tokenStore ────────────────────────────────────────────────────────────

describe("tokenStore", () => {
  it("starts empty", () => {
    expect(tokenStore.getAccess()).toBeNull();
    expect(tokenStore.getRefresh()).toBeNull();
    expect(tokenStore.getAccessJTI()).toBeNull();
  });

  it("set persists access + refresh", () => {
    tokenStore.set({
      access_token: "a",
      refresh_token: "r",
    });
    expect(tokenStore.getAccess()).toBe("a");
    expect(tokenStore.getRefresh()).toBe("r");
  });

  it("set persists access_jti when provided", () => {
    tokenStore.set({
      access_token: "a",
      refresh_token: "r",
      access_jti: "the-jti",
    });
    expect(tokenStore.getAccessJTI()).toBe("the-jti");
  });

  it("set without access_jti clears any prior JTI", () => {
    tokenStore.set({
      access_token: "a",
      refresh_token: "r",
      access_jti: "old",
    });
    tokenStore.set({
      access_token: "b",
      refresh_token: "r2",
      // no access_jti
    });
    expect(tokenStore.getAccessJTI()).toBeNull();
  });

  it("clear wipes everything", () => {
    tokenStore.set({
      access_token: "a",
      refresh_token: "r",
      access_jti: "j",
    });
    tokenStore.clear();
    expect(tokenStore.getAccess()).toBeNull();
    expect(tokenStore.getRefresh()).toBeNull();
    expect(tokenStore.getAccessJTI()).toBeNull();
  });
});

// ─── request / requestEnvelope ─────────────────────────────────────────────

describe("request", () => {
  it("attaches Authorization header from tokenStore", async () => {
    tokenStore.set({ access_token: "abc", refresh_token: "ref" });
    const spy = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      jsonResponse({ data: { ok: true } }),
    );
    await request("/api/v2/files");
    const headers = (spy.mock.calls[0][1]?.headers ?? new Headers()) as Headers;
    expect(headers.get("Authorization")).toBe("Bearer abc");
  });

  it("omits Authorization when noAuth is true", async () => {
    tokenStore.set({ access_token: "abc", refresh_token: "ref" });
    const spy = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      jsonResponse({ data: {} }),
    );
    await request("/api/v2/auth/login", { method: "POST", noAuth: true });
    const headers = (spy.mock.calls[0][1]?.headers ?? new Headers()) as Headers;
    expect(headers.get("Authorization")).toBeNull();
  });

  it("sets X-Share-Password when sharePassword is provided", async () => {
    const spy = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      jsonResponse({ data: {} }),
    );
    await request("/api/v2/public/shares/abc", {
      noAuth: true,
      sharePassword: "secret",
    });
    const headers = (spy.mock.calls[0][1]?.headers ?? new Headers()) as Headers;
    expect(headers.get("X-Share-Password")).toBe("secret");
  });

  it("returns data field unwrapped", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      jsonResponse({ data: { user_id: "42" } }),
    );
    const got = await request<{ user_id: string }>("/api/v2/auth/me");
    expect(got).toEqual({ user_id: "42" });
  });

  it("throws ApiError on 4xx", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          error: { code: "validation_error", message: "bad input" },
        }),
        { status: 400, headers: { "Content-Type": "application/json" } },
      ),
    );
    await expect(request("/api/v2/files")).rejects.toThrow(ApiError);
  });

  it("surfaces error code from envelope", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          error: { code: "quota_exceeded", message: "over the cap" },
        }),
        { status: 413, headers: { "Content-Type": "application/json" } },
      ),
    );
    try {
      await request("/api/v2/files");
      expect.fail("should have thrown");
    } catch (e) {
      expect(e).toBeInstanceOf(ApiError);
      expect((e as ApiError).status).toBe(413);
      expect((e as ApiError).code).toBe("quota_exceeded");
    }
  });

  it("429 includes Retry-After in message", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({ error: { code: "rate_limited", message: "Slow down" } }),
        {
          status: 429,
          headers: {
            "Content-Type": "application/json",
            "Retry-After": "30",
          },
        },
      ),
    );
    try {
      await request("/api/v2/files");
      expect.fail("should have thrown");
    } catch (e) {
      expect((e as ApiError).message).toContain("retry in 30s");
    }
  });

  it("uses Content-Length: 0 → null body for 204", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(null, { status: 204 }),
    );
    const result = await request("/api/v2/files/x", { method: "DELETE" });
    // 204 has no body — request returns undefined (typed as T).
    expect(result).toBeUndefined();
  });
});

// ─── 401 → refresh flow ────────────────────────────────────────────────────

describe("401 refresh", () => {
  it("retries with new token after successful refresh", async () => {
    tokenStore.set({ access_token: "old", refresh_token: "ref" });
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      // 1st call: 401
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ error: { code: "unauthorized", message: "expired" } }),
          { status: 401, headers: { "Content-Type": "application/json" } },
        ),
      )
      // 2nd call: refresh succeeds
      .mockResolvedValueOnce(
        jsonResponse({
          data: { access_token: "new", refresh_token: "ref2", access_jti: "jti2" },
        }),
      )
      // 3rd call: retry of original with new token succeeds
      .mockResolvedValueOnce(jsonResponse({ data: { ok: true } }));

    const got = await request<{ ok: boolean }>("/api/v2/files");
    expect(got).toEqual({ ok: true });
    expect(fetchSpy).toHaveBeenCalledTimes(3);

    // tokenStore was updated with the refreshed tokens.
    expect(tokenStore.getAccess()).toBe("new");
    expect(tokenStore.getRefresh()).toBe("ref2");
    expect(tokenStore.getAccessJTI()).toBe("jti2");

    // The retry used the new Bearer.
    const retryHeaders = (fetchSpy.mock.calls[2][1]?.headers ??
      new Headers()) as Headers;
    expect(retryHeaders.get("Authorization")).toBe("Bearer new");
  });

  it("clears tokens when refresh itself returns 401", async () => {
    tokenStore.set({ access_token: "old", refresh_token: "ref" });
    // Stub window.location so the redirect doesn't actually navigate.
    const originalLocation = window.location;
    Object.defineProperty(window, "location", {
      writable: true,
      value: { ...originalLocation, href: "/dashboard", pathname: "/dashboard", search: "" },
    });

    vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ error: { code: "unauthorized", message: "expired" } }),
          { status: 401, headers: { "Content-Type": "application/json" } },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ error: { code: "refresh_reuse", message: "bad" } }),
          { status: 401, headers: { "Content-Type": "application/json" } },
        ),
      );

    await expect(request("/api/v2/files")).rejects.toThrow(ApiError);
    expect(tokenStore.getAccess()).toBeNull();
    expect(tokenStore.getRefresh()).toBeNull();
  });

  it("does not loop refresh when noAuth", async () => {
    tokenStore.set({ access_token: "old", refresh_token: "ref" });
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({ error: { code: "unauthorized", message: "no" } }),
        { status: 401, headers: { "Content-Type": "application/json" } },
      ),
    );
    await expect(
      request("/api/v2/setup/status", { method: "POST", noAuth: true }),
    ).rejects.toThrow();
    // Only one fetch — no refresh attempt for noAuth requests.
    expect(fetchSpy).toHaveBeenCalledTimes(1);
  });
});

// ─── fetchAuthed ───────────────────────────────────────────────────────────

describe("fetchAuthed", () => {
  it("attaches Authorization and returns raw Response", async () => {
    tokenStore.set({ access_token: "abc", refresh_token: "ref" });
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response("binary data", { status: 200 }),
    );
    const res = await fetchAuthed("/api/v2/files/1:download");
    expect(res.status).toBe(200);
    expect(await res.text()).toBe("binary data");
  });

  it("transparently refreshes on 401", async () => {
    tokenStore.set({ access_token: "old", refresh_token: "ref" });
    const spy = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ error: { code: "unauthorized", message: "x" } }),
          { status: 401, headers: { "Content-Type": "application/json" } },
        ),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          data: { access_token: "fresh", refresh_token: "ref2", access_jti: "j" },
        }),
      )
      .mockResolvedValueOnce(new Response("retried", { status: 200 }));

    const res = await fetchAuthed("/api/v2/files/1:preview");
    expect(res.status).toBe(200);
    expect(await res.text()).toBe("retried");
    expect(spy).toHaveBeenCalledTimes(3);
    expect(tokenStore.getAccess()).toBe("fresh");
  });

  it("returns the 401 unchanged when there's no refresh token", async () => {
    // No refresh in store; cannot retry.
    tokenStore.set({ access_token: "only", refresh_token: "" });
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response("nope", { status: 401 }),
    );
    const res = await fetchAuthed("/api/v2/files/x");
    expect(res.status).toBe(401);
  });
});

// ─── requestEnvelope (returns full {data, meta}) ───────────────────────────

describe("requestEnvelope", () => {
  it("exposes meta alongside data", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      jsonResponse({
        data: { files: [{ id: "f1" }] },
        meta: { total: 100, has_more: true },
      }),
    );
    const env = await requestEnvelope<{ files: { id: string }[] }>(
      "/api/v2/files",
    );
    expect(env?.data?.files?.[0].id).toBe("f1");
    expect(env?.meta?.total).toBe(100);
    expect(env?.meta?.has_more).toBe(true);
  });
});
