"use client";

// Create or edit a webhook. On create, the signing secret is revealed once via
// <SecretReveal>. Event selection offers an "All events" toggle (stored as
// ["*"]) or a curated per-event picker.

import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Loader2 } from "lucide-react";

import { webhooks as webhooksApi, WEBHOOK_EVENTS, type Webhook } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogClose,
} from "@/components/ui/dialog";
import { SecretReveal } from "./SecretReveal";

export function WebhookDialog({
  open,
  onOpenChange,
  onSaved,
  webhook,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
  webhook?: Webhook | null;
}) {
  const editing = !!webhook;
  const [url, setUrl] = useState("");
  const [allEvents, setAllEvents] = useState(true);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [isActive, setIsActive] = useState(true);
  const [saving, setSaving] = useState(false);
  const [createdSecret, setCreatedSecret] = useState<string | null>(null);

  // Sync form state whenever the dialog opens (or the target webhook changes).
  useEffect(() => {
    if (!open) return;
    if (webhook) {
      setUrl(webhook.url);
      const all = webhook.events.length === 0 || webhook.events.includes("*");
      setAllEvents(all);
      setSelected(new Set(all ? [] : webhook.events));
      setIsActive(webhook.is_active);
    } else {
      setUrl("");
      setAllEvents(true);
      setSelected(new Set());
      setIsActive(true);
    }
    setCreatedSecret(null);
    setSaving(false);
  }, [open, webhook]);

  const toggleEvent = (value: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(value)) next.delete(value);
      else next.add(value);
      return next;
    });
  };

  const handleClose = (next: boolean) => {
    if (!next && createdSecret) onSaved();
    onOpenChange(next);
  };

  const submit = async () => {
    const trimmed = url.trim();
    if (!trimmed) {
      toast.error("Enter a URL");
      return;
    }
    const events = allEvents ? ["*"] : [...selected];
    if (!allEvents && events.length === 0) {
      toast.error("Select at least one event, or choose All events");
      return;
    }
    setSaving(true);
    try {
      if (editing && webhook) {
        await webhooksApi.update(webhook.id, { url: trimmed, events, is_active: isActive });
        toast.success("Webhook updated");
        onSaved();
        onOpenChange(false);
      } else {
        const created = await webhooksApi.create({ url: trimmed, events, is_active: isActive });
        if (created.secret) {
          setCreatedSecret(created.secret);
        } else {
          onSaved();
          onOpenChange(false);
        }
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to save webhook");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-md">
        {createdSecret ? (
          <>
            <DialogHeader>
              <DialogTitle>Webhook created</DialogTitle>
              <DialogDescription>
                Save the signing secret now — verify the <code className="font-mono text-xs">X-MyCloud-Signature</code> header with it.
              </DialogDescription>
            </DialogHeader>
            <SecretReveal value={createdSecret} label="Copy your signing secret now" />
            <DialogFooter>
              <DialogClose render={<Button type="button" />}>Done</DialogClose>
            </DialogFooter>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>{editing ? "Edit webhook" : "Add webhook"}</DialogTitle>
              <DialogDescription>
                We POST a signed JSON payload to your URL when the selected events happen.
              </DialogDescription>
            </DialogHeader>

            <div className="space-y-4">
              <div className="space-y-1.5">
                <Label htmlFor="webhook-url">Payload URL</Label>
                <Input
                  id="webhook-url"
                  type="url"
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                  placeholder="https://example.com/hooks/mycloud"
                  autoFocus
                />
              </div>

              <div className="space-y-2">
                <Label>Events</Label>
                <label className="flex cursor-pointer items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    className="accent-primary"
                    checked={allEvents}
                    onChange={(e) => setAllEvents(e.target.checked)}
                  />
                  <span>All events</span>
                </label>
                {!allEvents && (
                  <div className="grid grid-cols-1 gap-1.5 rounded-md border p-3 sm:grid-cols-2">
                    {WEBHOOK_EVENTS.map((ev) => (
                      <label key={ev.value} className="flex cursor-pointer items-center gap-2 text-sm">
                        <input
                          type="checkbox"
                          className="accent-primary"
                          checked={selected.has(ev.value)}
                          onChange={() => toggleEvent(ev.value)}
                        />
                        <span>{ev.label}</span>
                      </label>
                    ))}
                  </div>
                )}
              </div>

              <div className="flex items-center justify-between">
                <div className="space-y-0.5">
                  <Label htmlFor="webhook-active">Active</Label>
                  <p className="text-xs text-muted-foreground">Deliver events to this endpoint.</p>
                </div>
                <Switch id="webhook-active" checked={isActive} onCheckedChange={setIsActive} />
              </div>
            </div>

            <DialogFooter>
              <DialogClose render={<Button variant="outline" type="button" />}>Cancel</DialogClose>
              <Button type="button" onClick={submit} disabled={saving}>
                {saving ? (
                  <>
                    <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
                    Saving…
                  </>
                ) : editing ? (
                  "Save changes"
                ) : (
                  "Add webhook"
                )}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
