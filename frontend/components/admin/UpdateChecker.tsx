"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { admin as adminApi } from "@/lib/api";
import type { UpdateInfo } from "@/lib/types";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { RefreshCw, CheckCircle2, AlertCircle, ExternalLink, Loader2 } from "lucide-react";

// ── localStorage key ────────────────────────────────────────────────────────

const UPDATE_KEY = "mycloud_update_in_progress";

// ── Stage detection from log lines ──────────────────────────────────────────

const STAGES: { marker: string; label: string; progress: number }[] = [
  { marker: "Pre-flight checks",   label: "Running pre-flight checks…",      progress: 8  },
  { marker: "git pull",            label: "Pulling latest code…",             progress: 22 },
  { marker: "git fetch",           label: "Fetching latest code…",            progress: 18 },
  { marker: "Database migrations", label: "Running database migrations…",     progress: 42 },
  { marker: "Building services",   label: "Building Docker images…",          progress: 52 },
  { marker: "Restarting services", label: "Restarting services…",             progress: 86 },
  { marker: "Update complete",     label: "Wrapping up…",                     progress: 98 },
];

function getUpdateStage(lines: string[]): { label: string; progress: number } {
  let best = { label: "Starting update…", progress: 3 };
  for (const line of lines) {
    for (const s of STAGES) {
      if (line.includes(s.marker) && s.progress >= best.progress) {
        best = { label: s.label, progress: s.progress };
      }
    }
  }
  return best;
}

// ── Component ───────────────────────────────────────────────────────────────

