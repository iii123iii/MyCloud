"use client";

// List and revoke per-device sessions.
//
// The backend now records a row in the `sessions` table on every successful
// login + refresh, keyed by the access-token JTI. Logging out (or clicking
// "Sign out everywhere") writes `bl:<jti>` to Redis so the token stops
// working immediately even though it hasn't reached its natural expiry.

import { useState, useMemo } from "react";
import useSWR from "swr";
import { toast } from "sonner";
import {
  Smartphone, ShieldOff, Loader2, CheckCircle2, Globe, Monitor,
  Laptop, Tablet, Inbox, AlertCircle,
} from "lucide-react";
import { AlertDialog as AlertDialogPrimitive } from "@base-ui/react/alert-dialog";

import { sessions as sessionsApi, type Session } from "@/lib/api";
import { Button } from "@/components/ui/button";
import {
  Card, CardContent, CardDescription, CardHeader, CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { formatRelative, formatServerDateTime } from "@/lib/format";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// User-agent parsing helpers. The backend stores `user_agent` verbatim, so we
// do best-effort classification client-side. Order matters: Edge advertises
// itself as Chrome, Chrome advertises itself as Safari, etc.

type BrowserKind = "Edge" | "Chrome" | "Firefox" | "Safari" | "Other";
type DeviceKind  = "Mobile" | "Tablet" | "Desktop" | "Unknown";

function detectBrowser(ua: string): BrowserKind {
  const u = ua.toLowerCase();
  if (u.includes("edg/") || u.includes("edge/")) return "Edge";
  if (u.includes("firefox/") || u.includes("fxios")) return "Firefox";
  if (u.includes("chrome/") || u.includes("crios")) return "Chrome";
  if (u.includes("safari/")) return "Safari";
  return "Other";
}

function detectDevice(ua: string): DeviceKind {
  if (!ua) return "Unknown";
  const u = ua.toLowerCase();
  if (u.includes("ipad") || u.includes("tablet")) return "Tablet";
  if (u.includes("mobile") || u.includes("iphone") || u.includes("android")) return "Mobile";
  if (u.includes("mozilla") || u.includes("windows") || u.includes("macintosh") || u.includes("linux")) return "Desktop";
  return "Unknown";
}

// lucide-react has no per-browser icons, so we map every browser kind to a
// generic device-shaped icon. Mobile devices win over browser icon since the
// form factor is more informative.
function BrowserIcon({ ua, className }: { ua: string; className?: string }) {
  const device = detectDevice(ua);
  if (device === "Mobile") return <Smartphone className={className} />;
  if (device === "Tablet") return <Tablet className={className} />;
  const browser = detectBrowser(ua);
  if (browser === "Other") return <Globe className={className} />;
  // Desktop browsers — Monitor for Edge/Chrome (Chromium feel), Laptop for
  // Firefox/Safari just to provide a tiny bit of visual variety in the list.
  if (browser === "Edge" || browser === "Chrome") return <Monitor className={className} />;
  return <Laptop className={className} />;
}

// ---------------------------------------------------------------------------
// Confirm dialog (mirrors trash/page.tsx so the look is consistent).

function ConfirmDialog({
  trigger,
  title,
  description,
  confirmLabel,
  onConfirm,
}: {
  trigger: React.ReactNode;
  title: string;
  description: React.ReactNode;
  confirmLabel: string;
  onConfirm: () => void | Promise<void>;
}) {
  return (
    <AlertDialogPrimitive.Root>
      <AlertDialogPrimitive.Trigger render={trigger as React.ReactElement} />
      <AlertDialogPrimitive.Portal>
        <AlertDialogPrimitive.Backdrop
          className="fixed inset-0 isolate z-50 bg-black/10 duration-100 supports-backdrop-filter:backdrop-blur-xs data-open:animate-in data-open:fade-in-0 data-closed:animate-out data-closed:fade-out-0"
        />
        <AlertDialogPrimitive.Popup
          className={cn(
            "fixed top-1/2 left-1/2 z-50 grid w-full max-w-[calc(100%-2rem)] -translate-x-1/2 -translate-y-1/2 gap-4 rounded-xl bg-popover p-4 text-sm text-popover-foreground ring-1 ring-foreground/10 duration-100 outline-none sm:max-w-sm",
            "data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95",
          )}
        >
          <div className="flex flex-col gap-2 pr-2">
            <AlertDialogPrimitive.Title className="font-heading text-base leading-none font-medium">
              {title}
            </AlertDialogPrimitive.Title>
            <AlertDialogPrimitive.Description className="text-sm text-muted-foreground">
              {description}
            </AlertDialogPrimitive.Description>
          </div>
          <div className="-mx-4 -mb-4 flex flex-col-reverse gap-2 rounded-b-xl border-t bg-muted/50 p-4 sm:flex-row sm:justify-end">
            <AlertDialogPrimitive.Close render={<Button variant="outline" />}>
              Cancel
            </AlertDialogPrimitive.Close>
            <AlertDialogPrimitive.Close
              render={<Button variant="destructive" />}
              onClick={() => void onConfirm()}
            >
              {confirmLabel}
            </AlertDialogPrimitive.Close>
          </div>
        </AlertDialogPrimitive.Popup>
      </AlertDialogPrimitive.Portal>
    </AlertDialogPrimitive.Root>
  );
}

// ---------------------------------------------------------------------------

// Exported as a named component so the parent settings page can embed it
// as a tab — keeps the surrounding secondary nav visible while still letting
// /settings/sessions work as a direct URL.
export function SessionsView() {
  const { data, error, isLoading, mutate } = useSWR("sessions", () =>
    sessionsApi.list().then((r) => r.sessions ?? []),
  );
  const [revoking, setRevoking] = useState<string | null>(null);
  // Polite live-region announcement so screen-reader users hear when a
  // session disappears from the list. Cleared after each settle so repeated
  // revokes of the same label still re-announce.
  const [liveMessage, setLiveMessage] = useState("");

  const revoke = async (s: Session) => {
    setRevoking(s.jti);
    try {
      await sessionsApi.revoke(s.jti);
      toast.success("Device signed out");
      const label = s.device_label || "Unknown device";
      setLiveMessage(`${label} signed out.`);
      mutate();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to revoke";
      toast.error(msg);
      setLiveMessage(`Failed to revoke session: ${msg}`);
    } finally {
      setRevoking(null);
    }
  };

  // Group by device kind, surfacing the current device's group first so the
  // user always sees "you" before the rest.
  const grouped = useMemo(() => {
    if (!data) return [] as { kind: DeviceKind; items: Session[] }[];
    const buckets: Record<DeviceKind, Session[]> = {
      Desktop: [], Mobile: [], Tablet: [], Unknown: [],
    };
    for (const s of data) buckets[detectDevice(s.user_agent)].push(s);
    const order: DeviceKind[] = ["Desktop", "Mobile", "Tablet", "Unknown"];
    // Promote the bucket containing the current session to the top.
    const currentKind = data.find((s) => s.is_current);
    if (currentKind) {
      const k = detectDevice(currentKind.user_agent);
      order.sort((a, b) => (a === k ? -1 : b === k ? 1 : 0));
    }
    return order
      .map((kind) => ({ kind, items: buckets[kind] }))
      .filter((g) => g.items.length > 0);
  }, [data]);

  const hasSessions = !!data && data.length > 0;

  return (
    <div className="container max-w-4xl mx-auto py-8 px-4 space-y-6">
      {/* aria-live announcer for revoke success/failure. Visually hidden. */}
      <div role="status" aria-live="polite" aria-atomic="true" className="sr-only">
        {liveMessage}
      </div>

      <header className="space-y-1.5">
        <h1 className="text-2xl font-bold tracking-tight flex items-center gap-2.5">
          <Smartphone className="size-6 shrink-0" aria-hidden="true" />
          <span>Active sessions</span>
        </h1>
        <p className="text-sm text-muted-foreground leading-relaxed">
          Each row is a browser or app that successfully logged in. Revoking
          a session signs that device out within seconds.
        </p>
      </header>

      <Card>
        <CardHeader>
          <CardTitle>Devices</CardTitle>
          <CardDescription>
            If you don&apos;t recognise one of these, revoke it and change your password.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {isLoading && (
            <div
              className="space-y-3"
              role="status"
              aria-live="polite"
              aria-busy="true"
            >
              <span className="sr-only">Loading active sessions…</span>
              {[0, 1, 2].map((i) => (
                <div
                  key={i}
                  className="flex items-center gap-3 rounded-md border bg-card p-3 sm:p-4"
                >
                  <Skeleton className="size-9 shrink-0 rounded-md" />
                  <div className="flex-1 space-y-2">
                    <div className="flex items-center gap-2">
                      <Skeleton className="h-4 w-40" />
                      <Skeleton className="h-4 w-14 rounded-full" />
                    </div>
                    <Skeleton className="h-3 w-2/3" />
                  </div>
                  <Skeleton className="hidden sm:block h-8 w-24 rounded-lg" />
                </div>
              ))}
            </div>
          )}
          {error && !isLoading && (
            <Alert variant="destructive" role="alert">
              <AlertCircle aria-hidden="true" />
              <AlertTitle>Couldn&apos;t load your sessions</AlertTitle>
              <AlertDescription className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                <span>
                  Something went wrong fetching the list. Please try again.
                </span>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => void mutate()}
                  className="self-start sm:self-auto"
                >
                  Retry
                </Button>
              </AlertDescription>
            </Alert>
          )}
          {data && data.length === 0 && !error && (
            <div className="flex flex-col items-center justify-center gap-2 py-10 text-center">
              <div className="rounded-full bg-muted p-3">
                <Inbox className="size-6 text-muted-foreground" aria-hidden="true" />
              </div>
              <p className="text-sm font-medium">Nothing logged in yet</p>
              <p className="text-xs text-muted-foreground max-w-xs leading-relaxed">
                Log in from another device and it&apos;ll show up here so you can review or revoke it.
              </p>
            </div>
          )}
          {hasSessions && (
            <div className="space-y-6">
              {grouped.map((group) => (
                <section key={group.kind} className="space-y-2.5">
                  <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground flex items-center gap-2">
                    <span>{group.kind}</span>
                    <Badge variant="secondary" className="font-normal tabular-nums">
                      {group.items.length}
                    </Badge>
                  </h3>
                  <ul className="divide-y rounded-md border bg-card overflow-hidden">
                    {group.items.map((s) => {
                      const browser = detectBrowser(s.user_agent);
                      const browserLabel = browser === "Other" ? "Browser" : browser;
                      const isRevoking = revoking === s.jti;
                      const deviceLabel = s.device_label || "Unknown device";
                      const lastSeenRel = formatRelative(s.last_seen_at);
                      const lastSeenAbs = formatServerDateTime(s.last_seen_at);
                      const expiresRel = s.expires_at ? formatRelative(s.expires_at) : null;
                      const expiresAbs = s.expires_at ? formatServerDateTime(s.expires_at) : null;
                      return (
                        <li
                          key={s.jti}
                          className={cn(
                            "p-3 sm:p-4 flex flex-col sm:flex-row sm:items-center gap-3",
                            "transition-[background-color,opacity] duration-200 ease-out",
                            "hover:bg-muted/40 focus-within:bg-muted/40",
                            s.is_current && "bg-primary/5 hover:bg-primary/10",
                            isRevoking && "opacity-60",
                          )}
                        >
                          <div className="flex items-start gap-3 flex-1 min-w-0">
                            <div
                              className={cn(
                                "shrink-0 rounded-md border bg-background p-2 relative transition-colors",
                                s.is_current && "border-primary/40",
                              )}
                            >
                              <BrowserIcon
                                ua={s.user_agent}
                                className={cn(
                                  "size-5",
                                  s.is_current ? "text-primary" : "text-muted-foreground",
                                )}
                              />
                              {s.is_current && (
                                // Subtle pulse so the eye lands on "you" first.
                                <span
                                  className="absolute -top-0.5 -right-0.5 flex size-2.5"
                                  aria-hidden="true"
                                >
                                  <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-primary opacity-60" />
                                  <span className="relative inline-flex size-2.5 rounded-full bg-primary" />
                                </span>
                              )}
                            </div>
                            <div className="flex-1 min-w-0 space-y-1">
                              <div className="flex flex-wrap items-center gap-x-2 gap-y-1 font-medium leading-tight">
                                <span className="truncate">{deviceLabel}</span>
                                <Badge
                                  variant="outline"
                                  className="font-normal text-[10px] leading-none"
                                >
                                  {browserLabel}
                                </Badge>
                                {s.is_current && (
                                  <span className="inline-flex items-center gap-1 rounded-full bg-primary/10 text-primary px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide leading-none">
                                    <CheckCircle2 className="size-3 shrink-0" aria-hidden="true" />
                                    <span>This device</span>
                                  </span>
                                )}
                                {s.is_current && (
                                  <span className="inline-flex items-center gap-1 text-[10px] font-medium text-primary leading-none">
                                    <span
                                      className="relative flex size-1.5"
                                      aria-hidden="true"
                                    >
                                      <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-primary opacity-75" />
                                      <span className="relative inline-flex size-1.5 rounded-full bg-primary" />
                                    </span>
                                    <span>Active now</span>
                                  </span>
                                )}
                              </div>
                              <div className="text-xs text-muted-foreground flex flex-wrap items-center gap-x-2 gap-y-0.5">
                                {s.ip_address && (
                                  <span className="tabular-nums">
                                    <span className="sr-only">IP address </span>
                                    <span aria-hidden="true">IP </span>
                                    {s.ip_address}
                                  </span>
                                )}
                                {s.ip_address && (
                                  <span aria-hidden="true" className="opacity-50">·</span>
                                )}
                                <span title={lastSeenAbs}>
                                  <span aria-hidden="true">Last seen {lastSeenRel}</span>
                                  <span className="sr-only">
                                    Last seen {lastSeenRel} (on {lastSeenAbs})
                                  </span>
                                </span>
                                {expiresRel && (
                                  <>
                                    <span aria-hidden="true" className="opacity-50">·</span>
                                    <span title={expiresAbs ?? undefined}>
                                      <span aria-hidden="true">Expires {expiresRel}</span>
                                      <span className="sr-only">
                                        Expires {expiresRel}
                                        {expiresAbs ? ` (on ${expiresAbs})` : ""}
                                      </span>
                                    </span>
                                  </>
                                )}
                              </div>
                            </div>
                          </div>
                          <div className="sm:ml-2 self-stretch sm:self-center flex">
                            {s.is_current ? (
                              // Disable Revoke on the current device — the backend
                              // refuses it anyway (400 self_revoke). The user should
                              // Sign out from the sidebar to end this session.
                              <Button
                                variant="outline"
                                size="sm"
                                aria-disabled="true"
                                disabled
                                className="w-full sm:w-auto cursor-not-allowed"
                                title="Use Sign out to end this session"
                                aria-label={`Cannot revoke ${deviceLabel}; this is your current device. Use Sign out to end it.`}
                              >
                                <ShieldOff className="size-4 mr-1" aria-hidden="true" />
                                Sign out to end
                              </Button>
                            ) : (
                              <ConfirmDialog
                                trigger={
                                  <Button
                                    variant="outline"
                                    size="sm"
                                    disabled={isRevoking}
                                    aria-label={
                                      isRevoking
                                        ? `Revoking ${deviceLabel}…`
                                        : `Revoke session for ${deviceLabel}${s.ip_address ? ` at IP ${s.ip_address}` : ""}`
                                    }
                                    aria-busy={isRevoking || undefined}
                                    className={cn(
                                      "w-full sm:w-auto transition-transform",
                                      "focus-visible:ring-3 focus-visible:ring-ring/50",
                                      "hover:border-destructive/40 hover:text-destructive",
                                    )}
                                  >
                                    {isRevoking ? (
                                      <Loader2
                                        className="size-4 animate-spin mr-1"
                                        aria-hidden="true"
                                      />
                                    ) : (
                                      <ShieldOff
                                        className="size-4 mr-1"
                                        aria-hidden="true"
                                      />
                                    )}
                                    {isRevoking ? "Revoking…" : "Revoke"}
                                  </Button>
                                }
                                title="Revoke this session?"
                                description={
                                  <>
                                    <strong>{deviceLabel}</strong>
                                    {s.ip_address && <> at IP {s.ip_address}</>} will be signed
                                    out within a few seconds. The user will need to log in
                                    again to regain access.
                                  </>
                                }
                                confirmLabel="Revoke session"
                                onConfirm={() => revoke(s)}
                              />
                            )}
                          </div>
                        </li>
                      );
                    })}
                  </ul>
                </section>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/*
        "Sign out everywhere" — intentionally not rendered. The backend exposes
        only DELETE /api/v2/me/sessions/:jti (sessionsApi.revoke). A bulk
        endpoint (e.g. DELETE /api/v2/me/sessions) would be needed to implement
        this cleanly without firing N requests; revisit once that ships.
      */}
    </div>
  );
}

// Default export keeps /settings/sessions usable as a direct URL.
export default function SessionsPage() {
  return <SessionsView />;
}
