"use client";

import { useState } from "react";
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
import { folders as foldersApi } from "@/lib/api";

const schema = z.object({ name: z.string().min(1, "Folder name is required") });
type Form = z.infer<typeof schema>;

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  parentId?: string;
  onCreated: () => void;
}

export function NewFolderDialog({ open, onOpenChange, parentId, onCreated }: Props) {
  const { register, handleSubmit, reset, formState: { errors, isSubmitting } } = useForm<Form>({
    resolver: zodResolver(schema),
  });
  const [submitError, setSubmitError] = useState<string | null>(null);

  const onSubmit = async (data: Form) => {
    setSubmitError(null);
    try {
      await foldersApi.create({ name: data.name, parent_id: parentId });
      toast.success("Folder created");
      reset();
      onCreated();
      onOpenChange(false);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to create folder";
      setSubmitError(msg);
      toast.error(msg);
    }
  };

  const fieldError = errors.name?.message;
  const hasFieldError = Boolean(fieldError);

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) {
          reset();
          setSubmitError(null);
        }
        onOpenChange(o);
      }}
    >
      <DialogContent className="max-w-sm" aria-describedby="folder-name-help">
        <DialogHeader>
          <DialogTitle>New folder</DialogTitle>
          <DialogDescription id="folder-name-help">
            Give your new folder a name. Press Enter to create or Esc to cancel.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="folder-name">Folder name</Label>
            <Input
              id="folder-name"
              autoFocus
              autoComplete="off"
              placeholder="Untitled folder"
              disabled={isSubmitting}
              aria-invalid={hasFieldError || undefined}
              aria-describedby={cn(
                "folder-name-help",
                hasFieldError && "folder-name-error",
              )}
              {...register("name")}
            />
            {hasFieldError && (
              <p
                id="folder-name-error"
                role="alert"
                className="text-xs leading-tight text-destructive"
              >
                {fieldError}
              </p>
            )}
            {!hasFieldError && submitError && (
              <p
                role="alert"
                className="text-xs leading-tight text-destructive"
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
              onClick={() => {
                reset();
                setSubmitError(null);
                onOpenChange(false);
              }}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={isSubmitting}
              className="transition-colors hover:bg-primary/90"
            >
              {isSubmitting ? (
                <>
                  <Loader2 className="animate-spin" aria-hidden="true" />
                  <span>Creating…</span>
                </>
              ) : (
                "Create"
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
