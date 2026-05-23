"use client";

// Favourites pinned in the sidebar, with HTML5 drag-to-reorder.
//
// On drop, we PUT the new order to /api/v2/me/favorites; the SWR cache
// updates optimistically. The drag implementation uses native HTML5 DnD
// — sufficient for a vertical list with <20 items; no react-dnd dep.

import { useMemo, useState } from "react";
import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";
import useSWR from "swr";
import { Star, GripVertical, AlertCircle } from "lucide-react";

import {
  SidebarGroup, SidebarGroupContent, SidebarGroupLabel,
  SidebarMenu, SidebarMenuButton, SidebarMenuItem, SidebarMenuSkeleton,
} from "@/components/ui/sidebar";
import { tokenStore } from "@/lib/api";
import { cn } from "@/lib/utils";

const BASE = process.env.NEXT_PUBLIC_API_URL ?? "";

type Favorite = {
  folder_id: string;
  name: string;
  position: number;
};

async function fetchFavorites(): Promise<Favorite[]> {
  const token = tokenStore.getAccess();
  const res = await fetch(BASE + "/api/v2/me/favorites", {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  });
  if (!res.ok) return [];
  const json = await res.json().catch(() => null);
  return (json?.data?.favorites ?? json?.favorites ?? []) as Favorite[];
}

async function persistOrder(ids: string[]) {
  const token = tokenStore.getAccess();
  await fetch(BASE + "/api/v2/me/favorites", {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify({ folder_ids: ids }),
  });
}

// Shared per-item styling so favorites match the active/hover treatment used
// elsewhere in the sidebar (see navButtonClass in AppSidebar.tsx).
const favButtonClass = cn(
  "relative my-0.5 pl-2 pr-2 transition-colors",
  "hover:bg-sidebar-accent/80 hover:text-sidebar-accent-foreground",
  "data-active:bg-sidebar-accent data-active:text-sidebar-accent-foreground data-active:font-medium",
  "data-active:before:absolute data-active:before:left-0 data-active:before:top-1.5 data-active:before:bottom-1.5",
  "data-active:before:w-0.5 data-active:before:rounded-full data-active:before:bg-sidebar-primary",
);

const groupLabelClass = "gap-1.5 text-[11px] uppercase tracking-wider text-sidebar-foreground/60";

