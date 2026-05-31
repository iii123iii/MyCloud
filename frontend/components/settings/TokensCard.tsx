"use client";

// Personal access token list + create entry point. Each row shows the masked
// token (prefix…last_four), its scopes, and "last used … from <ip>" — the key
// signal for spotting a leaked token — plus a Revoke action.

import { useState } from "react";
import useSWR from "swr";
import { toast } from "sonner";
import { KeyRound, Plus, ShieldOff, Clock, Inbox } from "lucide-react";

import { tokens as tokensApi, type PersonalAccessToken } from "@/lib/api";
import { Button } from "@/components/ui/button";
import {
  Card, CardContent, CardDescription, CardHeader, CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { formatRelative, formatServerDateTime } from "@/lib/format";
import { CreateTokenDialog } from "@/components/modals/CreateTokenDialog";
import { ConfirmDialog } from "@/components/modals/ConfirmDialog";

export function TokensCard() {
  const { data, isLoading, mutate } = useSWR("pat-tokens", () =>
    tokensApi.list().then((r) => r.tokens ?? []),
  );
  const [createOpen, setCreateOpen] = useState(false);

  const revoke = async (t: PersonalAccessToken) => {
    try {
      await tokensApi.revoke(t.id);
      toast.success("Token revoked");
      mutate();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to revoke token");
    }
  };

  const items = data ?? [];

  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-3 space-y-0">
        <div className="space-y-1.5">
          <CardTitle className="flex items-center gap-2">
            <KeyRound className="h-4 w-4" aria-hidden="true" />
            Personal access tokens
          </CardTitle>
          <CardDescription>
            Authenticate scripts and CLIs with <code className="font-mono text-xs">Authorization: Bearer mc_pat_…</code> instead of your password.
          </CardDescription>
        </div>
        <Button size="sm" onClick={() => setCreateOpen(true)} className="shrink-0">
          <Plus className="h-4 w-4" aria-hidden="true" />
          New token
        </Button>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-2" aria-busy="true" aria-label="Loading tokens">
            {[0, 1].map((i) => (
              <Skeleton key={i} className="h-16 rounded-md" />
            ))}
          </div>
        ) : items.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-2 py-10 text-center">
            <div className="rounded-full bg-muted p-3">
              <Inbox className="size-6 text-muted-foreground" aria-hidden="true" />
            </div>
            <p className="text-sm font-medium">No tokens yet</p>
            <p className="text-xs text-muted-foreground max-w-xs leading-relaxed">
              Create a token to access the API from automation without sharing your password.
            </p>
          </div>
        ) : (
          <ul className="divide-y rounded-md border bg-card overflow-hidden">
            {items.map((t) => (
              <li key={t.id} className="flex flex-col gap-2 p-3 sm:flex-row sm:items-center sm:gap-3 sm:p-4">
                <div className="min-w-0 flex-1 space-y-1">
                  <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                    <span className="font-medium leading-tight">{t.name}</span>
                    <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground">
                      {t.token_prefix}…{t.last_four}
                    </code>
                  </div>
                  <div className="flex flex-wrap gap-1">
                    {t.scopes.map((s) => (
                      <Badge key={s} variant="secondary" className="font-normal text-[10px] leading-none">
                        {s}
                      </Badge>
                    ))}
                  </div>
                  <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted-foreground">
                    {t.last_used_at ? (
                      <span title={formatServerDateTime(t.last_used_at)}>
                        Last used {formatRelative(t.last_used_at)}
                        {t.last_used_ip ? ` from ${t.last_used_ip}` : ""}
                      </span>
                    ) : (
                      <span>Never used</span>
                    )}
                    {t.expires_at && (
                      <>
                        <span aria-hidden="true" className="opacity-50">·</span>
                        <span className="inline-flex items-center gap-1" title={formatServerDateTime(t.expires_at)}>
                          <Clock className="size-3" aria-hidden="true" />
                          Expires {formatRelative(t.expires_at)}
                        </span>
                      </>
                    )}
                  </div>
                </div>
                <div className="sm:ml-2">
                  <ConfirmDialog
                    trigger={
                      <Button
                        variant="outline"
                        size="sm"
                        className="w-full sm:w-auto hover:border-destructive/40 hover:text-destructive"
                      >
                        <ShieldOff className="size-4 mr-1" aria-hidden="true" />
                        Revoke
                      </Button>
                    }
                    title="Revoke this token?"
                    description={
                      <>
                        <strong>{t.name}</strong> will stop working immediately. Any scripts using it
                        will need a new token.
                      </>
                    }
                    confirmLabel="Revoke token"
                    onConfirm={() => revoke(t)}
                  />
                </div>
              </li>
            ))}
          </ul>
        )}
      </CardContent>

      <CreateTokenDialog open={createOpen} onOpenChange={setCreateOpen} onCreated={() => mutate()} />
    </Card>
  );
}
