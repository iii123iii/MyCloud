"use client";

import useSWR from "swr";
import { admin as adminApi } from "@/lib/api";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { StatsCards } from "@/components/admin/StatsCards";
import { UserTable } from "@/components/admin/UserTable";
import { ActivityLogTable } from "@/components/admin/ActivityLogTable";
import { UpdateChecker } from "@/components/admin/UpdateChecker";
import { Shield } from "lucide-react";

export default function AdminPage() {
  const { data: stats, mutate: mutateStats } = useSWR("admin-stats", adminApi.stats);
  const { data: usersData, mutate: mutateUsers } = useSWR("admin-users", adminApi.users);
  const { data: logsData }  = useSWR("admin-logs", () => adminApi.logs(200));

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center gap-2">
        <Shield className="h-5 w-5" />
        <h1 className="text-xl font-semibold">Admin panel</h1>
      </div>

      <StatsCards stats={stats} />

      <Tabs defaultValue="users">
        <TabsList>
          <TabsTrigger value="users">Users</TabsTrigger>
          <TabsTrigger value="logs">Activity log</TabsTrigger>
          <TabsTrigger value="updates">Updates</TabsTrigger>
        </TabsList>
        <TabsContent value="users" className="mt-4">
          <UserTable
            users={usersData?.users ?? []}
            onMutate={() => { mutateUsers(); mutateStats(); }}
          />
        </TabsContent>
        <TabsContent value="logs" className="mt-4">
          <ActivityLogTable logs={logsData?.logs ?? []} />
        </TabsContent>
        <TabsContent value="updates" className="mt-4">
          <UpdateChecker />
        </TabsContent>
      </Tabs>
    </div>
  );
}
