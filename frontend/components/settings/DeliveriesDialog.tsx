"use client";

// Recent delivery log for a webhook. Fetched lazily when the dialog opens.

import useSWR from "swr";

import { webhooks as webhooksApi } from "@/lib/api";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { formatRelative, formatServerDateTime } from "@/lib/format";

function statusVariant(status: string): "secondary" | "destructive" | "outline" {
  if (status === "success") return "secondary";
  if (status === "failed") return "destructive";
  return "outline";
}

export function DeliveriesDialog({
  webhookId,
  open,
  onOpenChange,
}: {
  webhookId: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { data, isLoading } = useSWR(
    open && webhookId ? ["webhook-deliveries", webhookId] : null,
    () => webhooksApi.deliveries(webhookId as string).then((r) => r.deliveries ?? []),
  );
  const deliveries = data ?? [];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Recent deliveries</DialogTitle>
          <DialogDescription>The last 50 delivery attempts for this webhook.</DialogDescription>
        </DialogHeader>

        {isLoading ? (
          <div className="space-y-2" aria-busy="true">
            {[0, 1, 2].map((i) => (
              <Skeleton key={i} className="h-9 rounded" />
            ))}
          </div>
        ) : deliveries.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted-foreground">No deliveries yet.</p>
        ) : (
          <div className="max-h-[60vh] overflow-y-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Event</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>HTTP</TableHead>
                  <TableHead>Attempts</TableHead>
                  <TableHead>When</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {deliveries.map((d) => (
                  <TableRow key={d.id}>
                    <TableCell className="font-mono text-xs">{d.event_type}</TableCell>
                    <TableCell>
                      <Badge variant={statusVariant(d.status)} className="font-normal capitalize">
                        {d.status}
                      </Badge>
                    </TableCell>
                    <TableCell className="tabular-nums">
                      {d.response_status ?? (d.error ? "—" : "")}
                    </TableCell>
                    <TableCell className="tabular-nums">{d.attempts}</TableCell>
                    <TableCell className="text-muted-foreground" title={formatServerDateTime(d.created_at)}>
                      {formatRelative(d.created_at)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
