"use client";

import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";

import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
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
  const { register, handleSubmit, reset, formState: { errors, isSubmitting } } = useForm<Form>({
    resolver: zodResolver(schema),
    defaultValues: { name: currentName },
  });

  useEffect(() => {
    if (open) reset({ name: currentName });
  }, [open, currentName, reset]);

  const onSubmit = async (data: Form) => {
    try {
      if (fileId)   await filesApi.update(fileId,     { name: data.name });
      if (folderId) await foldersApi.update(folderId, { name: data.name });
      toast.success("Renamed successfully");
      onRenamed();
      onOpenChange(false);
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Rename failed");
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>Rename</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="name">New name</Label>
            <Input id="name" autoFocus {...register("name")} />
            {errors.name && <p className="text-xs text-destructive">{errors.name.message}</p>}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
            <Button type="submit" disabled={isSubmitting}>{isSubmitting ? "Saving…" : "Rename"}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
