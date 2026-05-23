"use client";

// Empty folder onboarding.
//
// Replaces a bare "This folder is empty" with an actionable, animated
// drag-drop hint plus CTAs that open the platform file picker / new folder
// dialog. Designed to live inside the existing FileGrid empty branch.

import { useEffect, useRef } from "react";
import {
  UploadCloud,
  FolderPlus,
  Sparkles,
  MousePointerClick,
  SearchX,
  TriangleAlert,
  FolderOpen,
  RefreshCw,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

type Variant = "empty" | "no-results" | "error" | "drag";

type Props = {
  onUpload?: () => void;
  onNewFolder?: () => void;
  // hint customises the heading. Defaults to "This folder is empty".
  heading?: string;
  // Variant tunes copy + iconography for the four canonical states this
  // surface handles: a fresh empty folder, an empty search/filter result,
  // a fetch error, or a drag-target preview. Defaults to "empty" so
  // existing call sites (FileGrid) keep their current look untouched.
  variant?: Variant;
  // Optional override body copy — useful for error variants that want to
  // surface a specific message from the API.
  description?: string;
  // Retry handler for the error variant. When provided, swaps the default
  // CTA pair for a single "Try again" button.
  onRetry?: () => void;
  className?: string;
};

// Per-variant icon + copy. Keeping this as a local map (rather than four
// JSX branches) keeps the markup below a single, scannable tree.
const VARIANTS: Record<
  Variant,
  {
    Icon: typeof UploadCloud;
    heading: string;
    description: string;
    tone: "primary" | "muted" | "destructive";
  }
> = {
  empty: {
    Icon: UploadCloud,
    heading: "This folder is empty",
    description: "Drop files here to upload, or start with a new folder.",
    tone: "primary",
  },
  "no-results": {
    Icon: SearchX,
    heading: "No matches found",
    description:
      "Try a different search term or clear your filters to see more files.",
    tone: "muted",
  },
  error: {
    Icon: TriangleAlert,
    heading: "Something went wrong",
    description:
      "We couldn’t load this folder. Check your connection and try again.",
    tone: "destructive",
  },
  drag: {
    Icon: FolderOpen,
    heading: "Drop to upload",
    description: "Release your files anywhere on this page to add them here.",
    tone: "primary",
  },
};

export function EmptyState({
  onUpload,
  onNewFolder,
  heading,
  variant = "empty",
  description,
  onRetry,
  className,
}: Props) {
  const { Icon, heading: defaultHeading, description: defaultDescription, tone } =
    VARIANTS[variant];

  // When this surface replaces a list (the default behaviour for FileGrid),
  // move focus to the heading so screen readers announce the transition
  // and keyboard users land somewhere meaningful instead of <body>.
  const headingRef = useRef<HTMLHeadingElement | null>(null);
  useEffect(() => {
    headingRef.current?.focus({ preventScroll: true });
  }, [variant]);

  const isError = variant === "error";
  const isDrag = variant === "drag";
  const showActions = !isError && (onUpload || onNewFolder);

  // Tone maps to the halo/ring colour family. Keeping it data-driven means
  // adding future variants is a one-line change in the table above.
  const toneRing =
    tone === "destructive"
      ? "border-destructive/40"
      : tone === "muted"
        ? "border-muted-foreground/30"
        : "border-primary/40";
  const toneHalo =
    tone === "destructive"
      ? "from-destructive/15 via-destructive/5 to-transparent"
      : tone === "muted"
        ? "from-muted-foreground/15 via-muted-foreground/5 to-transparent"
        : "from-primary/15 via-primary/5 to-transparent";
  const toneIcon =
    tone === "destructive"
      ? "text-destructive"
      : tone === "muted"
        ? "text-muted-foreground"
        : "text-primary";
  const toneSparkle =
    tone === "destructive" ? "text-destructive/70" : "text-primary/80";

  return (
    <div
      role={isError ? "alert" : "status"}
      aria-live={isError ? "assertive" : "polite"}
      aria-atomic="true"
      className={cn(
        "mc-empty-fade-in flex min-h-[60vh] flex-col items-center justify-center gap-1 px-6 py-20 text-center text-muted-foreground",
        isDrag &&
          "rounded-xl border-2 border-dashed border-primary/50 bg-primary/[0.03]",
        className,
      )}
    >
      {/* Scoped keyframes — keeps the bundle lean (no framer-motion) while
          still giving the icon a gentle floating breath. */}
      <style>{`
        @keyframes mc-empty-float {
          0%, 100% { transform: translateY(0); }
          50%      { transform: translateY(-8px); }
        }
        @keyframes mc-empty-pulse {
          0%, 100% { opacity: 0.35; transform: scale(1); }
          50%      { opacity: 0.6;  transform: scale(1.08); }
        }
        @keyframes mc-empty-sparkle {
          0%, 100% { opacity: 0.35; transform: rotate(0deg) scale(1); }
          50%      { opacity: 1;    transform: rotate(12deg) scale(1.15); }
        }
        @keyframes mc-empty-fade-in {
          0%   { opacity: 0; transform: translateY(6px); }
          100% { opacity: 1; transform: translateY(0); }
        }
        .mc-empty-fade-in { animation: mc-empty-fade-in 320ms ease-out both; }
        @media (prefers-reduced-motion: reduce) {
          .mc-empty-float,
          .mc-empty-pulse,
          .mc-empty-sparkle,
          .mc-empty-fade-in { animation: none !important; }
        }
      `}</style>

      {/* Illustrative icon cluster: a soft gradient halo, dashed pulsing ring,
          floating cloud, and two twinkling sparkles to feel alive. */}
      <div
        className="relative mb-8 size-36 mc-empty-float"
        style={{ animation: "mc-empty-float 4.5s ease-in-out infinite" }}
        aria-hidden="true"
      >
        {/* Soft radial halo */}
        <span
          className={cn(
            "absolute inset-0 rounded-full bg-gradient-to-br blur-xl",
            toneHalo,
          )}
        />

        {/* Pulsing dashed ring */}
        <span
          className={cn(
            "absolute inset-2 rounded-full border-2 border-dashed mc-empty-pulse",
            toneRing,
          )}
          style={{ animation: "mc-empty-pulse 3s ease-in-out infinite" }}
        />

        {/* Inner solid disc as a backdrop for the icon */}
        <span className="absolute inset-5 rounded-full bg-background shadow-sm ring-1 ring-border/60" />

        {/* The hero icon */}
        <Icon
          className={cn(
            "absolute inset-0 m-auto size-16 drop-shadow-sm",
            toneIcon,
          )}
          strokeWidth={1.5}
          aria-hidden="true"
        />

        {/* Twinkling sparkles — only on the cheerful variants. */}
        {tone !== "destructive" && (
          <>
            <Sparkles
              className={cn(
                "absolute -top-1 -right-1 size-5 mc-empty-sparkle",
                toneSparkle,
              )}
              style={{ animation: "mc-empty-sparkle 2.8s ease-in-out infinite" }}
              aria-hidden="true"
            />
            <Sparkles
              className={cn(
                "absolute bottom-1 -left-2 size-4 mc-empty-sparkle",
                "text-primary/60",
              )}
              style={{
                animation: "mc-empty-sparkle 3.4s ease-in-out 0.6s infinite",
              }}
              aria-hidden="true"
            />
          </>
        )}
      </div>

      <h3
        ref={headingRef}
        tabIndex={-1}
        className="mb-2 text-xl font-semibold tracking-tight text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:ring-offset-2 focus-visible:ring-offset-background rounded-sm sm:text-2xl"
      >
        {heading ?? defaultHeading}
      </h3>
      <p className="max-w-md text-sm leading-relaxed text-muted-foreground sm:text-base">
        {description ?? defaultDescription}
      </p>

      {(showActions || isError) && (
        <div className="mt-8 flex flex-col gap-2 sm:flex-row sm:gap-3">
          {isError && onRetry && (
            <Button
              size="lg"
              onClick={onRetry}
              className="h-11 px-6 text-sm font-medium shadow-sm transition-all hover:-translate-y-0.5 hover:shadow-md focus-visible:-translate-y-0.5 focus-visible:shadow-md"
              aria-label="Retry loading this folder"
            >
              <RefreshCw className="mr-2 size-4" aria-hidden="true" />
              Try again
            </Button>
          )}
          {showActions && onUpload && (
            <Button
              size="lg"
              onClick={onUpload}
              className="h-11 px-6 text-sm font-medium shadow-sm transition-all hover:-translate-y-0.5 hover:shadow-md focus-visible:-translate-y-0.5 focus-visible:shadow-md"
              aria-label="Upload files to this folder"
            >
              <UploadCloud className="mr-2 size-4" aria-hidden="true" />
              Upload files
            </Button>
          )}
          {showActions && onNewFolder && (
            <Button
              variant="outline"
              size="lg"
              onClick={onNewFolder}
              className="h-11 px-6 text-sm font-medium transition-all hover:-translate-y-0.5 hover:bg-muted focus-visible:-translate-y-0.5"
              aria-label="Create a new folder here"
            >
              <FolderPlus className="mr-2 size-4" aria-hidden="true" />
              New folder
            </Button>
          )}
        </div>
      )}

      {variant === "empty" && (
        <p className="mt-7 inline-flex items-center gap-1.5 text-xs text-muted-foreground/80">
          <MousePointerClick className="size-3.5" aria-hidden="true" />
          Tip: you can drag &amp; drop files or folders anywhere on this page.
        </p>
      )}
    </div>
  );
}
