// WebSocket client for the /api/v2/ws endpoint.
//
// One connection per browser tab. The provider in components/ws-provider.tsx
// opens the connection when the user is authenticated and tears it down on
// logout. Code that wants live updates calls useTopic(topic, handler) — the
// provider deduplicates subscriptions and replays them on reconnect.
//
// Wire format mirrors backend-go/internal/wsHub:
//   { type, topic, ts, data, error? }
// Server-initiated frames have a type and (usually) a topic. We just dispatch
// to whoever subscribed to that topic and let them call SWR.mutate(key) to
// refresh state — we deliberately don't try to merge `data` into local cache
// because the WS message is a hint, not the canonical state.

import { tokenStore } from "./api";

export type WSEnvelope = {
  type:
    | "auth"
    | "sub"
    | "unsub"
    | "event"
    | "ping"
    | "pong"
    | "error"
    | "resync"
    // Server pushes this on session revocation so revoked devices can
    // boot themselves out immediately instead of waiting for the next 401.
    | "session_revoked"
    // QR device linking: a phone scanned the QR (awaiting the browser's
    // approval), and a device finished linking (tokens delivered). Both are
    // published on the user's own topic so the browser updates the QR panel
    // live without polling.
    | "device_link_scanned"
    | "device_link_consumed";
  topic?: string;
  ts?: number;
  data?: unknown;
  error?: string;
};

type Listener = (env: WSEnvelope) => void;

// Backoff schedule for reconnects (ms). Exponential with a cap so a server
// outage doesn't hammer the upstream once it returns.
const RECONNECT_DELAYS = [500, 1000, 2000, 4000, 8000, 16000, 30000];

export class MyCloudWS {
  private url: string;
  private socket: WebSocket | null = null;
  private listeners = new Map<string, Set<Listener>>();
  private subscribed = new Set<string>(); // topics we've asked the server for
  private authed = false;
  private attempt = 0;
  private closed = false;
  // Buffer outbound frames sent before auth completes so the call site
  // doesn't need to wait on the auth round-trip.
  private pending: WSEnvelope[] = [];

  constructor(baseUrl: string = "") {
    // Derive ws(s):// from the API base URL. If baseUrl is "" (same-origin
    // via nginx) we use the current location protocol.
    if (baseUrl.startsWith("http")) {
      this.url = baseUrl.replace(/^http/, "ws") + "/api/v2/ws";
    } else if (typeof window !== "undefined") {
      const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
      this.url = `${proto}//${window.location.host}/api/v2/ws`;
    } else {
      this.url = "";
    }
  }

  connect() {
    if (this.closed || typeof window === "undefined" || !this.url) return;
    if (this.socket && (this.socket.readyState === WebSocket.OPEN || this.socket.readyState === WebSocket.CONNECTING)) {
      return;
    }
    const sock = new WebSocket(this.url);
    this.socket = sock;

    sock.onopen = () => {
      // Authenticate before anything else. Server rejects subs until this
      // succeeds; we hold queued frames until then.
      const token = tokenStore.getAccess();
      if (token) {
        sock.send(JSON.stringify({ type: "auth", data: token }));
      }
    };

    sock.onmessage = (ev) => {
      let env: WSEnvelope;
      try {
        env = JSON.parse(ev.data);
      } catch {
        return;
      }
      if (env.type === "auth" && env.data && typeof env.data === "object") {
        this.authed = true;
        this.attempt = 0;
        // Replay subscriptions and flush pending outbound frames now that
        // the server will accept them.
        for (const topic of this.subscribed) {
          sock.send(JSON.stringify({ type: "sub", topic }));
        }
        for (const frame of this.pending) sock.send(JSON.stringify(frame));
        this.pending = [];
        return;
      }
      if (env.type === "error") {
        // Errors are informational; the connection survives them. We surface
        // them to topic listeners that asked for the affected topic.
        if (env.topic) this.fanout(env.topic, env);
        return;
      }
      if (env.topic) this.fanout(env.topic, env);
    };

    sock.onclose = () => {
      this.authed = false;
      this.socket = null;
      if (this.closed) return;
      const delay = RECONNECT_DELAYS[Math.min(this.attempt, RECONNECT_DELAYS.length - 1)];
      this.attempt += 1;
      setTimeout(() => this.connect(), delay);
    };

    sock.onerror = () => {
      // Let onclose handle the reconnect logic — onerror fires before close.
    };
  }

  close() {
    this.closed = true;
    if (this.socket) {
      this.socket.close();
      this.socket = null;
    }
    this.subscribed.clear();
    this.listeners.clear();
    this.pending = [];
  }

  subscribe(topic: string, fn: Listener): () => void {
    let set = this.listeners.get(topic);
    if (!set) {
      set = new Set();
      this.listeners.set(topic, set);
    }
    set.add(fn);
    // First listener on this topic — ask the server for it.
    if (set.size === 1) {
      this.subscribed.add(topic);
      this.sendOrQueue({ type: "sub", topic });
    }
    return () => {
      const cur = this.listeners.get(topic);
      if (!cur) return;
      cur.delete(fn);
      if (cur.size === 0) {
        this.listeners.delete(topic);
        this.subscribed.delete(topic);
        this.sendOrQueue({ type: "unsub", topic });
      }
    };
  }

  private sendOrQueue(frame: WSEnvelope) {
    if (this.socket && this.socket.readyState === WebSocket.OPEN && this.authed) {
      this.socket.send(JSON.stringify(frame));
    } else {
      this.pending.push(frame);
    }
  }

  private fanout(topic: string, env: WSEnvelope) {
    const set = this.listeners.get(topic);
    if (!set) return;
    for (const fn of set) {
      try {
        fn(env);
      } catch (e) {
        // Don't let one listener's throw take down the others.
        console.error("ws listener", e);
      }
    }
  }
}
