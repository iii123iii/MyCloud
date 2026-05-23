"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { AppSidebar } from "@/components/sidebar/AppSidebar";
import { SidebarProvider, SidebarInset, SidebarTrigger } from "@/components/ui/sidebar";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { WebSocketProvider } from "@/components/ws-provider";
import { CommandPalette } from "@/components/command-palette/CommandPalette";
import { UploadQueueProvider } from "@/components/upload/UploadQueueProvider";
import { tokenStore } from "@/lib/api";
import { cn } from "@/lib/utils";

export default function AppLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  // Track the auth-gate decision so we can render a calm skeleton while we
  // figure out whether to bounce to /login, rather than flashing the empty
  // app shell. Behavior is unchanged — the redirect logic is identical.
  const [authChecked, setAuthChecked] = useState(false);

  useEffect(() => {
    if (!tokenStore.getAccess()) {
      // Round-trip back to whatever protected URL the user typed/clicked
      // through after they finish signing in.
      const here =
        typeof window !== "undefined"
          ? window.location.pathname + window.location.search
          : "";
      const next = here && here !== "/login" ? `?next=${encodeURIComponent(here)}` : "";
      router.replace(`/login${next}`);
      return;
    }
    // Flipping the gate flag after the token check is a synchronisation
    // shape, not a derived-state shape — there's nowhere else this can live.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setAuthChecked(true);
  }, [router]);

  if (!authChecked) {
    return (
      <div
        role="status"
        aria-live="polite"
        aria-busy="true"
        aria-label="Loading workspace"
        className="flex min-h-svh w-full animate-fade-in"
      >
        {/* Sidebar skeleton */}
        <div
          aria-hidden="true"
          className="hidden md:flex w-64 shrink-0 flex-col gap-3 border-r bg-sidebar/40 px-3 py-4"
        >
          <div className="flex items-center gap-2.5 px-1 py-1">
            <Skeleton className="h-8 w-8 rounded-md" />
            <Skeleton className="h-4 w-24" />
          </div>
          <Separator className="opacity-60" />
          <div className="flex flex-col gap-1.5">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-8 w-full rounded-md" />
            ))}
          </div>
          <Separator className="opacity-60" />
          <div className="mt-auto flex flex-col gap-2">
            <Skeleton className="h-3 w-20" />
            <Skeleton className="h-2 w-full" />
          </div>
        </div>
        {/* Main skeleton */}
        <div className="flex flex-1 flex-col">
          <div className="flex h-12 items-center gap-2 border-b px-4">
            <Skeleton className="h-6 w-6 rounded-md" />
            <Separator orientation="vertical" />
            <Skeleton className="h-4 w-40" />
          </div>
          <div className="flex-1 space-y-4 p-6">
            <Skeleton className="h-7 w-48" />
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
              {Array.from({ length: 8 }).map((_, i) => (
                <Skeleton key={i} className="h-32 w-full rounded-lg" />
              ))}
            </div>
          </div>
        </div>
        <span className="sr-only">Loading your workspace, please wait.</span>
      </div>
    );
  }

  // WebSocketProvider wraps the whole authenticated shell so any nested page
  // can `useTopic("user:..."|"folder:..."|...)` for live updates without
  // having to coordinate connection lifecycle itself.
  return (
    <WebSocketProvider>
      {/* Skip-to-content link: visually hidden until focused via keyboard.
          Lets keyboard/screen-reader users bypass the sidebar nav and jump
          straight to the page's main region. */}
      <a
        href="#main-content"
        className={cn(
          "sr-only focus:not-sr-only",
          "focus:fixed focus:left-4 focus:top-4 focus:z-[60]",
          "focus:rounded-md focus:bg-primary focus:px-4 focus:py-2",
          "focus:text-sm focus:font-medium focus:text-primary-foreground",
          "focus:shadow-lg focus-visible:outline-none focus-visible:ring-2",
          "focus-visible:ring-ring focus-visible:ring-offset-2",
          "transition-all",
        )}
      >
        Skip to main content
      </a>
      {/* Cmd/Ctrl+K palette, mounted once near the root so every
          page can register commands and trigger it. */}
      <CommandPalette />
      {/* Persistent upload queue panel. Provider wraps the entire
          authed shell so any nested component can enqueue uploads. */}
      <UploadQueueProvider>
        <SidebarProvider>
          <AppSidebar />
          <SidebarInset>
            <header
              role="banner"
              className={cn(
                "sticky top-0 z-30 flex h-12 shrink-0 items-center gap-2",
                "border-b bg-background/80 px-4 backdrop-blur-sm",
                "supports-[backdrop-filter]:bg-background/70",
                "transition-colors",
              )}
            >
              <SidebarTrigger
                className={cn(
                  "-ml-1 transition-colors",
                  "hover:bg-accent hover:text-accent-foreground",
                  "focus-visible:outline-none focus-visible:ring-2",
                  "focus-visible:ring-ring focus-visible:ring-offset-1",
                )}
                aria-label="Toggle sidebar"
              />
              <span
                className="hidden text-sm leading-none tracking-tight text-muted-foreground sm:block"
                aria-hidden="true"
              >
                Encrypted cloud storage
              </span>
            </header>
            <main
              id="main-content"
              tabIndex={-1}
              aria-label="Main content"
              className={cn(
                "flex-1 overflow-auto outline-none",
                "focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset",
                "animate-fade-in",
              )}
            >
              {children}
            </main>
          </SidebarInset>
        </SidebarProvider>
      </UploadQueueProvider>
    </WebSocketProvider>
  );
}
