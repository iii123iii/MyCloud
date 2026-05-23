"use client";

import { useState, useEffect, use, useRef, useCallback, useMemo } from "react";
import { requests as requestsApi } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Progress } from "@/components/ui/progress";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import {
  Cloud,
  Lock,
  UploadCloud,
  CheckCircle2,
  AlertCircle,
  FileText,
  FolderUp,
  Loader2,
  Plus,
  Clock,
  ShieldAlert,
  Sparkles,
} from "lucide-react";
import { toast } from "sonner";
import { formatBytes, formatServerDateTime } from "@/lib/format";
import { cn } from "@/lib/utils";

interface PageProps {
  params: Promise<{ token: string }>;
}

interface RequestInfo {
  token: string;
  folder_name?: string;
  expires_at?: string;
  max_files?: number;
  uploads_remaining?: number;
  used_files: number;
}

type UploadStatus = "queued" | "uploading" | "done" | "error";
interface UploadItem {
  id: string;
  name: string;
  size: number;
  status: UploadStatus;
  progress: number;
  error?: string;
}

export default function PublicUploadRequestPage({ params }: PageProps) {
  const { token } = use(params);
  const [password, setPassword] = useState("");
  const [passwordRequired, setPasswordRequired] = useState(false);
  const [passwordError, setPasswordError] = useState(false);
  const [info, setInfo] = useState<RequestInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [notFound, setNotFound] = useState(false);
  const [gone, setGone] = useState(false);
  const [uploads, setUploads] = useState<UploadItem[]>([]);
  const [isDragging, setIsDragging] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const dragCounter = useRef(0);

  const resolve = useCallback(async (pwd?: string) => {
    setLoading(true);
    setNotFound(false);
    setGone(false);
    try {
      const res = await requestsApi.resolve(token, pwd);
      setInfo(res);
      setPasswordRequired(false);
      setPasswordError(false);
    } catch (err: unknown) {
      if (err && typeof err === "object" && "status" in err) {
        const status = (err as { status: number }).status;
        if (status === 401) {
          if (pwd) setPasswordError(true);
          setPasswordRequired(true);
        } else if (status === 410) {
          setGone(true);
        } else {
          setNotFound(true);
        }
      } else {
        setNotFound(true);
      }
    } finally {
      setLoading(false);
    }
  }, [token]);

  useEffect(() => {
    resolve();
  }, [resolve]);

  const uploadOne = useCallback(async (item: UploadItem, file: File) => {
    setUploads((u) => u.map((it) => (it.id === item.id ? { ...it, status: "uploading", progress: 5 } : it)));

    return new Promise<void>((resolve) => {
      const xhr = new XMLHttpRequest();
      xhr.open("POST", requestsApi.uploadUrl(token));
      if (password) xhr.setRequestHeader("X-Share-Password", password);

      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable) {
          const pct = Math.max(5, Math.min(99, Math.round((e.loaded / e.total) * 100)));
          setUploads((u) => u.map((it) => (it.id === item.id ? { ...it, progress: pct } : it)));
        }
      };

      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          setUploads((u) => u.map((it) => (it.id === item.id ? { ...it, status: "done", progress: 100 } : it)));
        } else {
          let message = `Upload failed (${xhr.status})`;
          try {
            const body = JSON.parse(xhr.responseText);
            if (body?.error?.message) message = body.error.message;
          } catch {
            /* ignore */
          }
          setUploads((u) => u.map((it) => (it.id === item.id ? { ...it, status: "error", error: message } : it)));
          if (xhr.status === 410) setGone(true);
        }
        resolve();
      };

      xhr.onerror = () => {
        setUploads((u) => u.map((it) => (it.id === item.id ? { ...it, status: "error", error: "Network error" } : it)));
        resolve();
      };

      const form = new FormData();
      form.append("file", file);
      xhr.send(form);
    });
  }, [token, password]);

  const onFilesPicked = useCallback(async (list: FileList | File[] | null) => {
    if (!list) return;
    const files = Array.from(list);
    if (!files.length) return;

    const newItems: UploadItem[] = files.map((f) => ({
      id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}-${f.name}`,
      name: f.name,
      size: f.size,
      status: "queued",
      progress: 0,
    }));
    setUploads((u) => [...u, ...newItems]);

    let successCount = 0;
    for (let i = 0; i < files.length; i++) {
      await uploadOne(newItems[i], files[i]);
      // re-read latest status synchronously via state callback
      setUploads((u) => {
        const cur = u.find((it) => it.id === newItems[i].id);
        if (cur?.status === "done") successCount++;
        return u;
      });
    }
    await resolve(password || undefined);
    if (successCount > 0) {
      toast.success(`${successCount} file${successCount === 1 ? "" : "s"} uploaded`);
    }
  }, [uploadOne, resolve, password]);

  const onDragEnter = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounter.current += 1;
    if (e.dataTransfer?.items && e.dataTransfer.items.length > 0) {
      setIsDragging(true);
    }
  }, []);

  const onDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounter.current -= 1;
    if (dragCounter.current <= 0) {
      dragCounter.current = 0;
      setIsDragging(false);
    }
  }, []);

  const onDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
  }, []);

  const onDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounter.current = 0;
    setIsDragging(false);
    if (e.dataTransfer?.files && e.dataTransfer.files.length > 0) {
      onFilesPicked(e.dataTransfer.files);
      e.dataTransfer.clearData();
    }
  }, [onFilesPicked]);

  const stats = useMemo(() => {
    const done = uploads.filter((u) => u.status === "done").length;
    const failed = uploads.filter((u) => u.status === "error").length;
    const active = uploads.filter((u) => u.status === "uploading" || u.status === "queued").length;
    return { done, failed, active, total: uploads.length };
  }, [uploads]);

  const allDoneNoActive = uploads.length > 0 && stats.active === 0 && stats.done > 0;
  const isFull = info?.max_files !== undefined && (info.uploads_remaining ?? 0) <= 0;

  // ---- HEADER (shared across states) ----
  const Header = (
    <div className="flex flex-col items-center gap-2.5 text-center">
      <div className="inline-flex items-center gap-2.5 rounded-full bg-primary/10 text-primary px-4 py-1.5 shadow-sm ring-1 ring-primary/15 transition-colors">
        <Cloud className="h-5 w-5" aria-hidden="true" />
        <span className="text-base font-semibold tracking-tight">MyCloud</span>
      </div>
      <p className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
        Secure file delivery
      </p>
    </div>
  );

  // ---- SKELETON ----
  if (loading && !info && !passwordRequired) {
    return (
      <div className="min-h-screen flex items-center justify-center p-4 sm:p-6 bg-gradient-to-b from-background to-muted/30">
        <div
          className="w-full max-w-lg space-y-6"
          role="status"
          aria-live="polite"
          aria-busy="true"
          aria-label="Loading upload link"
        >
          {Header}
          <Card className="shadow-sm">
            <CardHeader className="space-y-2.5">
              <Skeleton className="h-5 w-2/3" />
              <Skeleton className="h-3 w-1/2" />
            </CardHeader>
            <CardContent className="space-y-4">
              <Skeleton className="h-44 w-full rounded-xl" />
              <Skeleton className="h-10 w-full rounded-lg" />
            </CardContent>
          </Card>
          <span className="sr-only">Loading…</span>
        </div>
      </div>
    );
  }

  // ---- NOT FOUND ----
  if (notFound) {
    return (
      <div className="min-h-screen flex items-center justify-center p-4 sm:p-6 bg-gradient-to-b from-background to-muted/30">
        <div className="w-full max-w-lg space-y-6">
          {Header}
          <Card className="shadow-sm" role="alert" aria-live="polite">
            <CardContent className="py-12 px-6 text-center space-y-4">
              <div className="mx-auto h-14 w-14 rounded-full bg-muted flex items-center justify-center ring-1 ring-border">
                <AlertCircle className="h-6 w-6 text-muted-foreground" aria-hidden="true" />
              </div>
              <div className="space-y-1.5">
                <p className="text-base font-semibold tracking-tight">Upload link not found</p>
                <p className="text-sm leading-relaxed text-muted-foreground max-w-sm mx-auto">
                  This link may have been deleted or the URL is incorrect. Try asking the sender for a fresh link.
                </p>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    );
  }

  // ---- EXPIRED / FULL ----
  if (gone) {
    return (
      <div className="min-h-screen flex items-center justify-center p-4 sm:p-6 bg-gradient-to-b from-background to-muted/30">
        <div className="w-full max-w-lg space-y-6">
          {Header}
          <Card className="shadow-sm" role="alert" aria-live="polite">
            <CardContent className="py-12 px-6 text-center space-y-4">
              <div className="mx-auto h-14 w-14 rounded-full bg-amber-100 dark:bg-amber-950/60 flex items-center justify-center ring-1 ring-amber-200 dark:ring-amber-900">
                <Clock className="h-6 w-6 text-amber-600 dark:text-amber-400" aria-hidden="true" />
              </div>
              <div className="space-y-1.5">
                <p className="text-base font-semibold tracking-tight">
                  This upload link is no longer active
                </p>
                <p className="text-sm leading-relaxed text-muted-foreground max-w-sm mx-auto">
                  It may have expired or reached its upload limit. Please request a new link from the sender to continue.
                </p>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    );
  }

  // ---- PASSWORD ----
  if (passwordRequired && !info) {
    return (
      <div className="min-h-screen flex items-center justify-center p-4 sm:p-6 bg-gradient-to-b from-background to-muted/30">
        <div className="w-full max-w-lg space-y-6">
          {Header}
          <Card className="shadow-sm">
            <CardHeader className="space-y-3 text-center">
              <div className="mx-auto h-14 w-14 rounded-full bg-primary/10 flex items-center justify-center ring-1 ring-primary/15">
                <Lock className="h-6 w-6 text-primary" aria-hidden="true" />
              </div>
              <div className="space-y-1.5">
                <CardTitle className="text-lg tracking-tight">
                  Enter password to continue
                </CardTitle>
                <CardDescription className="leading-relaxed">
                  The sender protected this upload link with a password.
                </CardDescription>
              </div>
            </CardHeader>
            <CardContent>
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  if (password) resolve(password);
                }}
                className="space-y-4"
                aria-label="Password protected upload link"
              >
                <div className="space-y-1.5">
                  <Label htmlFor="upreq-password" className="text-sm font-medium">
                    Password
                  </Label>
                  <Input
                    id="upreq-password"
                    type="password"
                    autoFocus
                    autoComplete="off"
                    placeholder="Type the password"
                    value={password}
                    onChange={(e) => {
                      setPassword(e.target.value);
                      if (passwordError) setPasswordError(false);
                    }}
                    aria-invalid={passwordError || undefined}
                    aria-describedby={passwordError ? "upreq-password-error" : undefined}
                    className="transition-colors"
                  />
                  {passwordError && (
                    <p
                      id="upreq-password-error"
                      role="alert"
                      className="text-xs text-destructive flex items-center gap-1.5 pt-0.5"
                    >
                      <ShieldAlert className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                      That password didn&apos;t match. Try again.
                    </p>
                  )}
                </div>
                <Button
                  type="submit"
                  size="lg"
                  className="w-full transition-all"
                  disabled={loading || !password}
                >
                  {loading ? (
                    <Loader2 className="h-4 w-4 mr-2 animate-spin" aria-hidden="true" />
                  ) : (
                    <Lock className="h-4 w-4 mr-2" aria-hidden="true" />
                  )}
                  Unlock
                </Button>
              </form>
            </CardContent>
          </Card>
        </div>
      </div>
    );
  }

  // ---- MAIN UPLOAD UI ----
  const remainingCount =
    info?.max_files !== undefined ? (info.uploads_remaining ?? info.max_files) : undefined;
  const queueStatusText =
    stats.active > 0
      ? `Uploading ${stats.active} file${stats.active === 1 ? "" : "s"}…`
      : `${stats.done} done${stats.failed ? `, ${stats.failed} failed` : ""}`;

  return (
    <div className="min-h-screen flex items-start sm:items-center justify-center p-4 sm:p-6 bg-gradient-to-b from-background to-muted/30">
      <div className="w-full max-w-lg space-y-6 py-4 sm:py-0">
        {Header}

        <Card className="overflow-hidden shadow-sm">
          <CardHeader className="space-y-2.5">
            <div className="flex items-center gap-2">
              <FolderUp className="h-4 w-4 text-primary" aria-hidden="true" />
              <Badge variant="secondary" className="text-[10px] uppercase tracking-wider font-semibold">
                You&apos;re sending files
              </Badge>
            </div>
            <CardTitle className="text-xl leading-tight tracking-tight">
              {info?.folder_name ? (
                <>
                  Drop files into{" "}
                  <span className="text-primary">&ldquo;{info.folder_name}&rdquo;</span>
                </>
              ) : (
                "Send files to the requester"
              )}
            </CardTitle>
            <CardDescription className="leading-relaxed">
              Your uploads go directly to the owner&apos;s MyCloud. No account needed.
            </CardDescription>
          </CardHeader>

          <CardContent className="space-y-5">
            {/* Meta strip */}
            {info && (info.expires_at || info.max_files !== undefined) && (
              <div className="flex flex-wrap gap-2 text-xs">
                {info.expires_at && (
                  <Badge variant="outline" className="font-normal gap-1.5 py-1">
                    <Clock className="h-3 w-3" aria-hidden="true" />
                    Expires {formatServerDateTime(info.expires_at)}
                  </Badge>
                )}
                {info.max_files !== undefined && (
                  <Badge variant="outline" className="font-normal py-1">
                    {remainingCount} of {info.max_files} uploads remaining
                  </Badge>
                )}
              </div>
            )}

            {/* FULL state inline */}
            {isFull && (
              <Alert role="alert">
                <AlertCircle className="h-4 w-4" aria-hidden="true" />
                <AlertTitle>Upload limit reached</AlertTitle>
                <AlertDescription>
                  This link has accepted all the files it can. Please contact the sender for a new link.
                </AlertDescription>
              </Alert>
            )}

            {/* DROPZONE */}
            {!isFull && (
              <section
                role="region"
                aria-label={
                  info?.folder_name
                    ? `Upload files to ${info.folder_name}`
                    : "Upload files"
                }
                className="space-y-2"
              >
                <div className="flex items-center justify-between gap-2">
                  <Label
                    htmlFor="upreq-file-input"
                    className="text-xs font-medium text-muted-foreground"
                  >
                    Choose files to upload
                  </Label>
                  <Button
                    type="button"
                    variant="ghost"
                    size="xs"
                    onClick={() => fileInputRef.current?.click()}
                    aria-controls="upreq-file-input"
                  >
                    Browse files
                  </Button>
                </div>
                <div
                  role="button"
                  tabIndex={0}
                  onClick={() => fileInputRef.current?.click()}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      fileInputRef.current?.click();
                    }
                  }}
                  onDragEnter={onDragEnter}
                  onDragLeave={onDragLeave}
                  onDragOver={onDragOver}
                  onDrop={onDrop}
                  aria-label={
                    isDragging
                      ? "Release to upload files"
                      : "Drop files here or press Enter to choose files"
                  }
                  aria-describedby="upreq-dropzone-hint"
                  className={cn(
                    "group relative flex flex-col items-center justify-center gap-3 rounded-xl border-2 border-dashed",
                    "px-6 py-10 sm:py-14 text-center cursor-pointer outline-none select-none",
                    "transition-all duration-200 ease-out motion-reduce:transition-none",
                    "focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background",
                    isDragging
                      ? "border-primary bg-primary/10 scale-[1.01] shadow-inner ring-4 ring-primary/15"
                      : "border-border bg-muted/30 hover:bg-muted/60 hover:border-primary/60 active:scale-[0.995]"
                  )}
                >
                  <div
                    className={cn(
                      "h-14 w-14 rounded-full flex items-center justify-center",
                      "transition-all duration-200 motion-reduce:transition-none",
                      isDragging
                        ? "bg-primary text-primary-foreground scale-110 shadow-md"
                        : "bg-primary/10 text-primary group-hover:bg-primary/15 group-hover:scale-105"
                    )}
                    aria-hidden="true"
                  >
                    <UploadCloud
                      className={cn(
                        "h-7 w-7 transition-transform duration-200",
                        isDragging && "animate-bounce"
                      )}
                    />
                  </div>
                  <div className="space-y-1">
                    <p className="text-sm font-semibold tracking-tight">
                      {isDragging ? "Release to upload" : "Drop files here"}
                    </p>
                    <p
                      id="upreq-dropzone-hint"
                      className="text-xs text-muted-foreground leading-relaxed"
                    >
                      {isDragging ? (
                        <span aria-hidden="true">&nbsp;</span>
                      ) : (
                        <>
                          or{" "}
                          <span className="font-medium text-foreground underline decoration-dotted underline-offset-2">
                            click to browse
                          </span>
                          {" "}· multiple files supported
                        </>
                      )}
                    </p>
                  </div>
                  <input
                    ref={fileInputRef}
                    id="upreq-file-input"
                    type="file"
                    multiple
                    className="sr-only"
                    aria-label="Upload files"
                    onChange={(e) => {
                      onFilesPicked(e.target.files);
                      e.currentTarget.value = "";
                    }}
                  />
                </div>
                {remainingCount !== undefined && remainingCount > 0 && (
                  <p className="text-[11px] text-muted-foreground text-center">
                    {remainingCount} of {info!.max_files} upload{info!.max_files === 1 ? "" : "s"} remaining
                  </p>
                )}
              </section>
            )}

            {/* QUEUE */}
            {uploads.length > 0 && (
              <div className="space-y-2.5">
                <div className="flex items-center justify-between gap-2">
                  <p
                    className="text-xs font-medium text-muted-foreground"
                    aria-live="polite"
                    aria-atomic="true"
                  >
                    {queueStatusText}
                  </p>
                  {allDoneNoActive && !isFull && (
                    <Button
                      type="button"
                      variant="ghost"
                      size="xs"
                      onClick={() => fileInputRef.current?.click()}
                    >
                      <Plus className="h-3.5 w-3.5 mr-1" aria-hidden="true" /> Add more
                    </Button>
                  )}
                </div>
                <ul
                  className="rounded-lg border bg-card divide-y overflow-hidden shadow-xs"
                  aria-label="Upload progress"
                >
                  {uploads.map((u) => {
                    const detail =
                      u.status === "error"
                        ? (u.error ?? "Failed")
                        : u.status === "done"
                          ? formatBytes(u.size)
                          : u.status === "uploading"
                            ? `${u.progress}% · ${formatBytes(u.size)}`
                            : `${formatBytes(u.size)} · queued`;
                    return (
                      <li
                        key={u.id}
                        className="px-3 py-2.5 text-xs space-y-1.5 transition-colors hover:bg-muted/30"
                      >
                        <div className="flex items-center gap-2.5 min-w-0">
                          <div
                            className={cn(
                              "h-7 w-7 shrink-0 rounded-md flex items-center justify-center transition-colors",
                              u.status === "done"
                                ? "bg-green-100 dark:bg-green-950/60"
                                : u.status === "error"
                                  ? "bg-destructive/10"
                                  : u.status === "uploading"
                                    ? "bg-primary/10"
                                    : "bg-muted"
                            )}
                            aria-hidden="true"
                          >
                            {u.status === "done" ? (
                              <CheckCircle2 className="h-4 w-4 text-green-600 dark:text-green-400" />
                            ) : u.status === "error" ? (
                              <AlertCircle className="h-4 w-4 text-destructive" />
                            ) : u.status === "uploading" ? (
                              <Loader2 className="h-4 w-4 text-primary animate-spin" />
                            ) : (
                              <FileText className="h-4 w-4 text-muted-foreground" />
                            )}
                          </div>
                          <div className="min-w-0 flex-1">
                            <p className="truncate font-medium text-foreground" title={u.name}>
                              {u.name}
                            </p>
                            <p
                              className={cn(
                                "text-[11px] truncate tabular-nums",
                                u.status === "error"
                                  ? "text-destructive"
                                  : "text-muted-foreground"
                              )}
                            >
                              {detail}
                            </p>
                          </div>
                          <span className="sr-only">
                            {u.name}: {u.status === "done" ? "uploaded" : u.status === "error" ? "failed" : u.status === "uploading" ? `uploading, ${u.progress} percent` : "queued"}
                          </span>
                        </div>
                        {(u.status === "uploading" || u.status === "queued") && (
                          <Progress
                            value={u.status === "queued" ? 0 : u.progress}
                            aria-label={`Upload progress for ${u.name}`}
                          />
                        )}
                      </li>
                    );
                  })}
                </ul>
              </div>
            )}

            {/* DONE STATE banner */}
            {allDoneNoActive && stats.failed === 0 && (
              <div
                role="status"
                aria-live="polite"
                className="rounded-lg border border-green-200 bg-green-50 dark:bg-green-950/30 dark:border-green-900 px-4 py-3 flex items-start gap-3 transition-colors"
              >
                <div
                  className="h-9 w-9 shrink-0 rounded-full bg-green-100 dark:bg-green-900 flex items-center justify-center"
                  aria-hidden="true"
                >
                  <Sparkles className="h-4 w-4 text-green-600 dark:text-green-400" />
                </div>
                <div className="space-y-0.5 min-w-0">
                  <p className="text-sm font-semibold tracking-tight text-green-900 dark:text-green-100">
                    All done — {stats.done} file{stats.done === 1 ? "" : "s"} delivered
                  </p>
                  <p className="text-xs leading-relaxed text-green-700 dark:text-green-300">
                    Thanks! The recipient now has your files.
                  </p>
                </div>
              </div>
            )}
          </CardContent>
        </Card>

        <Separator className="opacity-50" />
        <p className="text-center text-[11px] text-muted-foreground leading-relaxed">
          Powered by MyCloud · Files are scanned and stored securely.
        </p>
      </div>
    </div>
  );
}