export function FavoritesGroup() {
  const { data, error, isLoading, mutate } = useSWR<Favorite[]>("favorites", fetchFavorites, {
    shouldRetryOnError: false,
  });
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const activeFolderId = pathname === "/dashboard" ? searchParams?.get("folder") ?? null : null;

  const [draggingId, setDraggingId] = useState<string | null>(null);
  const [dragOverId, setDragOverId] = useState<string | null>(null);
  // Local order overrides the server order while a drag is in progress so
  // the UI feels instant. On drop, we PUT and re-validate.
  const [localOrder, setLocalOrder] = useState<Favorite[] | null>(null);

  const items = useMemo(() => localOrder ?? data ?? [], [localOrder, data]);

  const handleDragStart = (id: string) => setDraggingId(id);
  const handleDragOver = (e: React.DragEvent, overId: string) => {
    e.preventDefault();
    setDragOverId(overId);
    if (!draggingId || draggingId === overId) return;
    const from = items.findIndex((i) => i.folder_id === draggingId);
    const to = items.findIndex((i) => i.folder_id === overId);
    if (from < 0 || to < 0) return;
    const next = items.slice();
    const [moved] = next.splice(from, 1);
    next.splice(to, 0, moved);
    setLocalOrder(next);
  };
  const handleDrop = async () => {
    setDraggingId(null);
    setDragOverId(null);
    if (!localOrder) return;
    const ids = localOrder.map((i) => i.folder_id);
    await persistOrder(ids);
    setLocalOrder(null);
    mutate();
  };

  // --- Loading state ---------------------------------------------------------
  if (isLoading && !data) {
    return (
      <SidebarGroup className="py-1" aria-label="Favourites" aria-busy="true">
        <SidebarGroupLabel className={groupLabelClass}>
          <Star className="h-3.5 w-3.5" aria-hidden="true" />
          Favourites
        </SidebarGroupLabel>
        <SidebarGroupContent>
          <SidebarMenu aria-hidden="true">
            {Array.from({ length: 3 }).map((_, i) => (
              <SidebarMenuItem key={i}>
                <SidebarMenuSkeleton showIcon className="my-0.5" />
              </SidebarMenuItem>
            ))}
          </SidebarMenu>
          <span className="sr-only">Loading favourites…</span>
        </SidebarGroupContent>
      </SidebarGroup>
    );
  }

  // --- Error state -----------------------------------------------------------
  if (error && !data) {
    return (
      <SidebarGroup className="py-1" aria-label="Favourites" role="region">
        <SidebarGroupLabel className={groupLabelClass}>
          <Star className="h-3.5 w-3.5" aria-hidden="true" />
          Favourites
        </SidebarGroupLabel>
        <SidebarGroupContent>
          <div
            role="alert"
            className="mx-1 my-0.5 flex items-start gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-2.5 py-2 text-xs text-destructive"
          >
            <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" />
            <div className="min-w-0 flex-1">
              <p className="font-medium leading-tight">Couldn’t load favourites</p>
              <button
                type="button"
                onClick={() => void mutate()}
                className={cn(
                  "mt-1 rounded-sm text-[11px] font-medium underline-offset-2 hover:underline",
                  "outline-none focus-visible:ring-2 focus-visible:ring-destructive/40",
                )}
              >
                Try again
              </button>
            </div>
          </div>
        </SidebarGroupContent>
      </SidebarGroup>
    );
  }

  // --- Empty state -----------------------------------------------------------
  if (!items || items.length === 0) {
    return (
      <SidebarGroup className="py-1" aria-label="Favourites" role="region">
        <SidebarGroupLabel className={groupLabelClass}>
          <Star className="h-3.5 w-3.5" aria-hidden="true" />
          Favourites
        </SidebarGroupLabel>
        <SidebarGroupContent>
          <p className="mx-2 my-1 text-[11px] leading-snug text-sidebar-foreground/60">
            Star a folder to pin it here.
          </p>
        </SidebarGroupContent>
      </SidebarGroup>
    );
  }

  // --- Populated list --------------------------------------------------------
  return (
    <SidebarGroup className="py-1" aria-label="Favourites" role="region">
      <SidebarGroupLabel className={groupLabelClass}>
        <Star className="h-3.5 w-3.5" aria-hidden="true" />
        Favourites
      </SidebarGroupLabel>
      <SidebarGroupContent>
        <SidebarMenu aria-label="Pinned folders">
          {items.map((fav) => {
            const isActive = activeFolderId === fav.folder_id;
            const isDragging = draggingId === fav.folder_id;
            const isDragOver = dragOverId === fav.folder_id && draggingId !== fav.folder_id;
            return (
              <SidebarMenuItem
                key={fav.folder_id}
                draggable
                onDragStart={() => handleDragStart(fav.folder_id)}
                onDragOver={(e) => handleDragOver(e, fav.folder_id)}
                onDrop={() => void handleDrop()}
                onDragEnd={() => {
                  setDraggingId(null);
                  setDragOverId(null);
                }}
                aria-roledescription="draggable favourite"
                className={cn(
                  "transition-[opacity,transform,background-color] duration-150 ease-out",
                  isDragging && "opacity-50 scale-[0.99]",
                  isDragOver && "bg-sidebar-accent/40 rounded-md",
                )}
              >
                <SidebarMenuButton
                  render={
                    <Link
                      href={`/dashboard?folder=${encodeURIComponent(fav.folder_id)}`}
                      aria-current={isActive ? "page" : undefined}
                    />
                  }
                  isActive={isActive}
                  tooltip={fav.name}
                  className={favButtonClass}
                >
                  <GripVertical
                    aria-hidden="true"
                    className={cn(
                      "h-3.5 w-3.5 shrink-0 cursor-grab text-sidebar-foreground/40",
                      "opacity-0 transition-opacity duration-150",
                      "group-hover/menu-item:opacity-100 group-focus-within/menu-item:opacity-100",
                      isDragging && "cursor-grabbing opacity-100",
                    )}
                  />
                  <span className="truncate">{fav.name}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            );
          })}
        </SidebarMenu>
        {/* Screen-reader-only live region announces reorder progress. */}
        <span aria-live="polite" className="sr-only">
          {draggingId
            ? `Reordering ${items.find((i) => i.folder_id === draggingId)?.name ?? "item"}`
            : ""}
        </span>
      </SidebarGroupContent>
    </SidebarGroup>
  );
}
