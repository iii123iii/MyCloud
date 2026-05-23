// Tests for lib/locks. The lock API is its own minimal client (separate
// from lib/api.ts to keep that surface narrow) — these tests cover the
// happy path, 409 conflict on a foreign-held lock, empty-body edge,
// idempotent release, and URL encoding of file IDs.

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { tokenStore, ApiError } from "./api";
import { locks } from "./locks";

beforeEach(() => {
  vi.restoreAllMocks();
  tokenStore.clear();
});
afterEach(() => vi.restoreAllMocks());

function jsonResponse(body: unknown, init: ResponseInit = {}) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
    ...init,
  });
}

describe("locks.acquire", () => {
  it("POSTs with kind + ttl_seconds in body", async () => {
    tokenStore.set({ access_token: "tok", refresh_token: "r" });
    const spy = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      jsonResponse({
        data: {
          id: "L1", file_id: "f1", user_id: "u1",
          kind: "edit", expires_at: "x", created_at: "y",
        },
      }),
    );
    const got = await locks.acquire("f1", { kind: "edit", ttl_seconds: 120 });
    expect(got.id).toBe("L1");
    const init = spy.mock.calls[0][1]!;
    expect(init.method).toBe("POST");
    const body = JSON.parse(init.body as string);
    expect(body).toEqual({ kind: "edit", ttl_seconds: 120 });
    expect((init.headers as Headers).get("Authorization")).toBe("Bearer tok");
  });

  it("rejects with ApiError on 409", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({ error: { code: "lock_held", message: "held by alice" } }),
        { status: 409, headers: { "Content-Type": "application/json" } },
      ),
    );
    await expect(locks.acquire("f1")).rejects.toMatchObject({
      status: 409,
      code: "lock_held",
    });
  });

  it("rejects with empty-body ApiError on null envelope", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(null, { status: 200, headers: { "Content-Length": "0" } }),
    );
    await expect(locks.acquire("f1")).rejects.toThrow(ApiError);
  });

  it("URL-encodes the fileId", async () => {
    const spy = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      jsonResponse({ data: { id: "L1", file_id: "x", user_id: "u", kind: "edit", expires_at: "", created_at: "" } }),
    );
    await locks.acquire("a/b c");
    const url = spy.mock.calls[0][0] as string;
    expect(url).toContain("a%2Fb%20c");
  });

  it("omits empty options from body", async () => {
    const spy = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      jsonResponse({ data: { id: "L1", file_id: "x", user_id: "u", kind: "edit", expires_at: "", created_at: "" } }),
    );
    await locks.acquire("f1");
    const body = JSON.parse(spy.mock.calls[0][1]!.body as string);
    expect(body).toEqual({});
  });
});

describe("locks.list", () => {
  it("defaults to empty list on 204", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(null, { status: 204 }),
    );
    const got = await locks.list("f1");
    expect(got).toEqual({ locks: [] });
  });

  it("encodes fileId", async () => {
    const spy = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      jsonResponse({ data: { locks: [] } }),
    );
    await locks.list("a/b c");
    expect(spy.mock.calls[0][0] as string).toContain("a%2Fb%20c");
  });

  it("returns the locks array", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      jsonResponse({
        data: {
          locks: [
            { id: "L1", file_id: "f1", user_id: "u1", kind: "edit", expires_at: "x", created_at: "y" },
          ],
        },
      }),
    );
    const got = await locks.list("f1");
    expect(got.locks).toHaveLength(1);
    expect(got.locks[0].id).toBe("L1");
  });
});

describe("locks.release", () => {
  it("DELETEs with default kind=edit", async () => {
    const spy = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(null, { status: 204 }),
    );
    await locks.release("f1");
    const url = spy.mock.calls[0][0] as string;
    expect(url).toContain("kind=edit");
    expect(spy.mock.calls[0][1]?.method).toBe("DELETE");
  });

  it("idempotent — resolves silently on empty body", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response("", { status: 200 }),
    );
    await expect(locks.release("f1")).resolves.toBeUndefined();
  });

  it("passes custom kind", async () => {
    const spy = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(null, { status: 204 }),
    );
    await locks.release("f1", "sync");
    expect(spy.mock.calls[0][0]).toContain("kind=sync");
  });

  it("rejects with ApiError on 401", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({ error: { code: "unauthorized", message: "x" } }),
        { status: 401, headers: { "Content-Type": "application/json" } },
      ),
    );
    await expect(locks.release("f1")).rejects.toMatchObject({ status: 401 });
  });
});

describe("locks (no token)", () => {
  it("omits Authorization header when tokenStore is empty", async () => {
    const spy = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      jsonResponse({ data: { locks: [] } }),
    );
    await locks.list("f1");
    const headers = spy.mock.calls[0][1]?.headers as Headers;
    expect(headers.get("Authorization")).toBeNull();
  });
});
