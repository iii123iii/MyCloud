"use client";

// Outbound webhook list + management. Per row: masked URL, event badges, a
// failure counter, an active toggle (optimistic), and actions to test, view
// deliveries, edit, rotate the secret, and delete.

import { useState } from "react";
import useSWR from "swr";
import { toast } from "sonner";
import {
  Webhook as WebhookIcon, Plus, Send, ListChecks, Pencil, RefreshCw, Trash2, Inbox, AlertTriangle,
} from "lucide-react";

import { webhooks as webhooksApi, type Webhook } from "@/lib/api";
import { Button } from "@/components/ui/button";
import {
  Card, CardContent, CardDescription, CardHeader, CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogClose,
} from "@/components/ui/dialog";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { WebhookDialog } from "@/components/modals/WebhookDialog";
import { DeliveriesDialog } from "./DeliveriesDialog";
import { ConfirmDialog } from "@/components/modals/ConfirmDialog";
import { SecretReveal } from "@/components/modals/SecretReveal";

function IconAction({
  label, onClick, disabled, children,
}: {
  label: string;
  onClick: () => void;
  disabled?: boolean;
  children: React.ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button variant="ghost" size="icon" onClick={onClick} disabled={disabled} aria-label={label}>
            {children}
          </Button>
        }
      />
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

export function WebhooksCard() {
  const { data, isLoading, mutate } = useSWR("webhooks", () =>
    webhooksApi.list().then((r) => r.webhooks ?? []),
  );
  const [createOpen, setCreateOpen] = useState(false);
  const [editing, setEditing] = useState<Webhook | null>(null);
  const [deliveriesFor, setDeliveriesFor] = useState<string | null>(null);
  const [rotatedSecret, setRotatedSecret] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const items = data ?? [];

  const toggleActive = async (wh: Webhook, next: boolean) => {
    setBusy(wh.id);
    // Optimistic flip.
    mutate(items.map((w) => (w.id === wh.id ? { ...w, is_active: next } : w)), false);
    try {
      await webhooksApi.update(wh.id, { is_active: next });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to update webhook");
    } finally {
      setBusy(null);
      mutate();
    }
  };

  const test = async (wh: Webhook) => {
    setBusy(wh.id);
    try {
      const res = await webhooksApi.test(wh.id);
      if (res.ok) toast.success(`Test delivered (HTTP ${res.response_status})`);
      else toast.error(`Test failed: ${res.error || `HTTP ${res.response_status}`}`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Test failed");
    } finally {
      setBusy(null);
      mutate();
    }
  };

  const rotate = async (wh: Webhook) => {
    try {
      const res = await webhooksApi.rotateSecret(wh.id);
      setRotatedSecret(res.secret);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to rotate secret");
    }
  };

  const remove = async (wh: Webhook) => {
    try {
      await webhooksApi.delete(wh.id);
      toast.success("Webhook deleted");
      mutate();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete webhook");
    }
  };

  return (
    <TooltipProvider>
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-3 space-y-0">
        <div className="space-y-1.5">
          <CardTitle className="flex items-center gap-2">
            <WebhookIcon className="h-4 w-4" aria-hidden="true" />
            Webhooks
          </CardTitle>
          <CardDescription>
            Receive a signed HTTP POST when your files, folders, or shares change.
          </CardDescription>
        </div>
        <Button size="sm" onClick={() => setCreateOpen(true)} className="shrink-0">
          <Plus className="h-4 w-4" aria-hidden="true" />
          Add webhook
        </Button>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-2" aria-busy="true" aria-label="Loading webhooks">
            {[0, 1].map((i) => (
              <Skeleton key={i} className="h-16 rounded-md" />
            ))}
          </div>
        ) : items.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-2 py-10 text-center">
            <div className="rounded-full bg-muted p-3">
              <Inbox className="size-6 text-muted-foreground" aria-hidden="true" />
            </div>
            <p className="text-sm font-medium">No webhooks yet</p>
            <p className="text-xs text-muted-foreground max-w-xs leading-relaxed">
              Add a webhook to notify another service when things happen in your drive.
            </p>
          </div>
        ) : (
          <ul className="divide-y rounded-md border bg-card overflow-hidden">
            {items.map((wh) => (
              <li key={wh.id} className="flex flex-col gap-2 p-3 sm:flex-row sm:items-center sm:gap-3 sm:p-4">
                <div className="min-w-0 flex-1 space-y-1">
                  <div className="flex items-center gap-2">
                    <code className="truncate font-mono text-xs" title={wh.url}>
                      {wh.url}
                    </code>
                    {wh.failure_count > 0 && (
                      <Badge variant="destructive" className="shrink-0 gap-1 font-normal text-[10px] leading-none">
                        <AlertTriangle className="size-3" aria-hidden="true" />
                        {wh.failure_count} failed
                      </Badge>
                    )}
                  </div>
                  <div className="flex flex-wrap gap-1">
                    {wh.events.length === 0 || wh.events.includes("*") ? (
                      <Badge variant="outline" className="font-normal text-[10px] leading-none">All events</Badge>
                    ) : (
                      wh.events.map((e) => (
                        <Badge key={e} variant="secondary" className="font-normal text-[10px] leading-none">
                          {e}
                        </Badge>
                      ))
                    )}
                  </div>
                </div>
                <div className="flex items-center gap-1">
                  <Tooltip>
                    <TooltipTrigger
                      render={
                        <Switch
                          checked={wh.is_active}
                          disabled={busy === wh.id}
                          onCheckedChange={(v) => toggleActive(wh, v)}
                          aria-label={wh.is_active ? "Disable webhook" : "Enable webhook"}
                        />
                      }
                    />
                    <TooltipContent>{wh.is_active ? "Active" : "Disabled"}</TooltipContent>
                  </Tooltip>
                  <IconAction label="Send test event" onClick={() => test(wh)} disabled={busy === wh.id}>
                    <Send className="h-4 w-4" />
                  </IconAction>
                  <IconAction label="View deliveries" onClick={() => setDeliveriesFor(wh.id)}>
                    <ListChecks className="h-4 w-4" />
                  </IconAction>
                  <IconAction label="Edit" onClick={() => setEditing(wh)}>
                    <Pencil className="h-4 w-4" />
                  </IconAction>
                  <ConfirmDialog
                    trigger={
                      <Button variant="ghost" size="icon" aria-label="Rotate secret">
                        <RefreshCw className="h-4 w-4" />
                      </Button>
                    }
                    title="Rotate signing secret?"
                    description="A new secret is issued and shown once. The old secret keeps working for a 24-hour grace period so you can update your receiver."
                    confirmLabel="Rotate secret"
                    onConfirm={() => rotate(wh)}
                  />
                  <ConfirmDialog
                    trigger={
                      <Button variant="ghost" size="icon" aria-label="Delete webhook" className="hover:text-destructive">
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    }
                    title="Delete this webhook?"
                    description={<>No more events will be delivered to <span className="font-mono">{wh.url}</span>.</>}
                    confirmLabel="Delete webhook"
                    onConfirm={() => remove(wh)}
                  />
                </div>
              </li>
            ))}
          </ul>
        )}
      </CardContent>

      <WebhookDialog open={createOpen} onOpenChange={setCreateOpen} onSaved={() => mutate()} />
      <WebhookDialog
        open={editing !== null}
        onOpenChange={(o) => !o && setEditing(null)}
        onSaved={() => mutate()}
        webhook={editing}
      />
      <DeliveriesDialog
        webhookId={deliveriesFor}
        open={deliveriesFor !== null}
        onOpenChange={(o) => !o && setDeliveriesFor(null)}
      />

      {/* One-time reveal of a rotated secret. */}
      <Dialog open={rotatedSecret !== null} onOpenChange={(o) => !o && setRotatedSecret(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>New signing secret</DialogTitle>
            <DialogDescription>
              Update your receiver with this secret. The previous one stays valid for 24 hours.
            </DialogDescription>
          </DialogHeader>
          {rotatedSecret && <SecretReveal value={rotatedSecret} label="Copy your new secret now" />}
          <DialogFooter>
            <DialogClose render={<Button type="button" />}>Done</DialogClose>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
    </TooltipProvider>
  );
}
