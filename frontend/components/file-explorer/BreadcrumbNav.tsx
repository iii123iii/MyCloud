"use client";

import { Fragment, useState, type KeyboardEvent } from "react";
import {
  Breadcrumb, BreadcrumbItem, BreadcrumbLink, BreadcrumbList,
  BreadcrumbPage, BreadcrumbSeparator, BreadcrumbEllipsis,
} from "@/components/ui/breadcrumb";
import { Home, Copy, Check, ChevronRight, Users } from "lucide-react";
import { toast } from "sonner";
import { cn } from "@/lib/utils";
import type { FolderItem } from "@/lib/types";

interface Props {
  path: FolderItem[];
  onNavigate: (index: number) => void;
  // When set, the home crumb renders this label and links here (e.g. "Shared
  // with me" → /shared) instead of the owner's "My Files" root. onNavigate(-1)
  // is wired by the parent to route to rootCrumb.href in that case.
  rootCrumb?: { label: string; href: string };
}

// Smaller, elegant chevron used as the separator between breadcrumb segments.
// `extraClass` lets callers tack on responsive utilities like `hidden sm:flex`
// without overriding the base sizing/colour.
function ElegantSeparator({ extraClass }: { extraClass?: string }) {
  return (
    <BreadcrumbSeparator
      className={cn(
        // Subtle separator colour with a smooth fade so transitions on path
        // change don't feel abrupt. `shrink-0` keeps chevrons from being
        // squeezed when long folder names compete for space.
        "shrink-0 [&>svg]:size-3 [&>svg]:transition-opacity [&>svg]:duration-150",
        "text-muted-foreground/50",
        extraClass,
      )}
    >
      <ChevronRight strokeWidth={1.75} />
    </BreadcrumbSeparator>
  );
}

