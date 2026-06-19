"use client";

// Link a new device (the phone) by showing a QR the phone scans.
//
// Flow: this already-authenticated browser mints a short-lived pairing
// (deviceLink.create) and renders the QR. The phone scans it and claims the
// code; the backend pushes `device_link_scanned` on our user topic, so we swap
// the QR for an approval card showing the phone's model + IP. On Approve the
// backend mints tokens; the phone collects them and the backend pushes
// `device_link_consumed`, which flips us to the success state.
//
// The verifier in the QR is the secret that authorises token retrieval — it is
// shown only inside the QR image, never as copyable text.

import { useCallback, useEffect, useState } from "react";
import { QRCodeSVG } from "qrcode.react";
import useSWR from "swr";
import { toast } from "sonner";
import {
  QrCode, Smartphone, Loader2, CheckCircle2, ShieldCheck, ShieldX,
  RefreshCw, AlertCircle, MonitorSmartphone,
} from "lucide-react";

import {
  deviceLink, auth as authApi,
  type DeviceLinkTicket, type DeviceLinkStatus,
} from "@/lib/api";
import { useTopic } from "@/components/ws-provider";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Card, CardContent, CardDescription, CardHeader, CardTitle,
} from "@/components/ui/card";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { cn } from "@/lib/utils";

type Phase =
  | "idle"          // no active QR
  | "waiting"       // QR shown, waiting for a scan
  | "scanned"       // phone scanned, awaiting our approval
  | "approving"     // we approved, waiting for the phone to collect tokens
  | "linked"        // done
  | "denied"        // we denied (or the phone was denied)
  | "expired";      // the code timed out

interface ScannedInfo {
  device_name?: string;
  device_model?: string;
  device_ip?: string;
  platform?: string;
}

// The payload encoded into the QR. The phone parses this JSON: `url` tells it
// which server to talk to (auto-config), `code`+`verifier` authorise the claim.
function qrPayload(ticket: DeviceLinkTicket, phoneUrl: string): string {
  const url =
    phoneUrl.trim() ||
    ticket.url ||
    process.env.NEXT_PUBLIC_API_URL ||
    (typeof window !== "undefined" ? window.location.origin : "");
  return JSON.stringify({ v: 1, url, code: ticket.code, verifier: ticket.verifier });
}

function secondsUntil(timestamp: string): number {
  const expiresAt = Date.parse(timestamp);
  if (!Number.isFinite(expiresAt)) return 0;
  return Math.max(0, Math.ceil((expiresAt - Date.now()) / 1000));
}

