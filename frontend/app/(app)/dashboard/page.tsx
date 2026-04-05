"use client";

import { Suspense } from "react";
import { FileExplorer } from "@/components/file-explorer/FileExplorer";
import { Skeleton } from "@/components/ui/skeleton";

export default function DashboardPage() {
  return (
    <div className="p-4 sm:p-6">
      <Suspense fallback={<DashboardSkeleton />}>
        <FileExplorer />
      </Suspense>
    </div>
  );
}

function DashboardSkeleton() {
  return (
    <div className="space-y-4 animate-in fade-in duration-300">
      {/* Toolbar skeleton */}
      <div className="flex items-center justify-between">
        <Skeleton className="h-7 w-40" />
        <div className="flex items-center gap-2">
          <Skeleton className="h-9 w-48" />
          <Skeleton className="h-9 w-9" />
          <Skeleton className="h-9 w-9" />
          <Skeleton className="h-9 w-28" />
          <Skeleton className="h-9 w-24" />
        </div>
      </div>
      {/* Grid skeleton */}
      <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-3">
        {Array.from({ length: 18 }).map((_, i) => (
          <Skeleton key={i} className="h-28 rounded-lg" />
        ))}
      </div>
    </div>
  );
}
