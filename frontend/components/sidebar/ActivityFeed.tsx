"use client";

// Live recent-activity feed in the sidebar.
//
// Holds a rolling buffer of the last 20 events received on the user:<id>
// WebSocket topic. Renders them as a compact list under the user info in
// the sidebar footer. Clicking an entry navigates to its resource (file
// or folder) so admins/sharing collaborators can react in one click.

import { createElement, useEffect, useState } from "react";
import useSWR from "swr";
import Link from "next/link";
import {
  Clock, ArrowRight, Pencil, FolderInput, Trash2, Undo2, Star, StarOff,
  Share2, Tag, TagsIcon, ShieldAlert, BellRing, Activity, Inbox, AlertCircle,
} from "lucide-react";

import { auth as authApi } from "@/lib/api";
import { useTopic } from "@/components/ws-provider";
import { formatRelative } from "@/lib/format";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

type ActivityEntry = {
  action: string;
  resource_type?: string;
  resource_id?: string;
  details_raw?: string;
  receivedAt: number;
};

// MAX bounds the buffer so a chatty user doesn't accumulate megabytes of
// state in React; the historic feed lives at /recent and is paginated.
const MAX_ENTRIES = 20;

export function ActivityFeed() {
  const { data: me, isLoading: meLoading, error: meError } = useSWR(
    "me",
    authApi.me,
    { shouldRetryOnError: false },
  );
  const userId = me?.id;
  const [entries, setEntries] = useState<ActivityEntry[]>([]);

  useTopic(
    userId ? `user:${userId}` : null,
    (env) => {
      if (env.type !== "event" || !env.data) return;
      // The server pushes a Data object matching publishActivity's payload.
      // Coerce defensively in case the wire shape evolves.
      const d = env.data as Partial<ActivityEntry>;
      setEntries((prev) => {
        const next: ActivityEntry = {
          action: d.action ?? "unknown",
          resource_type: d.resource_type,
          resource_id: d.resource_id,
          details_raw: d.details_raw,
          receivedAt: Date.now(),
        };
        return [next, ...prev].slice(0, MAX_ENTRIES);
      });
    },
  );

  // Prune entries older than 24h on a 60s timer so the buffer doesn't show
  // stale events after a long-idle session.
  useEffect(() => {
    const t = setInterval(() => {
      const cutoff = Date.now() - 24 * 3600 * 1000;
      setEntries((prev) => prev.filter((e) => e.receivedAt > cutoff));
    }, 60_000);
    return () => clearInterval(t);
  }, []);

  // Logged-out users never see the feed — preserve the original hidden
  // behaviour while signed-in users get loading/empty/error visuals.
  if (!userId && !meLoading && !meError) return null;

  return (
    <section
      aria-label="Activity feed"
      className="px-2 py-3 border-t"
    >
      <FeedHeader live={entries.length > 0} />

      {meError ? (
        <FeedError />
      ) : meLoading && !userId ? (
        <FeedSkeleton />
      ) : entries.length === 0 ? (
        <FeedEmpty />
      ) : (
        <ul
          aria-live="polite"
          aria-relevant="additions"
          className="-mx-1 divide-y divide-border/40"
        >
          {entries.map((e) => (
            // Composite key: receivedAt + action keeps animation keyed to
            // the actual event so re-renders don't re-trigger slide-in on
            // unchanged rows. Newest entry gets the slide-in via :first-child.
            <li
              key={`${e.receivedAt}-${e.action}`}
              className="first:animate-in first:fade-in-0 first:slide-in-from-top-1 first:duration-300 first:ease-out"
            >
              <ActivityRow entry={e} />
            </li>
          ))}
        </ul>
      )}

      {entries.length > 0 && (
        <Link
          href="/recent"
          className={cn(
            "mt-2 flex items-center justify-end gap-1 px-1 text-[10px] font-medium",
            "text-muted-foreground hover:text-foreground transition-colors",
            "rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:text-foreground",
            "group",
          )}
        >
          <span>View all in Recent</span>
          <ArrowRight className="size-3 transition-transform group-hover:translate-x-0.5" />
        </Link>
      )}
    </section>
  );
}

function FeedHeader({ live }: { live: boolean }) {
  return (
    <div className="flex items-center justify-between mb-2">
      <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
        <Clock className="size-3 shrink-0" aria-hidden="true" />
        <span className="leading-none tracking-tight">Recent activity</span>
        {/* "Live" pulse dot: entries existing implies the WS handler
            fired this session, so the connection is functional. */}
        {live && (
          <span
            className="relative flex size-1.5 ml-0.5"
            role="status"
            aria-label="Live"
          >
            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-500/60" />
            <span className="relative inline-flex size-1.5 rounded-full bg-emerald-500" />
          </span>
        )}
      </div>
    </div>
  );
}

