"use client";

// Route-segment error boundary for /(app)/* — catches uncaught render
// errors so the user sees an actionable message instead of a blank screen
// or the Next dev overlay. The reset() callback is provided by Next and
// re-renders the segment after the developer fixes the issue (in dev) or
// the user retries (in prod).
import { useEffect } from "react";
import { Button } from "@/components/ui/button";
import { AlertCircle } from "lucide-react";

export default function ErrorBoundary({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    // eslint-disable-next-line no-console
    console.error("Route error:", error);
  }, [error]);

  return (
    <div className="flex h-full w-full items-center justify-center p-12">
      <div className="flex max-w-md flex-col items-center gap-4 text-center">
        <div className="rounded-full bg-destructive/10 p-3 text-destructive">
          <AlertCircle className="size-8" />
        </div>
        <h2 className="text-lg font-semibold">Something went wrong</h2>
        <p className="text-sm text-muted-foreground">
          {error.message || "An unexpected error occurred while rendering this page."}
        </p>
        {error.digest ? (
          <p className="text-xs text-muted-foreground">
            Reference: <code className="font-mono">{error.digest}</code>
          </p>
        ) : null}
        <div className="flex gap-2">
          <Button onClick={() => reset()}>Try again</Button>
          <Button variant="outline" onClick={() => (window.location.href = "/dashboard")}>
            Back to dashboard
          </Button>
        </div>
      </div>
    </div>
  );
}
