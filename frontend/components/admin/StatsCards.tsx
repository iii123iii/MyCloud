"use client";

import { Users, Files, HardDrive, Database } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { formatBytes } from "@/lib/format";
import type { AdminStats } from "@/lib/types";

interface Props {
  stats?: AdminStats;
}

export function StatsCards({ stats }: Props) {
  const cards = [
    {
      label: "Total users",
      value: stats ? String(stats.total_users) : null,
      icon: Users,
    },
    {
      label: "Total files",
      value: stats ? String(stats.total_files) : null,
      icon: Files,
    },
    {
      label: "Storage used",
      value: stats ? formatBytes(stats.total_storage_used) : null,
      icon: HardDrive,
    },
    {
      label: "Total quota allocated",
      value: stats ? formatBytes(stats.total_quota) : null,
      icon: Database,
    },
  ];

  return (
    <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
      {cards.map(({ label, value, icon: Icon }) => (
        <Card key={label}>
          <CardHeader className="flex flex-row items-center justify-between pb-2 space-y-0">
            <CardTitle className="text-sm font-medium text-muted-foreground">{label}</CardTitle>
            <Icon className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            {value !== null
              ? <p className="text-2xl font-bold">{value}</p>
              : <Skeleton className="h-8 w-24" />
            }
          </CardContent>
        </Card>
      ))}
    </div>
  );
}