function FeedSkeleton() {
  return (
    <ul
      aria-hidden="true"
      className="-mx-1 divide-y divide-border/40"
    >
      {Array.from({ length: 3 }).map((_, i) => (
        <li key={i} className="flex items-center gap-2 px-1.5 py-1.5">
          <Skeleton className="size-3 shrink-0 rounded-full" />
          <Skeleton className="h-3 flex-1 min-w-0" />
          <Skeleton className="h-2.5 w-8 shrink-0" />
        </li>
      ))}
    </ul>
  );
}

function FeedEmpty() {
  return (
    <div className="flex flex-col items-center justify-center gap-1.5 px-2 py-4 text-center">
      <Inbox className="size-4 text-muted-foreground/60" aria-hidden="true" />
      <p className="text-xs text-muted-foreground">No recent activity</p>
      <p className="text-[10px] text-muted-foreground/70 leading-tight">
        New events will appear here in real time.
      </p>
    </div>
  );
}

function FeedError() {
  return (
    <div
      role="alert"
      className="flex items-center gap-2 px-2 py-3 text-xs text-muted-foreground"
    >
      <AlertCircle className="size-3.5 shrink-0 text-destructive/70" aria-hidden="true" />
      <span className="leading-snug">Couldn’t load activity.</span>
    </div>
  );
}

function ActivityRow({ entry }: { entry: ActivityEntry }) {
  const label = humanise(entry.action);
  // iconFor returns a stable lucide component reference per action. React 19's
  // react-hooks/static-components rule flags the `const Icon = iconFor(...)`
  // + `<Icon />` shape as if we were creating a new component each render —
  // we aren't, but the heuristic can't tell. Use createElement to sidestep it.
  const icon = createElement(iconFor(entry.action), {
    className:
      "size-3 shrink-0 opacity-70 transition-opacity group-hover:opacity-100",
    "aria-hidden": true,
  });
  const href = entry.resource_type === "folder" && entry.resource_id
    ? `/dashboard?folder=${encodeURIComponent(entry.resource_id)}`
    : entry.resource_type === "file" && entry.resource_id
      ? `/dashboard` // file landing — open dashboard; the file id isn't a route on its own
      : null;
  const iso = new Date(entry.receivedAt).toISOString();
  const time = formatRelative(iso);

  const rowClass = cn(
    "flex items-center gap-2 px-1.5 py-1.5 rounded-sm text-xs",
    "text-muted-foreground",
    "transition-colors duration-150",
    "hover:bg-sidebar-accent/60 hover:text-sidebar-accent-foreground",
  );

  const inner = (
    <>
      {icon}
      <span className="truncate flex-1 min-w-0 leading-tight" title={label}>
        {label}
      </span>
      <time
        dateTime={iso}
        aria-label={`${label}, ${time}`}
        className="text-[10px] shrink-0 tabular-nums text-muted-foreground/70 leading-tight"
      >
        {time}
      </time>
    </>
  );

  return href ? (
    <Link
      href={href}
      className={cn(
        "group block outline-none",
        "focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:ring-offset-0 rounded-sm",
      )}
    >
      <span className={rowClass}>{inner}</span>
    </Link>
  ) : (
    <span className={cn("group", rowClass)}>{inner}</span>
  );
}

// iconFor groups actions visually so a glance at the feed conveys the
// kind of event (rename vs. delete vs. share, etc.) without reading.
function iconFor(action: string) {
  switch (action) {
    case "file_rename":      return Pencil;
    case "file_move":        return FolderInput;
    case "file_soft_delete": return Trash2;
    case "file_restore":     return Undo2;
    case "file_star":        return Star;
    case "file_unstar":      return StarOff;
    case "share_create":
    case "share_delete":     return Share2;
    case "tag_attach":       return Tag;
    case "tag_detach":       return TagsIcon;
    case "rate_limit_block": return ShieldAlert;
    case "retention_notify": return BellRing;
    default:                 return Activity;
  }
}

// humanise turns server action names ("file_soft_delete") into UI strings
// ("Moved file to trash"). Falls back to a sentence-cased raw string.
function humanise(action: string): string {
  switch (action) {
    case "file_rename":      return "Renamed file";
    case "file_move":        return "Moved file";
    case "file_soft_delete": return "Moved file to trash";
    case "file_restore":     return "Restored file";
    case "file_star":        return "Starred file";
    case "file_unstar":      return "Unstarred file";
    case "share_create":     return "Created share";
    case "share_delete":     return "Deleted share";
    case "tag_attach":       return "Tagged file";
    case "tag_detach":       return "Removed tag";
    case "rate_limit_block": return "Rate limit hit";
    case "retention_notify": return "Retention reminder";
  }
  return action.replace(/_/g, " ").replace(/^./, (c) => c.toUpperCase());
}
