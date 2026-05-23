// Route-segment loading boundary for /(app)/* — shown while a page's
// async setup (suspense-aware data, lazy-loaded chunks) is still resolving.
// Replaces the per-page hand-rolled skeleton that every dashboard route
// reinvented. Pages can still override locally if they want a tailored
// fallback.
export default function Loading() {
  return (
    <div
      className="flex h-full w-full items-center justify-center p-12"
      role="status"
      aria-live="polite"
    >
      <div className="flex flex-col items-center gap-3 text-muted-foreground">
        <div className="size-8 animate-spin rounded-full border-2 border-current border-t-transparent" />
        <p className="text-sm">Loading…</p>
      </div>
    </div>
  );
}
