import { describe, it, expect, beforeEach, vi } from "vitest";
import { locks } from "./locks";
import { tokenStore, ApiError } from "./api";

const flushFetch = (response: Partial<Response>) =>
  vi.spyOn(globalThis, "fetch").mockResolvedValue(response as Response);

describe("locks.list", () => {
  beforeEach(() => {
    tokenStore.clear();
    vi.restoreAllMocks();
  });

  it("returns the locks array from the envelope", async () => {
    flushFetch({
      ok: true,
      status: 200,
      headers: new Headers({ "Content-Length": "100" }),
      json: async () => ({ data: { locks: [{ id: "1", file_id: "f1", user_id: "u", kind: "edit", expires_at: "", created_at: "" }] } }),
    });
    const result = await locks.list("f1");
    expect(result.locks).toHaveLength(1);
  });

  it("returns empty array for 204 No Content", async () => {
    flushFetch({
      ok: true,
      status: 204,
      headers: new Headers({ "Content-Length": "0" }),
      json: async () => null,
    });
    const result = await locks.list("f1");
    expect(result.locks).toEqual([]);
  });

  it("throws ApiError on non-2xx", async () => {
    flushFetch({
      ok: false,
      status: 403,
      statusText: "Forbidden",
      headers: new Headers({ "Content-Length": "20" }),
      json: async () => ({ error: { code: "no_access", message: "denied" } }),
    });
    await expect(locks.list("f1")).rejects.toBeInstanceOf(ApiError);
  });
});

describe("locks.acquire", () => {
  beforeEach(() => {
    tokenStore.clear();
    vi.restoreAllMocks();
  });

  it("posts kind + ttl_seconds when given", async () => {
    let captured: { url?: string; init?: RequestInit } = {};
    vi.spyOn(globalThis, "fetch").mockImplementation((url, init) => {
      captured = { url: url as string, init };
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: new Headers({ "Content-Length": "100" }),
        json: async () => ({
          data: { id: "L1", file_id: "f1", user_id: "u", kind: "edit", expires_at: "", created_at: "" },
        }),
      } as Response);
    });
    await locks.acquire("f1", { kind: "edit", ttl_seconds: 120 });
    const body = JSON.parse(captured.init?.body as string);
    expect(body.kind).toBe("edit");
    expect(body.ttl_seconds).toBe(120);
  });

  it("throws when server returns empty body", async () => {
    flushFetch({
      ok: true,
      status: 200,
      headers: new Headers({ "Content-Length": "0" }),
      json: async () => null,
    });
    await expect(locks.acquire("f1")).rejects.toBeInstanceOf(ApiError);
  });
});

describe("locks.release", () => {
  beforeEach(() => {
    tokenStore.clear();
    vi.restoreAllMocks();
  });

  it("calls DELETE with kind query param", async () => {
    let captured = "";
    vi.spyOn(globalThis, "fetch").mockImplementation((url, init) => {
      captured = url as string;
      expect(init?.method).toBe("DELETE");
      return Promise.resolve({
        ok: true,
        status: 204,
        headers: new Headers({ "Content-Length": "0" }),
        json: async () => null,
      } as Response);
    });
    await locks.release("f1", "office");
    expect(captured).toContain("kind=office");
  });

  it("defaults kind to 'edit' when omitted", async () => {
    let captured = "";
    vi.spyOn(globalThis, "fetch").mockImplementation((url) => {
      captured = url as string;
      return Promise.resolve({
        ok: true,
        status: 204,
        headers: new Headers({ "Content-Length": "0" }),
        json: async () => null,
      } as Response);
    });
    await locks.release("f1");
    expect(captured).toContain("kind=edit");
  });
});