export function BreadcrumbNav({ path, onNavigate, rootCrumb }: Props) {
  const homeLabel = rootCrumb?.label ?? "My Files";
  const HomeIcon = rootCrumb ? Users : Home;
  // Copy-path affordance. Builds "/Photos/2026/May" from the
  // breadcrumb stack and writes it to the clipboard.
  const [justCopied, setJustCopied] = useState(false);
  const fullPath = "/" + path.map((p) => p.name).join("/");
  const onCopy = async () => {
    if (typeof navigator === "undefined" || !navigator.clipboard) return;
    try {
      await navigator.clipboard.writeText(fullPath);
      setJustCopied(true);
      toast.success("Path copied");
      setTimeout(() => setJustCopied(false), 1200);
    } catch {
      toast.error("Could not copy");
    }
  };

  // Mobile collapse: when there are more than 3 segments, on small screens
  // we render Home + ellipsis + last segment. Middle segments stay rendered
  // for sm+ via responsive utilities so the DOM/structure is unchanged on
  // desktop. The ellipsis itself is hidden on sm+.
  const lastIndex = path.length - 1;
  const collapseMobile = path.length > 2;

  // Activate a clickable crumb via Enter/Space — `BreadcrumbLink` renders an
  // `<a>` without an `href`, which means browsers won't fire a synthetic click
  // for keyboard events. Re-implement the convention here so keyboard users
  // can traverse the trail.
  const onCrumbKeyDown = (
    e: KeyboardEvent<HTMLAnchorElement>,
    index: number,
  ) => {
    if (e.key === "Enter" || e.key === " " || e.key === "Spacebar") {
      e.preventDefault();
      onNavigate(index);
    }
  };

  // Shared classes for clickable segments. Hover affordance is segment-only:
  // we don't propagate hover to siblings, so only the segment under the
  // cursor takes on the accent foreground colour. Smooth colour transition
  // keeps the hover/active state from feeling abrupt. An underline on hover
  // reinforces that the segment is a link.
  const linkBase = cn(
    "inline-block max-w-[160px] truncate align-bottom rounded-sm px-1 py-0.5",
    "text-muted-foreground",
    "transition-colors duration-150 ease-out",
    "hover:text-foreground hover:underline hover:underline-offset-4 hover:decoration-foreground/40",
    "active:text-foreground/90",
    "focus-visible:text-foreground focus-visible:outline-none",
    "focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background",
    "cursor-pointer",
  );

  // Last segment (current folder) is the strongest typography in the trail.
  const pageBase = cn(
    "inline-block max-w-[160px] truncate align-bottom px-1 py-0.5",
    "font-medium text-foreground",
    "transition-colors duration-150 ease-out",
  );

  // Loading/empty placeholder: when the folder path hasn't resolved yet
  // (no segments, but the consumer hasn't told us we're at root either)
  // we still need to occupy the same toolbar height to avoid layout shift.
  // We always render *something* — at minimum a Home crumb labeled
  // "My Files" — so the toolbar never collapses. This branch matches the
  // overall h-9 toolbar height of the surrounding controls.
  return (
    <Breadcrumb
      // Override the primitive's lowercase label so screen readers announce
      // the landmark with title case, per WAI-ARIA breadcrumb guidance.
      aria-label="Breadcrumb"
      className="group/breadcrumb flex h-9 min-w-0 items-center"
    >
      <BreadcrumbList
        // Slightly looser gaps so the chevrons read as separators rather
        // than glyphs glued to the words. `flex-nowrap` on the outer list
        // keeps the trail on a single line; the file-explorer toolbar
        // wraps its clusters instead.
        className="flex-nowrap items-center gap-1.5 text-sm leading-none"
      >
        <BreadcrumbItem className="shrink-0">
          {(!rootCrumb && path.length === 0) ? (
            <BreadcrumbPage
              className={cn(pageBase, "inline-flex items-center gap-1.5")}
              aria-current="page"
            >
              <HomeIcon className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
              {homeLabel}
            </BreadcrumbPage>
          ) : (
            <BreadcrumbLink
              className={cn(linkBase, "inline-flex items-center gap-1.5")}
              onClick={() => onNavigate(-1)}
              onKeyDown={(e) => onCrumbKeyDown(e, -1)}
              role="button"
              tabIndex={0}
              aria-label={homeLabel}
            >
              <HomeIcon className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
              <span className="hidden sm:inline">{homeLabel}</span>
            </BreadcrumbLink>
          )}
        </BreadcrumbItem>

        {/* Mobile-only ellipsis: appears between Home and the last segment
            when there are intermediate folders that would be hidden on
            small viewports. Hidden on sm+ where the full trail is shown. */}
        {collapseMobile && (
          <Fragment>
            <ElegantSeparator extraClass="sm:hidden" />
            <BreadcrumbItem className="shrink-0 sm:hidden">
              <BreadcrumbEllipsis
                className="text-muted-foreground/70"
                aria-label="Hidden folders"
              />
            </BreadcrumbItem>
          </Fragment>
        )}

        {path.map((folder, i) => {
          const isLast = i === lastIndex;
          // On mobile, hide every middle segment (everything except the
          // last). On sm+ all segments stay visible and the mobile
          // ellipsis above disappears.
          const hideOnMobile = collapseMobile && !isLast;

          return (
            <Fragment key={folder.id}>
              {/* The separator that immediately precedes a hidden segment
                  also hides on mobile, so we don't end up with two
                  consecutive separators around the ellipsis. */}
              <ElegantSeparator extraClass={hideOnMobile ? "hidden sm:flex" : undefined} />
              <BreadcrumbItem
                className={cn(
                  "min-w-0",
                  hideOnMobile ? "hidden sm:inline-flex" : "inline-flex",
                )}
              >
                {isLast ? (
                  <BreadcrumbPage
                    className={pageBase}
                    title={folder.name}
                    aria-current="page"
                  >
                    {folder.name}
                  </BreadcrumbPage>
                ) : (
                  <BreadcrumbLink
                    className={linkBase}
                    onClick={() => onNavigate(i)}
                    onKeyDown={(e) => onCrumbKeyDown(e, i)}
                    role="button"
                    tabIndex={0}
                    title={folder.name}
                  >
                    {folder.name}
                  </BreadcrumbLink>
                )}
              </BreadcrumbItem>
            </Fragment>
          );
        })}

        {/* Copy-path icon, reveals on hover via the parent group/ utility.
            Always focusable so keyboard users can reach it; opacity-0 only hides
            it visually until hover/focus. */}
        {path.length > 0 && (
          <BreadcrumbItem className="shrink-0">
            <button
              type="button"
              onClick={onCopy}
              className={cn(
                "ml-1 inline-flex h-6 w-6 items-center justify-center rounded-sm",
                "text-muted-foreground",
                "opacity-0 group-hover/breadcrumb:opacity-100 focus-visible:opacity-100",
                "transition-[opacity,color,background-color] duration-150 ease-out",
                "hover:text-foreground hover:bg-accent/60",
                "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                "focus-visible:ring-offset-1 focus-visible:ring-offset-background",
                "focus-visible:text-foreground",
              )}
              aria-label={justCopied ? "Path copied" : "Copy folder path"}
              aria-live="polite"
            >
              {justCopied ? (
                <Check className="size-3.5" aria-hidden="true" />
              ) : (
                <Copy className="size-3.5" aria-hidden="true" />
              )}
            </button>
          </BreadcrumbItem>
        )}
      </BreadcrumbList>
    </Breadcrumb>
  );
}
