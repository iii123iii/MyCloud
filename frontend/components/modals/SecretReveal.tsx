"use client";

// One-time secret reveal block, shared by the create-token and webhook dialogs.
// A freshly minted token/secret is shown exactly once; this renders a warning,
// a read-only mono field, and a copy button (Copy → Check flash), mirroring the
// share-link reveal in ShareDialog.

import { useState } from "react";
import { AlertTriangle, Check, Copy } from "lucide-react";

import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";

export function SecretReveal({
  value,
  label = "Copy this secret now",
  description = "You won't be able to see it again after closing this dialog.",
}: {
  value: string;
  label?: string;
  description?: string;
}) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard can fail on insecure origins; the field is selectable as a
      // fallback so the user can copy manually.
    }
  };

  return (
    <div className="space-y-3">
      <Alert role="alert">
        <AlertTriangle className="h-4 w-4" aria-hidden="true" />
        <AlertTitle>{label}</AlertTitle>
        <AlertDescription>{description}</AlertDescription>
      </Alert>
      <div className="flex min-w-0 items-center gap-2">
        <Input
          value={value}
          readOnly
          onFocus={(e) => e.currentTarget.select()}
          className="min-w-0 flex-1 truncate font-mono text-xs"
          aria-label="Secret value"
        />
        <Button
          variant="outline"
          size="icon"
          onClick={copy}
          aria-label={copied ? "Copied" : "Copy to clipboard"}
          className="shrink-0"
        >
          {copied ? (
            <Check className="h-4 w-4 text-emerald-600 dark:text-emerald-400" />
          ) : (
            <Copy className="h-4 w-4" />
          )}
        </Button>
      </div>
    </div>
  );
}
