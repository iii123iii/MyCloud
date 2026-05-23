// Tests for lib/retention. Cover the GET-returns-null-on-404 quirk
// (handler returns 404 + not_found when no policy exists, client maps to
// null so SWR consumers can render an "add policy" empty state),
// plus PUT/DELETE shape and ApiError propagation.

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { tokenStore, ApiError } from "./api";
import { retention } from "./retention";

beforeEach(() => {
  vi.restoreAllMocks();
  tokenStore.clear();
});
afterEach(() => vi.restoreAllMocks());

const ok = (body: unknown) =>
  new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });

describe("retention.get", () => {
  it("returns parsed policy on 200", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      ok({
        data: {
          folder_id: "fld",
          retention_days: 30,
          action: "soft_delete",
          created_by: "u1",
          updated_at: "2026-05-22T10:00:00Z",
        },
      }),
    );
    const got = await retention.get("fld");
    expect(got?.action).toBe("soft_delete");
    expect(got?.retention_days).toBe(30);
  });

  it("maps 404 to null (no policy set)", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({ error: { code: "not_found", message: "x" } }),
        { status: 404, headers: { "Content-Type": "application/json" } },
      ),
    );
    const got = await retention.get("fld");
    expect(got).toBeNull();
  });

  it("propagates non-404 errors as ApiError", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({ error: { code: "internal_error", message: "x" } }),
        { status: 500, headers: { "Content-Type": "application/json" } },
      ),
    );
    await expect(retention.get("fld")).rejects.toMatchObject({ status: 500 });
  });

  it("encodes folderId", async () => {
    const spy = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      ok({ data: { folder_id: "a/b", retention_days: 1, action: "notify", created_by: "u", updated_at: "" } }),
    );
    await retention.get("a/b");
    expect(spy.mock.calls[0][0] as string).toContain("a%2Fb");
  });

  it("forwards Authorization header", async () => {
    tokenStore.set({ access_token: "tok", refresh_token: "r" });
    const spy = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      ok({ data: { folder_id: "fld", retention_days: 1, action: "notify", created_by: "u", updated_at: "" } }),
    );
    await retention.get("fld");
    const headers = spy.mock.calls[0][1]?.headers as Headers;
    expect(headers.get("Authorization")).toBe("Bearer tok");
  });
});

describe("retention.put", () => {
  it("sends PUT with policy body", async () => {
    const spy = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      ok({ data: { folder_id: "fld", retention_days: 60, action: "hard_delete" } }),
    );
    const got = await retention.put("fld", {
      retention_days: 60,
      action: "hard_delete",
    });
    expect(got?.retention_days).toBe(60);
    const init = spy.mock.calls[0][1]!;
    expect(init.method).toBe("PUT");
    const body = JSON.parse(init.body as string);
    expect(body).toEqual({ retention_days: 60, action: "hard_delete" });
  });

  it("ApiError on 400", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({ error: { code: "validation_error", message: "days too low" } }),
        { status: 400, headers: { "Content-Type": "application/json" } },
      ),
    );
    await expect(
      retention.put("fld", { retention_days: 0, action: "notify" }),
    ).rejects.toMatchObject({ status: 400, code: "validation_error" });
  });

  it("supports all three actions", async () => {
    const actions = ["soft_delete", "hard_delete", "notify"] as const;
    for (const action of actions) {
      const spy = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
        ok({ data: { folder_id: "fld", retention_days: 7, action } }),
      );
      const got = await retention.put("fld", { retention_days: 7, action });
      expect(got?.action).toBe(action);
      const body = JSON.parse(spy.mock.calls[0][1]!.body as string);
      expect(body.action).toBe(action);
      vi.restoreAllMocks();
    }
  });
});

describe("retention.remove", () => {
  it("DELETE returns null on 204", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(null, { status: 204 }),
    );
    await expect(retention.remove("fld")).resolves.toBeNull();
  });

  it("ApiError on 404", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({ error: { code: "not_found", message: "x" } }),
        { status: 404, headers: { "Content-Type": "application/json" } },
      ),
    );
    await expect(retention.remove("fld")).rejects.toMatchObject({ status: 404 });
  });
});
