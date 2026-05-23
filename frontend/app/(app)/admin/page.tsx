"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import useSWR from "swr";
import { toast } from "sonner";
import { admin as adminApi, auth as authApi } from "@/lib/api";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { StatsCards } from "@/components/admin/StatsCards";
import { UserTable } from "@/components/admin/UserTable";
import { ActivityLogTable } from "@/components/admin/ActivityLogTable";
import { UpdateChecker } from "@/components/admin/UpdateChecker";
import { cn } from "@/lib/utils";
import {
  Shield,
  Users as UsersIcon,
  ScrollText,
  RefreshCw,
  Download,
  Inbox,
  CircleAlert,
} from "lucide-react";

export default function AdminPage() {
  const router = useRouter();
  const { data: currentUser } = useSWR("me", authApi.me);
  const {
    data: stats,
    error: statsError,
    isLoading: statsLoading,
    isValidating: statsValidating,
    mutate: mutateStats,
  } = useSWR("admin-stats", adminApi.stats);
  const {
    data: usersData,
    error: usersError,
    isLoading: usersLoading,
    isValidating: usersValidating,
    mutate: mutateUsers,
  } = useSWR("admin-users", adminApi.users);
  const {
    data: logsData,
    error: logsError,
    isLoading: logsLoading,
    isValidating: logsValidating,
    mutate: mutateLogs,
  } = useSWR("admin-logs", () => adminApi.logs(200));

  // Redirect non-admins — the sidebar hides the link but a direct URL visit must also be blocked.
  useEffect(() => {
    if (currentUser && currentUser.role !== "admin") {
      router.replace("/dashboard");
    }
  }, [currentUser, router]);

  // Surface fetch failures as a toast so the admin knows the data they see may
  // be stale rather than silently rendering an empty table. Only fires when an
  // error transitions in, mirroring the toast pattern used in sibling pages.
  useEffect(() => {
    if (statsError) {
      toast.error(
        statsError instanceof Error
          ? `Could not load stats: ${statsError.message}`
          : "Could not load admin stats",
      );
    }
  }, [statsError]);
  useEffect(() => {
    if (usersError) {
      toast.error(
        usersError instanceof Error
          ? `Could not load users: ${usersError.message}`
          : "Could not load users",
      );
    }
  }, [usersError]);
  useEffect(() => {
    if (logsError) {
      toast.error(
        logsError instanceof Error
          ? `Could not load activity log: ${logsError.message}`
          : "Could not load activity log",
      );
    }
  }, [logsError]);

  const anyRefreshing = statsValidating || usersValidating || logsValidating;

  const refreshAll = () => {
    mutateStats();
    mutateUsers();
    mutateLogs();
  };

  return (
    <div className="min-h-full">
      <div className="p-4 sm:p-6 lg:p-8 space-y-6 max-w-[1400px] mx-auto">
        {/* Header */}
        <header className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3">
            <div
              aria-hidden
              className="flex h-10 w-10 items-center justify-center rounded-lg bg-destructive/10 ring-1 ring-destructive/20 transition-shadow duration-200 hover:ring-destructive/40"
            >
              <Shield className="h-5 w-5 text-destructive" />
            </div>
            <div className="min-w-0">
              <h1 className="text-xl sm:text-2xl font-semibold leading-tight tracking-tight">
                Admin panel
              </h1>
              <p className="text-xs sm:text-sm text-muted-foreground mt-0.5">
                Manage users, audit activity, and review system updates.
              </p>
            </div>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={refreshAll}
            disabled={anyRefreshing}
            aria-label="Refresh all admin data"
            aria-busy={anyRefreshing}
            className="self-start sm:self-auto transition-transform duration-150 hover:-translate-y-px active:translate-y-0"
          >
            <RefreshCw
              className={cn(
                "transition-transform",
                anyRefreshing && "animate-spin",
              )}
              aria-hidden
            />
            Refresh all
          </Button>
        </header>

        {/* Live-region status for assistive tech while any panel is refreshing. */}
        <span className="sr-only" aria-live="polite" aria-atomic="true">
          {anyRefreshing ? "Refreshing admin data" : ""}
        </span>

        {/* Stats */}
        <StatsCards stats={stats} />

        {/* Tabs */}
        <Tabs defaultValue="users" className="gap-4">
          <TabsList
            aria-label="Admin sections"
            className="w-full sm:w-fit flex-wrap"
          >
            <TabsTrigger
              value="users"
              className="transition-colors duration-150"
            >
              <UsersIcon aria-hidden />
              <span>Users</span>
            </TabsTrigger>
            <TabsTrigger
              value="logs"
              className="transition-colors duration-150"
            >
              <ScrollText aria-hidden />
              <span>Activity log</span>
            </TabsTrigger>
            <TabsTrigger
              value="updates"
              className="transition-colors duration-150"
            >
              <Download aria-hidden />
              <span>Updates</span>
            </TabsTrigger>
          </TabsList>

          <TabsContent value="users" className="space-y-3 mt-0">
            <TabHeader
              title="Users"
              description="Invite, reset, impersonate, or adjust quota for every account."
            />
            {usersLoading && !usersData ? (
              <TableSkeleton rows={6} ariaLabel="Loading users" />
            ) : usersError && !usersData ? (
              <ErrorState
                label="Couldn't load users"
                onRetry={() => mutateUsers()}
              />
            ) : (usersData?.users ?? []).length === 0 ? (
              <EmptyState
                icon={UsersIcon}
                title="No users yet"
                description="Invited accounts will appear here as soon as they sign up."
              />
            ) : (
              <UserTable
                users={usersData?.users ?? []}
                onMutate={() => {
                  mutateUsers();
                  mutateStats();
                }}
              />
            )}
          </TabsContent>

          <TabsContent value="logs" className="space-y-3 mt-0">
            <TabHeader
              title="Activity log"
              description="Last 200 audited actions across all users."
            />
            {logsLoading && !logsData ? (
              <TableSkeleton rows={8} ariaLabel="Loading activity log" />
            ) : logsError && !logsData ? (
              <ErrorState
                label="Couldn't load activity log"
                onRetry={() => mutateLogs()}
              />
            ) : (logsData?.logs ?? []).length === 0 ? (
              <EmptyState
                icon={Inbox}
                title="No activity recorded"
                description="Audited actions across all users will show up here."
              />
            ) : (
              <ActivityLogTable logs={logsData?.logs ?? []} />
            )}
          </TabsContent>

          <TabsContent value="updates" className="space-y-3 mt-0">
            <TabHeader
              title="Updates"
              description="Check for new releases and apply them in place."
            />
            <UpdateChecker />
          </TabsContent>
        </Tabs>

        {/* Show a hint that the stats are still loading the very first time
            so the page does not feel inert before SWR resolves. */}
        {statsLoading && !stats ? (
          <p
            className="text-xs text-muted-foreground"
            aria-live="polite"
          >
            Loading admin data…
          </p>
        ) : null}
      </div>
    </div>
  );
}

