"use client";

import { useState, useEffect, use, useRef } from "react";
import { requests as requestsApi } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Cloud, Lock, Upload, FolderUp, CheckCircle2, AlertCircle } from "lucide-react";
import { toast } from "sonner";
import { formatBytes, formatServerDateTime } from "@/lib/format";

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

export default function PublicUploadRequestPage({ params }: PageProps) {
  const { token } = use(params);
  const [password, setPassword] = useState("");
  const [passwordRequired, setPasswordRequired] = useState(false);
  const [info, setInfo] = useState<RequestInfo | null>(null);
  const [loading, setLoading] = useState(false);
  const [notFound, setNotFound] = useState(false);
  const [gone, setGone] = useState(false);
  const [uploads, setUploads] = useState<{ name: string; size: number; status: "uploading" | "done" | "error"; error?: string }[]>([]);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const resolve = async (pwd?: string) => {
    setLoading(true);
    setNotFound(false);
    setGone(false);
    try {
      const res = await requestsApi.resolve(token, pwd);
      setInfo(res);
      setPasswordRequired(false);
    } catch (err: unknown) {
      if (err && typeof err === "object" && "status" in err) {
        const status = (err as { status: number }).status;
        if (status === 401) setPasswordRequired(true);
        else if (status === 410) setGone(true);
        else setNotFound(true);
      } else {
        setNotFound(true);
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { resolve(); }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const uploadFile = async (file: File): Promise<void> => {
    const idx = uploads.length;
    setUploads((u) => [...u, { name: file.name, size: file.size, status: "uploading" }]);
    const form = new FormData();
    form.append("file", file);
    try {
      const headers: Record<string, string> = {};
      if (password) headers["X-Share-Password"] = password;
      const res = await fetch(requestsApi.uploadUrl(token), {
        method: "POST",
        headers,
        body: form,
      });
      if (!res.ok) {
        const body = await res.json().catch(() => null);
        const message = body?.error?.message ?? `Upload failed (${res.status})`;
        setUploads((u) => u.map((it, i) => i === idx ? { ...it, status: "error", error: message } : it));
        if (res.status === 410) setGone(true);
        return;
      }
      setUploads((u) => u.map((it, i) => i === idx ? { ...it, status: "done" } : it));
    } catch {
      setUploads((u) => u.map((it, i) => i === idx ? { ...it, status: "error", error: "Network error" } : it));
    }
  };

  const onFilesPicked = async (list: FileList | null) => {
    if (!list || !list.length) return;
    for (const f of Array.from(list)) {
      await uploadFile(f);
    }
    // Refresh remaining counter
    await resolve(password || undefined);
    toast.success("Upload complete");
  };

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <div className="w-full max-w-md space-y-4">
        <div className="flex items-center justify-center gap-2">
          <Cloud className="h-6 w-6" />
          <span className="text-xl font-bold">MyCloud</span>
        </div>

        {loading && (
          <Card><CardContent className="py-8 text-center text-muted-foreground">Loading…</CardContent></Card>
        )}

        {notFound && (
          <Card>
            <CardContent className="py-8 text-center">
              <p className="font-medium">Upload link not found</p>
              <p className="text-sm text-muted-foreground mt-1">This link may have been deleted.</p>
            </CardContent>
          </Card>
        )}

        {gone && (
          <Card>
            <CardContent className="py-8 text-center">
              <p className="font-medium">Upload link unavailable</p>
              <p className="text-sm text-muted-foreground mt-1">This link has expired or reached its limit.</p>
            </CardContent>
          </Card>
        )}

        {passwordRequired && !info && (
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Lock className="h-4 w-4" /> Password required
              </CardTitle>
              <CardDescription>This upload link is protected.</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-3">
                <div className="space-y-1.5">
                  <Label htmlFor="upreq-password">Password</Label>
                  <Input id="upreq-password" type="password" value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    onKeyDown={(e) => e.key === "Enter" && resolve(password)} />
                </div>
                <Button className="w-full" onClick={() => resolve(password)}>Continue</Button>
              </div>
            </CardContent>
          </Card>
        )}

        {info && (
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <FolderUp className="h-4 w-4" /> Upload files
              </CardTitle>
              <CardDescription>
                Files will be uploaded {info.folder_name ? `to "${info.folder_name}"` : "to the requester"}.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {(info.expires_at || info.max_files) && (
                <div className="text-xs text-muted-foreground space-y-0.5 border rounded p-3">
                  {info.expires_at && <p>Expires {formatServerDateTime(info.expires_at)}</p>}
                  {info.max_files !== undefined && (
                    <p>{info.uploads_remaining ?? info.max_files} of {info.max_files} uploads remaining</p>
                  )}
                </div>
              )}

              <input
                ref={fileInputRef}
                type="file"
                multiple
                className="hidden"
                onChange={(e) => onFilesPicked(e.target.files)}
              />
              <Button className="w-full" onClick={() => fileInputRef.current?.click()}>
                <Upload className="h-4 w-4 mr-2" /> Choose files
              </Button>

              {uploads.length > 0 && (
                <div className="border rounded divide-y">
                  {uploads.map((u, i) => (
                    <div key={i} className="flex items-center justify-between px-3 py-2 text-xs gap-2">
                      <div className="flex items-center gap-2 min-w-0">
                        {u.status === "done" && <CheckCircle2 className="h-3.5 w-3.5 text-green-600 shrink-0" />}
                        {u.status === "error" && <AlertCircle className="h-3.5 w-3.5 text-destructive shrink-0" />}
                        <span className="truncate">{u.name}</span>
                      </div>
                      <span className="text-muted-foreground shrink-0">
                        {u.status === "uploading" ? "Uploading…" : u.status === "done" ? formatBytes(u.size) : (u.error ?? "Failed")}
                      </span>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        )}
      </div>
    </div>
  );
}
