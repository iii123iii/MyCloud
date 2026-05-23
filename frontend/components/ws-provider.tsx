"use client";

// WebSocketProvider mounts at the protected app shell layout and keeps a
// single MyCloudWS instance alive for the session. Components use the
// `useTopic(topic, handler)` hook to subscribe declaratively — the hook
// handles cleanup and resubscription on remount.
//
// We open the connection lazily: only after a successful token check so
// anonymous landing pages don't fail with a flapping connection.
//
// Immediate-revoke: the provider also subscribes (silently) to the
// caller's own `user:<id>` topic and listens for `session_revoked` events.
// When an event carries our locally-stored access JTI, we clear tokens and
// hard-navigate to /login — no waiting for the next 401.

import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import useSWR from "swr";
import { toast } from "sonner";

import { MyCloudWS, type WSEnvelope } from "@/lib/ws";
import { tokenStore, auth as authApi } from "@/lib/api";

type WSContextValue = {
  subscribe: (topic: string, fn: (env: WSEnvelope) => void) => () => void;
};

// Default context value is a no-op rather than null — guarantees any
// consumer reading from context BEFORE the provider mounts gets a callable
// `.subscribe` and doesn't crash with "Cannot read properties of undefined".
const NOOP_CONTEXT: WSContextValue = {
  subscribe: () => () => {},
};

const WSContext = createContext<WSContextValue>(NOOP_CONTEXT);

export function WebSocketProvider({ children }: { children: ReactNode }) {
  // Hold the WS client in state, not a ref-mutated-during-render. State
  // means setWs(...) triggers a re-render that propagates a fresh context
  // value to consumers — until then, subscribers get NOOP_CONTEXT and
  // safely no-op rather than reaching for an undefined client.
  const [ws, setWs] = useState<MyCloudWS | null>(null);

  useEffect(() => {
    if (typeof window === "undefined") return;
    const instance = new MyCloudWS(process.env.NEXT_PUBLIC_API_URL ?? "");
    // Publishing the WS client to context via state is intentional — refs
    // wouldn't notify consumers. The React 19 lint rule flags this, but the
    // alternative (lazy useState initializer) would call `new MyCloudWS`
    // during SSR.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setWs(instance);
    // Only connect when we already have a token — the layout's auth guard
    // redirects to /login otherwise, so a successful render means we're
    // authenticated.
    if (tokenStore.getAccess()) {
      instance.connect();
    }
    return () => {
      instance.close();
      setWs(null);
    };
  }, []);

  // Re-memoise whenever ws changes so consumers see a fresh value that
  // closes over the live instance. The subscribe fn guards against ws
  // being null (initial render) by returning a no-op unsubscribe.
  const value = useMemo<WSContextValue>(
    () => ({
      subscribe: (topic, fn) => {
        if (!ws) return () => {};
        return ws.subscribe(topic, fn);
      },
    }),
    [ws],
  );

  return (
    <WSContext.Provider value={value}>
      <RevokeWatcher />
      {children}
    </WSContext.Provider>
  );
}

// RevokeWatcher subscribes to the caller's user:<id> topic and watches for
// session_revoked envelopes carrying our locally-stored access JTI. On
// match, it clears tokens and hard-redirects to /login so the page can't
// linger displaying stale data. SWR's /me fetch establishes our user id.
function RevokeWatcher() {
  const { data: me } = useSWR("me", authApi.me, { shouldRetryOnError: false });
  const userId = me?.id;
  useTopic(
    userId ? `user:${userId}` : null,
    (env) => {
      if (env.type !== "session_revoked") return;
      const payload = env.data as { jti?: string } | undefined;
      const revokedJTI = payload?.jti;
      const ownJTI = tokenStore.getAccessJTI();
      if (!revokedJTI || !ownJTI) return;
      if (revokedJTI === ownJTI) {
        toast.error("Your session was revoked from another device.", { duration: 5000 });
        tokenStore.clear();
        // Hard navigation so cached React state can't leak — we genuinely
        // want every in-flight SWR / WS resource thrown away.
        if (typeof window !== "undefined") {
          const here = window.location.pathname + window.location.search;
          const next =
            here && here !== "/login" ? `?next=${encodeURIComponent(here)}` : "";
          window.location.replace(`/login${next}`);
        }
      }
    },
  );
  return null;
}

// useTopic subscribes to a WS topic for the lifetime of the calling
// component. The handler should be a stable reference (memoised or
// declared outside render) — passing a new function every render won't
// cause re-subscribes (the subscription persists), but will reattach the
// listener which is wasteful.
export function useTopic(topic: string | null, handler: (env: WSEnvelope) => void) {
  const ctx = useContext(WSContext);
  // Standard "latest handler" ref pattern: stash the freshest handler so the
  // long-lived subscription always invokes the most recent closure without
  // forcing a re-subscribe. React 19's react-hooks/refs rule disallows ref
  // writes during render in general, but this idiom is documented and safe.
  const handlerRef = useRef(handler);
  // eslint-disable-next-line react-hooks/refs
  handlerRef.current = handler;

  useEffect(() => {
    // Belt-and-braces — the default context is a no-op object, but
    // someone could theoretically pass an alternate provider with a
    // missing subscribe. Bail safely instead of crashing.
    if (!topic) return;
    if (!ctx || typeof ctx.subscribe !== "function") return;
    const unsub = ctx.subscribe(topic, (env) => handlerRef.current(env));
    return unsub;
  }, [ctx, topic]);
}
