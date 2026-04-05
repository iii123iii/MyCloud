"use client";

import { useRef, useEffect } from "react";
import useSWRInfinite from "swr/infinite";
import { files as filesApi } from "@/lib/api";
import { FileRow } from "@/components/file-explorer/FileRow";
import { Skeleton } from "@/components/ui/skeleton";
import { Star, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";

const PAGE_SIZE = 50;

export default function StarredPage() {
  const {
    data, size, setSize, isValidating, mutate,
  } = useSWRInfinite(
    (i) => `starred-p${i}`,
    (key) => {
      const page = parseInt(key.split("-p").pop()!) + 1;
      return filesApi.list({ sort: "updated_at", order: "desc", all: true, page, page_size: PAGE_SIZE });
    },
  );

  // Client-side filter for starred (backend doesn't have a starred-only filter yet)
  const allFiles = data ? data.flatMap((d) => d.files).filter((f) => f.is_starred) : [];
  const hasMore  = data ? data[data.length - 1]?.has_more ?? false : false;
  const loading  = !data && isValidating;

  const sentinelRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!sentinelRef.current || !hasMore) return;
    const obs = new IntersectionObserver(
      ([e]) => { if (e.isIntersecting && !isValidating) setSize((s) => s + 1); },
      { rootMargin: "200px" }
    );
    obs.observe(sentinelRef.current);
    return () => obs.disconnect();
  }, [hasMore, isValidating, setSize]);

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center gap-2">
        <Star className="h-5 w-5" />
        <h1 className="text-xl font-semibold">Starred</h1>
      </div>

      {loading && (
        <div className="space-y-2">
          {Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-12 rounded" />)}
        </div>
      )}

      {!loading && allFiles.length === 0 && (
        <p className="text-muted-foreground text-sm">
          No starred files. Star a file to find it quickly here.
        </p>
      )}

      {allFiles.length > 0 && (
        <div className="border rounded-lg divide-y">
          {allFiles.map((file) => (
            <FileRow key={file.id} file={file} onMutate={mutate} />
          ))}
        </div>
      )}

      {hasMore && (
        <div ref={sentinelRef} className="flex justify-center py-4">
          {isValidating ? (
            <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
          ) : (
            <Button variant="ghost" size="sm" onClick={() => setSize((s) => s + 1)}>
              Load more
            </Button>
          )}
        </div>
      )}
    </div>
  );
}
