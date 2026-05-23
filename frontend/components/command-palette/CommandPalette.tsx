"use client";

// Cmd/Ctrl+K command palette with inline file search.
//
// We bind the keyboard shortcut with a plain document.keydown listener
// (the pattern straight from cmdk's docs) instead of going through our
// custom hotkey lib. The custom lib was either swallowing the event or
// firing in a context where preventDefault was too late — going direct
// removes the indirection entirely.
//
// The palette shows:
//   1. live file/folder search results from /api/v2/search (when typing)
//   2. navigation commands the user can run with one click

import { useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import useSWR from "swr";
import {
  Search as SearchIcon,
  FileText,
  Folder as FolderIcon,
  Loader2,
  LayoutDashboard,
  Clock,
  Star,
  Camera,
  Share2,
  Trash2,
  Smartphone,
} from "lucide-react";

import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "@/components/ui/command";
import { search as searchApi, files as filesApi } from "@/lib/api";
import type { SearchResult } from "@/lib/types";

type NavCmd = {
  id: string;
  label: string;
  group: string;
  icon: React.ElementType;
  href: string;
};

// Static nav table — no provider, no registry, just a list. The previous
// version used a global registry/subscribe pattern that made it possible
// for stale commands (pointing at deleted routes) to linger. A fresh
// array on every render means every entry definitely points to a route
// that exists in the build today.
const NAV_COMMANDS: NavCmd[] = [
  { id: "nav.dashboard", label: "Go to My Files",   group: "Navigate", icon: LayoutDashboard, href: "/dashboard" },
  { id: "nav.recent",    label: "Go to Recent",     group: "Navigate", icon: Clock,           href: "/recent"    },
  { id: "nav.starred",   label: "Go to Starred",    group: "Navigate", icon: Star,            href: "/starred"   },
  { id: "nav.photos",    label: "Go to Photos",     group: "Navigate", icon: Camera,          href: "/photos"    },
  { id: "nav.shared",    label: "Go to Shared",     group: "Navigate", icon: Share2,          href: "/shared"    },
  { id: "nav.trash",     label: "Go to Trash",      group: "Navigate", icon: Trash2,          href: "/trash"     },
  { id: "nav.sessions",  label: "Active sessions",  group: "Account",  icon: Smartphone,      href: "/settings/sessions" },
];

export function CommandPalette() {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");

  // Bind Cmd+K / Ctrl+K straight to document. This is the cmdk-recommended
  // pattern — fewer layers than our custom hotkey lib, no chance of stale
  // closures or wrong-target gotchas. capture:true so we beat the browser's
  // built-in handlers (Firefox's quick-search, Chrome's URL bar focus).
  useEffect(() => {
    const down = (e: KeyboardEvent) => {
      if (e.key === "k" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        e.stopPropagation();
        setOpen((v) => !v);
      }
    };
    document.addEventListener("keydown", down, { capture: true });
    return () => document.removeEventListener("keydown", down, { capture: true });
  }, []);

  // Reset typed text whenever the dialog closes so reopening starts fresh.
  useEffect(() => {
    if (!open) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setQuery("");
      setDebouncedQuery("");
    }
  }, [open]);

  // Debounce: 200ms idle before we fire a search. Avoids one request per
  // keystroke; cmdk's existing renderer is fast enough to scroll fluidly.
  useEffect(() => {
    if (query.trim() === "") {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setDebouncedQuery("");
      return;
    }
    const t = setTimeout(() => setDebouncedQuery(query.trim()), 200);
    return () => clearTimeout(t);
  }, [query]);

  // Backend-driven search. SWR caches so backspace + retype is instant.
  const { data, isLoading } = useSWR(
    open && debouncedQuery ? ["palette-search", debouncedQuery] : null,
    () => searchApi.query(debouncedQuery),
    { keepPreviousData: true, revalidateOnFocus: false },
  );
  const searchResults: SearchResult[] = data?.results ?? [];

  const goto = useCallback(
    (href: string) => {
      setOpen(false);
      router.push(href);
    },
    [router],
  );

  const handleSelectResult = useCallback(
    async (r: SearchResult) => {
      setOpen(false);
      if (r.type === "folder") {
        router.push(`/dashboard?folder=${encodeURIComponent(r.id)}`);
        return;
      }
      // The search response doesn't include folder_id, so look up the file
      // to find its parent folder, then navigate there and ask FileExplorer
      // to auto-open the preview for this file.
      try {
        const info = await filesApi.getInfo(r.id);
        const params = new URLSearchParams();
        if (info.folder_id) params.set("folder", info.folder_id);
        params.set("open", r.id);
        router.push(`/dashboard?${params.toString()}`);
      } catch {
        // Fallback: at least surface the file id so the explorer can try
        // to open it from wherever it lands.
        router.push(`/dashboard?open=${encodeURIComponent(r.id)}`);
      }
    },
    [router],
  );

  const showSearch = debouncedQuery !== "";
  const hasResults = searchResults.length > 0;

  // Group nav commands so the rendered list mirrors the original spec
  // (Navigate / Account etc.). Memoised so React doesn't re-bucket on
  // every typed character — only when the underlying list changes.
  const groupedNav = useMemo(() => {
    const m = new Map<string, NavCmd[]>();
    for (const cmd of NAV_COMMANDS) {
      if (!m.has(cmd.group)) m.set(cmd.group, []);
      m.get(cmd.group)!.push(cmd);
    }
    return Array.from(m.entries());
  }, []);

  return (
    <CommandDialog open={open} onOpenChange={setOpen}>
      <CommandInput
        placeholder="Search files or type a command…"
        value={query}
        onValueChange={setQuery}
      />
      <CommandList>
        <CommandEmpty>No matches.</CommandEmpty>

        {showSearch && (
          <CommandGroup
            heading={
              isLoading ? (
                <span className="flex items-center gap-2">
                  <Loader2 className="size-3 animate-spin" /> Searching…
                </span>
              ) : (
                <span className="flex items-center gap-2">
                  <SearchIcon className="size-3" /> Results
                </span>
              )
            }
          >
            {searchResults.slice(0, 12).map((r) => (
              <CommandItem
                key={`${r.type}:${r.id}`}
                // Composite value embeds the name so cmdk's built-in
                // fuzzy filter on the input still ranks results when the
                // user's typed query overlaps multiple names.
                value={`result:${r.type}:${r.id}:${r.name}`}
                onSelect={() => handleSelectResult(r)}
              >
                {r.type === "folder" ? (
                  <FolderIcon className="size-4 text-muted-foreground" />
                ) : (
                  <FileText className="size-4 text-muted-foreground" />
                )}
                <span className="truncate">{r.name}</span>
                <span className="ml-auto text-[10px] text-muted-foreground">
                  {r.type}
                </span>
              </CommandItem>
            ))}
            {!isLoading && !hasResults && (
              <div className="px-2 py-1.5 text-xs text-muted-foreground">
                No files matched.
              </div>
            )}
          </CommandGroup>
        )}

        {showSearch && <CommandSeparator />}

        {groupedNav.map(([group, items]) => (
          <CommandGroup key={group} heading={group}>
            {items.map((cmd) => {
              const Icon = cmd.icon;
              return (
                <CommandItem
                  key={cmd.id}
                  value={`nav:${cmd.id}:${cmd.label}`}
                  onSelect={() => goto(cmd.href)}
                >
                  <Icon className="size-4 text-muted-foreground" />
                  <span>{cmd.label}</span>
                </CommandItem>
              );
            })}
          </CommandGroup>
        ))}
      </CommandList>
    </CommandDialog>
  );
}
