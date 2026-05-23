"use client";

import { useState, useEffect, use } from "react";
import Link from "next/link";
import { shares as sharesApi } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Separator } from "@/components/ui/separator";
import { FileIcon } from "@/components/file-explorer/FileIcon";
import {
  Download,
  Cloud,
  Lock,
  Clock,
  ShieldCheck,
  CircleAlert,
  Link2Off,
  Loader2,
  RotateCw,
} from "lucide-react";
import { formatBytes, formatServerDateTime, mimeToLabel } from "@/lib/format";
import { cn } from "@/lib/utils";
import { toast } from "sonner";

// Next.js 16: params is a Promise
interface PageProps {
  params: Promise<{ token: string }>;
}

export default function PublicSharePage({ params }: PageProps) {
  const { token } = use(params);
  const [password, setPassword] = useState("");
  const [passwordRequired, setPasswordRequired] = useState(false);
  const [passwordError, setPasswordError] = useState(false);
  const [shareInfo, setShareInfo] = useState<{
    permission: string;
    file_id?: string;
    file_name?: string;
    file_size?: number;
    mime_type?: string;
    expires_at?: string;
    downloads_remaining?: number;
    download_limit?: number;
  } | null>(null);
  const [loading, setLoading] = useState(true);
  const [notFound, setNotFound] = useState(false);
  const [gone, setGone] = useState(false);
  const [downloading, setDownloading] = useState(false);

  const resolve = async (pwd?: string) => {
    setLoading(true);
    setNotFound(false);
    setGone(false);
    try {
      const info = await sharesApi.resolve(token, pwd);
      setShareInfo(info);
      setPasswordRequired(false);
      setPasswordError(false);
    } catch (err: unknown) {
      if (err && typeof err === "object" && "status" in err) {
        const status = (err as { status: number }).status;
        if (status === 401) {
          // If we already had a password attempt, this is a wrong-password error
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
  };

  // Auto-resolve on first mount (no password needed)
  useEffect(() => { resolve(); }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const handleDownload = async () => {
    if (downloading) return;
    setDownloading(true);
    const url = sharesApi.downloadUrl(token);
    try {
      const res = await fetch(url, password ? { headers: { "X-Share-Password": password } } : undefined);
      if (res.status === 410) {
        setGone(true);
        setShareInfo(null);
        return;
      }
      if (!res.ok) throw new Error("Download failed");
      const blob = await res.blob();
      const blobUrl = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = blobUrl;
      a.download = shareInfo?.file_name ?? "download";
      a.click();
      URL.revokeObjectURL(blobUrl);
      // Refresh remaining-download counter
      void resolve(password || undefined);
    } catch {
      toast.error("Download failed");
    } finally {
      setDownloading(false);
    }
  };

  return (
    <div className="min-h-screen flex flex-col bg-gradient-to-b from-background to-muted/30">
      {/* Branded header */}
      <header className="w-full border-b bg-background/80 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="mx-auto flex h-14 max-w-5xl items-center justify-between px-4 sm:px-6">
          <Link
            href="/"
            className="group flex items-center gap-2 rounded-md text-foreground outline-none transition-opacity hover:opacity-80 focus-visible:ring-3 focus-visible:ring-ring/50"
            aria-label="MyCloud home"
          >
            <span className="flex h-8 w-8 items-center justify-center rounded-md bg-primary/10 text-primary transition-colors duration-200 group-hover:bg-primary/15">
              <Cloud className="h-4 w-4" aria-hidden="true" />
            </span>
            <span className="text-base font-semibold tracking-tight">MyCloud</span>
          </Link>
          <div className="hidden items-center gap-1.5 text-xs text-muted-foreground sm:flex">
            <ShieldCheck className="h-3.5 w-3.5" aria-hidden="true" />
            <span>Secure share</span>
          </div>
        </div>
      </header>

      {/* Main content */}
      <main className="flex flex-1 items-start justify-center px-4 py-8 sm:items-center sm:py-12">
        <div className="w-full max-w-md">
          {loading && (
            <Card
              className="animate-in fade-in slide-in-from-bottom-2 duration-300"
              aria-busy="true"
              aria-live="polite"
            >
              <CardHeader className="items-center gap-2 pb-4 text-center">
                <Skeleton className="h-20 w-20 rounded-2xl" />
                <Skeleton className="mt-4 h-5 w-52" />
                <Skeleton className="mt-1 h-3.5 w-28" />
              </CardHeader>
              <CardContent className="space-y-4">
                <Skeleton className="h-10 w-full rounded-lg" />
                <Skeleton className="h-px w-full" />
                <div className="space-y-2">
                  <Skeleton className="h-3.5 w-3/4" />
                  <Skeleton className="h-3.5 w-2/3" />
                </div>
              </CardContent>
              <span className="sr-only">Loading shared file&hellip;</span>
            </Card>
          )}

          {!loading && notFound && (
            <Card
              className="animate-in fade-in slide-in-from-bottom-2 duration-300"
              role="alert"
            >
              <CardHeader className="items-center gap-2 pb-2 text-center">
                <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-muted">
                  <Link2Off className="h-8 w-8 text-muted-foreground" aria-hidden="true" />
                </div>
                <CardTitle
                  role="heading"
                  aria-level={1}
                  className="mt-4 text-xl font-semibold tracking-tight"
                >
                  Link not available
                </CardTitle>
                <CardDescription className="max-w-xs text-balance">
                  This share link no longer exists or has been revoked by its owner.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4 text-center">
                <p className="text-xs text-muted-foreground">
                  Double-check the URL or ask the sender for a new link.
                </p>
                <Button
                  variant="outline"
                  size="lg"
                  className="w-full"
                  onClick={() => resolve()}
                  aria-label="Try resolving this link again"
                >
                  <RotateCw className="mr-2 h-4 w-4" aria-hidden="true" />
                  Try again
                </Button>
              </CardContent>
            </Card>
          )}

          {!loading && gone && (
            <Card
              className="animate-in fade-in slide-in-from-bottom-2 duration-300"
              role="alert"
            >
              <CardHeader className="items-center gap-2 pb-2 text-center">
                <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-amber-500/10">
                  <Clock className="h-8 w-8 text-amber-600 dark:text-amber-500" aria-hidden="true" />
                </div>
                <CardTitle
                  role="heading"
                  aria-level={1}
                  className="mt-4 text-xl font-semibold tracking-tight"
                >
                  This link has expired
                </CardTitle>
                <CardDescription className="max-w-xs text-balance">
                  The share you&rsquo;re looking for has expired or reached its download limit.
                </CardDescription>
              </CardHeader>
              <CardContent className="text-center">
                <p className="text-xs text-muted-foreground">
                  Reach out to the sender &mdash; they can generate a fresh link.
                </p>
              </CardContent>
            </Card>
          )}

          {!loading && passwordRequired && !shareInfo && (
            <Card className="animate-in fade-in slide-in-from-bottom-2 duration-300 ease-out">
              <CardHeader className="items-center gap-2 pb-2 text-center">
                <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-primary/10">
                  <Lock className="h-8 w-8 text-primary" aria-hidden="true" />
                </div>
                <CardTitle
                  role="heading"
                  aria-level={1}
                  className="mt-4 text-xl font-semibold tracking-tight"
                >
                  Password protected
                </CardTitle>
                <CardDescription className="max-w-xs text-balance">
                  Enter the password the sender shared with you to view this file.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <form
                  className="space-y-5"
                  onSubmit={(e) => {
                    e.preventDefault();
                    if (password) resolve(password);
                  }}
                >
                  <div className="space-y-2">
                    <Label htmlFor="share-password">Password</Label>
                    <Input
                      id="share-password"
                      type="password"
                      autoFocus
                      autoComplete="off"
                      placeholder="Enter password"
                      value={password}
                      onChange={(e) => {
                        setPassword(e.target.value);
                        if (passwordError) setPasswordError(false);
                      }}
                      aria-invalid={passwordError || undefined}
                      aria-describedby={passwordError ? "share-password-error" : undefined}
                      className={cn(
                        "h-10",
                        passwordError && "border-destructive focus-visible:ring-destructive/20",
                      )}
                    />
                    <p
                      id="share-password-error"
                      role="alert"
                      aria-live="polite"
                      className={cn(
                        "flex min-h-4 items-center gap-1.5 text-xs text-destructive transition-opacity duration-200",
                        passwordError ? "opacity-100" : "opacity-0",
                      )}
                    >
                      {passwordError && (
                        <>
                          <CircleAlert className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                          <span>Incorrect password. Try again.</span>
                        </>
                      )}
                    </p>
                  </div>
                  <Button
                    type="submit"
                    size="lg"
                    className="w-full"
                    disabled={!password || loading}
                  >
                    {loading ? (
                      <>
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
                        Verifying&hellip;
                      </>
                    ) : (
                      <>
                        <Lock className="mr-2 h-4 w-4" aria-hidden="true" />
                        Unlock
                      </>
                    )}
                  </Button>
                </form>
              </CardContent>
            </Card>
          )}

          {!loading && shareInfo && (
            <Card className="overflow-hidden animate-in fade-in slide-in-from-bottom-2 duration-300 ease-out">
              <CardHeader className="items-center gap-2 pb-4 text-center">
                <div className="flex h-20 w-20 items-center justify-center rounded-2xl bg-muted ring-1 ring-foreground/5">
                  <FileIcon mime={shareInfo.mime_type ?? ""} className="h-10 w-10" />
                </div>
                <CardTitle
                  role="heading"
                  aria-level={1}
                  className="mt-4 max-w-full break-all px-2 text-xl font-semibold leading-snug tracking-tight"
                >
                  {shareInfo.file_name ?? "Shared file"}
                </CardTitle>
                <CardDescription className="flex flex-wrap items-center justify-center gap-x-2 gap-y-0.5 text-xs">
                  {shareInfo.mime_type && <span>{mimeToLabel(shareInfo.mime_type)}</span>}
                  {shareInfo.mime_type && shareInfo.file_size !== undefined && (
                    <span aria-hidden="true">&middot;</span>
                  )}
                  {shareInfo.file_size !== undefined && (
                    <span>{formatBytes(shareInfo.file_size)}</span>
                  )}
                </CardDescription>
              </CardHeader>

              <CardContent className="space-y-4">
                <Button
                  size="lg"
                  className="w-full transition-transform duration-150 hover:bg-primary/90 active:scale-[0.99]"
                  onClick={handleDownload}
                  disabled={downloading}
                  aria-label={`Download ${shareInfo.file_name ?? "file"}`}
                >
                  {downloading ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
                      Downloading&hellip;
                    </>
                  ) : (
                    <>
                      <Download className="mr-2 h-4 w-4" aria-hidden="true" />
                      Download
                    </>
                  )}
                </Button>

                {(shareInfo.expires_at || shareInfo.download_limit !== undefined) && (
                  <>
                    <Separator />
                    <dl className="space-y-2 text-xs text-muted-foreground">
                      {shareInfo.expires_at && (
                        <div className="flex items-center gap-2">
                          <Clock className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                          <dt className="sr-only">Expires</dt>
                          <dd>Expires {formatServerDateTime(shareInfo.expires_at)}</dd>
                        </div>
                      )}
                      {shareInfo.download_limit !== undefined && (
                        <div className="flex items-center gap-2">
                          <Download className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                          <dt className="sr-only">Downloads remaining</dt>
                          <dd>
                            {shareInfo.downloads_remaining ?? shareInfo.download_limit} of{" "}
                            {shareInfo.download_limit} downloads remaining
                          </dd>
                        </div>
                      )}
                    </dl>
                  </>
                )}
              </CardContent>
            </Card>
          )}
        </div>
      </main>

      {/* Footer */}
      <footer className="w-full px-4 py-6 sm:px-6">
        <div className="mx-auto flex max-w-5xl items-center justify-center text-xs text-muted-foreground">
          <Link
            href="/"
            className="inline-flex items-center gap-1.5 rounded-md px-2 py-1 outline-none transition-colors hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50"
          >
            <Cloud className="h-3 w-3" aria-hidden="true" />
            <span>
              Powered by <span className="font-medium">MyCloud</span>
            </span>
          </Link>
        </div>
      </footer>
    </div>
  );
}
