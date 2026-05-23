// Tests for useAuth — the SWR-backed hook that surfaces the current user
// plus a logout helper. The hook went through a refactor (audit Stream 5)
// to read from the shared SWR cache; these tests pin that behaviour.

import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { useAuth } from "./useAuth";
import { tokenStore } from "@/lib/api";

// Wrap renderHook with a fresh SWR cache so tests don't leak the "me" key
// across cases. Using `provider: () => new Map()` gives each render its
// own isolated cache.
function withSWR(ui: () => React.ReactElement) {
  return (
    <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
      {ui()}
    </SWRConfig>
  );
}

const wrapper = ({ children }: { children: React.ReactNode }) => (
  <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
    {children}
  </SWRConfig>
);

// Stub next/navigation to capture push() calls.
const pushMock = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock, replace: pushMock }),
}));

beforeEach(() => {
  vi.restoreAllMocks();
  tokenStore.clear();
  pushMock.mockReset();
});
afterEach(() => vi.restoreAllMocks());

describe("useAuth", () => {
  it("no token → loading=false, user=null, no fetch", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch");
    const { result } = renderHook(() => useAuth(), { wrapper });
    expect(result.current.user).toBeNull();
    expect(result.current.loading).toBe(false);
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("with token, resolves to user from /auth/me", async () => {
    tokenStore.set({ access_token: "tok", refresh_token: "r" });
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          data: {
            id: "u-1",
            username: "alice",
            email: "a@b.com",
            role: "user",
            quota_bytes: 0,
            used_bytes: 0,
            is_active: true,
            must_change_password: false,
            created_at: "",
          },
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );
    const { result } = renderHook(() => useAuth(), { wrapper });
    await waitFor(() => {
      expect(result.current.user?.username).toBe("alice");
    });
    expect(result.current.loading).toBe(false);
  });

  it("/auth/me failure clears tokenStore", async () => {
    tokenStore.set({ access_token: "stale", refresh_token: "r" });
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({ error: { code: "unauthorized", message: "x" } }),
        { status: 401, headers: { "Content-Type": "application/json" } },
      ),
    );
    // refresh attempt also fails so /auth/me ultimately propagates error.
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response("{}", { status: 401 }),
    );
    const { result } = renderHook(() => useAuth(), { wrapper });
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    // After the error onError fires and tokenStore.clear() runs.
    expect(tokenStore.getAccess()).toBeNull();
  });

  it("logout() calls auth.logout and navigates to /login", async () => {
    tokenStore.set({ access_token: "tok", refresh_token: "r" });
    vi.spyOn(globalThis, "fetch")
      // 1. /auth/me on mount
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            data: {
              id: "u-1",
              username: "alice",
              email: "a@b.com",
              role: "user",
              quota_bytes: 0,
              used_bytes: 0,
              is_active: true,
              must_change_password: false,
              created_at: "",
            },
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      )
      // 2. POST /auth/logout
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ data: { message: "ok" } }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
    const { result } = renderHook(() => useAuth(), { wrapper });
    await waitFor(() => {
      expect(result.current.user?.username).toBe("alice");
    });

    await result.current.logout();
    expect(tokenStore.getAccess()).toBeNull();
    expect(pushMock).toHaveBeenCalledWith("/login");
  });
});
