"use client";

import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { formatRelative } from "@/lib/format";
import type { ActivityLog } from "@/lib/types";

const actionColor: Record<string, string> = {
  upload:   "default",
  download: "secondary",
  delete:   "destructive",
  login:    "outline",
  share:    "default",
};

interface Props {
  logs: ActivityLog[];
}

export function ActivityLogTable({ logs }: Props) {
  if (!logs.length) {
    return <p className="text-muted-foreground text-sm py-4">No activity yet.</p>;
  }

  return (
    <div className="border rounded-lg overflow-hidden">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Action</TableHead>
            <TableHead>User</TableHead>
            <TableHead>Resource</TableHead>
            <TableHead>IP</TableHead>
            <TableHead>When</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {logs.map((log) => (
            <TableRow key={log.id}>
              <TableCell>
                <Badge variant={(actionColor[log.action] ?? "outline") as "default" | "secondary" | "destructive" | "outline"}>
                  {log.action}
                </Badge>
              </TableCell>
              <TableCell className="text-sm">{log.username ?? "—"}</TableCell>
              <TableCell className="text-sm text-muted-foreground">
                {log.resource_type ?? "—"}
              </TableCell>
              <TableCell className="text-sm text-muted-foreground font-mono">
                {log.ip_address ?? "—"}
              </TableCell>
              <TableCell className="text-sm text-muted-foreground">
                {formatRelative(log.created_at)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