// ─── Local presentational helpers ────────────────────────────────────────────
// Defined inline (rather than as new files in components/admin/*) so we honor
// the "do not touch components/admin/*" constraint while keeping the page
// itself compact and readable.

function TabHeader({
  title,
  description,
}: {
  title: string;
  description?: string;
}) {
  return (
    <div className="min-w-0 space-y-0.5 pb-1">
      <h2 className="text-sm font-semibold leading-tight">{title}</h2>
      {description ? (
        <p className="text-xs text-muted-foreground leading-snug">
          {description}
        </p>
      ) : null}
    </div>
  );
}

function TableSkeleton({
  rows = 6,
  ariaLabel = "Loading",
}: {
  rows?: number;
  ariaLabel?: string;
}) {
  return (
    <div
      role="status"
      aria-live="polite"
      aria-label={ariaLabel}
      className="overflow-hidden rounded-lg ring-1 ring-foreground/10 bg-card"
    >
      <div className="border-b px-4 py-3">
        <Skeleton className="h-4 w-32" />
      </div>
      <div className="divide-y">
        {Array.from({ length: rows }).map((_, i) => (
          <div
            key={i}
            className="flex items-center gap-4 px-4 py-3"
            style={{ opacity: 1 - i * 0.08 }}
          >
            <Skeleton className="h-8 w-8 rounded-full shrink-0" />
            <div className="flex-1 space-y-2 min-w-0">
              <Skeleton className="h-3 w-1/3" />
              <Skeleton className="h-3 w-1/2" />
            </div>
            <Skeleton className="h-6 w-16 hidden sm:block" />
          </div>
        ))}
      </div>
      <span className="sr-only">{ariaLabel}…</span>
    </div>
  );
}

function EmptyState({
  icon: Icon,
  title,
  description,
}: {
  icon: React.ComponentType<{ className?: string; "aria-hidden"?: boolean }>;
  title: string;
  description: string;
}) {
  return (
    <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed bg-card/40 py-12 px-6 text-center transition-colors duration-200 hover:bg-card/60">
      <div className="rounded-full bg-muted p-3">
        <Icon className="h-5 w-5 text-muted-foreground" aria-hidden />
      </div>
      <div className="space-y-1">
        <p className="text-sm font-medium">{title}</p>
        <p className="text-xs text-muted-foreground max-w-sm">{description}</p>
      </div>
    </div>
  );
}

function ErrorState({
  label,
  onRetry,
}: {
  label: string;
  onRetry: () => void;
}) {
  return (
    <div
      role="alert"
      className="flex flex-col items-center justify-center gap-3 rounded-lg border border-destructive/30 bg-destructive/5 py-10 px-6 text-center"
    >
      <div className="rounded-full bg-destructive/10 p-3">
        <CircleAlert className="h-5 w-5 text-destructive" aria-hidden />
      </div>
      <div className="space-y-1">
        <p className="text-sm font-medium">{label}</p>
        <p className="text-xs text-muted-foreground max-w-sm">
          Something went wrong while contacting the server. Try again in a moment.
        </p>
      </div>
      <Button
        variant="outline"
        size="sm"
        onClick={onRetry}
        className="transition-colors duration-150"
      >
        <RefreshCw aria-hidden />
        Retry
      </Button>
    </div>
  );
}
