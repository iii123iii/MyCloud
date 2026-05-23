"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import useSWR from "swr";
import {
  LayoutDashboard, Clock, Star, Share2, Trash2, Camera, Tag, Bookmark,
  Settings, Shield, LogOut, Cloud,
} from "lucide-react";
import { tags as tagsApi, smartFolders as smartFoldersApi } from "@/lib/api";
import { FavoritesGroup } from "@/components/sidebar/FavoritesGroup";

import {
  Sidebar, SidebarContent, SidebarFooter, SidebarGroup, SidebarGroupContent,
  SidebarGroupLabel, SidebarHeader, SidebarMenu, SidebarMenuButton,
  SidebarMenuItem, SidebarMenuSkeleton, SidebarSeparator,
} from "@/components/ui/sidebar";
import { Progress } from "@/components/ui/progress";
import { Skeleton } from "@/components/ui/skeleton";
import { auth as authApi } from "@/lib/api";
import { formatBytes } from "@/lib/format";
import { toast } from "sonner";
import { cn } from "@/lib/utils";

const navItems = [
  { label: "My Files",  href: "/dashboard", icon: LayoutDashboard },
  { label: "Recent",    href: "/recent",    icon: Clock },
  { label: "Starred",   href: "/starred",   icon: Star },
  { label: "Photos",    href: "/photos",    icon: Camera },
  { label: "Shared",    href: "/shared",    icon: Share2 },
  { label: "Trash",     href: "/trash",     icon: Trash2 },
];

// Shared styling for nav buttons: pronounced hover + active indicator
// (left accent bar + tinted background) without overriding the base
// SidebarMenuButton primitive in ui/sidebar.tsx.
const navButtonClass = cn(
  "relative my-0.5 gap-2.5",
  "motion-safe:transition-[background-color,color,box-shadow] motion-safe:duration-150",
  "hover:bg-sidebar-accent/80 hover:text-sidebar-accent-foreground",
  "focus-visible:ring-2 focus-visible:ring-sidebar-ring focus-visible:ring-offset-1 focus-visible:ring-offset-sidebar",
  "data-active:bg-sidebar-accent data-active:text-sidebar-accent-foreground data-active:font-medium",
  "data-active:before:absolute data-active:before:left-0 data-active:before:top-1.5 data-active:before:bottom-1.5",
  "data-active:before:w-0.5 data-active:before:rounded-full data-active:before:bg-sidebar-primary",
  "[&_svg]:text-sidebar-foreground/70 data-active:[&_svg]:text-sidebar-accent-foreground motion-safe:[&_svg]:transition-colors",
);

// Shared classes for SidebarGroupLabel — keeps Account / Administration /
// Tags / Smart folders visually identical and easy to scan.
const groupLabelClass =
  "px-2 text-[11px] font-medium uppercase tracking-[0.08em] text-sidebar-foreground/55";

