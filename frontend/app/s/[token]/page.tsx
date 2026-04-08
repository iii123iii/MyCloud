"use client";

import { useState, useEffect, use } from "react";
import { shares as sharesApi } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Download, Cloud, Lock, FileText } from "lucide-react";
import { formatBytes } from "@/lib/format";
import { toast } from "sonner";

// Next.js 16: params is a Promise
interface PageProps {
  params: Promise<{ token: string }>;
}

export default function PublicSharePage({ params }: PageProps) {
  const { token } = use(params);
  const [password, setPassword] = useState("");
  const [passwordRequired, setPasswordRequired] = useState(false);
  const [shareInfo, setShareInfo] = useState<{
    permission: string;
    file_id?: string;
    file_name?: string;
    file_size?: number;
    mime_type?: string;
  } | null>(null);
  const [loading, setLoading] = useState(false);
  const [notFound, setNotFound] = useState(false);

  const resolve = async (pwd?: string) => {
    setLoading(true);
    setNotFound(false);
    try {
      const info = await sharesApi.resolve(token, pwd);
      setShareInfo(info);
      setPasswordRequired(false);
    } catch (err: unknown) {
      // Check if password is required
      if (err && typeof err === "object" && "status" in err && (err as { status: number }).status === 401) {
        setPasswordRequired(true);
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
    const url = sharesApi.downloadUrl(token);
    try {
      // Can't easily set custom header on anchor—use fetch + blob
      const res = await fetch(url, password ? { headers: { "X-Share-Password": password } } : undefined);
      if (!res.ok) throw new Error("Download failed");
      const blob = await res.blob();
      const blobUrl = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = blobUrl;
      a.download = shareInfo?.file_name ?? "download";
      a.click();
      URL.revokeObjectURL(blobUrl);
    } catch {
      toast.error("Download failed");
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <div className="w-full max-w-md space-y-4">
        <div className="flex items-center justify-center gap-2">
          <Cloud className="h-6 w-6" />
          <span className="text-xl font-bold">MyCloud</span>
        </div>

        {loading && (
          <Card>
            <CardContent className="py-8 text-center text-muted-foreground">Loading share…</CardContent>
          </Card>
        )}

        {notFound && (
          <Card>
            <CardContent className="py-8 text-center">
              <p className="font-medium">Share not found</p>
              <p className="text-sm text-muted-foreground mt-1">
                This link may have expired or been deleted.
              </p>
            </CardContent>
          </Card>
        )}

        {passwordRequired && !shareInfo && (
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Lock className="h-4 w-4" />
                Password required
              </CardTitle>
              <CardDescription>This share is protected. Enter the password to access it.</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-3">
                <div className="space-y-1.5">
                  <Label htmlFor="share-password">Password</Label>
                  <Input
                    id="share-password"
                    type="password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    onKeyDown={(e) => e.key === "Enter" && resolve(password)}
                  />
                </div>
                <Button className="w-full" onClick={() => resolve(password)}>
                  Access share
                </Button>
              </div>
            </CardContent>
          </Card>
        )}

        {shareInfo && (
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <FileText className="h-4 w-4" />
                Shared file
              </CardTitle>
              <CardDescription>Someone shared a file with you via MyCloud.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="border rounded-lg p-4 space-y-1">
                <p className="font-medium truncate">{shareInfo.file_name ?? "Unknown file"}</p>
                {shareInfo.file_size !== undefined && (
                  <p className="text-sm text-muted-foreground">{formatBytes(shareInfo.file_size)}</p>
                )}
                {shareInfo.mime_type && (
                  <p className="text-xs text-muted-foreground">{shareInfo.mime_type}</p>
                )}
              </div>
              <Button className="w-full" onClick={handleDownload}>
                <Download className="h-4 w-4 mr-2" />
                Download
              </Button>
            </CardContent>
          </Card>
        )}
      </div>
    </div>
  );
}
