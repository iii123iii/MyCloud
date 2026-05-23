"use client";

import { useEffect, useRef, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { Loader2 } from "lucide-react";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";
import { files as filesApi, folders as foldersApi } from "@/lib/api";

const schema = z.object({ name: z.string().min(1, "Name is required") });
type Form = z.infer<typeof schema>;

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  fileId?: string;
  folderId?: string;
  currentName: string;
  onRenamed: () => void;
}

export function RenameDialog({ open, onOpenChange, fileId, folderId, currentName, onRenamed }: Props) {
  const {
    register,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<Form>({
    resolver: zodResolver(schema),
    defaultValues: { name: currentName },
  });

  const [submitError, setSubmitError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const { ref: nameFieldRef, ...nameField } = register("name");

  // Reset form + clear stale errors whenever the dialog opens for a new target.
  // We're synchronising local state with a prop change here, which the React 19
  // lint rule flags but is the right shape for this resettable dialog.
  useEffect(() => {
    if (open) {
      reset({ name: currentName });
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setSubmitError(null);
    }
  }, [open, currentName, reset]);

  // Auto-focus the input on open and pre-select the name without its extension
  // so the user can start typing a replacement immediately.
  useEffect(() => {
    if (!open) return;
    // Wait one frame so base-ui has finished its own focus management.
    const id = window.requestAnimationFrame(() => {
      const el = inputRef.current;
      if (!el) return;
      el.focus();
      const value = el.value;
      const dot = value.lastIndexOf(".");
      // For files with an extension select only the stem; for folders or
      // dotfiles select everything.
      if (fileId && dot > 0) {
        el.setSelectionRange(0, dot);
      } else {
        el.select();
      }
    });
    return () => window.cancelAnimationFrame(id);
  }, [open, fileId]);

  const onSubmit = async (data: Form) => {
    setSubmitError(null);
    try {
      if (fileId)   await filesApi.update(fileId,     { name: data.name });
      if (folderId) await foldersApi.update(folderId, { name: data.name });
      toast.success("Renamed successfully");
      onRenamed();
      onOpenChange(false);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Rename failed";
      setSubmitError(message);
      toast.error(message);
    }
  };

  const fieldError = errors.name?.message;
  const describedBy = [
    fieldError ? "rename-name-error" : null,
    submitError ? "rename-submit-error" : "rename-name-help",
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>Rename</DialogTitle>
          <DialogDescription>
            Enter a new name for {fileId ? "this file" : "this folder"}.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit(onSubmit)} className="space-y-4" noValidate>
          <div className="space-y-1.5">
            <Label htmlFor="rename-name">New name</Label>
            <Input
              id="rename-name"
              autoComplete="off"
              spellCheck={false}
              disabled={isSubmitting}
              aria-invalid={!!fieldError || !!submitError}
              aria-describedby={describedBy || undefined}
              {...nameField}
              ref={(el) => {
                nameFieldRef(el);
                inputRef.current = el;
              }}
            />
            {fieldError ? (
              <p
                id="rename-name-error"
                role="alert"
                className="text-xs font-medium text-destructive"
              >
                {fieldError}
              </p>
            ) : (
              <p id="rename-name-help" className="text-xs text-muted-foreground">
                Press Enter to save or Esc to cancel.
              </p>
            )}
            {submitError && (
              <p
                id="rename-submit-error"
                role="alert"
                aria-live="polite"
                className="text-xs font-medium text-destructive"
              >
                {submitError}
              </p>
            )}
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={isSubmitting}
              onClick={() => onOpenChange(false)}
              className="min-w-20"
            >
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={isSubmitting}
              aria-busy={isSubmitting}
              className={cn("min-w-20 transition-colors", isSubmitting && "cursor-progress")}
            >
              {isSubmitting ? (
                <>
                  <Loader2 className="size-3.5 animate-spin" aria-hidden="true" />
                  <span>Saving…</span>
                </>
              ) : (
                "Rename"
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