export function AppSidebar() {
  const pathname = usePathname();
  const router = useRouter();
  const { data: user, isLoading: userLoading } = useSWR("me", authApi.me, {
    shouldRetryOnError: false,
  });

  const usedPct = user && user.quota_bytes > 0
    ? Math.min(100, Math.round((user.used_bytes / user.quota_bytes) * 100))
    : 0;

  const isActiveRoute = (href: string) =>
    pathname === href || pathname.startsWith(href + "/");

  const logout = async () => {
    await authApi.logout();
    toast.success("Signed out");
    router.push("/login");
  };

  return (
    <Sidebar>
      <SidebarHeader className="sticky top-0 z-10 border-b border-sidebar-border/60 bg-sidebar px-3 py-3">
        <Link
          href="/dashboard"
          aria-label="MyCloud home"
          className={cn(
            "flex items-center gap-2.5 rounded-md outline-none",
            "motion-safe:transition-opacity hover:opacity-90",
            "focus-visible:ring-2 focus-visible:ring-sidebar-ring focus-visible:ring-offset-1 focus-visible:ring-offset-sidebar",
          )}
        >
          <span
            aria-hidden="true"
            className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-sidebar-primary text-sidebar-primary-foreground shadow-sm"
          >
            <Cloud className="h-[18px] w-[18px]" />
          </span>
          <span className="text-base font-semibold tracking-tight leading-none">MyCloud</span>
        </Link>
      </SidebarHeader>

      <SidebarContent
        aria-label="Primary"
        role="navigation"
        className="gap-1 px-1 py-2"
      >
        <SidebarGroup className="py-1" aria-label="Workspace">
          <SidebarGroupContent>
            <SidebarMenu>
              {navItems.map(({ label, href, icon: Icon }) => {
                const active = isActiveRoute(href);
                return (
                  <SidebarMenuItem key={href}>
                    <SidebarMenuButton
                      render={
                        <Link
                          href={href}
                          aria-current={active ? "page" : undefined}
                        />
                      }
                      isActive={active}
                      tooltip={label}
                      className={navButtonClass}
                    >
                      <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />
                      <span className="truncate">{label}</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                );
              })}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        <SidebarSeparator className="my-1 opacity-60" />
        <FavoritesGroup />
        <TagsGroup />
        <SmartFoldersGroup />

        {user?.role === "admin" && (
          <>
            <SidebarSeparator className="my-1 opacity-60" />
            <SidebarGroup className="py-1" aria-label="Administration">
              <SidebarGroupLabel className={groupLabelClass}>
                Administration
              </SidebarGroupLabel>
              <SidebarGroupContent>
                <SidebarMenu>
                  <SidebarMenuItem>
                    <SidebarMenuButton
                      render={
                        <Link
                          href="/admin"
                          aria-current={pathname === "/admin" ? "page" : undefined}
                        />
                      }
                      isActive={pathname === "/admin"}
                      tooltip="Admin panel"
                      className={navButtonClass}
                    >
                      <Shield className="h-4 w-4 shrink-0" aria-hidden="true" />
                      <span className="truncate">Admin panel</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                </SidebarMenu>
              </SidebarGroupContent>
            </SidebarGroup>
          </>
        )}
      </SidebarContent>

      <SidebarFooter className="gap-2 border-t border-sidebar-border/60 pt-2">
        <div
          className="px-3 py-1.5 group-data-[collapsible=icon]:hidden"
          aria-busy={userLoading || undefined}
        >
          <div className="space-y-1.5 rounded-md bg-sidebar-accent/40 px-2.5 py-2">
            {user ? (
              <>
                <div className="flex items-baseline justify-between gap-2">
                  <span className="text-xs font-medium text-sidebar-foreground/80">
                    Storage
                  </span>
                  <span
                    className="text-[11px] tabular-nums text-sidebar-foreground/60"
                    aria-label={`${usedPct} percent used`}
                  >
                    {usedPct}%
                  </span>
                </div>
                <Progress
                  value={usedPct}
                  className="h-1.5"
                  aria-label="Storage used"
                />
                <div className="text-[11px] tabular-nums text-sidebar-foreground/60">
                  {formatBytes(user.used_bytes)}{" "}
                  <span className="opacity-60">of</span>{" "}
                  {formatBytes(user.quota_bytes)}
                </div>
              </>
            ) : (
              <>
                <div className="flex items-baseline justify-between gap-2">
                  <Skeleton className="h-3 w-14" />
                  <Skeleton className="h-3 w-8" />
                </div>
                <Skeleton className="h-1.5 w-full rounded-full" />
                <Skeleton className="h-3 w-24" />
                <span className="sr-only">Loading storage usage</span>
              </>
            )}
          </div>
        </div>
        <SidebarSeparator className="opacity-60" />
        <SidebarMenu aria-label="Account actions">
          <SidebarMenuItem>
            <SidebarMenuButton
              render={
                <Link
                  href="/settings"
                  aria-current={pathname === "/settings" ? "page" : undefined}
                />
              }
              isActive={pathname === "/settings"}
              tooltip="Settings"
              className={navButtonClass}
            >
              <Settings className="h-4 w-4 shrink-0" aria-hidden="true" />
              <span className="truncate">Settings</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
          <SidebarMenuItem>
            <SidebarMenuButton
              type="button"
              onClick={logout}
              tooltip="Sign out"
              aria-label="Sign out"
              className={cn(
                "relative my-0.5 gap-2.5",
                "motion-safe:transition-[background-color,color,box-shadow] motion-safe:duration-150",
                "hover:bg-destructive/10 hover:text-destructive",
                "focus-visible:ring-2 focus-visible:ring-destructive/40 focus-visible:ring-offset-1 focus-visible:ring-offset-sidebar",
                "[&_svg]:text-sidebar-foreground/70 motion-safe:[&_svg]:transition-colors hover:[&_svg]:text-destructive",
              )}
            >
              <LogOut className="h-4 w-4 shrink-0" aria-hidden="true" />
              <span className="truncate">Sign out</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
        <div className="px-3 pb-1.5 pt-0.5 group-data-[collapsible=icon]:hidden">
          {user ? (
            <>
              <p className="text-xs font-medium truncate leading-tight">{user.username}</p>
              <p className="text-[11px] text-muted-foreground truncate leading-tight">{user.email}</p>
            </>
          ) : (
            <div className="space-y-1" aria-hidden="true">
              <Skeleton className="h-3 w-24" />
              <Skeleton className="h-3 w-32" />
            </div>
          )}
        </div>
      </SidebarFooter>
    </Sidebar>
  );
}

