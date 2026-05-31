"use client";

// Create a personal access token. Two states gated on `createdToken`:
//   1. form  — name, scope checkboxes, optional expiry
//   2. reveal — the plaintext token shown once via <SecretReveal>
//
// "*" (full access) is a deliberate explicit choice: ticking it disables and
// supersedes the rest, and the client sends ["*"] rather than the expanded set.

import { useState } from "react";
import { toast } from "sonner";
import { Loader2 } from "lucide-react";

import { tokens as tokensApi, PAT_SCOPES, type PersonalAccessToken } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
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

const READ_SCOPES = PAT_SCOPES.filter((s) => s.read).map((s) => s.value);

export function CreateTokenDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: () => void;
}) {
  const [name, setName] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [expiresAt, setExpiresAt] = useState("");
  const [creating, setCreating] = useState(false);
  const [created, setCreated] = useState<PersonalAccessToken | null>(null);

  const fullAccess = selected.has("*");

  const toggle = (value: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (value === "*") {
        // Full access supersedes everything else.
        return next.has("*") ? new Set() : new Set(["*"]);
      }
      next.delete("*");
      if (next.has(value)) next.delete(value);
      else next.add(value);
      return next;
    });
  };

  const presetReadOnly = () => setSelected(new Set(READ_SCOPES));
  // Exactly what the desktop sync client needs (read + write files/folders).
  const presetDesktopSync = () => setSelected(new Set(["files:read", "files:write"]));

  const reset = () => {
    setName("");
    setSelected(new Set());
    setExpiresAt("");
    setCreated(null);
    setCreating(false);
  };

  const handleClose = (next: boolean) => {
    if (!next) {
      if (created) onCreated(); // refresh list once the user dismisses the reveal
      reset();
    }
    onOpenChange(next);
  };

  const submit = async () => {
    const trimmed = name.trim();
    if (!trimmed) {
      toast.error("Give the token a name");
      return;
    }
    const scopes = fullAccess ? ["*"] : [...selected];
    if (scopes.length === 0) {
      toast.error("Select at least one scope");
      return;
    }
    setCreating(true);
    try {
      const tok = await tokensApi.create({
        name: trimmed,
        scopes,
        // datetime-local has no seconds/zone; normalise to an ISO instant the
        // backend's datetime parser accepts.
        expires_at: expiresAt ? new Date(expiresAt).toISOString() : undefined,
      });
      setCreated(tok);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create token");
    } finally {
      setCreating(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-md">
        {created ? (
          <>
            <DialogHeader>
              <DialogTitle>Token created</DialogTitle>
              <DialogDescription>
                Copy <span className="font-medium">{created.name}</span> now — this is the only time
                it will be shown.
              </DialogDescription>
            </DialogHeader>
            {created.token && <SecretReveal value={created.token} label="Copy your token now" />}
            <DialogFooter>
              <DialogClose render={<Button type="button" />}>Done</DialogClose>
            </DialogFooter>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>Create personal access token</DialogTitle>
              <DialogDescription>
                Tokens authenticate API requests as you, limited to the scopes you select.
              </DialogDescription>
            </DialogHeader>

            <div className="space-y-4">
              <div className="space-y-1.5">
                <Label htmlFor="token-name">Name</Label>
                <Input
                  id="token-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="e.g. CI deploy bot"
                  maxLength={100}
                  autoFocus
                />
              </div>

              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <Label>Scopes</Label>
                  <div className="flex gap-1">
                    <Button type="button" variant="ghost" size="sm" onClick={presetDesktopSync}>
                      Desktop sync
                    </Button>
                    <Button type="button" variant="ghost" size="sm" onClick={presetReadOnly}>
                      Read-only
                    </Button>
                  </div>
                </div>
                <div className="space-y-1.5 rounded-md border p-3">
                  {PAT_SCOPES.map((scope) => {
                    const isStar = scope.value === "*";
                    const checked = isStar ? fullAccess : selected.has(scope.value);
                    const disabled = !isStar && fullAccess;
                    return (
                      <label
                        key={scope.value}
                        className="flex cursor-pointer items-start gap-2 text-sm data-disabled:cursor-not-allowed"
                        data-disabled={disabled || undefined}
                      >
                        <input
                          type="checkbox"
                          className="mt-0.5 accent-primary disabled:opacity-40"
                          checked={checked}
                          disabled={disabled}
                          onChange={() => toggle(scope.value)}
                        />
                        <span className={disabled ? "text-muted-foreground" : undefined}>
                          {scope.label}
                          {isStar && (
                            <span className="ml-1 text-xs text-muted-foreground">
                              — supersedes the others
                            </span>
                          )}
                        </span>
                      </label>
                    );
                  })}
                </div>
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="token-expiry">Expiry (optional)</Label>
                <Input
                  id="token-expiry"
                  type="datetime-local"
                  value={expiresAt}
                  onChange={(e) => setExpiresAt(e.target.value)}
                />
                <p className="text-xs text-muted-foreground">
                  Leave blank for a token that never expires.
                </p>
              </div>
            </div>

            <DialogFooter>
              <DialogClose render={<Button variant="outline" type="button" />}>Cancel</DialogClose>
              <Button type="button" onClick={submit} disabled={creating}>
                {creating ? (
                  <>
                    <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
                    Creating…
                  </>
                ) : (
                  "Create token"
                )}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
