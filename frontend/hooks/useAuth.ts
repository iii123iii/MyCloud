"use client";

// Wraps the centralized SWR cache for "/auth/me" so consumers get a single
// source of truth. Previously useAuth() ran its own fetch + setState, in
// parallel with the components that called `useSWR("me", auth.me)` directly,
// producing two independent caches that could drift on revalidation. Now
// every reader goes through the same SWR key and benefits from dedup +
// shared revalidation.

import { useCallback } from "react";
import { useRouter } from "next/navigation";
import useSWR, { useSWRConfig } from "swr";
import { auth, tokenStore } from "@/lib/api";
import type { User } from "@/lib/types";

interface AuthState {
  user: User | null;
  loading: boolean;
}

export function useAuth(): AuthState & { logout: () => Promise<void> } {
  const router = useRouter();
  const { mutate } = useSWRConfig();
  // Suspend the fetch when there's no access token — saves a guaranteed-401
  // request on the public auth pages. The key stays stable across renders
  // so SWR doesn't churn the cache.
  const hasToken = typeof window !== "undefined" && !!tokenStore.getAccess();
  const { data, error, isLoading } = useSWR<User>(hasToken ? "me" : null, auth.me, {
    revalidateOnFocus: false,
    shouldRetryOnError: false,
    onError: () => {
      tokenStore.clear();
    },
  });

  const logout = useCallback(async () => {
    await auth.logout();
    // Clear the SWR cache for "me" so any other consumer rerenders to the
    // unauth shape instead of holding the stale user object.
    await mutate("me", undefined, { revalidate: false });
    router.push("/login");
  }, [router, mutate]);

  return {
    user: data ?? null,
    loading: isLoading || (hasToken && !data && !error),
    logout,
  };
}