// Hover + focus + transition styling for "data-driven" sidebar links
// (tags, smart folders). Lighter than navButtonClass — no active-state
// accent bar, since these are query-string filters rather than routes.
const secondaryButtonClass = cn(
  "my-0.5 gap-2.5",
  "motion-safe:transition-[background-color,color,box-shadow] motion-safe:duration-150",
  "hover:bg-sidebar-accent/80 hover:text-sidebar-accent-foreground",
  "focus-visible:ring-2 focus-visible:ring-sidebar-ring focus-visible:ring-offset-1 focus-visible:ring-offset-sidebar",
);

function TagsGroup() {
  const { data, isLoading } = useSWR("tags", () => tagsApi.list(), {
    shouldRetryOnError: false,
  });
  const tags = data?.tags ?? [];

  if (isLoading) {
    return (
      <>
        <SidebarSeparator className="my-1 opacity-60" />
        <SidebarGroup className="py-1" aria-label="Tags" aria-busy="true">
          <SidebarGroupLabel className={groupLabelClass}>Tags</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuSkeleton showIcon />
              </SidebarMenuItem>
              <SidebarMenuItem>
                <SidebarMenuSkeleton showIcon />
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </>
    );
  }

  if (!tags.length) return null;
  return (
    <>
      <SidebarSeparator className="my-1 opacity-60" />
      <SidebarGroup className="py-1" aria-label="Tags">
        <SidebarGroupLabel className={groupLabelClass}>Tags</SidebarGroupLabel>
        <SidebarGroupContent>
          <SidebarMenu>
            {tags.map((t) => (
              <SidebarMenuItem key={t.id}>
                <SidebarMenuButton
                  render={<Link href={`/dashboard?tag=${t.id}`} />}
                  tooltip={t.name}
                  className={secondaryButtonClass}
                >
                  <span
                    aria-hidden="true"
                    className="inline-block h-2 w-2 shrink-0 rounded-full ring-1 ring-inset ring-black/5"
                    style={{ backgroundColor: t.color }}
                  />
                  <span className="truncate">{t.name}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            ))}
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>
    </>
  );
}

function SmartFoldersGroup() {
  const { data, isLoading } = useSWR(
    "smart-folders",
    () => smartFoldersApi.list(),
    { shouldRetryOnError: false },
  );
  const items = data?.smart_folders ?? [];

  if (isLoading) {
    return (
      <>
        <SidebarSeparator className="my-1 opacity-60" />
        <SidebarGroup className="py-1" aria-label="Smart folders" aria-busy="true">
          <SidebarGroupLabel className={groupLabelClass}>
            Smart folders
          </SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuSkeleton showIcon />
              </SidebarMenuItem>
              <SidebarMenuItem>
                <SidebarMenuSkeleton showIcon />
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </>
    );
  }

  if (!items.length) return null;
  return (
    <>
      <SidebarSeparator className="my-1 opacity-60" />
      <SidebarGroup className="py-1" aria-label="Smart folders">
        <SidebarGroupLabel className={groupLabelClass}>
          Smart folders
        </SidebarGroupLabel>
        <SidebarGroupContent>
          <SidebarMenu>
            {items.map((sf) => (
              <SidebarMenuItem key={sf.id}>
                <SidebarMenuButton
                  render={<Link href={`/dashboard?smart=${sf.id}`} />}
                  tooltip={sf.name}
                  className={secondaryButtonClass}
                >
                  {sf.color ? (
                    <span
                      aria-hidden="true"
                      className="inline-block h-2 w-2 shrink-0 rounded-full ring-1 ring-inset ring-black/5"
                      style={{ backgroundColor: sf.color }}
                    />
                  ) : (
                    <Bookmark className="h-4 w-4 shrink-0" aria-hidden="true" />
                  )}
                  <span className="truncate">{sf.name}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            ))}
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>
    </>
  );
}

// Mark imports as used so TS doesn't complain on unused side-imports.
void Tag;
