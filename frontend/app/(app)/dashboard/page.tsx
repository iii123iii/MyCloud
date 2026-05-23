// Server component shell — renders skeleton + chrome on the server, hydrates
// only the FileExplorer island on the client. Previously marked "use client"
// even though every interactive piece lives inside FileExplorer; that
// shipped this whole skeleton's component tree to the client bundle for no
// benefit. The cn() import is server-safe (it's just clsx).
import { Suspense } from "react";
import { FileExplorer } from "@/components/file-explorer/FileExplorer";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

export default function DashboardPage() {
  return (
    <div className="p-4 sm:p-6 lg:p-8">
      {/* Visually hidden landmark heading so screen readers announce the
          page title even though FileExplorer renders its own toolbar. */}
      <h1 className="sr-only">My files</h1>
      <Suspense fallback={<DashboardSkeleton />}>
        {/* Subtle fade/slide-in so the explorer doesn't pop in jarringly
            when the suspended URL state resolves. Honors prefers-reduced-motion
            via the motion-safe: variants on the animation utilities. */}
        <div
          className={cn(
            "motion-safe:animate-in motion-safe:fade-in motion-safe:slide-in-from-bottom-1",
            "motion-safe:duration-300 motion-safe:ease-out",
          )}
        >
          <FileExplorer />
        </div>
      </Suspense>
    </div>
  );
}

// Skeleton that mirrors FileExplorer's actual structure (toolbar + breadcrumb,
// a folders row, then a files grid) so the layout shift on hydration is
// minimal. The dimensions roughly match real cards (p-3 rounded-xl, gap-3).
function DashboardSkeleton() {
  return (
    <div
      className={cn(
        "space-y-6 sm:space-y-8",
        "motion-safe:animate-in motion-safe:fade-in motion-safe:duration-300",
      )}
      role="status"
      aria-busy="true"
      aria-live="polite"
      aria-label="Loading your files"
    >
      {/* Screen-reader-only status so assistive tech announces the load. */}
      <span className="sr-only">Loading your files, please wait.</span>

      {/* Toolbar: breadcrumb (left) + search/view-toggle/actions (right). */}
      <div className="flex flex-wrap items-center gap-2" aria-hidden="true">
        <div className="flex items-center gap-2">
          <Skeleton className="h-5 w-16 rounded" />
          <Skeleton className="h-3 w-12 rounded ml-1" />
        </div>
        <div className="ml-auto flex items-center gap-2">
          <Skeleton className="h-9 w-56 rounded-md" />
          <Skeleton className="h-6 w-px" />
          <Skeleton className="h-8 w-8 rounded-md" />
          <Skeleton className="h-8 w-8 rounded-md" />
          <Skeleton className="h-9 w-28 rounded-md hidden sm:block" />
          <Skeleton className="h-9 w-24 rounded-md" />
        </div>
      </div>

      {/* Folders section. */}
      <section className="space-y-3" aria-hidden="true">
        <Skeleton className="h-4 w-20 rounded" />
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
          {Array.from({ length: 4 }).map((_, i) => (
            <FolderCardSkeleton key={i} />
          ))}
        </div>
      </section>

      {/* Files section. */}
      <section className="space-y-3" aria-hidden="true">
        <Skeleton className="h-4 w-16 rounded" />
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
          {Array.from({ length: 12 }).map((_, i) => (
            <FileCardSkeleton key={i} />
          ))}
        </div>
      </section>
    </div>
  );
}

function FolderCardSkeleton() {
  return (
    <div
      className={cn(
        "flex flex-col gap-2 rounded-xl border border-border/60 bg-card/40 p-3",
        "transition-colors duration-200",
      )}
    >
      <div className="flex items-center gap-2">
        <Skeleton className="h-5 w-5 shrink-0 rounded" />
        <Skeleton className="h-3.5 max-w-[70%] flex-1 rounded" />
      </div>
      <Skeleton className="h-2.5 w-12 rounded" />
    </div>
  );
}

function FileCardSkeleton() {
  return (
    <div
      className={cn(
        "flex flex-col gap-2 rounded-xl border border-border/60 bg-card/40 p-3",
        "transition-colors duration-200",
      )}
    >
      {/* Icon + name row. */}
      <div className="flex items-center gap-2">
        <Skeleton className="h-5 w-5 shrink-0 rounded" />
        <Skeleton className="h-3.5 max-w-[75%] flex-1 rounded" />
      </div>
      {/* Thumbnail area. */}
      <Skeleton className="aspect-[4/3] w-full rounded-md" />
      {/* Metadata row. */}
      <div className="flex items-center justify-between">
        <Skeleton className="h-2.5 w-14 rounded" />
        <Skeleton className="h-2.5 w-10 rounded" />
      </div>
    </div>
  );
}