export function LinkDeviceView() {
  const { data: me } = useSWR("me", authApi.me, { shouldRetryOnError: false });
  const userId = me?.id;

  const [ticket, setTicket] = useState<DeviceLinkTicket | null>(null);
  const [phase, setPhase] = useState<Phase>("idle");
  const [scanned, setScanned] = useState<ScannedInfo | null>(null);
  const [secondsLeft, setSecondsLeft] = useState(0);
  const [busy, setBusy] = useState(false);
  const [phoneUrl, setPhoneUrl] = useState("");

  // Reset everything back to the start.
  const reset = useCallback(() => {
    setTicket(null);
    setPhase("idle");
    setScanned(null);
    setSecondsLeft(0);
    setPhoneUrl("");
  }, []);

  const start = useCallback(async () => {
    setBusy(true);
    try {
      const t = await deviceLink.create();
      setTicket(t);
      setPhoneUrl(t.url);
      setScanned(null);
      setPhase("waiting");
      setSecondsLeft(secondsUntil(t.expires_at));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Couldn't start device linking");
    } finally {
      setBusy(false);
    }
  }, []);

  // Apply a status snapshot (from WS push or the polling fallback) to our phase.
  const applyStatus = useCallback((s: DeviceLinkStatus) => {
    setPhase((prev) => {
      if (prev === "idle" || prev === "linked" || prev === "denied" || prev === "expired") {
        return prev; // terminal / not active — ignore late updates
      }
      switch (s.state) {
        case "awaiting_approval":
          if (s.device_name || s.device_model || s.device_ip || s.platform) {
            setScanned({
              device_name: s.device_name,
              device_model: s.device_model,
              device_ip: s.device_ip,
              platform: s.platform,
            });
          }
          return prev === "approving" ? prev : "scanned";
        case "approved":
          return "approving";
        case "consumed":
          return "linked";
        case "denied":
          return "denied";
        case "expired":
          return "expired";
        default:
          return prev;
      }
    });
  }, []);

  // Live updates over the existing WS. device_link_scanned → approval card;
  // device_link_consumed → success.
  useTopic(userId ? `user:${userId}` : null, (env) => {
    if (!ticket) return;
    const data = env.data as { code?: string } & ScannedInfo;
    if (!data || data.code !== ticket.code) return;
    if (env.type === "device_link_scanned") {
      setScanned({
        device_name: data.device_name,
        device_model: data.device_model,
        device_ip: data.device_ip,
        platform: data.platform,
      });
      setPhase((p) => (p === "waiting" ? "scanned" : p));
    } else if (env.type === "device_link_consumed") {
      setPhase("linked");
    }
  });

  // Countdown while the QR is live. At zero, the unscanned code has expired.
  useEffect(() => {
    if (!ticket) return;
    if (phase !== "waiting" && phase !== "scanned" && phase !== "approving") return;
    if (secondsLeft <= 0) {
      if (phase === "waiting") setPhase("expired");
      return;
    }
    const t = setTimeout(() => setSecondsLeft(secondsUntil(ticket.expires_at)), 1000);
    return () => clearTimeout(t);
  }, [ticket, phase, secondsLeft]);

  // Polling fallback in case the WS is down — cheap, short-lived, only while
  // a pairing is in flight.
  useEffect(() => {
    if (!ticket) return;
    if (phase !== "waiting" && phase !== "scanned" && phase !== "approving") return;
    const id = setInterval(async () => {
      try {
        applyStatus(await deviceLink.status(ticket.code));
      } catch {
        /* transient — keep trying until the phase changes or it expires */
      }
    }, 2500);
    return () => clearInterval(id);
  }, [ticket, phase, applyStatus]);

  const approve = async () => {
    if (!ticket) return;
    setBusy(true);
    try {
      await deviceLink.approve(ticket.code);
      setPhase("approving");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Couldn't approve the device");
    } finally {
      setBusy(false);
    }
  };

  const deny = async () => {
    if (!ticket) return;
    setBusy(true);
    try {
      await deviceLink.deny(ticket.code);
    } catch {
      /* best-effort */
    } finally {
      setBusy(false);
      setPhase("denied");
    }
  };

  return (
    <div className="container max-w-2xl mx-auto py-8 px-4 space-y-6">
      <header className="space-y-1.5">
        <h1 className="text-2xl font-bold tracking-tight flex items-center gap-2.5">
          <MonitorSmartphone className="size-6 shrink-0" aria-hidden="true" />
          <span>Link a device</span>
        </h1>
        <p className="text-sm text-muted-foreground leading-relaxed">
          Sign in on the MyCloud mobile app without typing your password. Show a
          QR code here, scan it with the app, then approve the device below.
        </p>
      </header>

      <Card>
        <CardHeader>
          <CardTitle>Scan to sign in</CardTitle>
          <CardDescription>
            Open the MyCloud app on your phone, choose “Scan QR to sign in”, and
            point it at the code. The code expires after two minutes.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {phase === "idle" && (
            <div className="flex flex-col items-center justify-center gap-4 py-8 text-center">
              <div className="rounded-full bg-muted p-4">
                <QrCode className="size-8 text-muted-foreground" aria-hidden="true" />
              </div>
              <p className="text-sm text-muted-foreground max-w-sm leading-relaxed">
                Generate a one-time QR code, then scan it with the app on the
                device you want to sign in.
              </p>
              <Button onClick={() => void start()} disabled={busy}>
                {busy ? (
                  <Loader2 className="size-4 mr-1 animate-spin" aria-hidden="true" />
                ) : (
                  <QrCode className="size-4 mr-1" aria-hidden="true" />
                )}
                Show QR code
              </Button>
            </div>
          )}

          {phase === "waiting" && ticket && (
            <div className="flex flex-col items-center gap-4 py-4">
              <div className="rounded-xl bg-white p-4 ring-1 ring-foreground/10">
                <QRCodeSVG value={qrPayload(ticket, phoneUrl)} size={208} level="M" />
              </div>
              <label className="w-full max-w-sm space-y-1.5 text-left text-xs text-muted-foreground">
                <span>Phone server URL</span>
                <Input
                  value={phoneUrl}
                  onChange={(event) => setPhoneUrl(event.target.value)}
                  placeholder="http://192.168.1.142:8080"
                  className="font-mono text-xs"
                />
              </label>
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                <span>Waiting for a device to scan…</span>
              </div>
              <p className="text-xs text-muted-foreground tabular-nums">
                Expires in {secondsLeft}s
              </p>
            </div>
          )}

          {(phase === "scanned" || phase === "approving") && (
            <div className="flex flex-col items-center gap-5 py-4">
              <div className="rounded-full bg-primary/10 p-3">
                <Smartphone className="size-7 text-primary" aria-hidden="true" />
              </div>
              <div className="text-center space-y-1">
                <p className="text-sm font-medium">A device wants to sign in</p>
                <p className="text-base font-semibold">
                  {scanned?.device_name || scanned?.device_model || "New device"}
                </p>
                <div className="text-xs text-muted-foreground space-x-2">
                  {scanned?.platform && <span>{scanned.platform}</span>}
                  {scanned?.device_ip && (
                    <>
                      <span aria-hidden="true" className="opacity-50">·</span>
                      <span className="tabular-nums">IP {scanned.device_ip}</span>
                    </>
                  )}
                </div>
              </div>

              <Alert>
                <AlertCircle aria-hidden="true" />
                <AlertTitle>Only approve devices you recognise</AlertTitle>
                <AlertDescription>
                  Approving signs this device into your account with full access.
                </AlertDescription>
              </Alert>

              {phase === "approving" ? (
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                  <span>Approved — finishing on the device…</span>
                </div>
              ) : (
                <div className="flex w-full flex-col-reverse sm:flex-row gap-2 sm:justify-center">
                  <Button variant="outline" onClick={() => void deny()} disabled={busy}>
                    <ShieldX className="size-4 mr-1" aria-hidden="true" />
                    Deny
                  </Button>
                  <Button onClick={() => void approve()} disabled={busy}>
                    {busy ? (
                      <Loader2 className="size-4 mr-1 animate-spin" aria-hidden="true" />
                    ) : (
                      <ShieldCheck className="size-4 mr-1" aria-hidden="true" />
                    )}
                    Approve
                  </Button>
                </div>
              )}
            </div>
          )}

          {phase === "linked" && (
            <div className="flex flex-col items-center justify-center gap-3 py-8 text-center">
              <div className="rounded-full bg-emerald-500/10 p-4">
                <CheckCircle2 className="size-8 text-emerald-600" aria-hidden="true" />
              </div>
              <p className="text-sm font-medium">Device linked</p>
              <p className="text-xs text-muted-foreground max-w-xs leading-relaxed">
                {scanned?.device_name || scanned?.device_model || "The device"} is
                now signed in. You can see it under Active sessions.
              </p>
              <Button variant="outline" onClick={reset}>
                <QrCode className="size-4 mr-1" aria-hidden="true" />
                Link another device
              </Button>
            </div>
          )}

          {(phase === "denied" || phase === "expired") && (
            <div className="flex flex-col items-center justify-center gap-3 py-8 text-center">
              <div className={cn(
                "rounded-full p-4",
                phase === "denied" ? "bg-destructive/10" : "bg-muted",
              )}>
                {phase === "denied" ? (
                  <ShieldX className="size-8 text-destructive" aria-hidden="true" />
                ) : (
                  <AlertCircle className="size-8 text-muted-foreground" aria-hidden="true" />
                )}
              </div>
              <p className="text-sm font-medium">
                {phase === "denied" ? "Device denied" : "Code expired"}
              </p>
              <p className="text-xs text-muted-foreground max-w-xs leading-relaxed">
                {phase === "denied"
                  ? "No device was signed in. Generate a new code to try again."
                  : "The QR code timed out before it was scanned and approved."}
              </p>
              <Button variant="outline" onClick={() => void start()} disabled={busy}>
                <RefreshCw className="size-4 mr-1" aria-hidden="true" />
                Show a new code
              </Button>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

// Default export keeps /settings/devices usable as a direct URL.
export default function LinkDevicePage() {
  return <LinkDeviceView />;
}