export function UpdateChecker() {
  const [info, setInfo]                     = useState<UpdateInfo | null>(null);
  const [checking, setChecking]             = useState(false);
  const [applying, setApplying]             = useState(false);
  const [awaitingRestart, setAwaitingRestart] = useState(false);
  const [applyMsg, setApplyMsg]             = useState<{ ok: boolean; text: string } | null>(null);
  const [error, setError]                   = useState<string | null>(null);
  const [sawOffline, setSawOffline]         = useState(false);
  const [logLines, setLogLines]             = useState<string[]>([]);
  const logRef = useRef<HTMLDivElement>(null);

  // ── Auto-scroll log to bottom ──────────────────────────────────────────────

  useEffect(() => {
    if (logRef.current) {
      logRef.current.scrollTop = logRef.current.scrollHeight;
    }
  }, [logLines]);

  // ── On mount: auto-detect an update that was running before the page refreshed ──

  useEffect(() => {
    if (!localStorage.getItem(UPDATE_KEY)) return;

    setAwaitingRestart(true);

    // Immediately try to fetch the current status & log.
    adminApi.fetchUpdateStatus()
      .then((data) => {
        // If the update already finished while we were away, clean up quietly.
        if (!data.update_in_progress && data.update_status !== "running") {
          setAwaitingRestart(false);
          localStorage.removeItem(UPDATE_KEY);
          setInfo((prev) => (prev ? { ...prev, ...data } : null));
          if (data.update_status === "failed") {
            setApplyMsg({ ok: false, text: data.update_status_message });
          } else if (data.update_status === "succeeded") {
            setApplyMsg({ ok: true, text: data.update_status_message });
            setInfo((prev) => prev ? { ...prev, update_available: false } : prev);
          }
        }
      })
      .catch(() => {
        // Server is down — already restarting.
        setSawOffline(true);
      });

    adminApi.fetchUpdateLog()
      .then((data) => { if (data?.lines?.length) setLogLines(data.lines); })
      .catch(() => {});
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // ── Poll status + log while an update is running ───────────────────────────

  useEffect(() => {
    if (!awaitingRestart) {
      setSawOffline(false);
      return;
    }

    // Fetch log immediately so users don't wait for the first tick.
    adminApi.fetchUpdateLog()
      .then((data) => { if (data?.lines?.length) setLogLines(data.lines); })
      .catch(() => {});

    const statusTimer = window.setInterval(async () => {
      try {
        const data = await adminApi.fetchUpdateStatus();
        if (sawOffline) {
          localStorage.removeItem(UPDATE_KEY);
          window.location.reload();
          return;
        }
        setInfo((current) => (current ? { ...current, ...data } : current));
        if (!data.update_in_progress) {
          setAwaitingRestart(false);
          localStorage.removeItem(UPDATE_KEY);
          if (data.update_status === "failed") {
            setApplyMsg({ ok: false, text: data.update_status_message });
          } else if (data.update_status === "succeeded") {
            setApplyMsg({ ok: true, text: data.update_status_message });
            // The update was applied — clear the "update available" banner immediately.
            setInfo((current) => current ? { ...current, update_available: false } : current);
          }
        }
      } catch {
        setSawOffline(true);
      }
    }, 1500); // ↑ was 4000 ms

    const logTimer = window.setInterval(async () => {
      try {
        const data = await adminApi.fetchUpdateLog();
        if (data?.lines?.length) setLogLines(data.lines);
      } catch {
        // ignore — status poll handles offline detection
      }
    }, 1000); // ↑ was 2000 ms

    return () => {
      window.clearInterval(statusTimer);
      window.clearInterval(logTimer);
    };
  }, [awaitingRestart, sawOffline]);

  // ── Stage / progress derived from log lines ────────────────────────────────

  const { stageLabel, stageProgress } = useMemo(() => {
    if (sawOffline) {
      return { stageLabel: "Server restarting, waiting for it to come back…", stageProgress: 92 };
    }
    if (!logLines.length) {
      return { stageLabel: info?.update_status_message || "Update in progress…", stageProgress: 3 };
    }
    const s = getUpdateStage(logLines);
    return { stageLabel: s.label, stageProgress: s.progress };
  }, [sawOffline, logLines, info?.update_status_message]);

  // ── Actions ────────────────────────────────────────────────────────────────

  async function handleCheck() {
    setChecking(true);
    setError(null);
    setApplyMsg(null);
    try {
      const data = await adminApi.checkUpdate();
      setInfo(data);
      if (!data.update_in_progress) setAwaitingRestart(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to check for updates");
    } finally {
      setChecking(false);
    }
  }

  async function handleApply() {
    if (!info?.apply_supported) return;
    if (!confirm("Apply the update now? The server may restart briefly.")) return;

    setApplying(true);
    setApplyMsg(null);
    setLogLines([]);
    try {
      const res = await adminApi.applyUpdate(info?.latest);
      setInfo((current) =>
        current
          ? { ...current, update_in_progress: true, update_status: "running", update_status_message: res.message }
          : current,
      );
      setApplyMsg({ ok: true, text: res.message });
      localStorage.setItem(UPDATE_KEY, "1");
      setAwaitingRestart(true);
    } catch (err) {
      setApplyMsg({
        ok: false,
        text: err instanceof Error ? err.message : "Update failed",
      });
    } finally {
      setApplying(false);
    }
  }

  function formatDate(iso: string) {
    try {
      return new Date(iso).toLocaleDateString(undefined, { dateStyle: "medium" });
    } catch {
      return iso;
    }
  }

  const updateActionDisabled = applying || awaitingRestart || Boolean(info?.update_in_progress);
  const updateRunning        = awaitingRestart || info?.update_in_progress || info?.update_status === "running";

  // ── Render ─────────────────────────────────────────────────────────────────

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-3">
        <CardTitle className="text-base font-semibold">Server updates</CardTitle>
        <Button
          variant="outline"
          size="sm"
          onClick={handleCheck}
          disabled={checking || updateActionDisabled}
        >
          {checking ? (
            <>
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              Checking...
            </>
          ) : (
            <>
              <RefreshCw className="mr-2 h-4 w-4" />
              Check for updates
            </>
          )}
        </Button>
      </CardHeader>

      <CardContent className="space-y-4">
        {info && (
          <div className="flex items-center gap-3 text-sm">
            <span className="text-muted-foreground">Running</span>
            <Badge variant="secondary">{info.current}</Badge>
            <span className="text-muted-foreground">Latest</span>
            <Badge variant={info.update_available ? "destructive" : "default"}>
              {info.latest}
            </Badge>
          </div>
        )}

        {info && !info.update_available && !updateRunning && (
          <div className="flex items-center gap-2 text-sm text-green-600 dark:text-green-400">
            <CheckCircle2 className="h-4 w-4" />
            You&apos;re running the latest version.
          </div>
        )}

        {info && info.update_available && (
          <div className="rounded-lg border border-amber-200 bg-amber-50 p-4 space-y-3 dark:border-amber-800 dark:bg-amber-950">
            <div className="flex items-start justify-between gap-4">
              <div>
                <p className="font-medium text-sm">
                  {info.release_name || info.latest} available
                </p>
                <p className="text-xs text-muted-foreground mt-0.5">
                  Published {formatDate(info.published_at)}
                </p>
              </div>
              <a
                href={info.release_url}
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground shrink-0"
              >
                GitHub <ExternalLink className="h-3 w-3" />
              </a>
            </div>

            {info.release_notes && (
              <pre className="text-xs text-muted-foreground whitespace-pre-wrap font-sans leading-relaxed max-h-32 overflow-y-auto">
                {info.release_notes}
              </pre>
            )}

            {!info.apply_supported && (
              <div className="rounded-md border border-border/60 bg-background/70 p-3 text-sm text-muted-foreground">
                {info.apply_message}
              </div>
            )}

            {info.apply_supported && (
              <Button
                size="sm"
                onClick={handleApply}
                disabled={updateActionDisabled}
                className="w-full sm:w-auto"
              >
                {applying ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Starting update...
                  </>
                ) : updateActionDisabled ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Updating...
                  </>
                ) : (
                  <>Update to {info.latest}</>
                )}
              </Button>
            )}
          </div>
        )}

        {/* ── Live progress while update is running ── */}
        {updateRunning && (
          <div className="space-y-3">
            {/* Stage label + spinner */}
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-3.5 w-3.5 animate-spin shrink-0" />
              <span>{stageLabel}</span>
            </div>

            {/* Progress bar */}
            <div className="space-y-1">
              <div className="h-1.5 w-full bg-muted rounded-full overflow-hidden">
                <div
                  className="h-full bg-primary rounded-full transition-all duration-700 ease-out"
                  style={{ width: `${stageProgress}%` }}
                />
              </div>
              <p className="text-xs text-muted-foreground text-right tabular-nums">
                {stageProgress}%
              </p>
            </div>

            {/* Live log terminal */}
            {logLines.length > 0 && (
              <div
                ref={logRef}
                className="rounded-md bg-black text-green-400 font-mono text-xs p-3 max-h-64 overflow-y-auto leading-relaxed"
              >
                {logLines.map((line, i) => (
                  <div key={i}>{line || "\u00A0"}</div>
                ))}
                <div className="inline-block w-2 h-3.5 bg-green-400 animate-pulse ml-0.5 align-middle" />
              </div>
            )}
          </div>
        )}

        {/* ── Final status: failed ── */}
        {!updateRunning && info?.update_status === "failed" && (
          <div className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">
            <div className="flex items-center gap-2 font-medium mb-1">
              <AlertCircle className="h-4 w-4" />
              Update failed
            </div>
            <div>{info.update_status_message || "Update failed."}</div>
            {logLines.length > 0 && (
              <div
                ref={logRef}
                className="mt-2 rounded bg-black text-red-300 font-mono text-xs p-2 max-h-48 overflow-y-auto leading-relaxed"
              >
                {logLines.map((line, i) => (
                  <div key={i}>{line || "\u00A0"}</div>
                ))}
              </div>
            )}
          </div>
        )}

        {/* ── Final status: succeeded ── */}
        {!updateRunning && info?.update_status === "succeeded" && (
          <div className="rounded-md border border-green-200 bg-green-50 p-3 text-sm text-green-700 dark:border-green-900 dark:bg-green-950 dark:text-green-300">
            <div className="flex items-center gap-2 font-medium">
              <CheckCircle2 className="h-4 w-4" />
              {info.update_status_message || "Update succeeded."}
            </div>
          </div>
        )}

        {applyMsg && !updateRunning && info?.update_status !== "failed" && info?.update_status !== "succeeded" && (
          <div
            className={`flex items-start gap-2 rounded-md p-3 text-sm ${
              applyMsg.ok
                ? "bg-green-50 text-green-700 dark:bg-green-950 dark:text-green-300"
                : "bg-red-50 text-red-700 dark:bg-red-950 dark:text-red-300"
            }`}
          >
            {applyMsg.ok ? (
              <CheckCircle2 className="h-4 w-4 mt-0.5 shrink-0" />
            ) : (
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            )}
            {applyMsg.text}
          </div>
        )}

        {error && (
          <div className="flex items-start gap-2 rounded-md bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950 dark:text-red-300">
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            {error}
          </div>
        )}

        {!info && !checking && !error && !updateRunning && (
          <p className="text-sm text-muted-foreground">
            Click &ldquo;Check for updates&rdquo; to compare your running version against the
            latest GitHub release.
          </p>
        )}
      </CardContent>
    </Card>
  );
}
