"use client";

import { useState } from "react";
import useSWR from "swr";
import { Camera, Loader2 } from "lucide-react";

import { photos as photosApi, tokenStore, type Photo } from "@/lib/api";
import { PreviewModal } from "@/components/modals/PreviewModal";
import { Skeleton } from "@/components/ui/skeleton";
import { parseServerDate } from "@/lib/format";
import type { FileItem } from "@/lib/types";

// Group photos by month (e.g. "2026-05").
function groupByMonth(photos: Photo[]): Array<{ key: string; label: string; items: Photo[] }> {
  const map = new Map<string, Photo[]>();
  for (const p of photos) {
    const dt = parseServerDate(p.shot_at || p.created_at);
    const key = `${dt.getUTCFullYear()}-${String(dt.getUTCMonth() + 1).padStart(2, "0")}`;
    const bucket = map.get(key) ?? [];
    bucket.push(p);
    map.set(key, bucket);
  }
  return Array.from(map.entries())
    .sort((a, b) => (a[0] < b[0] ? 1 : -1))
    .map(([key, items]) => {
      const [y, m] = key.split("-");
      const label = new Date(Number(y), Number(m) - 1).toLocaleString(undefined, {
        year: "numeric", month: "long",
      });
      return { key, label, items };
    });
}

export default function PhotosPage() {
  const { data, isLoading } = useSWR("photos", () => photosApi.list());
  const [preview, setPreview] = useState<FileItem | null>(null);

  if (isLoading) {
    return (
      <div className="p-6 grid grid-cols-2 sm:grid-cols-4 md:grid-cols-6 gap-2">
        {Array.from({ length: 18 }).map((_, i) => <Skeleton key={i} className="aspect-square rounded" />)}
      </div>
    );
  }
  const groups = groupByMonth(data?.photos ?? []);

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center gap-2">
        <Camera className="h-5 w-5" />
        <h1 className="text-xl font-semibold">Photos</h1>
      </div>

      {!groups.length && (
        <p className="text-sm text-muted-foreground">No images yet. Upload some.</p>
      )}

      {groups.map((g) => (
        <section key={g.key} className="space-y-2">
          <h2 className="text-sm font-medium text-muted-foreground">{g.label}</h2>
          <div className="grid grid-cols-2 sm:grid-cols-4 md:grid-cols-6 gap-2">
            {g.items.map((p) => (
              <button
                key={p.id}
                onClick={() => setPreview({
                  id: p.id, name: p.name, size_bytes: p.size_bytes,
                  mime_type: p.mime_type, is_starred: false,
                  created_at: p.created_at, updated_at: p.created_at,
                })}
                className="aspect-square overflow-hidden rounded bg-muted hover:opacity-90 focus:outline-none focus:ring-2 focus:ring-ring"
              >
                <ThumbImage photoId={p.id} alt={p.name} />
              </button>
            ))}
          </div>
        </section>
      ))}

      {preview && (
        <PreviewModal
          file={preview}
          open={!!preview}
          onOpenChange={(o) => !o && setPreview(null)}
        />
      )}
    </div>
  );
}

// ThumbImage fetches the encrypted thumb URL with the auth header and exposes
// it as a blob URL. While loading it occupies the same square so the grid
// doesn't reflow when the image lands.
function ThumbImage({ photoId, alt }: { photoId: string; alt: string }) {
  const { data } = useSWR(["thumb", photoId], async () => {
    const url = photosApi.thumbUrl(photoId);
    const token = tokenStore.getAccess();
    const res = await fetch(url, token ? { headers: { Authorization: `Bearer ${token}` } } : undefined);
    if (!res.ok) return null;
    const blob = await res.blob();
    return URL.createObjectURL(blob);
  });
  if (data === undefined) {
    return (
      <div className="w-full h-full flex items-center justify-center">
        <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
      </div>
    );
  }
  if (!data) return <div className="w-full h-full" />;
  // eslint-disable-next-line @next/next/no-img-element
  return <img src={data} alt={alt} className="w-full h-full object-cover" draggable={false} />;
}
