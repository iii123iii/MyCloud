"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import useSWR from "swr";
import {
  LayoutDashboard, Clock, Star, Share2, Trash2, Camera, Tag, Bookmark,
  Settings, Shield, LogOut, Cloud,
} from "lucide-react";
import { tags as tagsApi, smartFolders as smartFoldersApi } from "@/lib/api";

import {
  Sidebar, SidebarContent, SidebarFooter, SidebarGroup, SidebarGroupContent,
  SidebarGroupLabel, SidebarHeader, SidebarMenu, SidebarMenuButton,
  SidebarMenuItem, SidebarSeparator,
} from "@/components/ui/sidebar";
import { Progress } from "@/components/ui/progress";
import { auth as authApi } from "@/lib/api";
import { formatBytes } from "@/lib/format";
import { toast } from "sonner";

const navItems = [
  { label: "My Files",  href: "/dashboard", icon: LayoutDashboard },
  { label: "Recent",   href: "/recent",    icon: Clock },
  { label: "Starred",  href: "/starred",   icon: Star },
  { label: "Photos",   href: "/photos",    icon: Camera },
  { label: "Shared",   href: "/shared",    icon: Share2 },
  { label: "Trash",    href: "/trash",     icon: Trash2 },
];

export function AppSidebar() {
  const pathname = usePathname();
  const router = useRouter();
  const { data: user } = useSWR("me", authApi.me, { shouldRetryOnError: false });

  const usedPct = user && user.quota_bytes > 0
    ? Math.min(100, Math.round((user.used_bytes / user.quota_bytes) * 100))
    : 0;

  const logout = async () => {
    await authApi.logout();
    toast.success("Signed out");
    router.push("/login");
  };

  return (
    <Sidebar>
      <SidebarHeader>
        <div className="flex items-center gap-2 px-2 py-1">
          <Cloud className="h-5 w-5" />
          <span className="font-bold text-base">MyCloud</span>
        </div>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              {navItems.map(({ label, href, icon: Icon }) => (
                <SidebarMenuItem key={href}>
                  <SidebarMenuButton render={<Link href={href} />} isActive={pathname === href || pathname.startsWith(href + "/")}>
                      <Icon className="h-4 w-4" />
                      {label}
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        <TagsGroup />
        <SmartFoldersGroup />

        {user?.role === "admin" && (
          <>
            <SidebarSeparator />
            <SidebarGroup>
              <SidebarGroupLabel>Administration</SidebarGroupLabel>
              <SidebarGroupContent>
                <SidebarMenu>
                  <SidebarMenuItem>
                    <SidebarMenuButton render={<Link href="/admin" />} isActive={pathname === "/admin"}>
                      <Shield className="h-4 w-4" />
                      Admin panel
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                </SidebarMenu>
              </SidebarGroupContent>
            </SidebarGroup>
          </>
        )}
      </SidebarContent>

      <SidebarFooter>
        {user && (
          <div className="px-3 py-2 space-y-2">
            <div className="space-y-1">
              <div className="flex justify-between text-xs text-muted-foreground">
                <span>Storage</span>
                <span>{formatBytes(user.used_bytes)} / {formatBytes(user.quota_bytes)}</span>
              </div>
              <Progress value={usedPct} className="h-1.5" />
            </div>
          </div>
        )}
        <SidebarSeparator />
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton render={<Link href="/settings" />} isActive={pathname === "/settings"}>
              <Settings className="h-4 w-4" />
              Settings
            </SidebarMenuButton>
          </SidebarMenuItem>
          <SidebarMenuItem>
            <SidebarMenuButton onClick={logout}>
              <LogOut className="h-4 w-4" />
              Sign out
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
        {user && (
          <div className="px-3 pb-2">
            <p className="text-xs font-medium truncate">{user.username}</p>
            <p className="text-xs text-muted-foreground truncate">{user.email}</p>
          </div>
        )}
      </SidebarFooter>
    </Sidebar>
  );
}

function TagsGroup() {
  const { data } = useSWR("tags", () => tagsApi.list(), { shouldRetryOnError: false });
  const tags = data?.tags ?? [];
  if (!tags.length) return null;
  return (
    <>
      <SidebarSeparator />
      <SidebarGroup>
        <SidebarGroupLabel>Tags</SidebarGroupLabel>
        <SidebarGroupContent>
          <SidebarMenu>
            {tags.map((t) => (
              <SidebarMenuItem key={t.id}>
                <SidebarMenuButton render={<Link href={`/dashboard?tag=${t.id}`} />}>
                  <span className="inline-block w-2 h-2 rounded-full" style={{ backgroundColor: t.color }} />
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
  const { data } = useSWR("smart-folders", () => smartFoldersApi.list(), { shouldRetryOnError: false });
  const items = data?.smart_folders ?? [];
  if (!items.length) return null;
  return (
    <>
      <SidebarSeparator />
      <SidebarGroup>
        <SidebarGroupLabel>Smart folders</SidebarGroupLabel>
        <SidebarGroupContent>
          <SidebarMenu>
            {items.map((sf) => (
              <SidebarMenuItem key={sf.id}>
                <SidebarMenuButton render={<Link href={`/dashboard?smart=${sf.id}`} />}>
                  {sf.color ? (
                    <span className="inline-block w-2 h-2 rounded-full" style={{ backgroundColor: sf.color }} />
                  ) : (
                    <Bookmark className="h-4 w-4" />
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
