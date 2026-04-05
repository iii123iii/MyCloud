"use client";

import { useEffect, useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import { auth, tokenStore } from "@/lib/api";
import type { User } from "@/lib/types";

interface AuthState {
  user: User | null;
  loading: boolean;
}

export function useAuth(): AuthState & { logout: () => Promise<void> } {
  const [state, setState] = useState<AuthState>({ user: null, loading: true });
  const router = useRouter();

  useEffect(() => {
    const token = tokenStore.getAccess();
    if (!token) {
      setState({ user: null, loading: false });
      return;
    }
    auth.me()
      .then((user) => setState({ user, loading: false }))
      .catch(() => {
        tokenStore.clear();
        setState({ user: null, loading: false });
      });
  }, []);

  const logout = useCallback(async () => {
    await auth.logout();
    router.push("/login");
  }, [router]);

  return { ...state, logout };
}
