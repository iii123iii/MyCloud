"use client";

import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
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
  /** When true, omit the User column (use this in per-user views). */
  hideUserColumn?: boolean;
}

function actionVariant(action: string): "default" | "secondary" | "destructive" | "outline" {
  // Match against the leading namespace ("file.upload" → "file") and legacy
  // underscore form ("file_upload" → "file") so both render consistently.
  const head = action.split(/[._]/)[0];
  return (actionColor[head] ?? actionColor[action] ?? "outline") as
    "default" | "secondary" | "destructive" | "outline";
}

/**
 * Renders a value in a cell that may overflow horizontally: the visible text
 * truncates to a single line with an ellipsis and a Tooltip exposes the full
 * value on hover/focus. Falls back to a plain placeholder when empty.
 */
function TruncatedCell({
  value,
  placeholder = "—",
  className,
  maxWidthClass = "max-w-[12rem] sm:max-w-[16rem]",
}: {
  value: string | null | undefined;
  placeholder?: string;
  className?: string;
  maxWidthClass?: string;
}) {
  if (value === null || value === undefined || value === "") {
    return <span className={cn("text-muted-foreground", className)}>{placeholder}</span>;
  }
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span
            className={cn(
              "block truncate align-middle cursor-default",
              maxWidthClass,
              className,
            )}
          >
            {value}
          </span>
        }
      />
      <TooltipContent
        side="top"
        align="start"
        className="max-w-sm break-all text-left"
      >
        <span className="line-clamp-3 whitespace-pre-wrap">{value}</span>
      </TooltipContent>
    </Tooltip>
  );
}

export function ActivityLogTable({ logs, hideUserColumn }: Props) {
  if (!logs.length) {
    return <p className="text-muted-foreground text-sm py-4">No activity yet.</p>;
  }

  return (
    <TooltipProvider delay={150}>
      <div className="border rounded-lg w-full max-w-full overflow-hidden">
        {/* Table primitive already provides an overflow-x-auto scroller; this
            wrapper just bounds the width so the scrollbar appears on narrow
            viewports instead of expanding the page. */}
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-[1%]">Action</TableHead>
              {!hideUserColumn && <TableHead>User</TableHead>}
              <TableHead>Resource</TableHead>
              <TableHead>IP</TableHead>
              <TableHead className="w-[1%]">When</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {logs.map((log) => (
              <TableRow key={log.id}>
                <TableCell className="align-middle">
                  <Tooltip>
                    <TooltipTrigger
                      render={
                        <span className="inline-flex max-w-[10rem] sm:max-w-[14rem] align-middle cursor-default">
                          <Badge
                            variant={actionVariant(log.action)}
                            className="max-w-full"
                          >
                            <span className="truncate">{log.action}</span>
                          </Badge>
                        </span>
                      }
                    />
                    <TooltipContent
                      side="top"
                      align="start"
                      className="max-w-sm break-all text-left"
                    >
                      <span className="line-clamp-3 whitespace-pre-wrap">{log.action}</span>
                    </TooltipContent>
                  </Tooltip>
                </TableCell>
                {!hideUserColumn && (
                  <TableCell className="text-sm align-middle">
                    <TruncatedCell value={log.username} />
                  </TableCell>
                )}
                <TableCell className="text-sm text-muted-foreground align-middle">
                  <TruncatedCell value={log.resource_type} />
                </TableCell>
                <TableCell className="text-sm text-muted-foreground font-mono align-middle">
                  <TruncatedCell
                    value={log.ip_address}
                    maxWidthClass="max-w-[8rem] sm:max-w-[12rem]"
                    className="font-mono"
                  />
                </TableCell>
                <TableCell className="text-sm text-muted-foreground align-middle">
                  {formatRelative(log.created_at)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </TooltipProvider>
  );
}
